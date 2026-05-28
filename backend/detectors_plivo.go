package main

import (
	"net/http"
	"regexp"
)

// Plivo uses an Auth ID (20-char alphanumeric, starts with MA or SA) and
// an Auth Token (40-char alphanumeric). The pattern matches the Auth ID
// (the Twilio-equivalent of an Account SID). Validation uses Basic auth
// against the /Account/<AuthID>/ endpoint.
var plivoPattern = regexp.MustCompile(`(?i)(?:plivo[_-]?(?:auth[_-]?)?(?:id|sid))["'\s:=]+([MS]A[A-Z0-9]{18})`)

// CheckPlivo validates a Plivo Auth ID by hitting the account endpoint
// with Basic auth (id:token). Without a paired token we use id:id which
// 401s deterministically — only 200 confirms a real authenticated key.
// Uses do429Retry for rate-limit resilience.
func (a *AWSScanner) CheckPlivo(key, sourceURL string) bool {
	url := "https://api.plivo.com/v1/Account/" + key + "/"
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return false
	}
	req.SetBasicAuth(key, key) // best-effort without paired token
	req.Header.Set("Accept", "application/json")

	resp, err := do429Retry(client, req, 3)
	if err != nil || resp == nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == 200
}
