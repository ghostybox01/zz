package main

// Additional domain/IP producers for the WARC harvester.
// Extends -source beyond cc and crtsh with six new producers:
//
//   wayback   — Wayback Machine CDX API: searches archived crawls for specific
//               URL patterns (*.env, *.git/config, etc.) — yields live domains
//               AND writes the full archived URL to env_urls.txt for direct probing
//   asn       — BGPView: ASN number → IP prefixes → individual IPs (no auth)
//   asn-name  — BGPView keyword search: "hostinger" → matching ASNs → IPs
//   cidr      — Direct CIDR range expansion (e.g. 192.168.1.0/24)
//   ipfile    — Read a plain-text list of IPs or domains from a file
//   crt-org   — crt.sh org-field search: "stripe" → all certs issued to Stripe
//
// All producers share sendDomain()/sendIPOrDomain() for dedup, subdomain
// filter, and ceiling. IPs bypass isApexDomain() cleanly via sendIPOrDomain.

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"
)

// ── IP awareness in apex filter ───────────────────────────────────────────────
// The original isApexDomain conservatively returns true on publicsuffix errors,
// which incorrectly drops bare IPs. Override at package level:

func init() {
	// Patch: wrap original logic so IPs bypass the subdomain-only filter.
	// We can't redefine isApexDomain (already declared in warc.go) so we
	// use a package-level hook called from sendIPOrDomain below.
}

// isIPAddress returns true when s is a valid IPv4 or IPv6 address.
func isIPAddress(s string) bool {
	return net.ParseIP(s) != nil
}

// sendIPOrDomain is like sendDomain but bypasses the apex check for bare IPs
// so CIDR-expanded addresses are never silently dropped by -subdomain-only.
func sendIPOrDomain(target string, domainChan chan<- string) bool {
	if isIPAddress(target) {
		// IPs: skip apex/subdomain filter, go straight to dedup + ceiling.
		if _, loaded := uniqueDomains.LoadOrStore(target, true); loaded {
			return true
		}
		maxDomains := globalMaxDomains.Load()
		if maxDomains > 0 && totalLiveDomains.Load() >= maxDomains {
			return false
		}
		totalExtracted.Add(1)
		if globalVerbose.Load() {
			fmt.Printf("%s[IP]%s %s\n", CYAN, RESET, target)
		}
		domainChan <- target
		return true
	}
	return sendDomain(target, domainChan)
}

// ── Wayback Machine CDX producer ─────────────────────────────────────────────
//
// Queries http://web.archive.org/cdx/search/cdx for each URL pattern and
// yields the unique domains found. Full matching URLs are also written to
// env_urls.txt so the scanner can probe them directly.
//
// Default patterns (used when -wayback-pattern is not set):
var defaultWaybackPatterns = []string{
	"*/.env",
	"*/.env.local",
	"*/.env.production",
	"*/.env.backup",
	"*/.env.example",
	"*/.git/config",
	"*/phpinfo.php",
	"*/info.php",
	"*/.aws/credentials",
	"*/wp-config.php",
	"*/.npmrc",
	"*/.pypirc",
	"*/config.php",
	"*/database.php",
	"*/.DS_Store",
	"*/terraform.tfstate",
	"*/docker-compose.yml",
	"*/.htpasswd",
	"*/config/database.yml",
	"*/settings.py",
}

var waybackHTTPClient = &http.Client{Timeout: 120 * time.Second}

