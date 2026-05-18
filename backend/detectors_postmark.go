package main

import (
	"net/http"
	"regexp"
)

// Postmark server tokens are 36-char UUIDs (Postmark uses RFC4122 v4 format).
// Account tokens follow the same shape. The X-Postmark-Server-Token header
// validates against /server endpoint; a 200 confirms the key.
var postmarkPattern = regexp.MustCompile(`(?i)(?:postmark[_-]?(?:server[_-]?)?token|POSTMARK_API_TOKEN)["'\s:=]+([a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12})`)

// CheckPostmark validates a Postmark server token by hitting GET /server
// with the X-Postmark-Server-Token header. Returns true on 200 (token is
// valid and active). 401/422 → false. Other statuses → false (treat as
// not-confirmed rather than risk a false positive).
//
// Reuses the package-level do429Retry helper (defined in main.go by HMS
// Frontline) for rate-limit resilience.
func (a *AWSScanner) CheckPostmark(key, sourceURL string) bool {
	req, err := http.NewRequest("GET", "https://api.postmarkapp.com/server", nil)
	if err != nil {
		return false
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Postmark-Server-Token", key)

	resp, err := do429Retry(httpClient, req, 3)
	if err != nil || resp == nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == 200
}
