// RAVEN X — WARC live-domain harvester.
// Pulls Common Crawl WARC paths + crt.sh certificate transparency entries,
// extracts FQDNs, tests them for liveness, and writes the survivors out.
//
// Created by https://t.me/boxxboyy

package main

import (
	"bufio"
	"compress/gzip"
	"crypto/tls"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"math/rand"
	"net"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/schollz/progressbar/v3"
	"golang.org/x/net/publicsuffix"
)

const (
	MAX_BUFFER_SIZE = 1 * 1024 * 1024 // 1 MB buffer per worker (was 10 MB — caused OOM with 500 workers)
)

var (
	// Colors
	RED    = "\033[91m"
	GREEN  = "\033[92m"
	YELLOW = "\033[93m"
	CYAN   = "\033[96m"
	RESET  = "\033[0m"

	// Stats
	totalProcessed   atomic.Int64
	totalDomains     atomic.Int64
	totalExtracted   atomic.Int64
	totalLiveDomains atomic.Int64
	uniqueDomains    = sync.Map{}

	// File writing mutex (shared for all output files)
	fileMutex sync.Mutex

	// Side-channel output files — opened in main, written under fileMutex.
	// envURLFile  receives full URLs whose path matches the env filter regex.
	// crtshFile   receives domains surfaced via crt.sh (pre-liveness-test).
	envURLFile *os.File
	crtshFile  *os.File

	// Global max domains check
	globalMaxDomains atomic.Int64

	// Global verbose flag
	globalVerbose atomic.Bool

	// Global subdomain-only filter flag. When true, FQDNs whose
	// publicsuffix-derived eTLD+1 equals the FQDN itself (apex/registered
	// domain) are dropped at the channel writer site, so the filter applies
	// uniformly to every producer (CC extractor + crt.sh).
	globalSubdomainOnly atomic.Bool

	// Regex patterns
	urlRegex       = regexp.MustCompile(`WARC-Target-URI:\s+(https?://[^\s]+)`)
	envFilterRegex = regexp.MustCompile(`(?i)(\.env|/env|env\.|config\.env|env\.php|env\.json|environment)`)

	// HTTP client — initialised in main() so the -insecure flag can be
	// applied before any connection is attempted.
	httpClient *http.Client
)

// Live status codes (2xx and 3xx are considered live)
var liveStatusCodes = map[int]bool{
	200: true, // OK
	201: true, // Created
	202: true, // Accepted
	203: true, // Non-Authoritative Information
	204: true, // No Content
	205: true, // Reset Content
	206: true, // Partial Content
	207: true, // Multi-Status
	208: true, // Already Reported
	226: true, // IM Used
	300: true, // Multiple Choices
	301: true, // Moved Permanently
	302: true, // Found
	303: true, // See Other
	304: true, // Not Modified
	307: true, // Temporary Redirect
	308: true, // Permanent Redirect
}

type CollectionInfo struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Timegate string `json:"timegate"`
	CdxAPI   string `json:"cdx-api"`
}

func getAvailableSnapshots() ([]string, error) {
	var foundSnapshots []string
	var body []byte

	maxRetries := 3
	for attempt := 1; attempt <= maxRetries; attempt++ {
		if attempt > 1 {
			time.Sleep(time.Duration(attempt) * 2 * time.Second)
		}

		req, err := http.NewRequest("GET", "https://index.commoncrawl.org/collinfo.json", nil)
		if err != nil {
			return nil, fmt.Errorf("failed to create request: %w", err)
		}

		req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
		req.Header.Set("Accept", "application/json, text/plain, */*")
		req.Header.Set("Accept-Language", "en-US,en;q=0.9")
		req.Header.Set("Connection", "keep-alive")
		req.Header.Set("Referer", "https://index.commoncrawl.org/")

		client := http.Client{Timeout: 30 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			if attempt == maxRetries {
				return nil, fmt.Errorf("failed to fetch after %d attempts: %w", maxRetries, err)
			}
			continue
		}

		if resp.StatusCode != http.StatusOK {
			resp.Body.Close() // explicit close — defer inside a loop leaks connections
			if attempt == maxRetries {
				return nil, fmt.Errorf("HTTP %d: Failed to fetch collection info", resp.StatusCode)
			}
			continue
		}

		var readErr error
		body, readErr = io.ReadAll(resp.Body)
		resp.Body.Close() // explicit close after read — same reason
		if readErr != nil {
			if attempt == maxRetries {
				return nil, fmt.Errorf("failed to read response: %w", readErr)
			}
			continue
		}

		break
	}

	if body == nil {
		return nil, fmt.Errorf("could not fetch snapshot list")
	}

	var collections []CollectionInfo
	if err := json.Unmarshal(body, &collections); err != nil {
		// Fallback: try regex parsing
		re := regexp.MustCompile(`"id"\s*:\s*"CC-MAIN-[^"]+"`)
		matches := re.FindAllString(string(body), -1)
		for _, match := range matches {
			parts := strings.Split(match, `"`)
			if len(parts) >= 4 {
				snapshotID := parts[3]
				if strings.HasPrefix(snapshotID, "CC-MAIN-") {
					foundSnapshots = append(foundSnapshots, snapshotID)
				}
			}
		}
	} else {
		for _, coll := range collections {
			if strings.HasPrefix(coll.ID, "CC-MAIN-") {
				foundSnapshots = append(foundSnapshots, coll.ID)
			}
		}
	}

	return foundSnapshots, nil
}