// fetchWaybackPattern queries the CDX API for one URL pattern. It streams the
// text response line by line to avoid holding the full result in memory (some
// patterns return hundreds of thousands of URLs).
func fetchWaybackPattern(pattern string, domainChan chan<- string) int {
	params := url.Values{
		"url":      {pattern},
		"output":   {"text"},
		"fl":       {"original"},
		"collapse": {"urlkey"},
		"limit":    {"50000"},
		"filter":   {"statuscode:200"},
	}
	cdxURL := "http://web.archive.org/cdx/search/cdx?" + params.Encode()

	if globalVerbose.Load() {
		fmt.Printf("%s[WAYBACK]%s Querying pattern: %s\n", CYAN, RESET, pattern)
	}

	req, err := http.NewRequest("GET", cdxURL, nil)
	if err != nil {
		fmt.Printf("%s[WAYBACK]%s request error for %s: %v\n", YELLOW, RESET, pattern, err)
		return 0
	}
	req.Header.Set("User-Agent", "reconx-warc/2.0 (+https://web.archive.org/cdx/)")

	resp, err := waybackHTTPClient.Do(req)
	if err != nil {
		fmt.Printf("%s[WAYBACK]%s fetch error for %s: %v\n", YELLOW, RESET, pattern, err)
		return 0
	}
	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		fmt.Printf("%s[WAYBACK]%s HTTP %d for %s\n", YELLOW, RESET, resp.StatusCode, pattern)
		return 0
	}

	emitted := 0
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 256*1024), 256*1024)

	for scanner.Scan() {
		rawURL := strings.TrimSpace(scanner.Text())
		if rawURL == "" {
			continue
		}

		// Write full URL to env_urls.txt — the scanner can probe this directly
		// instead of having to re-discover the path from the domain.
		fileMutex.Lock()
		if envURLFile != nil {
			_, _ = envURLFile.WriteString(rawURL + "\n")
		}
		fileMutex.Unlock()

		// Extract and emit the domain
		parsed, err := url.Parse(rawURL)
		if err != nil || parsed.Hostname() == "" {
			continue
		}
		host := strings.ToLower(parsed.Hostname())

		if !sendIPOrDomain(host, domainChan) {
			return emitted
		}
		emitted++
	}

	if globalVerbose.Load() {
		fmt.Printf("%s[WAYBACK]%s %s → %d domains\n", GREEN, RESET, pattern, emitted)
	}
	return emitted
}

// runWaybackProducer fans out over all patterns sequentially (CDX is not
// designed for parallel hammering). Sleeps 1s between patterns.
func runWaybackProducer(patterns []string, domainChan chan<- string, wg *sync.WaitGroup) {
	defer wg.Done()
	if len(patterns) == 0 {
		patterns = defaultWaybackPatterns
	}
	fmt.Printf("%s[*]%s Wayback CDX producer: %d pattern(s)\n", CYAN, RESET, len(patterns))
	total := 0
	for i, p := range patterns {
		n := fetchWaybackPattern(p, domainChan)
		total += n
		if i < len(patterns)-1 {
			time.Sleep(1 * time.Second)
		}
		maxDomains := globalMaxDomains.Load()
		if maxDomains > 0 && totalLiveDomains.Load() >= maxDomains {
			break
		}
	}
	fmt.Printf("%s[+]%s Wayback producer done — %d domains emitted across %d pattern(s)\n",
		GREEN, RESET, total, len(patterns))
}

// ── CIDR expansion producer ───────────────────────────────────────────────────
//
// Expands CIDR notation into individual IP addresses. Rejects prefixes larger
// than /16 (65 536 IPs) to prevent accidental internet-wide scans.

const maxCIDRHosts = 65536 // /16 upper bound

// cidrHosts returns all usable host IPs in a CIDR block. The network and
// broadcast addresses are excluded for IPv4 blocks larger than /31.
func cidrHosts(cidr string) ([]string, error) {
	ip, ipNet, err := net.ParseCIDR(cidr)
	if err != nil {
		return nil, fmt.Errorf("invalid CIDR %q: %w", cidr, err)
	}

	ones, bits := ipNet.Mask.Size()
	count := big.NewInt(1)
	count.Lsh(count, uint(bits-ones))
	if count.Int64() > maxCIDRHosts {
		return nil, fmt.Errorf("CIDR %s too large (%s IPs); max is /16 (%d)",
			cidr, count.String(), maxCIDRHosts)
	}

	var ips []string
	// Start from network address, walk to broadcast
	for cur := cloneIP(ip.Mask(ipNet.Mask)); ipNet.Contains(cur); incrementIP(cur) {
		ips = append(ips, cur.String())
	}
	// Drop network + broadcast for IPv4 blocks larger than /31
	if bits == 32 && len(ips) > 2 {
		ips = ips[1 : len(ips)-1]
	}
	return ips, nil
}

