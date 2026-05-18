package main

import (
	"net/http"
	"regexp"
)

// Heroku auth tokens are UUID v4 (36-char). The Heroku Platform API
// requires `Accept: application/vnd.heroku+json; version=3` and validates
// the token via GET /account.
var herokuPattern = regexp.MustCompile(`(?i)(?:heroku[_-]?(?:api[_-]?)?(?:key|token))["'\s:=]+([a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12})`)

// CheckHeroku validates a Heroku API token by fetching the account.
// 200 = key is valid; 401 = invalid; anything else = not confirmed.
// Uses do429Retry for rate-limit resilience.
func (a *AWSScanner) CheckHeroku(key, sourceURL string) bool {
	req, err := http.NewRequest("GET", "https://api.heroku.com/account", nil)
	if err != nil {
		return false
	}
	req.Header.Set("Accept", "application/vnd.heroku+json; version=3")
	req.Header.Set("Authorization", "Bearer "+key)

	resp, err := do429Retry(httpClient, req, 3)
	if err != nil || resp == nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == 200
}
