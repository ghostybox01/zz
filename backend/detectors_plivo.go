package main

import (
	"fmt"
	"net/http"
	"regexp"
)

// Plivo uses an Auth ID (20-char alphanumeric, starts with MA or SA) and
// an Auth Token (40-char alphanumeric). The pattern matches the Auth ID.
// Validation uses Basic auth against the /Account/<AuthID>/ endpoint.
var plivoPattern = regexp.MustCompile(`(?i)(?:plivo[_-]?(?:auth[_-]?)?(?:id|sid))["'\s:=]+([MS]A[A-Z0-9]{18})`)

// CheckPlivo validates a Plivo Auth ID by hitting the account endpoint
// with Basic auth (id:id for best-effort without paired token).
// Only 200 confirms a real authenticated key.
func (a *AWSScanner) CheckPlivo(key, sourceURL string) bool {
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
		return true
	}
	return false
}