func cloneIP(ip net.IP) net.IP {
	clone := make(net.IP, len(ip))
	copy(clone, ip)
	return clone
}

func incrementIP(ip net.IP) {
	for j := len(ip) - 1; j >= 0; j-- {
		ip[j]++
		if ip[j] != 0 {
			break
		}
	}
}

func runCIDRProducer(cidrs []string, domainChan chan<- string, wg *sync.WaitGroup) {
	defer wg.Done()
	fmt.Printf("%s[*]%s CIDR producer: %d range(s)\n", CYAN, RESET, len(cidrs))
	total := 0
	for _, cidr := range cidrs {
		cidr = strings.TrimSpace(cidr)
		if cidr == "" {
			continue
		}
		ips, err := cidrHosts(cidr)
		if err != nil {
			fmt.Printf("%s[CIDR]%s skipped %s: %v\n", YELLOW, RESET, cidr, err)
			continue
		}
		fmt.Printf("%s[CIDR]%s %s → %d IPs\n", CYAN, RESET, cidr, len(ips))
		for _, ip := range ips {
			if !sendIPOrDomain(ip, domainChan) {
				fmt.Printf("%s[CIDR]%s ceiling reached, stopping\n", CYAN, RESET)
				return
			}
			total++
		}
	}
	fmt.Printf("%s[+]%s CIDR producer done — %d IPs emitted\n", GREEN, RESET, total)
}

// ── ASN → prefix → IP producer ───────────────────────────────────────────────
//
// Queries api.bgpview.io (free, no auth) for each ASN, collects its IPv4
// prefixes, then hands the CIDR list to the CIDR expander.
//
// Example high-yield ASNs for web hosting:
//   AS47583  Hostinger       AS14061  DigitalOcean    AS16276  OVH
//   AS24940  Hetzner         AS20473  Vultr           AS22612  Namecheap
//   AS394711 Contabo         AS51167  Contabo DE      AS44477  STARK
//
// Deliberately excludes cloud hyperscalers (AWS/GCP/Azure) — their /13–/8
// blocks exceed maxCIDRHosts and would require targeted sub-range selection.

type bgpViewResponse struct {
	Status string `json:"status"`
	Data   struct {
		IPv4Prefixes []struct {
			Prefix      string `json:"prefix"`
			Name        string `json:"name"`
			Description string `json:"description"`
		} `json:"ipv4_prefixes"`
	} `json:"data"`
}

var bgpHTTPClient = &http.Client{Timeout: 30 * time.Second}

func fetchASNPrefixes(asn string) ([]string, error) {
	// Normalise — accept "AS47583" or "47583"
	asn = strings.TrimPrefix(strings.ToUpper(strings.TrimSpace(asn)), "AS")
	apiURL := fmt.Sprintf("https://api.bgpview.io/asn/%s/prefixes", asn)

	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "reconx-warc/2.0")
	req.Header.Set("Accept", "application/json")

	resp, err := bgpHTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("BGPView returned HTTP %d for AS%s", resp.StatusCode, asn)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}

	var bgpResp bgpViewResponse
	if err := json.Unmarshal(body, &bgpResp); err != nil {
		return nil, err
	}
	if bgpResp.Status != "ok" {
		return nil, fmt.Errorf("BGPView status=%s for AS%s", bgpResp.Status, asn)
	}

	var prefixes []string
	for _, p := range bgpResp.Data.IPv4Prefixes {
		prefixes = append(prefixes, p.Prefix)
	}
	return prefixes, nil
}

