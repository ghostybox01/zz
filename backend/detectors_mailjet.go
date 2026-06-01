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
var mailjetPattern = regexp.MustCompile(`(?i)(?:mailjet[_-]?(?:api[_-]?)?(?:key|public))["'\s:=]+([a-f0-9]{32})`)

// CheckMailjet validates a Mailjet API key against the user endpoint.
// Uses the key as both user and password for best-effort validation —
// only a 200 confirms a valid key.
func (a *AWSScanner) CheckMailjet(key, sourceURL string) bool {
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
		return true
	}
	return false
}
