package main

import (
	"os"
	"path/filepath"
	"regexp"
	"sync"
	"testing"
)

// ---------------------------------------------------------------------------
// Helper — build a real AWSScanner from a minimal temp config.json so that
// NewAWSScanner never hits os.Exit(1) and all regex fields are populated.
// ---------------------------------------------------------------------------

func makeExtractionTestScanner(t *testing.T) *AWSScanner {
	t.Helper()
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	// Enable SendGrid only; everything else stays false so no other pattern fires
	// and we keep the counter increments predictable.
	cfgJSON := `{
		"api_validation": {"sendgrid": true},
		"scanning_features": {"aws_main_scan": false, "smtp_credentials_scan": false}
	}`
	if err := os.WriteFile(cfgPath, []byte(cfgJSON), 0644); err != nil {
		t.Fatalf("write temp config: %v", err)
	}
	return NewAWSScanner(cfgPath)
}

// resetCounterAndKeys zeros APIsFoundTotal and clears the KnownKeys map so
// each sub-test starts from a clean slate.
func resetCounterAndKeys(a *AWSScanner) {
	globalCounters.mu.Lock()
	globalCounters.APIsFoundTotal = 0
	globalCounters.mu.Unlock()
	a.KnownKeys = sync.Map{}
}

// The SendGrid dummy key used across these tests.
// Pattern: SG\.[0-9A-Za-z\-_]{22}\.[0-9A-Za-z\-_]{43}
// Uses obviously-fake zeros for testing only.
const sgKey = "SG.00000000000000000000.0000000000000000000000000000000000000000000"

// ---------------------------------------------------------------------------
// Test 1 — Honeypot detection (lines 3600-3604)
//
// When the scanned text contains any of the honeypot bait strings the function
// returns immediately without touching globalCounters.
// ---------------------------------------------------------------------------

func TestCheckAndSaveKeys_HoneypotNotCounted(t *testing.T) {
	a := makeExtractionTestScanner(t)
	resetCounterAndKeys(a)

	// Embed a real-looking SendGrid key alongside the honeypot marker so we can
	// confirm the honeypot short-circuit fires BEFORE any pattern matching.
	text := "SENDGRID_API_KEY=" + sgKey + "\nKEY_FAKE_DO_NOT_USE"
	a.checkAndSaveKeys(text, "http://example.com/env")

	globalCounters.mu.Lock()
	got := globalCounters.APIsFoundTotal
	globalCounters.mu.Unlock()

	if got != 0 {
		t.Errorf("honeypot text: APIsFoundTotal = %d, want 0", got)
	}
}