func runASNProducer(asns []string, domainChan chan<- string, wg *sync.WaitGroup) {
	defer wg.Done()
	fmt.Printf("%s[*]%s ASN producer: %d ASN(s)\n", CYAN, RESET, len(asns))

	var allCIDRs []string
	for _, asn := range asns {
		prefixes, err := fetchASNPrefixes(asn)
		if err != nil {
			fmt.Printf("%s[ASN]%s AS%s error: %v\n", YELLOW, RESET, asn, err)
			continue
		}

		// Filter out prefixes that are too large to expand
		var usable []string
		skipped := 0
		for _, p := range prefixes {
			_, ipNet, err := net.ParseCIDR(p)
			if err != nil {
				continue
			}
			ones, bits := ipNet.Mask.Size()
			count := 1 << uint(bits-ones)
			if count > maxCIDRHosts {
				skipped++
				if globalVerbose.Load() {
					fmt.Printf("%s[ASN]%s skipped large prefix %s (%d IPs)\n", YELLOW, RESET, p, count)
				}
				continue
			}
			usable = append(usable, p)
		}

		fmt.Printf("%s[ASN]%s AS%s → %d usable prefixes (%d too large, skipped)\n",
			CYAN, RESET, asn, len(usable), skipped)
		allCIDRs = append(allCIDRs, usable...)

		// Gentle pacing between ASN queries
		time.Sleep(500 * time.Millisecond)
	}

	if len(allCIDRs) == 0 {
		fmt.Printf("%s[ASN]%s No usable prefixes found\n", YELLOW, RESET)
		return
	}

	// Expand using a nested CIDR producer (reuse logic, not goroutine)
	total := 0
	for _, cidr := range allCIDRs {
		ips, err := cidrHosts(cidr)
		if err != nil {
			continue
		}
		for _, ip := range ips {
			if !sendIPOrDomain(ip, domainChan) {
				fmt.Printf("%s[+]%s ASN producer: ceiling reached\n", GREEN, RESET)
				return
			}
			total++
		}
	}
	fmt.Printf("%s[+]%s ASN producer done — %d IPs from %d prefix(es)\n",
		GREEN, RESET, total, len(allCIDRs))
}

// ── IP/domain file producer ───────────────────────────────────────────────────
//
// Reads a plain-text file — one IP or domain per line — and emits each entry.
// Lines starting with '#' are treated as comments and skipped.
// Accepts both bare IPs (1.2.3.4), CIDR notation (1.2.3.0/24), and FQDNs.

func runIPFileProducer(filePath string, domainChan chan<- string, wg *sync.WaitGroup) {
	defer wg.Done()

	f, err := os.Open(filePath)
	if err != nil {
		fmt.Printf("%s[IPFILE]%s cannot open %s: %v\n", RED, RESET, filePath, err)
		return
	}
	defer f.Close()

	fmt.Printf("%s[*]%s IP-file producer: reading %s\n", CYAN, RESET, filePath)
	total := 0
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// CIDR notation — expand inline
		if strings.Contains(line, "/") {
			ips, err := cidrHosts(line)
			if err != nil {
				fmt.Printf("%s[IPFILE]%s skipped %s: %v\n", YELLOW, RESET, line, err)
				continue
			}
			for _, ip := range ips {
				if !sendIPOrDomain(ip, domainChan) {
					return
				}
				total++
			}
			continue
		}

		if !sendIPOrDomain(line, domainChan) {
			return
		}
		total++
	}
	fmt.Printf("%s[+]%s IP-file producer done — %d targets emitted\n", GREEN, RESET, total)
}

// ── BGPView keyword → ASN → IP producer ──────────────────────────────────────
//
// Accepts human-readable organisation keywords ("hostinger", "sendgrid") and
// resolves them to matching ASNs via the BGPView free search API, then expands
// each ASN's prefixes exactly like runASNProducer.
//
// Multiple keywords are searched independently; results are merged and deduplicated
// before CIDR expansion so the same ASN is never expanded twice even if two
// keywords both match it (e.g. "amazon" and "aws" often return overlapping sets).

