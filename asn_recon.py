#!/usr/bin/env python3
"""
asn_recon.py — ASN → Domains + IPs recon tool

Given one or more ASNs, produces a scan-ready target list by:
  1. Fetching all IP prefixes (CIDR ranges) via BGPView
  2. Running concurrent reverse DNS on every IP in range
  3. Pulling certificate transparency domains from crt.sh
  4. Optional Shodan search (set SHODAN_API_KEY env var)

Output files (written to --out-dir, default: ./recon_<ASN>/):
  ips.txt        — every routable IP in the ASN ranges
  domains.txt    — unique FQDNs from rdns + crt.sh + Shodan
  combined.txt   — domains first, then IPs (ready to feed the scanner)
  summary.txt    — stats per source

Usage:
  python3 asn_recon.py AS13335
  python3 asn_recon.py AS13335 AS14061 AS24940
  python3 asn_recon.py AS13335 --skip-rdns --workers 200
  python3 asn_recon.py AS13335 --cidr-only        # just dump the CIDRs, no expansion
  python3 asn_recon.py AS13335 --max-ips 50000     # cap IP expansion (default 100k)
  SHODAN_API_KEY=xxx python3 asn_recon.py AS13335
"""

import argparse
import ipaddress
import json
import os
import socket
import sys
import time
import urllib.error
import urllib.request
from collections import defaultdict
from concurrent.futures import ThreadPoolExecutor, as_completed
from pathlib import Path

# ── Colour helpers (no deps) ──────────────────────────────────────────────────

RESET = "\033[0m"
BOLD  = "\033[1m"
GREEN = "\033[32m"
CYAN  = "\033[36m"
YELLOW= "\033[33m"
RED   = "\033[31m"
DIM   = "\033[2m"

def c(text, colour): return f"{colour}{text}{RESET}"
def ok(msg):    print(c("  ✓ ", GREEN)  + msg)
def info(msg):  print(c("  → ", CYAN)   + msg)
def warn(msg):  print(c("  ⚠ ", YELLOW) + msg)
def err(msg):   print(c("  ✗ ", RED)    + msg)
def hdr(msg):   print(f"\n{BOLD}{msg}{RESET}")


# ── Rate-limited HTTP GET ─────────────────────────────────────────────────────

def fetch_json(url: str, retries: int = 3, pause: float = 1.0) -> dict | list | None:
    headers = {
        "User-Agent": "reconx-asn-recon/1.0",
        "Accept":     "application/json",
    }
    for attempt in range(1, retries + 1):
        try:
            req = urllib.request.Request(url, headers=headers)
            with urllib.request.urlopen(req, timeout=15) as r:
                return json.loads(r.read())
        except urllib.error.HTTPError as e:
            if e.code == 429:
                wait = pause * (2 ** attempt)
                warn(f"Rate limited by {url[:60]} — waiting {wait:.0f}s")
                time.sleep(wait)
            elif e.code == 404:
                return None
            else:
                if attempt == retries:
                    err(f"HTTP {e.code} fetching {url[:80]}")
        except Exception as e:
            if attempt == retries:
                err(f"Error fetching {url[:80]}: {e}")
            time.sleep(pause)
    return None


# ── Source 1: IP prefixes for an ASN (RIPE Stat primary, HackerTarget fallback) ──

def get_prefixes_ripe(num: str) -> list[str]:
    """RIPE Stat announced-prefixes — authoritative, no auth required."""
    url = f"https://stat.ripe.net/data/announced-prefixes/data.json?resource=AS{num}"
    data = fetch_json(url)
    if not data or data.get("status") not in ("ok", "supported"):
        return []
    cidrs = []
    for entry in data.get("data", {}).get("prefixes", []):
        prefix = entry.get("prefix", "")
        if prefix and ":" not in prefix:   # IPv4 only
            cidrs.append(prefix)
    return cidrs


def get_prefixes_hackertarget(num: str) -> list[str]:
    """HackerTarget aslookup — plain-text CIDR list, IPv4 only."""
    url = f"https://api.hackertarget.com/aslookup/?q=AS{num}"
    try:
        req = urllib.request.Request(url, headers={"User-Agent": "reconx-asn-recon/1.0"})
        with urllib.request.urlopen(req, timeout=15) as r:
            text = r.read().decode()
    except Exception:
        return []
    cidrs = []
    for line in text.splitlines():
        line = line.strip()
        if "/" in line and not line.startswith('"'):
            cidrs.append(line)
    return cidrs


