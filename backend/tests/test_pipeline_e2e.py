#!/usr/bin/env python3
"""
End-to-end LIVE integration test for the RAVEN X / ReconX dashboard pipeline.

Boots raven/app.py in a subprocess against a TEMPORARY working directory
(so its hard-coded relative DB_PATH='raven_results.db' and
RESULTS_DIR='ResultJS' land in /tmp, never the real data dir), seeds a fake
valid_stripe.txt finding, exercises /api/clear -> seed -> wait-for-import ->
/api/stats, and confirms the credential appears both in the JSON response
and in the on-disk SQLite table.

No real worker VPSes, no SSH, no scanner binary required.
Pure stdlib + `requests`. Self-contained. Idempotent. <30s wall time.

Run from the raven/ directory:
    python3 tests/test_pipeline_e2e.py
Exit code 0 = pass, non-zero = fail.
"""
from __future__ import annotations

import json
import os
import shutil
import signal
import sqlite3
import subprocess
import sys
import tempfile
import textwrap
import time
import urllib.error
import urllib.request
from pathlib import Path

# --------------------------------------------------------------------------- #
# Config                                                                      #
# --------------------------------------------------------------------------- #

HERE = Path(__file__).resolve().parent
RAVEN_DIR = HERE.parent                       # .../raven
APP_PY = RAVEN_DIR / "app.py"

HOST = "127.0.0.1"
PORT = 5099                                    # avoid clash with prod 5000 / macOS AirPlay
BASE_URL = f"http://{HOST}:{PORT}"

SEED_LINE = "https://test.example/.env:sk:live:sk_test_REDACTED_FIXTURE_PLACEHOLDER"
SEED_MARKER = "sk_test_REDACTED_FIXTURE_PLACEHOLDER"

BOOT_TIMEOUT = 15.0
IMPORT_TIMEOUT = 10.0
REQUEST_TIMEOUT = 5.0

# --------------------------------------------------------------------------- #
# Helpers                                                                     #
# --------------------------------------------------------------------------- #


def log(msg: str) -> None:
    print(f"[e2e] {msg}", flush=True)


def http_get_json(path: str) -> dict:
    req = urllib.request.Request(BASE_URL + path, method="GET")
    with urllib.request.urlopen(req, timeout=REQUEST_TIMEOUT) as resp:
        return json.loads(resp.read().decode("utf-8"))


def http_post_json(path: str, payload: dict | None = None) -> dict:
    body = json.dumps(payload or {}).encode("utf-8")
    req = urllib.request.Request(
        BASE_URL + path,
        data=body,
        method="POST",
        headers={"Content-Type": "application/json"},
    )
    with urllib.request.urlopen(req, timeout=REQUEST_TIMEOUT) as resp:
        return json.loads(resp.read().decode("utf-8"))


def wait_for_server(deadline: float) -> None:
    """Poll /api/stats until it returns 200 or the deadline expires."""
    last_err: Exception | None = None
    while time.time() < deadline:
        try:
            http_get_json("/api/stats")
            return
        except (urllib.error.URLError, ConnectionError, OSError) as e:
            last_err = e
            time.sleep(0.25)
    raise RuntimeError(f"Server did not come up within {BOOT_TIMEOUT}s: {last_err!r}")


def write_bootstrap(tmpdir: Path) -> Path:
    """Write a tiny launcher that imports the real app and runs it on PORT.

    We do this instead of executing app.py directly because app.py hardcodes
    port 5000 in its __main__ block.
    """
    bootstrap = tmpdir / "_run_app.py"
    bootstrap.write_text(textwrap.dedent(f"""
        import os, sys
        sys.path.insert(0, {str(RAVEN_DIR)!r})
        os.chdir({str(tmpdir)!r})
        # Import the dashboard module; this defines `app` + `socketio` and
        # registers all routes. We then call its init + run ourselves so we
        # control host/port and avoid the production banner port.
        import app as raven_app
        raven_app.init_db()
        # No background monitor thread — keeps teardown clean; we drive
        # imports manually in the test.
        raven_app.socketio.run(
            raven_app.app,
            host={HOST!r},
            port={PORT},
            debug=False,
            allow_unsafe_werkzeug=True,
        )
    """).strip() + "\n")
    return bootstrap


def seed_stripe_file(tmpdir: Path) -> Path:
    results_dir = tmpdir / "ResultJS"
    results_dir.mkdir(exist_ok=True)
    target = results_dir / "valid_stripe.txt"
    target.write_text(SEED_LINE + "\n", encoding="utf-8")
    # Bump mtime so import_from_files() definitely picks it up after /api/clear
    # cleared the cache (which it does anyway, but be explicit).
    os.utime(target, None)
    return target


def run_import_in_app_process(tmpdir: Path) -> None:
    """app.py exposes import_from_files() but not over HTTP. Easiest reliable
    trigger is a one-shot python invocation that imports the module in the
    same cwd and calls the function — mimics the production 2s monitor."""
    code = textwrap.dedent(f"""
        import sys, os
        sys.path.insert(0, {str(RAVEN_DIR)!r})
        os.chdir({str(tmpdir)!r})
        import app as raven_app
        raven_app.init_db()
        v, h = raven_app.import_from_files()
        print(f"imported_valid={{v}} imported_hits={{h}}")
    """).strip()
    res = subprocess.run(
        [sys.executable, "-c", code],
        capture_output=True, text=True, timeout=IMPORT_TIMEOUT,
    )
    if res.returncode != 0:
        raise RuntimeError(
            f"import_from_files helper failed:\nSTDOUT:{res.stdout}\nSTDERR:{res.stderr}"
        )
    log(f"import helper: {res.stdout.strip()}")


