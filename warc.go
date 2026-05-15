
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
)

const (
	MAX_BUFFER_SIZE = 10 * 1024 * 1024 // 10 MB buffer for large records
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

	// File writing mutex
	fileMutex sync.Mutex

	// Global max domains check
	globalMaxDomains atomic.Int64

	// Global verbose flag
	globalVerbose atomic.Bool

	// Regex patterns
	urlRegex       = regexp.MustCompile(`WARC-Target-URI:\s+(https?://[^\s]+)`)
	envFilterRegex = regexp.MustCompile(`(?i)(\.env|/env|env\.|config\.env|env\.php|env\.json|environment)`)

	// HTTP client for connection testing with custom transport to suppress errors
	httpClient = &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: false,
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
			// Follow redirects but limit to 5
			if len(via) >= 5 {
				return http.ErrUseLastResponse
			}
			return nil
		},
	}
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
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			if attempt == maxRetries {
				return nil, fmt.Errorf("HTTP %d: Failed to fetch collection info", resp.StatusCode)
			}
			continue
		}

		var readErr error
		body, readErr = io.ReadAll(resp.Body)
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

func selectRandomSnapshots(maxDomains int64) []string {
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

	// Determine number of snapshots based on max-domains
	// More domains = more snapshots needed
	numSnapshots := 1
	if maxDomains > 1000000 {
		numSnapshots = 3
	} else if maxDomains > 500000 {
		numSnapshots = 2
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
func extractWarcFile(warcURL string, domainChan chan<- string, wg *sync.WaitGroup, bar *progressbar.ProgressBar, extractSem chan struct{}) {
	defer wg.Done()
	defer func() {
		if r := recover(); r != nil {
			// Silently handle panics
		}
		bar.Add(1)
		<-extractSem
	}()

	extractSem <- struct{}{} // Acquire semaphore

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
		// Silently skip connection errors
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

			if domain == "" {
				continue
			}

			// Check if already seen
			if _, loaded := uniqueDomains.LoadOrStore(domain, true); loaded {
				continue
			}

			// Check limit before sending to channel
			maxDomains := globalMaxDomains.Load()
			if maxDomains > 0 {
				currentTotal := totalLiveDomains.Load()
				if currentTotal >= maxDomains {
					return
				}
			}

			totalExtracted.Add(1)

			// Verbose: log domain received/extracted
			if globalVerbose.Load() {
				fmt.Printf("%s[EXTRACTED]%s Domain received: %s\n", CYAN, RESET, domain)
			}

			// Send domain to channel for testing (blocking to ensure no domain is lost)
			// Check limit again before blocking send
			maxDomainsCheck := globalMaxDomains.Load()
			if maxDomainsCheck > 0 {
				currentTotal := totalLiveDomains.Load()
				if currentTotal >= maxDomainsCheck {
					return
				}
			}

			domainChan <- domain // Blocking send - ensures all domains are tested

			// Check limit after sending
			if maxDomains > 0 {
				current := totalLiveDomains.Load()
				if current >= maxDomains {
					return
				}
			}
		}
	}

	// Handle scanner errors silently
	if err := scanner.Err(); err != nil {
		// Silently skip scanner errors
	}

	totalProcessed.Add(1)
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

func main() {
	var (
		maxDomains     = flag.Int64("max-domains", 10000, "Maximum live domains to extract (required)")
		outputFile     = flag.String("output", "live_domains.txt", "Output file for live domains")
		extractWorkers = flag.Int("extract-workers", 200, "Number of concurrent workers for WARC extraction (grabber)")
		testWorkers    = flag.Int("test-workers", 100, "Number of concurrent workers for connection testing")
		limit          = flag.Int("limit", 0, "Limit number of WARC files to process (0 = auto)")
		channelSize    = flag.Int("channel-size", 10000, "Size of domain channel buffer")
		verbose        = flag.Bool("verbose", false, "Enable verbose mode to see extracted, live, and dead domains")
	)
	flag.Parse()

	if *maxDomains <= 0 {
		fmt.Printf("%s[ERROR]%s max-domains must be greater than 0\n", RED, RESET)
		fmt.Println("Usage: ./warc_live_checker -max-domains 10000 [options]")
		flag.PrintDefaults()
		os.Exit(1)
	}

	// Set global verbose flag
	globalVerbose.Store(*verbose)

	fmt.Printf("%s[*]%s WARC Live Domain Checker - Extract & Test Domains\n", CYAN, RESET)
	if *verbose {
		fmt.Printf("%s[*]%s Verbose mode enabled\n", CYAN, RESET)
	}
	fmt.Println()

	// Select random snapshots based on max-domains
	fmt.Printf("%s[*]%s Selecting random CC-MAIN snapshots...\n", CYAN, RESET)
	snapshotList := selectRandomSnapshots(*maxDomains)

	if len(snapshotList) == 0 {
		fmt.Printf("%s[ERROR]%s No snapshots available\n", RED, RESET)
		os.Exit(1)
	}

	fmt.Printf("%s[+]%s Selected %d snapshot(s): %s\n", GREEN, RESET, len(snapshotList), strings.Join(snapshotList, ", "))

	// Collect all WARC URLs from selected snapshots
	var allWarcURLs []string
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
	filesToProcess := totalFiles

	if *limit > 0 && *limit < totalFiles {
		filesToProcess = *limit
		fmt.Printf("%s[INFO]%s Limiting to %d files (out of %d)\n", YELLOW, RESET, filesToProcess, totalFiles)
	} else {
		// Auto-limit based on max-domains
		// Estimate: each file might yield 10-100 live domains
		estimatedFilesNeeded := int(*maxDomains / 50) // Conservative estimate
		if estimatedFilesNeeded < filesToProcess {
			filesToProcess = estimatedFilesNeeded
			if filesToProcess < 100 {
				filesToProcess = 100 // Minimum 100 files
			}
		}
		if filesToProcess > 10000 {
			filesToProcess = 10000 // Maximum 10k files
		}
		fmt.Printf("%s[INFO]%s Auto-limiting to %d files to reach ~%d live domains\n", YELLOW, RESET, filesToProcess, *maxDomains)
	}

	fmt.Printf("%s[+]%s Found %d WARC files total\n", GREEN, RESET, totalFiles)
	fmt.Printf("%s[*]%s Extraction workers (grabber): %d\n", CYAN, RESET, *extractWorkers)
	fmt.Printf("%s[*]%s Testing workers (live check): %d\n", CYAN, RESET, *testWorkers)
	fmt.Printf("%s[*]%s Processing %d files in parallel\n", CYAN, RESET, filesToProcess)
	fmt.Printf("%s[INFO]%s Target: %d live domains\n", YELLOW, RESET, *maxDomains)

	// Create output file
	outFile, err := os.Create(*outputFile)
	if err != nil {
		fmt.Printf("%s[ERROR]%s Failed to create output file: %v\n", RED, RESET, err)
		os.Exit(1)
	}
	defer outFile.Close()

	// No header - clean output with only domains

	// Progress bar
	bar := progressbar.NewOptions(filesToProcess,
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

	// Create domain channel for communication between extractor and tester
	domainChan := make(chan string, *channelSize)

	// Semaphores for controlling concurrency
	extractSem := make(chan struct{}, *extractWorkers)
	testSem := make(chan struct{}, *testWorkers)

	// Wait groups
	var extractWg sync.WaitGroup
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

	// Start extractor workers (grabber) - they extract domains and send to channel
	for i := 0; i < filesToProcess; i++ {
		if *maxDomains > 0 {
			current := totalLiveDomains.Load()
			if current >= *maxDomains {
				fmt.Printf("\n%s[INFO]%s Reached max live domains limit (%d), stopping new file starts\n", GREEN, RESET, *maxDomains)
				break
			}
		}

		extractWg.Add(1)
		go extractWarcFile(allWarcURLs[i], domainChan, &extractWg, bar, extractSem)
	}

	// Wait for all extractors to finish
	extractWg.Wait()

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
