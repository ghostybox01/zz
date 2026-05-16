#!/usr/bin/env python3
"""Unit test for backend.app._parse_creds_text.

Covers every input form the fleet-bootstrap textarea should accept.
Run from the backend/ directory:
    python3 tests/test_parse_creds.py
Exit code 0 = pass.
"""
from __future__ import annotations

import os
import sys

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, os.path.dirname(HERE))

from app import _parse_creds_text  # noqa: E402


def one(text: str):
    rows = _parse_creds_text(text)
    assert len(rows) == 1, f"expected 1 row, got {len(rows)} for {text!r}: {rows}"
    return rows[0]


def check(text, **expected):
    r = one(text)
    for k, v in expected.items():
        assert r[k] == v, f"{text!r}: field {k} = {r[k]!r}, expected {v!r}"


def main() -> int:
    # 1) Pipe-delimited combo — the failing case from the screenshot.
    check("170.64.170.204|root|password123",
          host="170.64.170.204", port=22, user="root",
          auth_kind="password", secret="password123")
    check("217.144.154.180|root|LexaOrJake10976",
          host="217.144.154.180", port=22, user="root",
          auth_kind="password", secret="LexaOrJake10976")
    check("178.79.179.116|admin|admin",
          host="178.79.179.116", port=22, user="admin",
          auth_kind="password", secret="admin")

    # 2) Pipe-delimited with explicit port.
    check("1.2.3.4|2222|root|password123",
          host="1.2.3.4", port=2222, user="root",
          auth_kind="password", secret="password123")

    # 3) Colon-delimited combo without legacy auth_kind marker.
    check("1.2.3.4:root:mypass",
          host="1.2.3.4", port=22, user="root",
          auth_kind="password", secret="mypass")
    check("1.2.3.4:2222:root:mypass",
          host="1.2.3.4", port=2222, user="root",
          auth_kind="password", secret="mypass")

    # 4) Whitespace/tab combo.
    check("1.2.3.4 root mypass",
          host="1.2.3.4", port=22, user="root",
          auth_kind="password", secret="mypass")
    check("1.2.3.4\troot\tmypass",
          host="1.2.3.4", port=22, user="root",
          auth_kind="password", secret="mypass")
    check("1.2.3.4 2222 root mypass",
          host="1.2.3.4", port=2222, user="root",
          auth_kind="password", secret="mypass")

    # 5) Legacy forms still work.
    check("198.51.100.10",
          host="198.51.100.10", port=22, user="root",
          auth_kind="key", secret="")
    check("root@198.51.100.10",
          host="198.51.100.10", port=22, user="root",
          auth_kind="key", secret="")
    check("deploy@worker-02.example.com:2222",
          host="worker-02.example.com", port=2222, user="deploy",
          auth_kind="key", secret="")
    check("root@198.51.100.20:22:key:/root/.ssh/worker.pem",
          host="198.51.100.20", port=22, user="root",
          auth_kind="key", secret="/root/.ssh/worker.pem")
    check("deploy@198.51.100.21:2222:password:s3cretPass!",
          host="198.51.100.21", port=2222, user="deploy",
          auth_kind="password", secret="s3cretPass!")

    # 6) Auth_kind auto-detection: filesystem path → key.
    check("1.2.3.4|root|/home/me/.ssh/id_ed25519",
          host="1.2.3.4", port=22, user="root",
          auth_kind="key", secret="/home/me/.ssh/id_ed25519")
    check("1.2.3.4|root|~/.ssh/id_ed25519",
          host="1.2.3.4", port=22, user="root",
          auth_kind="key", secret="~/.ssh/id_ed25519")

    # 7) Passwords with embedded delimiters survive (rejoined with same delim).
    check("1.2.3.4|root|p|ss|word",
          host="1.2.3.4", port=22, user="root",
          auth_kind="password", secret="p|ss|word")
    check("1.2.3.4:root:p:ss:word",
          host="1.2.3.4", port=22, user="root",
          auth_kind="password", secret="p:ss:word")

    # 8) IPv4:port in the host field (pipe-delim line).
    check("1.2.3.4:22|root|mypass",
          host="1.2.3.4", port=22, user="root",
          auth_kind="password", secret="mypass")

    # 9) Comments + blank lines skipped; multi-line input.
    multi = """# fleet
1.2.3.4|root|pass1

# next
5.6.7.8|admin|pass2
"""
    rows = _parse_creds_text(multi)
    assert len(rows) == 2, f"expected 2 rows, got {len(rows)}: {rows}"
    assert rows[0]["host"] == "1.2.3.4" and rows[0]["secret"] == "pass1"
    assert rows[1]["host"] == "5.6.7.8" and rows[1]["secret"] == "pass2"

    # 10) Semicolon delimiter.
    check("1.2.3.4;root;mypass",
          host="1.2.3.4", port=22, user="root",
          auth_kind="password", secret="mypass")

    print("OK — all parse_creds cases pass")
    return 0


if __name__ == "__main__":
    sys.exit(main())