def get_prefixes(asn: str) -> list[str]:
    """Return IPv4 CIDR list for an ASN. Tries RIPE Stat then HackerTarget."""
    num = asn.lstrip("ASas")

    cidrs = get_prefixes_ripe(num)
    if cidrs:
        return cidrs

    warn(f"RIPE Stat returned nothing for AS{num} — trying HackerTarget…")
    cidrs = get_prefixes_hackertarget(num)
    if cidrs:
        return cidrs

    warn(f"No prefixes found for AS{num} from any source")
    return []


# ── Source 2: Reverse DNS ─────────────────────────────────────────────────────

def rdns(ip: str) -> str | None:
    try:
        name = socket.gethostbyaddr(ip)[0]
        return name.rstrip(".")
    except Exception:
        return None


def reverse_dns_range(ips: list[str], workers: int, max_ips: int) -> dict[str, str]:
    """
    Run reverse DNS on up to max_ips IPs concurrently.
    Returns {ip: hostname} for IPs that resolved.
    """
    if len(ips) > max_ips:
        warn(f"Capping rdns at {max_ips:,} IPs (range has {len(ips):,})")
        ips = ips[:max_ips]

    results: dict[str, str] = {}
    done = 0
    total = len(ips)

    with ThreadPoolExecutor(max_workers=workers) as pool:
        futures = {pool.submit(rdns, ip): ip for ip in ips}
        for fut in as_completed(futures):
            ip = futures[fut]
            hostname = fut.result()
            if hostname:
                results[ip] = hostname
            done += 1
            if done % 500 == 0 or done == total:
                pct = done / total * 100
                print(f"\r    rdns: {done:,}/{total:,} ({pct:.0f}%) — {len(results):,} resolved", end="", flush=True)

    print()  # newline after progress
    return results


# ── Source 3: Certificate Transparency via crt.sh ────────────────────────────

def crtsh_by_asn(asn: str) -> list[str]:
    # Per-IP queries in crtsh_for_ips() give better coverage than org-name search.
    return []


def crtsh_by_ip(ip: str) -> list[str]:
    """Get all SANs from certs ever issued for this IP address."""
    url = f"https://crt.sh/?q={ip}&output=json"
    data = fetch_json(url, retries=2, pause=2.0)
    if not data or not isinstance(data, list):
        return []
    domains = set()
    for entry in data:
        for name in (entry.get("name_value") or "").split("\n"):
            name = name.strip().lstrip("*.")
            if name and "." in name and not name.startswith("-"):
                domains.add(name.lower())
    return list(domains)


def crtsh_for_ips(ips: list[str], workers: int, max_ips: int = 500) -> list[str]:
    """
    Query crt.sh serially with a delay — it rate-limits any concurrency hard.
    Caps at max_ips (default 500); use --crtsh-limit to raise it.
    """
    sample = ips[:max_ips]
    if len(ips) > max_ips:
        warn(f"crt.sh: sampling {max_ips:,} of {len(ips):,} IPs to avoid rate limits")

    found: set[str] = set()
    for i, ip in enumerate(sample, 1):
        domains = crtsh_by_ip(ip)
        found.update(domains)
        if i % 20 == 0 or i == len(sample):
            print(f"\r    crt.sh: {i:,}/{len(sample):,} IPs queried — {len(found):,} domains", end="", flush=True)
        time.sleep(1.2)   # stay well under crt.sh rate limit

    print()
    return list(found)


# ── Source 4: Shodan (optional) ───────────────────────────────────────────────

def shodan_asn(asn: str, api_key: str) -> tuple[list[str], list[str]]:
    """
    Search Shodan for hosts in this ASN.
    Returns (ips, hostnames).
    Requires a paid Shodan API key for full results.
    """
    num = asn.lstrip("ASas")
    url = f"https://api.shodan.io/shodan/host/search?key={api_key}&query=asn:AS{num}&facets=ip&minify=true"
    data = fetch_json(url)
    if not data or "matches" not in data:
        return [], []

    ips: list[str] = []
    hostnames: list[str] = []
    for match in data.get("matches", []):
        ip = match.get("ip_str", "")
        if ip:
            ips.append(ip)
        for h in match.get("hostnames", []):
            if h:
                hostnames.append(h.lower())

    # Shodan paginates — fetch next pages
    total = data.get("total", 0)
    page = 1
    while len(ips) < total and page < 10:   # cap at 10 pages
        page += 1
        purl = url + f"&page={page}"
        pdata = fetch_json(purl, pause=1.0)
        if not pdata or "matches" not in pdata:
            break
        for match in pdata.get("matches", []):
            ip = match.get("ip_str", "")
            if ip:
                ips.append(ip)
            for h in match.get("hostnames", []):
                if h:
                    hostnames.append(h.lower())
        time.sleep(1)   # Shodan rate limit

    return ips, hostnames


