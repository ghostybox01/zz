package main

import (
	"net/http"
	"regexp"
)

// Datadog API keys are 32-char lowercase hex. The validation endpoint
// is /api/v1/validate which checks the key via the DD-API-KEY header.
// Returns 200 on valid, 403 on invalid.
var datadogPattern = regexp.MustCompile(`(?i)(?:datadog|DD)[_-]?(?:api[_-]?)?key["'\s:=]+([a-f0-9]{32})`)

// CheckDatadog validates a Datadog API key against the /validate endpoint.
// 200 = key is valid for the org; anything else = not confirmed.
// Uses do429Retry for rate-limit resilience.
func (a *AWSScanner) CheckDatadog(key, sourceURL string) bool {
	req, err := http.NewRequest("GET", "https://api.datadoghq.com/api/v1/validate", nil)
	if err != nil {
		return false
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("DD-API-KEY", key)

	resp, err := do429Retry(httpClient, req, 3)
	if err != nil || resp == nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == 200
}