# --------------------------------------------------------------------------- #
# Main                                                                        #
# --------------------------------------------------------------------------- #


def main() -> int:
    if not APP_PY.exists():
        log(f"FATAL: cannot find {APP_PY}")
        return 2

    tmpdir = Path(tempfile.mkdtemp(prefix="raven_e2e_"))
    log(f"tmpdir: {tmpdir}")

    proc: subprocess.Popen | None = None
    try:
        # ---- 1. boot Flask in a subprocess against tmpdir -------------------
        bootstrap = write_bootstrap(tmpdir)
        env = os.environ.copy()
        env["PYTHONUNBUFFERED"] = "1"
        proc = subprocess.Popen(
            [sys.executable, str(bootstrap)],
            cwd=str(tmpdir),
            env=env,
            stdout=subprocess.PIPE,
            stderr=subprocess.STDOUT,
        )
        log(f"launched app.py shim, pid={proc.pid}, port={PORT}")
        wait_for_server(time.time() + BOOT_TIMEOUT)
        log("server is up")

        # ---- 2. POST /api/clear --------------------------------------------
        cleared = http_post_json("/api/clear")
        assert cleared.get("success") is True, f"/api/clear failed: {cleared}"
        log("/api/clear OK")

        # ---- 3. drop seed file AFTER clear (clear wipes ResultJS/) ---------
        seeded = seed_stripe_file(tmpdir)
        log(f"seeded {seeded}")

        # ---- 4. trigger import_from_files() --------------------------------
        # /api/clear wiped file_mtimes in the live process, so a fresh
        # import_from_files() call (in a sibling process sharing the same
        # sqlite file) will see the new mtime and ingest the row.
        run_import_in_app_process(tmpdir)

        # ---- 5. GET /api/stats and assert ----------------------------------
        stats = http_get_json("/api/stats")
        type_counts = stats.get("type_counts") or {}
        recent = stats.get("recent_findings") or []

        log(f"type_counts: {type_counts}")
        log(f"recent_findings rows: {len(recent)}")

        assert type_counts.get("Stripe", 0) >= 1, (
            f"expected Stripe>=1 in type_counts, got {type_counts!r}"
        )

        # recent_findings rows are tuples: (type, key_value, source_url, ts, metadata)
        flat = " ".join(json.dumps(r) for r in recent)
        assert SEED_MARKER in flat, (
            f"expected seed marker {SEED_MARKER!r} in recent_findings, got {recent!r}"
        )
        # And the type column should be 'Stripe' for at least one row
        assert any((isinstance(r, (list, tuple)) and r and r[0] == "Stripe") for r in recent), (
            f"no Stripe row in recent_findings: {recent!r}"
        )
        log("/api/stats assertions passed")

        # ---- 6. confirm SQLite directly ------------------------------------
        db_path = tmpdir / "raven_results.db"
        assert db_path.exists(), f"DB not found at {db_path}"
        conn = sqlite3.connect(str(db_path))
        try:
            cur = conn.cursor()
            cur.execute(
                "SELECT type, key_value, source_url, metadata, status "
                "FROM credentials WHERE type='Stripe'"
            )
            rows = cur.fetchall()
        finally:
            conn.close()

        log(f"sqlite rows: {rows}")
        assert rows, "no Stripe rows in sqlite credentials table"
        joined = " ".join(" ".join(map(str, r)) for r in rows)
        assert SEED_MARKER in joined, (
            f"seed marker {SEED_MARKER!r} not found in any Stripe row: {rows!r}"
        )
        assert any(r[4] == "valid" for r in rows), (
            f"expected at least one Stripe row with status='valid', got {rows!r}"
        )
        log("sqlite assertions passed")

        log("ALL CHECKS PASSED")
        return 0

    except AssertionError as e:
        log(f"ASSERTION FAILED: {e}")
        return 1
    except Exception as e:
        log(f"ERROR: {type(e).__name__}: {e}")
        return 1
    finally:
        # ---- teardown -------------------------------------------------------
        if proc is not None and proc.poll() is None:
            log(f"terminating server pid={proc.pid}")
            try:
                proc.send_signal(signal.SIGTERM)
                try:
                    proc.wait(timeout=4)
                except subprocess.TimeoutExpired:
                    proc.kill()
                    proc.wait(timeout=2)
            except Exception as e:
                log(f"teardown kill error: {e}")
            # Drain any captured output so it shows up on CI failures
            try:
                if proc.stdout:
                    tail = proc.stdout.read().decode("utf-8", errors="replace")
                    if tail.strip():
                        log("---- server output ----")
                        for line in tail.splitlines()[-30:]:
                            print(f"    {line}")
                        log("---- end server output ----")
            except Exception:
                pass

        try:
            shutil.rmtree(tmpdir, ignore_errors=True)
            log(f"removed tmpdir {tmpdir}")
        except Exception as e:
            log(f"tmpdir cleanup error: {e}")


if __name__ == "__main__":
    sys.exit(main())