func selectRandomSnapshots(maxDomains int64, snapshotsRequested int) []string {
	allSnapshots, err := getAvailableSnapshots()
	if err != nil {
		fmt.Printf("%s[WARNING]%s Failed to fetch snapshots: %v\n", YELLOW, RESET, err)
		fmt.Printf("%s[*]%s Using fallback snapshots...\n", CYAN, RESET)
		// Fallback to common snapshots
		allSnapshots = []string{
			"CC-MAIN-2024-50", "CC-MAIN-2024-46", "CC-MAIN-2024-42", "CC-MAIN-2024-38",
			"CC-MAIN-2024-33", "CC-MAIN-2024-30", "CC-MAIN-2024-27", "CC-MAIN-2024-23",
			"CC-MAIN-2024-18", "CC-MAIN-2024-14", "CC-MAIN-2024-10", "CC-MAIN-2024-06",
		}
	}

	if len(allSnapshots) == 0 {
		return []string{}
	}

	// Explicit operator override wins: snapshotsRequested > 0 forces that
	// count regardless of max-domains. The size-based heuristic only kicks
	// in when the operator hasn't expressed a preference (==0).
	numSnapshots := snapshotsRequested
	if numSnapshots <= 0 {
		numSnapshots = 1
		if maxDomains > 1000000 {
			numSnapshots = 3
		} else if maxDomains > 500000 {
			numSnapshots = 2
		}
	}

	// Limit to available snapshots
	if numSnapshots > len(allSnapshots) {
		numSnapshots = len(allSnapshots)
	}

	// Randomly select snapshots
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	selected := make([]string, 0, numSnapshots)
	used := make(map[int]bool)

	for len(selected) < numSnapshots {
		idx := rng.Intn(len(allSnapshots))
		if !used[idx] {
			used[idx] = true
			selected = append(selected, allSnapshots[idx])
		}
	}

	return selected
}

