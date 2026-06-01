package main

import (
	"fmt"
	"net/http"
	"regexp"
)

// SparkPost API keys are 40-character lowercase hex strings. Validation
// hits GET /api/v1/account with the key in the Authorization header
// (no Bearer prefix — SparkPost uses raw-key auth).
var sparkpostPattern = regexp.MustCompile(`(?i)(?:sparkpost[_-]?(?:api[_-]?)?key|SPARKPOST_API_KEY)["'\s:=]+([a-f0-9]{40})`)

// CheckSparkPost validates a SparkPost API key against the /account endpoint.
// 200 = key is valid for the account; anything else = not confirmed.
func (a *AWSScanner) CheckSparkPost(key, sourceURL string) bool {
	req, err := http.NewRequest("GET", "https://api.sparkpost.com/api/v1/account", nil)
	if err != nil {
		return false
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", key)

	resp, err := do429Retry(client, req, 3)
	if err != nil || resp == nil {
		return false
	}
	defer resp.Body.Close()

	if resp.StatusCode == 200 {
		a.logValid("SparkPost", fmt.Sprintf("Key: %s", key))
		a.saveIntoFile(fmt.Sprintf("%s:%s", sanitizeSource(sourceURL), key), "valid_sparkpost.txt")
		a.storeValidKeyLimit("SparkPost", key, "Active")
		return true
	}
	return false
}