# ── Expand CIDRs to individual IPs ───────────────────────────────────────────

def expand_cidrs(cidrs: list[str], max_ips: int) -> list[str]:
    ips: list[str] = []
    for cidr in cidrs:
        try:
            net = ipaddress.ip_network(cidr, strict=False)
            # Skip private/loopback ranges
            if net.is_private or net.is_loopback or net.is_link_local:
                continue
            for ip in net.hosts():
                ips.append(str(ip))
                if len(ips) >= max_ips:
                    warn(f"Reached max IP cap ({max_ips:,}) — truncating")
                    return ips
        except ValueError:
            warn(f"Invalid CIDR: {cidr}")
    return ips


# ── Output helpers ────────────────────────────────────────────────────────────

def write_list(path: Path, items: list[str]) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text("\n".join(sorted(set(items))) + "\n", encoding="utf-8")


# ── Main ──────────────────────────────────────────────────────────────────────

def process_asn(asn: str, args: argparse.Namespace, out_dir: Path) -> dict:
    asn = asn.upper()
    if not asn.startswith("AS"):
        asn = "AS" + asn

    hdr(f"[{asn}] Starting recon")

    stats: dict = {
        "asn": asn,
        "cidrs": 0,
        "total_ips": 0,
        "rdns_resolved": 0,
        "crtsh_domains": 0,
        "shodan_ips": 0,
        "shodan_domains": 0,
        "unique_ips": 0,
        "unique_domains": 0,
    }

    # ── Step 1: IP prefixes ───────────────────────────────────────────────────
    info("Fetching IP prefixes from BGPView…")
    cidrs = get_prefixes(asn)
    if not cidrs:
        err(f"No prefixes found for {asn}")
        return stats

    stats["cidrs"] = len(cidrs)
    ok(f"{len(cidrs)} CIDR ranges found")

    if args.cidr_only:
        cidr_file = out_dir / f"{asn}_cidrs.txt"
        write_list(cidr_file, cidrs)
        ok(f"CIDRs written to {cidr_file}")
        return stats

    # ── Step 2: Expand to IPs ─────────────────────────────────────────────────
    info(f"Expanding CIDRs to IPs (cap: {args.max_ips:,})…")
    all_ips = expand_cidrs(cidrs, args.max_ips)
    stats["total_ips"] = len(all_ips)
    ok(f"{len(all_ips):,} IPs in range")

    collected_domains: set[str] = set()
    collected_ips: set[str]     = set(all_ips)

    # ── Step 3: Reverse DNS ───────────────────────────────────────────────────
    if not args.skip_rdns and all_ips:
        info(f"Reverse DNS ({args.workers} workers)…")
        rdns_map = reverse_dns_range(all_ips, args.workers, args.max_rdns)
        stats["rdns_resolved"] = len(rdns_map)
        collected_domains.update(rdns_map.values())
        ok(f"{len(rdns_map):,} hostnames from rdns")

    # ── Step 4: Certificate transparency ─────────────────────────────────────
    if getattr(args, "crtsh", False) and not args.skip_crtsh and all_ips:
        info("Querying crt.sh (certificate transparency)…")
        crt_domains = crtsh_for_ips(all_ips, args.workers, max_ips=args.crtsh_limit)
        stats["crtsh_domains"] = len(crt_domains)
        collected_domains.update(crt_domains)
        ok(f"{len(crt_domains):,} domains from crt.sh")

    # ── Step 5: Shodan (optional) ─────────────────────────────────────────────
    shodan_key = args.shodan_key or os.environ.get("SHODAN_API_KEY", "")
    if shodan_key:
        info("Querying Shodan…")
        sh_ips, sh_hosts = shodan_asn(asn, shodan_key)
        stats["shodan_ips"]     = len(sh_ips)
        stats["shodan_domains"] = len(sh_hosts)
        collected_ips.update(sh_ips)
        collected_domains.update(sh_hosts)
        ok(f"Shodan: {len(sh_ips):,} IPs, {len(sh_hosts):,} hostnames")
    else:
        info("Shodan skipped (set SHODAN_API_KEY env var or --shodan-key to enable)")

    # ── Write output files ────────────────────────────────────────────────────
    ip_list     = sorted(collected_ips,     key=lambda x: ipaddress.ip_address(x) if is_ip(x) else ipaddress.ip_address("0.0.0.0"))
    domain_list = sorted(collected_domains)

    stats["unique_ips"]     = len(ip_list)
    stats["unique_domains"] = len(domain_list)

    write_list(out_dir / "ips.txt",     ip_list)
    write_list(out_dir / "domains.txt", domain_list)

    # combined: domains first (higher signal), then IPs
    combined = domain_list + ip_list
    (out_dir / "combined.txt").write_text("\n".join(combined) + "\n", encoding="utf-8")

    # summary
    summary_lines = [
        f"ASN:             {asn}",
        f"CIDR ranges:     {stats['cidrs']}",
        f"IPs in range:    {stats['total_ips']:,}",
        f"rdns resolved:   {stats['rdns_resolved']:,}",
        f"crt.sh domains:  {stats['crtsh_domains']:,}",
        f"Shodan IPs:      {stats['shodan_ips']:,}",
        f"Shodan domains:  {stats['shodan_domains']:,}",
        f"─────────────────────────────",
        f"Unique IPs out:  {stats['unique_ips']:,}",
        f"Unique domains:  {stats['unique_domains']:,}",
        f"Combined total:  {len(combined):,}",
    ]
    (out_dir / "summary.txt").write_text("\n".join(summary_lines) + "\n")

    hdr(f"[{asn}] Done")
    for line in summary_lines:
        print(f"  {line}")

    return stats