func getWarcPaths(snapshotID string) ([]string, error) {
	indexURL := fmt.Sprintf("https://data.commoncrawl.org/crawl-data/%s/warc.paths.gz", snapshotID)

	req, err := http.NewRequest("GET", indexURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Encoding", "gzip, deflate, br")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("Connection", "keep-alive")
	req.Header.Set("Referer", "https://commoncrawl.org/")

	client := http.Client{
		Timeout: 60 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
			return nil
		},
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch WARC index: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusForbidden {
		return nil, fmt.Errorf("403 Forbidden: Snapshot '%s' may not exist", snapshotID)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP status not OK: %s (Status: %d)", resp.Status, resp.StatusCode)
	}

	gzReader, err := gzip.NewReader(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to create gzip reader: %w", err)
	}
	defer gzReader.Close()

	var paths []string
	scanner := bufio.NewScanner(gzReader)
	for scanner.Scan() {
		p := scanner.Text()
		paths = append(paths, fmt.Sprintf("https://data.commoncrawl.org/%s", p))
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error scanning gzip: %w", err)
	}

	return paths, nil
}

func extractDomain(urlStr string) string {
	parsedURL, err := url.Parse(urlStr)
	if err != nil || parsedURL.Hostname() == "" {
		return ""
	}

	return strings.ToLower(parsedURL.Hostname())
}

// isApexDomain reports whether the given FQDN equals its own registered
// (eTLD+1) domain, per the public suffix list. Used by the -subdomain-only
// filter to drop apex entries and keep only real subdomains.
//
// Conservative on error: if publicsuffix can't classify the input (rare —
// malformed labels), we return true so the entry is treated as apex and
// dropped, rather than leak ambiguous output past the filter.
func isApexDomain(fqdn string) bool {
	if fqdn == "" {
		return true
	}
	etld1, err := publicsuffix.EffectiveTLDPlusOne(fqdn)
	if err != nil {
		return true
	}
	return strings.EqualFold(etld1, fqdn)
}

// sendDomain is the single channel-write site shared by every producer
// (CC extractor + crt.sh). It enforces the subdomain-only filter, the
// dedup sync.Map, the max-domains ceiling, and verbose logging in one
// place so both producers behave identically.
//
// Returns false if the global max-domains ceiling has been reached, so
// the caller can short-circuit its loop.
func sendDomain(domain string, domainChan chan<- string) bool {
	if domain == "" {
		return true
	}

	// Subdomain-only filter sits BEFORE the dedup map — otherwise the apex
	// would occupy a slot in uniqueDomains and silently mask a later real
	// subdomain insertion attempt with the same name (can't happen for
	// FQDN equality, but keeps the invariant clean for future producers).
	if globalSubdomainOnly.Load() && isApexDomain(domain) {
		if globalVerbose.Load() {
			fmt.Printf("%s[FILTERED]%s Apex dropped (subdomain-only): %s\n", YELLOW, RESET, domain)
		}
		return true
	}

	// Dedup across all producers.
	if _, loaded := uniqueDomains.LoadOrStore(domain, true); loaded {
		return true
	}

	// Ceiling check — same pattern the CC extractor used inline before
	// this helper existed.
	maxDomains := globalMaxDomains.Load()
	if maxDomains > 0 {
		currentTotal := totalLiveDomains.Load()
		if currentTotal >= maxDomains {
			return false
		}
	}

	totalExtracted.Add(1)

	if globalVerbose.Load() {
		fmt.Printf("%s[EXTRACTED]%s Domain received: %s\n", CYAN, RESET, domain)
	}

	domainChan <- domain // blocking send — ensures no domain is dropped

	if maxDomains > 0 {
		current := totalLiveDomains.Load()
		if current >= maxDomains {
			return false
		}
	}
	return true
}

func testDomainConnection(domain string) (bool, int) {
	// Try HTTPS first, then HTTP
	urls := []string{
		fmt.Sprintf("https://%s", domain),
		fmt.Sprintf("http://%s", domain),
	}

	for _, testURL := range urls {
		// Try HEAD first
		req, err := http.NewRequest("HEAD", testURL, nil)
		if err != nil {
			continue
		}

		req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
		req.Header.Set("Accept", "*/*")
		req.Header.Set("Connection", "close")
		req.Close = true

		resp, err := httpClient.Do(req)
		if err != nil {
			// Silently skip connection errors (timeout, DNS, TLS, etc.)
			continue
		}

		// Drain response body to avoid "unhandled response" errors
		if resp.Body != nil {
			_, _ = io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
		}

		statusCode := resp.StatusCode
		if liveStatusCodes[statusCode] {
			return true, statusCode
		}

		// If HEAD fails with method not allowed, try GET
		if statusCode == 405 || statusCode == 501 {
			req, err := http.NewRequest("GET", testURL, nil)
			if err != nil {
				continue
			}
			req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
			req.Header.Set("Accept", "*/*")
			req.Header.Set("Connection", "close")
			req.Close = true

			// Limit response size to avoid downloading large files
			req.Header.Set("Range", "bytes=0-1024")

			resp, err := httpClient.Do(req)
			if err != nil {
				// Silently skip connection errors
				continue
			}

			// Drain response body
			if resp.Body != nil {
				_, _ = io.Copy(io.Discard, resp.Body)
				resp.Body.Close()
			}

			statusCode := resp.StatusCode
			if liveStatusCodes[statusCode] {
				return true, statusCode
			}
		}
	}

	return false, 0
}

func splitWARCRecord(data []byte, atEOF bool) (advance int, token []byte, err error) {
	if atEOF && len(data) == 0 {
		return 0, nil, nil
	}

	if i := strings.Index(string(data), "\n\n"); i >= 0 {
		return i + 2, data[0:i], nil
	}
	if i := strings.Index(string(data), "\r\n\r\n"); i >= 0 {
		return i + 4, data[0:i], nil
	}

	if atEOF {
		return len(data), data, nil
	}

	return 0, nil, nil
}

// Extract domains from WARC file and send to channel (grabber)
// processWarcFile fetches and parses one WARC file, sending extracted domains to domainChan.
// It has no sync primitives of its own — callers manage concurrency via the worker pool.
func processWarcFile(warcURL string, domainChan chan<- string, bar *progressbar.ProgressBar) {
	defer func() {
		if r := recover(); r != nil {
		}
		bar.Add(1)
	}()

	req, err := http.NewRequest("GET", warcURL, nil)
	if err != nil {
		return
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; WARC-Live-Checker/1.0)")
	req.Close = true

	client := http.Client{
		Timeout: 300 * time.Second,
		Transport: &http.Transport{
			DisableKeepAlives: true,
		},
	}

	resp, err := client.Do(req)
	if err != nil {
		return
	}
	defer func() {
		if resp != nil && resp.Body != nil {
			_, _ = io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return
	}

	gzReader, err := gzip.NewReader(resp.Body)
	if err != nil {
		return
	}
	defer gzReader.Close()

	scanner := bufio.NewScanner(gzReader)
	scanner.Split(splitWARCRecord)

	buffer := make([]byte, 0, MAX_BUFFER_SIZE)
	scanner.Buffer(buffer, MAX_BUFFER_SIZE)

	for scanner.Scan() {
		record := scanner.Bytes()
		matches := urlRegex.FindAllSubmatch(record, -1)

		for _, match := range matches {
			if len(match) < 2 {
				continue
			}

			urlStr := string(match[1])
			domain := extractDomain(urlStr)

			if envFilterRegex.MatchString(urlStr) {
				fileMutex.Lock()
				if envURLFile != nil {
					_, _ = envURLFile.WriteString(urlStr + "\n")
				}
				fileMutex.Unlock()
				if globalVerbose.Load() {
					fmt.Printf("%s[ENV-HIT]%s %s\n", YELLOW, RESET, urlStr)
				}
			}

			if !sendDomain(domain, domainChan) {
				return
			}
		}
	}

	if err := scanner.Err(); err != nil {
	}

	totalProcessed.Add(1)
}

// extractWarcWorker is a long-lived pool goroutine. It pulls WARC URLs from urlChan
// and calls processWarcFile for each. Only N of these goroutines exist (N = extractWorkers),
// replacing the old pattern of spawning one goroutine per file (up to 10,000).
func extractWarcWorker(urlChan <-chan string, domainChan chan<- string, wg *sync.WaitGroup, bar *progressbar.ProgressBar) {
	defer wg.Done()
	for warcURL := range urlChan {
		processWarcFile(warcURL, domainChan, bar)
	}
}

// Test domain connection (tester worker)
func testDomainWorker(domainChan <-chan string, outputFile *os.File, wg *sync.WaitGroup, testSem chan struct{}) {
	defer wg.Done()

	for domain := range domainChan {
		testSem <- struct{}{} // Acquire semaphore

		// Check limit before testing
		maxDomains := globalMaxDomains.Load()
		if maxDomains > 0 {
			currentTotal := totalLiveDomains.Load()
			if currentTotal >= maxDomains {
				<-testSem
				return
			}
		}

		totalDomains.Add(1)

		// Test connection (errors are silently handled inside)
		isLive, statusCode := testDomainConnection(domain)

		if isLive {
			totalLiveDomains.Add(1)

			// Verbose: log live domain
			if globalVerbose.Load() {
				fmt.Printf("%s[LIVE]%s Domain is live: %s (Status: %d)\n", GREEN, RESET, domain, statusCode)
			}

			// Write to file with lock (thread-safe) - only domain, no status code
			fileMutex.Lock()
			_, _ = outputFile.WriteString(domain + "\n")
			fileMutex.Unlock()
		} else {
			// Verbose: log dead domain
			if globalVerbose.Load() {
				fmt.Printf("%s[DEAD]%s Domain is dead: %s\n", RED, RESET, domain)
			}
		}

		<-testSem // Release semaphore

		// Check limit after processing
		if maxDomains > 0 {
			current := totalLiveDomains.Load()
			if current >= maxDomains {
				return
			}
		}
	}
}

// crtshRecord matches a single JSON entry returned by https://crt.sh/?output=json.
// We only need name_value; everything else (issuer, serial, dates) is discarded.
type crtshRecord struct {
	NameValue string `json:"name_value"`
}

// fetchCrtShPivot queries crt.sh for a single pivot ("%.<tld>" or
// "%.<domain>"), parses the JSON response, splits multi-line name_value
// fields (crt.sh packs multiple SANs into one record separated by '\n'),
// and emits each unique FQDN through sendDomain. Returns when the
// max-domains ceiling is reached or the query completes.
//
// crt.sh is touchy about parallel hammering — caller sleeps ~2 s between
// pivots; per-query timeout is generous because the JSON payload for
// popular TLDs can be tens of MB.
func fetchCrtShPivot(pivot string, domainChan chan<- string) {
	queryURL := fmt.Sprintf("https://crt.sh/?q=%s&output=json", url.QueryEscape(pivot))
	if globalVerbose.Load() {
		fmt.Printf("%s[CRTSH]%s Querying %s\n", CYAN, RESET, queryURL)
	}

	req, err := http.NewRequest("GET", queryURL, nil)
	if err != nil {
		fmt.Printf("%s[CRTSH]%s request build failed for %s: %v\n", YELLOW, RESET, pivot, err)
		return
	}
	// crt.sh asks operators to identify themselves; an honest UA also
	// makes it easier for them to throttle politely instead of blackholing.
	req.Header.Set("User-Agent", "reconx-warc/1.0 (+https://crt.sh)")
	req.Header.Set("Accept", "application/json")

	client := http.Client{Timeout: 120 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("%s[CRTSH]%s fetch failed for %s: %v\n", YELLOW, RESET, pivot, err)
		return
	}
	defer func() {
		if resp.Body != nil {
			_, _ = io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
		}
	}()

	if resp.StatusCode != http.StatusOK {
		fmt.Printf("%s[CRTSH]%s HTTP %d for %s\n", YELLOW, RESET, resp.StatusCode, pivot)
		return
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Printf("%s[CRTSH]%s read failed for %s: %v\n", YELLOW, RESET, pivot, err)
		return
	}

	var records []crtshRecord
	if err := json.Unmarshal(body, &records); err != nil {
		fmt.Printf("%s[CRTSH]%s JSON parse failed for %s: %v\n", YELLOW, RESET, pivot, err)
		return
	}

	emitted := 0
	for _, rec := range records {
		// name_value may pack several SAN entries on separate lines.
		for _, raw := range strings.Split(rec.NameValue, "\n") {
			name := strings.TrimSpace(strings.ToLower(raw))
			if name == "" {
				continue
			}
			// Wildcards (*.example.com) are not directly testable — strip
			// the leading wildcard label and let dedup catch duplicates.
			name = strings.TrimPrefix(name, "*.")
			// Skip anything that doesn't look like a hostname (e.g.
			// email-name SANs include '@').
			if strings.ContainsAny(name, " @/\\") {
				continue
			}
			if !sendDomain(name, domainChan) {
				return
			}
			// Write every crt.sh-sourced domain to a dedicated side file so
			// it can be scanned first — cert-transparency hits are more
			// targeted than random CC domains.
			fileMutex.Lock()
			if crtshFile != nil {
				_, _ = crtshFile.WriteString(name + "\n")
			}
			fileMutex.Unlock()
			emitted++
		}
	}
	if globalVerbose.Load() {
		fmt.Printf("%s[CRTSH]%s %s yielded %d names (%d records)\n",
			CYAN, RESET, pivot, emitted, len(records))
	}
}

// runCrtShProducer fans out across the configured TLD and domain pivots,
// sleeping between queries to respect crt.sh rate limits. Closes nothing
// (the domain channel is owned by main, which waits for both producers
// and then closes it).
func runCrtShProducer(tlds, domains []string, domainChan chan<- string, wg *sync.WaitGroup) {
	defer wg.Done()

	type pivot struct {
		label string
		query string
	}
	var pivots []pivot
	for _, t := range tlds {
		t = strings.TrimSpace(strings.TrimPrefix(strings.ToLower(t), "."))
		if t == "" {
			continue
		}
		pivots = append(pivots, pivot{label: "tld=" + t, query: "%." + t})
	}
	for _, d := range domains {
		d = strings.TrimSpace(strings.ToLower(d))
		if d == "" {
			continue
		}
		pivots = append(pivots, pivot{label: "domain=" + d, query: "%." + d})
	}

	if len(pivots) == 0 {
		return
	}

	fmt.Printf("%s[*]%s crt.sh producer starting (%d pivot(s))\n", CYAN, RESET, len(pivots))

	for i, p := range pivots {
		// Bail if the harvest has already hit its ceiling.
		maxDomains := globalMaxDomains.Load()
		if maxDomains > 0 && totalLiveDomains.Load() >= maxDomains {
			fmt.Printf("%s[CRTSH]%s ceiling reached, stopping after %d pivot(s)\n",
				CYAN, RESET, i)
			return
		}
		fetchCrtShPivot(p.query, domainChan)
		// crt.sh asks clients to space queries — ~2 s is the figure
		// quoted in their docs/PSA forum threads. Skip the sleep on the
		// final pivot so we don't add latency to harvest shutdown.
		if i < len(pivots)-1 {
			time.Sleep(2 * time.Second)
		}
	}
	fmt.Printf("%s[+]%s crt.sh producer done (%d pivot(s) queried)\n", GREEN, RESET, len(pivots))
}

// parseCSVFlag splits a comma-separated -source / -crt-tld / -crt-domain
// flag value into trimmed lowercase tokens, dropping empties.
func parseCSVFlag(raw string) []string {
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(strings.ToLower(p))
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func main() {
	var (
		maxDomains     = flag.Int64("max-domains", 10000, "Maximum live domains to extract (required)")
		outputFile     = flag.String("output", "live_domains.txt", "Output file for live domains")
		extractWorkers = flag.Int("extract-workers", 50, "Number of concurrent workers for WARC extraction (grabber)")
		testWorkers    = flag.Int("test-workers", 25, "Number of concurrent workers for connection testing")
		limit          = flag.Int("limit", 0, "Limit number of WARC files to process (0 = auto)")
		channelSize    = flag.Int("channel-size", 10000, "Size of domain channel buffer")
		verbose        = flag.Bool("verbose", false, "Enable verbose mode to see extracted, live, and dead domains")
		snapshots      = flag.Int("snapshots", 0, "Number of CC-MAIN snapshots to span (0 = auto based on max-domains: 1/<500k, 2/<1M, 3/>1M)")
		sourceFlag     = flag.String("source", "cc", "Comma-separated list of producers: cc, crtsh (default 'cc' preserves legacy behavior)")
		crtTLDFlag     = flag.String("crt-tld", "", "Comma-separated TLDs for crt.sh TLD pivot (e.g. 'com,net,io'); required when crtsh is in -source unless -crt-domain is set")
		crtDomainFlag  = flag.String("crt-domain", "", "Comma-separated registered domains for crt.sh domain pivot (e.g. 'example.com,foo.io'); alternative to -crt-tld")
		subdomainOnly  = flag.Bool("subdomain-only", false, "Drop any FQDN whose eTLD+1 equals itself (apex/registered domain), applies to every producer")
		insecure       = flag.Bool("insecure", false, "Skip TLS certificate verification — catches live hosts with self-signed or expired certs that would otherwise be missed")

		// New producer flags — each implies its source without needing -source=…
		waybackPatternFlag = flag.String("wayback-pattern", "", "Comma-separated Wayback CDX URL patterns (e.g. '*/.env,*/.git/config'); empty = 20 default patterns. Implies -source=wayback")
		asnFlag            = flag.String("asn", "", "Comma-separated ASN numbers (e.g. 'AS47583,14061'). BGPView → CIDR → IPs. Implies -source=asn")
		asnNameFlag        = flag.String("asn-name", "", "Comma-separated org keywords for BGPView name search (e.g. 'hostinger,sendgrid'). Resolves to ASNs then IPs. Implies -source=asn-name")
		cidrFlag           = flag.String("cidr", "", "Comma-separated CIDR ranges to expand (e.g. '192.168.1.0/24'); max /16 per range. Implies -source=cidr")
		ipFileFlag         = flag.String("ip-file", "", "Path to a file with IPs/FQDNs (one per line; '#' comments; CIDR notation accepted). Implies -source=ipfile")
		crtOrgFlag         = flag.String("crt-org", "", "Comma-separated org keywords for crt.sh O= search (e.g. 'stripe,sendgrid'). Finds all certs issued to matching orgs. Implies -source=crt-org")
	)
	flag.Parse()

	if *maxDomains <= 0 {
		fmt.Printf("%s[ERROR]%s max-domains must be greater than 0\n", RED, RESET)
		fmt.Println("Usage: ./warc_live_checker -max-domains 10000 [options]")
		flag.PrintDefaults()
		os.Exit(1)
	}

	// Initialise the HTTP client here so the -insecure flag is applied before
	// any connection is attempted. Previously httpClient was a package-level
	// literal with InsecureSkipVerify hard-coded to false, which caused
	// self-signed / expired-cert hosts to silently fail the liveness check.
	httpClient = &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: *insecure, //nolint:gosec // intentional, user-opted-in
			},
			DialContext: (&net.Dialer{
				Timeout:   5 * time.Second,
				KeepAlive: 0,
			}).DialContext,
			DisableKeepAlives:     true,
			MaxIdleConns:          0,
			MaxIdleConnsPerHost:   0,
			IdleConnTimeout:       0,
			TLSHandshakeTimeout:   5 * time.Second,
			ExpectContinueTimeout: 0,
			ResponseHeaderTimeout: 5 * time.Second,
		},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return http.ErrUseLastResponse
			}
			return nil
		},
	}
	if *insecure {
		fmt.Printf("%s[WARN]%s TLS verification disabled (-insecure) — self-signed certs accepted\n", YELLOW, RESET)
	}

	// Set global verbose + subdomain-only flags
	globalVerbose.Store(*verbose)
	globalSubdomainOnly.Store(*subdomainOnly)

	// Resolve which producers are enabled. Default is 'cc' (legacy
	// behavior). We allow an empty -source value too — that's just
	// treated as 'cc' so older callers don't break.
	sources := parseCSVFlag(*sourceFlag)
	if len(sources) == 0 {
		sources = []string{"cc"}
	}
	ccEnabled := false
	crtshEnabled := false
	waybackEnabled := false
	asnEnabled := false
	asnNameEnabled := false
	cidrEnabled := false
	ipFileEnabled := false
	crtOrgEnabled := false
	for _, s := range sources {
		switch s {
		case "cc":
			ccEnabled = true
		case "crtsh", "crt.sh":
			crtshEnabled = true
		case "wayback":
			waybackEnabled = true
		case "asn":
			asnEnabled = true
		case "asn-name", "asnname":
			asnNameEnabled = true
		case "cidr":
			cidrEnabled = true
		case "ipfile", "ip-file":
			ipFileEnabled = true
		case "crt-org", "crtorg":
			crtOrgEnabled = true
		default:
			fmt.Printf("%s[ERROR]%s unknown source %q (valid: cc, crtsh, wayback, asn, asn-name, cidr, ipfile, crt-org)\n", RED, RESET, s)
			os.Exit(2)
		}
	}

	// Convenience: passing a shortcut flag enables its producer even when the
	// source isn't listed in -source, so callers don't have to manage both.
	if *asnFlag != "" {
		asnEnabled = true
	}
	if *asnNameFlag != "" {
		asnNameEnabled = true
	}
	if *cidrFlag != "" {
		cidrEnabled = true
	}
	if *ipFileFlag != "" {
		ipFileEnabled = true
	}
	if *waybackPatternFlag != "" {
		waybackEnabled = true
	}
	if *crtOrgFlag != "" {
		crtOrgEnabled = true
	}

	crtTLDs := parseCSVFlag(*crtTLDFlag)
	crtDomains := parseCSVFlag(*crtDomainFlag)
	if crtshEnabled && len(crtTLDs) == 0 && len(crtDomains) == 0 {
		fmt.Printf("%s[ERROR]%s -source includes crtsh but neither -crt-tld nor -crt-domain was set\n", RED, RESET)
		os.Exit(2)
	}
	if asnEnabled && *asnFlag == "" {
		fmt.Printf("%s[ERROR]%s -source includes asn but -asn flag is not set\n", RED, RESET)
		os.Exit(2)
	}
	if asnNameEnabled && *asnNameFlag == "" {
		fmt.Printf("%s[ERROR]%s -source includes asn-name but -asn-name flag is not set\n", RED, RESET)
		os.Exit(2)
	}
	if cidrEnabled && *cidrFlag == "" {
		fmt.Printf("%s[ERROR]%s -source includes cidr but -cidr flag is not set\n", RED, RESET)
		os.Exit(2)
	}
	if ipFileEnabled && *ipFileFlag == "" {
		fmt.Printf("%s[ERROR]%s -source includes ipfile but -ip-file flag is not set\n", RED, RESET)
		os.Exit(2)
	}
	if crtOrgEnabled && *crtOrgFlag == "" {
		fmt.Printf("%s[ERROR]%s -source includes crt-org but -crt-org flag is not set\n", RED, RESET)
		os.Exit(2)
	}
	if !ccEnabled && !crtshEnabled && !waybackEnabled && !asnEnabled && !asnNameEnabled && !cidrEnabled && !ipFileEnabled && !crtOrgEnabled {
		fmt.Printf("%s[ERROR]%s no producers enabled (resolved sources=%v)\n", RED, RESET, sources)
		os.Exit(2)
	}

	fmt.Printf("%s[*]%s WARC Live Domain Checker - Extract & Test Domains\n", CYAN, RESET)
	if *verbose {
		fmt.Printf("%s[*]%s Verbose mode enabled\n", CYAN, RESET)
	}
	{
		var enabled []string
		if ccEnabled {
			enabled = append(enabled, "cc")
		}
		if crtshEnabled {
			enabled = append(enabled, "crtsh")
		}
		if waybackEnabled {
			enabled = append(enabled, "wayback")
		}
		if asnEnabled {
			enabled = append(enabled, "asn")
		}
		if asnNameEnabled {
			enabled = append(enabled, "asn-name")
		}
		if cidrEnabled {
			enabled = append(enabled, "cidr")
		}
		if ipFileEnabled {
			enabled = append(enabled, "ipfile")
		}
		if crtOrgEnabled {
			enabled = append(enabled, "crt-org")
		}
		fmt.Printf("%s[*]%s Producers enabled: %s\n", CYAN, RESET, strings.Join(enabled, ", "))
		if *subdomainOnly {
			fmt.Printf("%s[*]%s Subdomain-only filter active (apex/eTLD+1 entries dropped)\n", CYAN, RESET)
		}
		if crtshEnabled {
			if len(crtTLDs) > 0 {
				fmt.Printf("%s[*]%s crt.sh TLD pivots: %s\n", CYAN, RESET, strings.Join(crtTLDs, ", "))
			}
			if len(crtDomains) > 0 {
				fmt.Printf("%s[*]%s crt.sh domain pivots: %s\n", CYAN, RESET, strings.Join(crtDomains, ", "))
			}
		}
		if waybackEnabled {
			patterns := parseCSVFlag(*waybackPatternFlag)
			if len(patterns) == 0 {
				fmt.Printf("%s[*]%s Wayback patterns: %d default patterns\n", CYAN, RESET, len(defaultWaybackPatterns))
			} else {
				fmt.Printf("%s[*]%s Wayback patterns: %s\n", CYAN, RESET, strings.Join(patterns, ", "))
			}
		}
		if asnEnabled {
			fmt.Printf("%s[*]%s ASN targets: %s\n", CYAN, RESET, *asnFlag)
		}
		if asnNameEnabled {
			fmt.Printf("%s[*]%s ASN name keywords: %s\n", CYAN, RESET, *asnNameFlag)
		}
		if crtOrgEnabled {
			fmt.Printf("%s[*]%s crt.sh org keywords: %s\n", CYAN, RESET, *crtOrgFlag)
		}
		if cidrEnabled {
			fmt.Printf("%s[*]%s CIDR ranges: %s\n", CYAN, RESET, *cidrFlag)
		}
		if ipFileEnabled {
			fmt.Printf("%s[*]%s IP/domain file: %s\n", CYAN, RESET, *ipFileFlag)
		}
	}
	fmt.Println()

	// CC snapshot enumeration is only meaningful when the CC producer is
	// actually enabled — skip the slow collinfo.json + warc.paths.gz
	// round trips entirely on a crtsh-only run.
	var allWarcURLs []string
	filesToProcess := 0
	if ccEnabled {
		fmt.Printf("%s[*]%s Selecting random CC-MAIN snapshots...\n", CYAN, RESET)
		snapshotList := selectRandomSnapshots(*maxDomains, *snapshots)

		if len(snapshotList) == 0 {
			fmt.Printf("%s[ERROR]%s No snapshots available\n", RED, RESET)
			os.Exit(1)
		}

		fmt.Printf("%s[+]%s Selected %d snapshot(s): %s\n", GREEN, RESET, len(snapshotList), strings.Join(snapshotList, ", "))

		for _, snapID := range snapshotList {
			fmt.Printf("%s[*]%s Fetching WARC paths from %s...\n", CYAN, RESET, snapID)
			warcURLs, err := getWarcPaths(snapID)
			if err != nil {
				fmt.Printf("%s[WARNING]%s Failed to get paths from %s: %v\n", YELLOW, RESET, snapID, err)
				continue
			}
			allWarcURLs = append(allWarcURLs, warcURLs...)
			fmt.Printf("%s[+]%s Found %d WARC files from %s\n", GREEN, RESET, len(warcURLs), snapID)
		}

		if len(allWarcURLs) == 0 {
			fmt.Printf("%s[ERROR]%s No WARC files found from any snapshot\n", RED, RESET)
			os.Exit(1)
		}

		totalFiles := len(allWarcURLs)
		filesToProcess = totalFiles

		if *limit > 0 && *limit < totalFiles {
			filesToProcess = *limit
			fmt.Printf("%s[INFO]%s Limiting to %d files (out of %d)\n", YELLOW, RESET, filesToProcess, totalFiles)
		} else {
			// Adaptive auto-limit: tiered estimate of live domains per WARC
			// file. The old fixed /50 overestimated yield, causing too few
			// files to be queued for small targets and the ceiling never
			// being hit. New tiers use a more conservative yield floor:
			//   ≤1k targets  → assume 10 live/file, min 50 files
			//   ≤10k targets → assume 20 live/file, min 100 files
			//   >10k targets → assume 30 live/file, min 200 files
			// The ceiling check inside sendDomain stops producers early when
			// the target is reached, so over-queuing is safe and cheap.
			var yieldPerFile, minFiles int
			switch {
			case *maxDomains <= 1000:
				yieldPerFile, minFiles = 10, 50
			case *maxDomains <= 10000:
				yieldPerFile, minFiles = 20, 100
			default:
				yieldPerFile, minFiles = 30, 200
			}
			estimatedFilesNeeded := int(*maxDomains/int64(yieldPerFile)) + 1
			if estimatedFilesNeeded < minFiles {
				estimatedFilesNeeded = minFiles
			}
			if estimatedFilesNeeded < filesToProcess {
				filesToProcess = estimatedFilesNeeded
			}
			if filesToProcess > 500000 {
				filesToProcess = 500000
			}
			fmt.Printf("%s[INFO]%s Auto-limiting to %d files (yield est. %d/file) to reach ~%d live domains\n",
				YELLOW, RESET, filesToProcess, yieldPerFile, *maxDomains)
		}

		fmt.Printf("%s[+]%s Found %d WARC files total\n", GREEN, RESET, totalFiles)
		fmt.Printf("%s[*]%s Extraction workers (grabber): %d\n", CYAN, RESET, *extractWorkers)
		fmt.Printf("%s[*]%s Processing %d files (%d extract workers)\n", CYAN, RESET, filesToProcess, *extractWorkers)
	}
	fmt.Printf("%s[*]%s Testing workers (live check): %d\n", CYAN, RESET, *testWorkers)
	fmt.Printf("%s[INFO]%s Target: %d live domains\n", YELLOW, RESET, *maxDomains)

	// Create output file
	outFile, err := os.Create(*outputFile)
	if err != nil {
		fmt.Printf("%s[ERROR]%s Failed to create output file: %v\n", RED, RESET, err)
		os.Exit(1)
	}
	defer outFile.Close()

	// Side-channel outputs — non-fatal if they can't be created.
	if f, err := os.Create("env_urls.txt"); err == nil {
		envURLFile = f
		defer f.Close()
		fmt.Printf("%s[*]%s env URL hits → env_urls.txt\n", CYAN, RESET)
	}
	if f, err := os.Create("crtsh_domains.txt"); err == nil {
		crtshFile = f
		defer f.Close()
		fmt.Printf("%s[*]%s crt.sh domain hits → crtsh_domains.txt\n", CYAN, RESET)
	}

	// Progress bar — sized for whichever producers are active. A nil bar
	// would crash extractWarcFile's bar.Add(1); when CC is disabled we
	// give it 1 unit and never advance it (the bar is then a no-op tile).
	progressTotal := filesToProcess
	if progressTotal <= 0 {
		progressTotal = 1
	}
	bar := progressbar.NewOptions(progressTotal,
		progressbar.OptionSetDescription(fmt.Sprintf("%s[PROGRESS]%s Processing WARC files...", CYAN, RESET)),
		progressbar.OptionSetTheme(progressbar.Theme{
			Saucer:        "=",
			SaucerPadding: " ",
			BarStart:      "[",
			BarEnd:        "]",
		}),
		progressbar.OptionSetWidth(80),
		progressbar.OptionThrottle(100*time.Millisecond),
	)

	// Set global max domains
	globalMaxDomains.Store(*maxDomains)

	// Create domain channel for communication between producers and testers.
	domainChan := make(chan string, *channelSize)

	testSem := make(chan struct{}, *testWorkers)

	// One producer wait group covers CC extractors AND the crt.sh
	// goroutine — the channel can only be closed once both producers have
	// finished so the testers see the full union.
	var producerWg sync.WaitGroup
	var testWg sync.WaitGroup

	startTime := time.Now()

	// Start tester workers (they will wait for domains from channel)
	for i := 0; i < *testWorkers; i++ {
		testWg.Add(1)
		go testDomainWorker(domainChan, outFile, &testWg, testSem)
	}

	// Progress reporting goroutine
	stopProgress := make(chan bool)
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				live := totalLiveDomains.Load()
				tested := totalDomains.Load()
				extracted := totalExtracted.Load()
				processed := totalProcessed.Load()
				fmt.Printf("\r%s[PROGRESS]%s Live: %d/%d | Tested: %d | Extracted: %d | Files: %d/%d", CYAN, RESET,
					live, *maxDomains, tested, extracted, processed, filesToProcess)
			case <-stopProgress:
				return
			}
		}
	}()

	// Start the crt.sh producer goroutine alongside the CC extractors
	// (when enabled). It writes into the same domainChan via sendDomain,
	// so dedup + subdomain filter apply uniformly.
	if crtshEnabled {
		producerWg.Add(1)
		go runCrtShProducer(crtTLDs, crtDomains, domainChan, &producerWg)
	}

	// Wayback CDX producer — queries archived crawls for env/config URL patterns
	// and emits the live domains. Full matching URLs also land in env_urls.txt.
	if waybackEnabled {
		producerWg.Add(1)
		go runWaybackProducer(parseCSVFlag(*waybackPatternFlag), domainChan, &producerWg)
	}

	// ASN producer — resolves each ASN to its IPv4 prefixes via BGPView, then
	// expands each prefix into individual IPs (capped at /16 per prefix).
	if asnEnabled {
		producerWg.Add(1)
		go runASNProducer(parseCSVFlag(*asnFlag), domainChan, &producerWg)
	}

	// ASN-name producer — keyword search: "hostinger" → matching ASNs → IPs.
	// Results are merged and deduplicated before expansion so overlapping
	// keywords (e.g. "amazon"+"aws") never expand the same prefix twice.
	if asnNameEnabled {
		producerWg.Add(1)
		go runASNNameProducer(parseCSVFlag(*asnNameFlag), domainChan, &producerWg)
	}

	// crt.sh org producer — keyword search on the certificate O= field.
	// "stripe" → all certs ever issued to Stripe, Inc., across all their domains.
	if crtOrgEnabled {
		producerWg.Add(1)
		go runCrtShOrgProducer(parseCSVFlag(*crtOrgFlag), domainChan, &producerWg)
	}

	// CIDR producer — expands comma-separated CIDR ranges directly into IPs.
	if cidrEnabled {
		producerWg.Add(1)
		go runCIDRProducer(parseCSVFlag(*cidrFlag), domainChan, &producerWg)
	}

	// IP-file producer — reads a plain-text file of IPs, FQDNs, or CIDRs.
	if ipFileEnabled {
		producerWg.Add(1)
		go runIPFileProducer(*ipFileFlag, domainChan, &producerWg)
	}

	// Start CC extractor worker pool. A feeder goroutine sends WARC URLs into urlChan;
	// exactly extractWorkers goroutines drain it. This replaces the old pattern of
	// spawning up to 10,000 goroutines (one per file) and throttling with a semaphore.
	if ccEnabled {
		urlChan := make(chan string, *extractWorkers*2)
		go func() {
			for i := 0; i < filesToProcess; i++ {
				if *maxDomains > 0 && totalLiveDomains.Load() >= *maxDomains {
					break
				}
				urlChan <- allWarcURLs[i]
			}
			close(urlChan)
		}()
		for i := 0; i < *extractWorkers; i++ {
			producerWg.Add(1)
			go extractWarcWorker(urlChan, domainChan, &producerWg, bar)
		}
	}

	// Wait for ALL producers (CC + crt.sh) before closing the channel —
	// otherwise a late crt.sh emit would panic on a closed channel.
	producerWg.Wait()

	// Close channel to signal testers that no more domains will come
	// This allows testers to finish processing remaining domains in channel
	close(domainChan)

	// Wait for all testers to finish processing remaining domains
	testWg.Wait()

	stopProgress <- true
	bar.Finish()

	elapsed := time.Since(startTime)
	finalLive := totalLiveDomains.Load()
	finalTested := totalDomains.Load()
	filesProcessed := totalProcessed.Load()

	finalExtracted := totalExtracted.Load()

	fmt.Printf("\n%s[✓]%s Extraction and testing complete!\n", GREEN, RESET)
	fmt.Printf("%s[i]%s Live domains found: %d", CYAN, RESET, finalLive)
	if *maxDomains > 0 {
		percentage := float64(finalLive) / float64(*maxDomains) * 100
		fmt.Printf(" / %d (%.1f%%)", *maxDomains, percentage)
	}
	fmt.Printf("\n")
	fmt.Printf("%s[i]%s Total domains extracted: %d\n", CYAN, RESET, finalExtracted)
	fmt.Printf("%s[i]%s Total domains tested: %d\n", CYAN, RESET, finalTested)
	if finalTested > 0 {
		liveRate := float64(finalLive) / float64(finalTested) * 100
		fmt.Printf("%s[i]%s Live rate: %.2f%%\n", CYAN, RESET, liveRate)
	}
	fmt.Printf("%s[i]%s Files processed: %d/%d\n", CYAN, RESET, filesProcessed, filesToProcess)
	fmt.Printf("%s[i]%s Time taken: %s\n", CYAN, RESET, elapsed.Round(time.Second))
	fmt.Printf("%s[i]%s Output saved to: %s\n", CYAN, RESET, *outputFile)
	if elapsed.Seconds() > 0 {
		testSpeed := float64(finalTested) / elapsed.Seconds()
		extractSpeed := float64(finalExtracted) / elapsed.Seconds()
		fmt.Printf("%s[i]%s Extraction speed: %.0f domains/second\n", CYAN, RESET, extractSpeed)
		fmt.Printf("%s[i]%s Testing speed: %.0f domains/second\n", CYAN, RESET, testSpeed)
	}
}