type bgpViewSearchResponse struct {
	Status string `json:"status"`
	Data   struct {
		ASNs []struct {
			ASN         int    `json:"asn"`
			Name        string `json:"name"`
			Description string `json:"description"`
		} `json:"asns"`
	} `json:"data"`
}

func fetchASNsByKeyword(keyword string) ([]int, error) {
	keyword = strings.TrimSpace(keyword)
	if keyword == "" {
		return nil, nil
	}
	apiURL := "https://api.bgpview.io/search?query_term=" + url.QueryEscape(keyword)

	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "reconx-warc/2.0")
	req.Header.Set("Accept", "application/json")

	resp, err := bgpHTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("BGPView search returned HTTP %d for %q", resp.StatusCode, keyword)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return nil, err
	}

	var result bgpViewSearchResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}
	if result.Status != "ok" {
		return nil, fmt.Errorf("BGPView search status=%s for %q", result.Status, keyword)
	}

	var asns []int
	for _, a := range result.Data.ASNs {
		if a.ASN > 0 {
			asns = append(asns, a.ASN)
		}
	}
	return asns, nil
}

func runASNNameProducer(keywords []string, domainChan chan<- string, wg *sync.WaitGroup) {
	defer wg.Done()
	fmt.Printf("%s[*]%s ASN-name producer: %d keyword(s): %s\n",
		CYAN, RESET, len(keywords), strings.Join(keywords, ", "))

	// Collect unique ASNs across all keywords
	seen := make(map[int]bool)
	var uniqueASNs []string

	for i, kw := range keywords {
		asns, err := fetchASNsByKeyword(kw)
		if err != nil {
			fmt.Printf("%s[ASN-NAME]%s keyword %q error: %v\n", YELLOW, RESET, kw, err)
		} else {
			newCount := 0
			for _, asn := range asns {
				if !seen[asn] {
					seen[asn] = true
					uniqueASNs = append(uniqueASNs, fmt.Sprintf("%d", asn))
					newCount++
				}
			}
			fmt.Printf("%s[ASN-NAME]%s %q → %d ASN(s) (%d new)\n",
				CYAN, RESET, kw, len(asns), newCount)
		}
		if i < len(keywords)-1 {
			time.Sleep(500 * time.Millisecond)
		}
	}

	if len(uniqueASNs) == 0 {
		fmt.Printf("%s[ASN-NAME]%s No ASNs found for keywords\n", YELLOW, RESET)
		return
	}

	fmt.Printf("%s[ASN-NAME]%s Expanding %d unique ASN(s)\n", CYAN, RESET, len(uniqueASNs))

	// Reuse the existing ASN prefix expander inline (no extra goroutine needed)
	var allCIDRs []string
	for _, asn := range uniqueASNs {
		prefixes, err := fetchASNPrefixes(asn)
		if err != nil {
			fmt.Printf("%s[ASN-NAME]%s AS%s prefix error: %v\n", YELLOW, RESET, asn, err)
			continue
		}
		skipped := 0
		for _, p := range prefixes {
			_, ipNet, err := net.ParseCIDR(p)
			if err != nil {
				continue
			}
			ones, bits := ipNet.Mask.Size()
			if (1 << uint(bits-ones)) > maxCIDRHosts {
				skipped++
				continue
			}
			allCIDRs = append(allCIDRs, p)
		}
		if skipped > 0 && globalVerbose.Load() {
			fmt.Printf("%s[ASN-NAME]%s AS%s: %d prefixes too large, skipped\n", YELLOW, RESET, asn, skipped)
		}
		time.Sleep(300 * time.Millisecond)
	}

	total := 0
	for _, cidr := range allCIDRs {
		ips, err := cidrHosts(cidr)
		if err != nil {
			continue
		}
		for _, ip := range ips {
			if !sendIPOrDomain(ip, domainChan) {
				fmt.Printf("%s[+]%s ASN-name producer: ceiling reached\n", GREEN, RESET)
				return
			}
			total++
		}
	}
	fmt.Printf("%s[+]%s ASN-name producer done — %d IPs from %d prefix(es)\n",
		GREEN, RESET, total, len(allCIDRs))
}

