package main

import (
	"net/http"
	"regexp"
)

// Mailjet uses a public API key + private secret key pair, both 32-char hex.
// The pattern matches the public key shape; the validator uses basic auth
// (publicKey:secretKey). For credentials found in pairs we treat any
// 32-char hex string preceded by `mailjet` context as a public-key candidate.
// (Pair-validation is best-effort — without the secret we 401 cleanly.)
var mailjetPattern = regexp.MustCompile(`(?i)(?:mailjet[_-]?(?:api[_-]?)?(?:key|public))["'\s:=]+([a-f0-9]{32})`)

// CheckMailjet validates a Mailjet API key against the user endpoint.
// Without a paired secret a plain GET 401s, so we attempt Basic auth with
// the key as both user and password — the 401 vs 200 still distinguishes
// "real key shape, wrong secret" from "no such key" (Mailjet returns 401
// for both, but the body differs). For a deterministic check we accept
// only 200; everything else is "not confirmed".
func (a *AWSScanner) CheckMailjet(key, sourceURL string) bool {
	req, err := http.NewRequest("GET", "https://api.mailjet.com/v3/REST/user", nil)
	if err != nil {
		return false
	}
	req.SetBasicAuth(key, key) // best-effort without paired secret
	req.Header.Set("Accept", "application/json")

	resp, err := do429Retry(client, req, 3)
	if err != nil || resp == nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == 200
}
