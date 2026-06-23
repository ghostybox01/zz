#!/usr/bin/env python3
"""
asn_analysis.py — rank hits by ASN (Autonomous System Number)

Reads a hits-export JSON file, extracts the hostname from each hit URL,
resolves domains to IPs, looks up ASN via BGPView (free, no auth),
and prints a ranked table of which ASNs produced the most hits.

Usage:
    python3 asn_analysis.py hits-export-2026-06-22.json
    python3 asn_analysis.py hits-export-2026-06-22.json --top 20
    python3 asn_analysis.py hits-export-2026-06-22.json --type stripe
    python3 asn_analysis.py hits-export-2026-06-22.json --csv asn_report.csv
"""

import argparse
import csv
import json
import socket
import sys
import time
from collections import defaultdict
from urllib.parse import urlparse

import urllib.request
import urllib.error


# ── BGPView IP → ASN lookup ───────────────────────────────────────────────────

_asn_cache: dict = {}   # ip → (asn_number, asn_name, asn_description)
_ip_cache: dict  = {}   # hostname → ip


def resolve_ip(hostname: str) -> str | None:
    """Resolve a hostname to its first IPv4 address. Bare IPs pass through."""
    if hostname in _ip_cache:
        return _ip_cache[hostname]
    try:
        ip = socket.gethostbyname(hostname)
        _ip_cache[hostname] = ip
        return ip
    except OSError:
        _ip_cache[hostname] = None
        return None


def asn_lookup(ip: str) -> tuple[int, str, str]:
    """
    Query ipinfo.io/ip/json and return (asn_number, asn_name, org_string).
    Falls back to (0, "UNROUTED", "") on any failure.
    Free tier: 50k requests/month, no auth needed.
    """
    if ip in _asn_cache:
        return _asn_cache[ip]

    url = f"https://ipinfo.io/{ip}/json"
    try:
        req = urllib.request.Request(url, headers={"User-Agent": "reconx-asn-analysis/1.0", "Accept": "application/json"})
        with urllib.request.urlopen(req, timeout=8) as resp:
            data = json.loads(resp.read())

        # org field looks like "AS47583 Hostinger International Limited"
        org = data.get("org", "")
        if org and org.startswith("AS"):
            parts = org.split(" ", 1)
            try:
                asn_num = int(parts[0][2:])
                asn_name = parts[1] if len(parts) > 1 else org
            except ValueError:
                asn_num, asn_name = 0, org
        else:
            asn_num, asn_name = 0, org or "UNROUTED"

        country = data.get("country", "")
        result = (asn_num, asn_name, country)
    except Exception:
        result = (0, "LOOKUP_FAILED", "")

    _asn_cache[ip] = result
    time.sleep(0.05)   # gentle pacing
    return result


# ── Main ──────────────────────────────────────────────────────────────────────

def main():
    parser = argparse.ArgumentParser(description="Rank scan hits by ASN")
    parser.add_argument("hits_file", help="Path to hits-export JSON file")
    parser.add_argument("--top", type=int, default=25, help="How many ASNs to show (default 25)")
    parser.add_argument("--type", dest="filter_type", default="", help="Filter by hit type/regexName (e.g. stripe, smtp, aws)")
    parser.add_argument("--csv", dest="csv_out", default="", help="Also write full results to a CSV file")
    args = parser.parse_args()

    with open(args.hits_file) as f:
        hits = json.load(f)

    if not isinstance(hits, list):
        print("ERROR: expected a JSON array at the top level", file=sys.stderr)
        sys.exit(1)

    # Optional filter by credential type
    if args.filter_type:
        flt = args.filter_type.lower()
        hits = [
            h for h in hits
            if flt in str(h.get("regexName", "")).lower()
            or flt in str(h.get("type", "")).lower()
        ]
        print(f"Filtered to {len(hits)} hits matching '{args.filter_type}'")

    print(f"Processing {len(hits)} hits — resolving hosts and looking up ASNs…\n")

    # Count hits per ASN
    asn_hits:   dict[int, int]  = defaultdict(int)
    asn_meta:   dict[int, tuple] = {}   # asn → (name, description)
    asn_types:  dict[int, dict]  = defaultdict(lambda: defaultdict(int))
    failed_resolve = 0
    total = len(hits)

    for i, hit in enumerate(hits, 1):
        url = hit.get("url", "")
        if not url:
            continue

        parsed = urlparse(url)
        hostname = parsed.hostname or ""
        if not hostname:
            continue

        ip = resolve_ip(hostname)
        if not ip:
            failed_resolve += 1
            continue

        asn_num, asn_name, asn_desc = asn_lookup(ip)

        asn_hits[asn_num] += 1
        asn_meta[asn_num] = (asn_name, asn_desc)
        hit_type = hit.get("regexName") or hit.get("type") or "unknown"
        asn_types[asn_num][hit_type] += 1

        if i % 20 == 0 or i == total:
            pct = i / total * 100
            print(f"  {i}/{total} ({pct:.0f}%)  —  {len(asn_hits)} unique ASNs so far", end="\r")

    print(f"\n\nDone. {failed_resolve} hosts could not be resolved.\n")

    # Sort by hit count descending
    ranked = sorted(asn_hits.items(), key=lambda x: x[1], reverse=True)

    # ── Print table ───────────────────────────────────────────────────────────
    total_hits = sum(asn_hits.values())
    print(f"{'Rank':<5} {'ASN':<8} {'Hits':<7} {'%':>5}  {'Name':<20} {'Description'}")
    print("─" * 90)

    for rank, (asn_num, count) in enumerate(ranked[: args.top], 1):
        name, desc = asn_meta.get(asn_num, ("", ""))
        pct = count / total_hits * 100
        asn_str = f"AS{asn_num}" if asn_num else "—"
        print(f"{rank:<5} {asn_str:<8} {count:<7} {pct:>4.1f}%  {name:<20} {desc}")

        # Top credential types for this ASN
        types = sorted(asn_types[asn_num].items(), key=lambda x: x[1], reverse=True)[:4]
        type_str = "  ".join(f"{t}({n})" for t, n in types)
        print(f"      {'':8} {'':7}        └─ {type_str}")

    print("─" * 90)
    print(f"Total hits accounted for: {total_hits}  |  Unique ASNs: {len(asn_hits)}")

    # ── CSV export ────────────────────────────────────────────────────────────
    if args.csv_out:
        with open(args.csv_out, "w", newline="") as f:
            writer = csv.writer(f)
            writer.writerow(["rank", "asn", "asn_name", "description", "hits", "pct", "top_types"])
            for rank, (asn_num, count) in enumerate(ranked, 1):
                name, desc = asn_meta.get(asn_num, ("", ""))
                pct = count / total_hits * 100
                types = sorted(asn_types[asn_num].items(), key=lambda x: x[1], reverse=True)[:5]
                type_str = " | ".join(f"{t}:{n}" for t, n in types)
                writer.writerow([rank, f"AS{asn_num}", name, desc, count, f"{pct:.2f}", type_str])
        print(f"\nCSV written to {args.csv_out}")


if __name__ == "__main__":
    main()