def is_ip(s: str) -> bool:
    try:
        ipaddress.ip_address(s)
        return True
    except ValueError:
        return False


def main():
    parser = argparse.ArgumentParser(
        description="ASN → Domains + IPs recon tool",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog=__doc__,
    )
    parser.add_argument("asns", nargs="+", help="One or more ASNs (e.g. AS13335 14061)")
    parser.add_argument("--out-dir",     default="",    help="Output directory (default: ./recon_<ASN>)")
    parser.add_argument("--workers",     type=int, default=100, help="Concurrent rdns workers (default: 100)")
    parser.add_argument("--max-ips",     type=int, default=100_000, help="Max IPs to expand per ASN (default: 100k)")
    parser.add_argument("--max-rdns",    type=int, default=50_000,  help="Max IPs to rdns per ASN (default: 50k)")
    parser.add_argument("--crtsh-limit", type=int, default=1000,    help="Max IPs to query on crt.sh (default: 1000)")
    parser.add_argument("--skip-rdns",   action="store_true", help="Skip reverse DNS step")
    parser.add_argument("--skip-crtsh",  action="store_true", default=True,  help="Skip crt.sh (default: skipped — use --crtsh to enable)")
    parser.add_argument("--crtsh",       action="store_true", help="Enable crt.sh certificate transparency queries (slow, rate-limited)")
    parser.add_argument("--cidr-only",   action="store_true", help="Only dump CIDR ranges, no expansion")
    parser.add_argument("--shodan-key",  default="",    help="Shodan API key (or set SHODAN_API_KEY env var)")
    args = parser.parse_args()

    all_stats = []
    for asn in args.asns:
        asn_clean = asn.upper().lstrip("AS")
        asn_tag   = f"AS{asn_clean}"

        if args.out_dir:
            out_dir = Path(args.out_dir) / asn_tag
        else:
            out_dir = Path(f"recon_{asn_tag}")

        out_dir.mkdir(parents=True, exist_ok=True)

        stats = process_asn(asn, args, out_dir)
        all_stats.append(stats)

    # If multiple ASNs, also write a merged combined.txt
    if len(args.asns) > 1:
        merged_dir = Path(args.out_dir) if args.out_dir else Path("recon_merged")
        merged_dir.mkdir(parents=True, exist_ok=True)

        all_domains: set[str] = set()
        all_ips_set: set[str] = set()

        for asn in args.asns:
            asn_tag = "AS" + asn.upper().lstrip("AS")
            base = Path(args.out_dir) / asn_tag if args.out_dir else Path(f"recon_{asn_tag}")
            d_file = base / "domains.txt"
            i_file = base / "ips.txt"
            if d_file.exists():
                all_domains.update(d_file.read_text().splitlines())
            if i_file.exists():
                all_ips_set.update(i_file.read_text().splitlines())

        all_domains.discard("")
        all_ips_set.discard("")

        merged_domains = sorted(all_domains)
        merged_ips     = sorted(all_ips_set, key=lambda x: ipaddress.ip_address(x) if is_ip(x) else ipaddress.ip_address("0.0.0.0"))
        merged_combined = merged_domains + merged_ips

        write_list(merged_dir / "domains.txt",  merged_domains)
        write_list(merged_dir / "ips.txt",      merged_ips)
        (merged_dir / "combined.txt").write_text("\n".join(merged_combined) + "\n")

        hdr("Merged output")
        ok(f"{len(merged_domains):,} unique domains → {merged_dir}/domains.txt")
        ok(f"{len(merged_ips):,} unique IPs      → {merged_dir}/ips.txt")
        ok(f"{len(merged_combined):,} combined       → {merged_dir}/combined.txt")


if __name__ == "__main__":
    main()
