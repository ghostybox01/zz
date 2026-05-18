package main

import (
	"net/http"
	"regexp"
)

// Mailtrap API tokens are 32-char hex (sending API tokens). Validation
// hits GET /api/accounts with the Api-Token header. A 200 confirms the
// token is bound to a real account.
var mailtrapPattern = regexp.MustCompile(`(?i)(?:mailtrap[_-]?(?:api[_-]?)?(?:token|key)|MAILTRAP_API_TOKEN)["'\s:=]+([a-f0-9]{32})`)

// CheckMailtrap validates a Mailtrap API token by listing accounts.
// 200 = token is valid; anything else = not confirmed.
// Uses do429Retry for rate-limit resilience.
func (a *AWSScanner) CheckMailtrap(key, sourceURL string) bool {
	req, err := http.NewRequest("GET", "https://mailtrap.io/api/accounts", nil)
	if err != nil {
		return false
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Api-Token", key)

	resp, err := do429Retry(httpClient, req, 3)
	if err != nil || resp == nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == 200
}