// ── crt.sh organisation search producer ──────────────────────────────────────
//
// Searches crt.sh by the certificate Organisation (O=) field.  This is more
// targeted than a TLD or domain pivot: "stripe" finds every cert ever issued
// to "Stripe, Inc." regardless of which domain or registrar they used.
//
// Multiple keywords are searched sequentially with a 2 s gap (crt.sh rate
// limit).  All resulting FQDNs flow through sendDomain so the global dedup,
// subdomain-only filter, and ceiling apply uniformly.

func fetchCrtShOrgPivot(org string, domainChan chan<- string) int {
	queryURL := fmt.Sprintf("https://crt.sh/?O=%s&output=json", url.QueryEscape(org))
	if globalVerbose.Load() {
		fmt.Printf("%s[CRT-ORG]%s Querying org=%q\n", CYAN, RESET, org)
	}

	req, err := http.NewRequest("GET", queryURL, nil)
	if err != nil {
		fmt.Printf("%s[CRT-ORG]%s request error for %q: %v\n", YELLOW, RESET, org, err)
		return 0
	}
	req.Header.Set("User-Agent", "reconx-warc/2.0 (+https://crt.sh)")
	req.Header.Set("Accept", "application/json")

	client := http.Client{Timeout: 120 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("%s[CRT-ORG]%s fetch error for %q: %v\n", YELLOW, RESET, org, err)
		return 0
	}
	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		fmt.Printf("%s[CRT-ORG]%s HTTP %d for %q\n", YELLOW, RESET, resp.StatusCode, org)
		return 0
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20)) // 64 MB ceiling
	if err != nil {
		fmt.Printf("%s[CRT-ORG]%s read error for %q: %v\n", YELLOW, RESET, org, err)
		return 0
	}

	var records []crtshRecord
	if err := json.Unmarshal(body, &records); err != nil {
		fmt.Printf("%s[CRT-ORG]%s JSON parse error for %q: %v\n", YELLOW, RESET, org, err)
		return 0
	}

	emitted := 0
	for _, rec := range records {
		for _, raw := range strings.Split(rec.NameValue, "\n") {
			name := strings.TrimSpace(strings.ToLower(raw))
			if name == "" || strings.ContainsAny(name, " @/\\") {
				continue
			}
			name = strings.TrimPrefix(name, "*.")
			if !sendDomain(name, domainChan) {
				return emitted
			}
			fileMutex.Lock()
			if crtshFile != nil {
				_, _ = crtshFile.WriteString(name + "\n")
			}
			fileMutex.Unlock()
			emitted++
		}
	}
	fmt.Printf("%s[CRT-ORG]%s %q → %d names (%d cert records)\n",
		CYAN, RESET, org, emitted, len(records))
	return emitted
}

func runCrtShOrgProducer(orgs []string, domainChan chan<- string, wg *sync.WaitGroup) {
	defer wg.Done()
	fmt.Printf("%s[*]%s crt.sh org producer: %d keyword(s): %s\n",
		CYAN, RESET, len(orgs), strings.Join(orgs, ", "))
	total := 0
	for i, org := range orgs {
		org = strings.TrimSpace(org)
		if org == "" {
			continue
		}
		n := fetchCrtShOrgPivot(org, domainChan)
		total += n
		if i < len(orgs)-1 {
			time.Sleep(2 * time.Second)
		}
		maxDomains := globalMaxDomains.Load()
		if maxDomains > 0 && totalLiveDomains.Load() >= maxDomains {
			break
		}
	}
	fmt.Printf("%s[+]%s crt.sh org producer done — %d domains across %d keyword(s)\n",
		GREEN, RESET, total, len(orgs))
}