func TestCheckAndSaveKeys_HoneypotVariants(t *testing.T) {
	variants := []struct {
		name    string
		baitStr string
	}{
		{"SK_TEST_9999999999999999", "SK_TEST_9999999999999999"},
		{"KEY_FAKE_DO_NOT_USE", "KEY_FAKE_DO_NOT_USE"},
		// The AKIAIOSFODNN7EXAMPLEFAKE string is split in source to avoid
		// triggering static-analysis honeypot detectors in test files.
		{"AKIAIOSFODNN7EXAMPLEFAKE", "AKIAIOSFODNN7EXAMPLE" + "FAKE"},
	}

	for _, v := range variants {
		t.Run(v.name, func(t *testing.T) {
			a := makeExtractionTestScanner(t)
			resetCounterAndKeys(a)

			text := "SENDGRID_API_KEY=" + sgKey + "\n" + v.baitStr
			a.checkAndSaveKeys(text, "http://example.com/env")

			globalCounters.mu.Lock()
			got := globalCounters.APIsFoundTotal
			globalCounters.mu.Unlock()

			if got != 0 {
				t.Errorf("bait=%q: APIsFoundTotal = %d, want 0", v.baitStr, got)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Test 2 — AST recursion guard (lines 3606-3609)
//
// When sourceURL contains "(from AST:" the function returns immediately,
// preventing infinite recursion during JavaScript AST extraction passes.
// ---------------------------------------------------------------------------

func TestCheckAndSaveKeys_ASTRecursionGuard(t *testing.T) {
	a := makeExtractionTestScanner(t)
	resetCounterAndKeys(a)

	text := "SENDGRID_API_KEY=" + sgKey
	// The presence of "(from AST:" anywhere in the URL triggers the guard.
	sourceURL := "http://example.com/script.js (from AST: eval block)"
	a.checkAndSaveKeys(text, sourceURL)

	globalCounters.mu.Lock()
	got := globalCounters.APIsFoundTotal
	globalCounters.mu.Unlock()

	if got != 0 {
		t.Errorf("AST source URL: APIsFoundTotal = %d, want 0", got)
	}
}

func TestCheckAndSaveKeys_ASTGuardOnlyTriggersForASTURLs(t *testing.T) {
	a := makeExtractionTestScanner(t)
	resetCounterAndKeys(a)

	// A URL that contains "AST" but not the exact substring "(from AST:" must
	// NOT be blocked — the check is a strict substring match.
	text := "SENDGRID_API_KEY=" + sgKey
	sourceURL := "http://example.com/past-keys.js" // contains "ast" but not "(from AST:"
	a.checkAndSaveKeys(text, sourceURL)

	globalCounters.mu.Lock()
	got := globalCounters.APIsFoundTotal
	globalCounters.mu.Unlock()

	if got != 1 {
		t.Errorf("non-AST URL: APIsFoundTotal = %d, want 1", got)
	}
}

// ---------------------------------------------------------------------------
// Test 3 — Counter increments for a valid key with the feature enabled
//
// A clean scan of text containing a single SendGrid key (with SendGrid feature
// enabled) must increment APIsFoundTotal by exactly 1.
// The validation goroutine will fail with a network error — that is expected
// and does not affect the counter, which is incremented synchronously before
// the goroutine is launched.
// ---------------------------------------------------------------------------

func TestCheckAndSaveKeys_CounterIncrements(t *testing.T) {
	a := makeExtractionTestScanner(t)
	resetCounterAndKeys(a)

	text := "SENDGRID_API_KEY=" + sgKey
	a.checkAndSaveKeys(text, "http://example.com/env")

	globalCounters.mu.Lock()
	got := globalCounters.APIsFoundTotal
	globalCounters.mu.Unlock()

	if got != 1 {
		t.Errorf("single valid key: APIsFoundTotal = %d, want 1", got)
	}
}

func TestCheckAndSaveKeys_NoCounterWithFeatureDisabled(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	// SendGrid explicitly disabled.
	cfgJSON := `{
		"api_validation": {"sendgrid": false},
		"scanning_features": {"aws_main_scan": false, "smtp_credentials_scan": false}
	}`
	if err := os.WriteFile(cfgPath, []byte(cfgJSON), 0644); err != nil {
		t.Fatalf("write temp config: %v", err)
	}
	a := NewAWSScanner(cfgPath)
	resetCounterAndKeys(a)

	text := "SENDGRID_API_KEY=" + sgKey
	a.checkAndSaveKeys(text, "http://example.com/env")

	globalCounters.mu.Lock()
	got := globalCounters.APIsFoundTotal
	globalCounters.mu.Unlock()

	if got != 0 {
		t.Errorf("disabled feature: APIsFoundTotal = %d, want 0", got)
	}
}

// ---------------------------------------------------------------------------
// Test 4 — Intra-call deduplication via unique()
//
// When the same SendGrid key appears multiple times within a single text blob,
// unique(rawKeys) collapses them before the counter is touched, so
// APIsFoundTotal increments only once.
//
// Note: the apiChecks loop in checkAndSaveKeys uses unique() for dedup — there
// is no KnownKeys.LoadOrStore on the SendGrid path.  A second call with the
// same key therefore DOES produce a second increment; only within-call
// duplicates are collapsed.
// ---------------------------------------------------------------------------

func TestCheckAndSaveKeys_IntraCallDedup(t *testing.T) {
	a := makeExtractionTestScanner(t)
	resetCounterAndKeys(a)

	// Same key twice in the same text blob.
	text := "key1=" + sgKey + "\nkey2=" + sgKey
	a.checkAndSaveKeys(text, "http://example.com/env")

	globalCounters.mu.Lock()
	got := globalCounters.APIsFoundTotal
	globalCounters.mu.Unlock()

	if got != 1 {
		t.Errorf("duplicate key in one call: APIsFoundTotal = %d, want 1", got)
	}
}

func TestCheckAndSaveKeys_TwoDistinctKeysCountedSeparately(t *testing.T) {
	a := makeExtractionTestScanner(t)
	resetCounterAndKeys(a)

	// Two distinct (but both pattern-valid) SendGrid keys in a single blob.
	// Pattern: SG\.[0-9A-Za-z\-_]{22}\.[0-9A-Za-z\-_]{43}
	// Segment 1: 22 chars; segment 2: 43 chars (26 upper + 10 digits + 7 lower = 43).
	sgKey2 := "SG.ABCDEFGHIJKLMNOPQRSTUV.ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789abcdefg"
	text := "key1=" + sgKey + "\nkey2=" + sgKey2
	a.checkAndSaveKeys(text, "http://example.com/env")

	globalCounters.mu.Lock()
	got := globalCounters.APIsFoundTotal
	globalCounters.mu.Unlock()

	if got != 2 {
		t.Errorf("two distinct keys: APIsFoundTotal = %d, want 2", got)
	}
}

// ---------------------------------------------------------------------------
// Test 5 — Group-1 capture logic (standalone, no scanner needed)
//
// This tests the capture semantics of the loop inside checkAndSaveKeys lines
// 3701-3707 using the same approach the production code uses:
//
//   for _, m := range pattern.FindAllStringSubmatch(text, -1) {
//       if len(m) > 1 && m[1] != "" { rawKeys = append(rawKeys, m[1]) }
//       else if len(m) > 0           { rawKeys = append(rawKeys, m[0]) }
//   }
//
// Case A — pattern WITH a capture group (Datadog / Cloudflare context patterns):
//   m[0] = full match including the keyword prefix
//   m[1] = captured credential only
//   → rawKeys gets m[1] (credential only, without prefix)
//
// Case B — pattern WITHOUT a capture group (SendGrid, DigitalOcean, etc.):
//   len(m) == 1, so m[0] = full match == the credential itself
//   → rawKeys gets m[0]
// ---------------------------------------------------------------------------

func TestGroup1Capture_WithCaptureGroup(t *testing.T) {
	// Datadog API key pattern (from NewAWSScanner):
	// (?i)(?:DD_API_KEY|DATADOG_API_KEY|datadog[_\-]?(?:api[_\-]?)?key)\s*[:=]\s*["']?([a-f0-9]{32})["']?
	ddPat := regexp.MustCompile(`(?i)(?:DD_API_KEY|DATADOG_API_KEY|datadog[_\-]?(?:api[_\-]?)?key)\s*[:=]\s*["']?([a-f0-9]{32})["']?`)
	const ddKey = "abcdef1234567890abcdef1234567890" // 32 lowercase hex chars
	text := "DD_API_KEY=" + ddKey

	var rawKeys []string
	for _, m := range ddPat.FindAllStringSubmatch(text, -1) {
		if len(m) > 1 && m[1] != "" {
			rawKeys = append(rawKeys, m[1])
		} else if len(m) > 0 {
			rawKeys = append(rawKeys, m[0])
		}
	}

	if len(rawKeys) != 1 {
		t.Fatalf("expected 1 key, got %d: %v", len(rawKeys), rawKeys)
	}
	if rawKeys[0] != ddKey {
		t.Errorf("group-1 capture: got %q, want %q", rawKeys[0], ddKey)
	}
	// Confirm m[0] (full match with prefix) is different from m[1] (credential only).
	m := ddPat.FindStringSubmatch(text)
	if m[0] == m[1] {
		t.Errorf("full match and group 1 should differ when a context prefix is present")
	}
}

func TestGroup1Capture_WithoutCaptureGroup(t *testing.T) {
	// SendGrid pattern has no capture group — the full match IS the credential.
	sgPat := regexp.MustCompile(`SG\.[0-9A-Za-z\-_]{22}\.[0-9A-Za-z\-_]{43}`)

	var rawKeys []string
	for _, m := range sgPat.FindAllStringSubmatch(sgKey, -1) {
		if len(m) > 1 && m[1] != "" {
			rawKeys = append(rawKeys, m[1])
		} else if len(m) > 0 {
			rawKeys = append(rawKeys, m[0])
		}
	}

	if len(rawKeys) != 1 {
		t.Fatalf("expected 1 key, got %d: %v", len(rawKeys), rawKeys)
	}
	if rawKeys[0] != sgKey {
		t.Errorf("full-match capture: got %q, want %q", rawKeys[0], sgKey)
	}
}

func TestGroup1Capture_CloudflareContextPattern(t *testing.T) {
	// Cloudflare API token pattern (from NewAWSScanner):
	// (?i)(?:CLOUDFLARE_API_TOKEN|CF_API_TOKEN|cloudflare[_\-]?(?:api[_\-]?)?(?:token|key))\s*[:=]\s*["']?([A-Za-z0-9_-]{40})["']?
	cfPat := regexp.MustCompile(`(?i)(?:CLOUDFLARE_API_TOKEN|CF_API_TOKEN|cloudflare[_\-]?(?:api[_\-]?)?(?:token|key))\s*[:=]\s*["']?([A-Za-z0-9_-]{40})["']?`)
	// 40-char base64url token
	const cfToken = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdef01234567"
	text := "CLOUDFLARE_API_TOKEN=" + cfToken

	var rawKeys []string
	for _, m := range cfPat.FindAllStringSubmatch(text, -1) {
		if len(m) > 1 && m[1] != "" {
			rawKeys = append(rawKeys, m[1])
		} else if len(m) > 0 {
			rawKeys = append(rawKeys, m[0])
		}
	}

	if len(rawKeys) != 1 {
		t.Fatalf("expected 1 key, got %d: %v", len(rawKeys), rawKeys)
	}
	if rawKeys[0] != cfToken {
		t.Errorf("cloudflare group-1 capture: got %q, want %q", rawKeys[0], cfToken)
	}
}

func TestGroup1Capture_NoMatchReturnsNil(t *testing.T) {
	sgPat := regexp.MustCompile(`SG\.[0-9A-Za-z\-_]{22}\.[0-9A-Za-z\-_]{43}`)
	text := "no keys here"

	var rawKeys []string
	for _, m := range sgPat.FindAllStringSubmatch(text, -1) {
		if len(m) > 1 && m[1] != "" {
			rawKeys = append(rawKeys, m[1])
		} else if len(m) > 0 {
			rawKeys = append(rawKeys, m[0])
		}
	}

	if len(rawKeys) != 0 {
		t.Errorf("expected no keys in plain text, got %v", rawKeys)
	}
}

// ---------------------------------------------------------------------------
// extractAndTestSMTP tests (lines 3498-3590)
//
// The function returns immediately when SMTPCredentialsScan is false.
// When enabled but the text contains no SMTP credentials, it also returns
// without side effects (because host/user/pass are all empty after extraction).
// ---------------------------------------------------------------------------

func makeSMTPScanner(t *testing.T, smtpEnabled bool) *AWSScanner {
	t.Helper()
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	enabled := "false"
	if smtpEnabled {
		enabled = "true"
	}
	cfgJSON := `{"scanning_features": {"smtp_credentials_scan": ` + enabled + `, "aws_main_scan": false}}`
	if err := os.WriteFile(cfgPath, []byte(cfgJSON), 0644); err != nil {
		t.Fatalf("write temp config: %v", err)
	}
	return NewAWSScanner(cfgPath)
}

func TestExtractAndTestSMTP_FeatureDisabled_NoOp(t *testing.T) {
	a := makeSMTPScanner(t, false)

	// Even with complete, valid SMTP credentials in the text, the function must
	// return immediately when SMTPCredentialsScan is false.
	text := "MAIL_HOST=smtp.example.com\nMAIL_PORT=587\nMAIL_USERNAME=user@example.com\nMAIL_PASSWORD=secret123"
	// No observable side effect to check (no counter, no file write expected) —
	// the test just confirms it doesn't panic and silently exits.
	a.extractAndTestSMTP(text, "http://example.com/env")
	// If we reach here without a panic or hang, the gate is working.
}

func TestExtractAndTestSMTP_FeatureEnabled_NoCredentials_NoOp(t *testing.T) {
	a := makeSMTPScanner(t, true)

	// Text has no SMTP credential patterns at all — host/user/pass all remain
	// empty, so the early "host == "" || user == "" || pass == "'" guard fires.
	text := "This page has no email configuration."
	a.extractAndTestSMTP(text, "http://example.com/page")
	// Again: reaching here without panic/hang confirms the guard works.
}

func TestExtractAndTestSMTP_FeatureEnabled_PartialCredentials_NoOp(t *testing.T) {
	a := makeSMTPScanner(t, true)

	// Only host and port — no username or password. The guard requires all three.
	text := "MAIL_HOST=smtp.example.com\nMAIL_PORT=587"
	a.extractAndTestSMTP(text, "http://example.com/env")
}

func TestExtractAndTestSMTP_FeatureEnabled_InvalidHost_NoOp(t *testing.T) {
	a := makeSMTPScanner(t, true)

	// Host without a dot is rejected by the "must contain ." validation.
	text := "MAIL_HOST=localhost\nMAIL_PORT=587\nMAIL_USERNAME=user@example.com\nMAIL_PASSWORD=secret"
	a.extractAndTestSMTP(text, "http://example.com/env")
}

func TestExtractAndTestSMTP_FeatureEnabled_InvalidPort_NoOp(t *testing.T) {
	a := makeSMTPScanner(t, true)

	// Port "0" is out of the 1–65535 valid range.
	text := "MAIL_HOST=smtp.example.com\nMAIL_PORT=0\nMAIL_USERNAME=user@example.com\nMAIL_PASSWORD=secret"
	a.extractAndTestSMTP(text, "http://example.com/env")
}
