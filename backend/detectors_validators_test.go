package main

// detectors_validators_test.go
//
// Tests every Check* validator method using a mock HTTP transport that intercepts
// calls made via the global `client` variable. Each TestValidator_<Name> function
// runs three or four sub-tests:
//
//   gate_off          — feature flag disabled → returns false immediately
//   valid_key         — mock returns expected success → returns true, APIsValidated == 1
//   invalid_key       — mock returns 401 → returns false, APIsValidated == 0
//   valid_status_200_wrong_body (some validators only) — 200 but body fails extra check
//
// NOTE: These tests are NOT run in parallel at the top level because they all share
// the global `client` and `globalCounters` variables. Parallel execution would cause
// data races and spurious failures.
//
// Run with:  go test -v -run TestValidator_ ./...

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// ---------------------------------------------------------------------------
// Mock transport
// ---------------------------------------------------------------------------

type mockRoute struct {
	urlSubstr  string
	statusCode int
	body       string
}

type mockTransport struct {
	routes []mockRoute
}

func (m *mockTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	for _, route := range m.routes {
		if strings.Contains(req.URL.String(), route.urlSubstr) {
			return &http.Response{
				StatusCode: route.statusCode,
				Body:       io.NopCloser(strings.NewReader(route.body)),
				Header:     make(http.Header),
			}, nil
		}
	}
	// Default: unauthorized
	return &http.Response{
		StatusCode: 401,
		Body:       io.NopCloser(strings.NewReader(`{"error":"unauthorized"}`)),
		Header:     make(http.Header),
	}, nil
}

func setMockClient(t *testing.T, routes []mockRoute) {
	t.Helper()
	orig := client
	client = &http.Client{Transport: &mockTransport{routes: routes}}
	t.Cleanup(func() {
		client = orig
	})
}

// ---------------------------------------------------------------------------
// Scanner construction helpers
// ---------------------------------------------------------------------------

// validatorScanner creates a scanner with all API validation features enabled.
func validatorScanner(t *testing.T) *AWSScanner {
	t.Helper()
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	cfgJSON := `{
		"api_validation": {
			"openai": true, "anthropic": true, "twilio": true, "sendgrid": true,
			"stripe": true, "mailgun": true, "telnyx": true, "messagebird": true,
			"brevo": true, "xsmtp": true, "mandrill": true, "mailersend": true,
			"nexmo": true, "github": true, "gcp_api_key": true, "crypto_wallet": true,
			"slack": true, "discord": true, "cloudflare": true, "digitalocean": true,
			"shopify": true, "hubspot": true, "heroku": true, "datadog": true,
			"postmark": true, "sparkpost": true, "mailtrap": true, "mailjet": true,
			"plivo": true, "tencent": true, "ai_all": true
		},
		"scanning_features": {
			"aws_main_scan": false,
			"smtp_credentials_scan": false
		}
	}`
	os.WriteFile(cfgPath, []byte(cfgJSON), 0644)
	return NewAWSScanner(cfgPath)
}

// scannerWithFeature creates a scanner with a single named feature set to enabled/disabled.
// The config JSON is built inline; all other api_validation flags default to false.
func scannerWithSingleFeature(t *testing.T, featureKey string, enabled bool) *AWSScanner {
	t.Helper()
	val := "false"
	if enabled {
		val = "true"
	}
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	cfgJSON := `{"api_validation": {"` + featureKey + `": ` + val + `}}`
	os.WriteFile(cfgPath, []byte(cfgJSON), 0644)
	return NewAWSScanner(cfgPath)
}

// resetValidatorState zeroes global counters and clears KnownKeys on the scanner.
func resetValidatorState(a *AWSScanner) {
	globalCounters.mu.Lock()
	globalCounters.APIsValidated = 0
	globalCounters.APIsFoundTotal = 0
	globalCounters.mu.Unlock()
	a.KnownKeys = sync.Map{}
}

// chdirTemp switches the working directory to a temp dir so saveIntoFile writes
// don't pollute the source tree. The original directory is restored via Cleanup.
func chdirTemp(t *testing.T) {
	t.Helper()
	orig, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	tmp := t.TempDir()
	if err := os.Chdir(tmp); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { os.Chdir(orig) })
}

// ---------------------------------------------------------------------------
// Helpers for counter assertions
// ---------------------------------------------------------------------------

func assertValidated(t *testing.T, want int) {
	t.Helper()
	globalCounters.mu.Lock()
	got := globalCounters.APIsValidated
	globalCounters.mu.Unlock()
	if got != want {
		t.Errorf("APIsValidated = %d, want %d", got, want)
	}
}

func assertFoundTotal(t *testing.T, want int) {
	t.Helper()
	globalCounters.mu.Lock()
	got := globalCounters.APIsFoundTotal
	globalCounters.mu.Unlock()
	if got != want {
		t.Errorf("APIsFoundTotal = %d, want %d", got, want)
	}
}

// ---------------------------------------------------------------------------
// TestValidator_OpenAI
// ---------------------------------------------------------------------------

func TestValidator_OpenAI(t *testing.T) {
	chdirTemp(t)
	const key = "sk-proj-ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmno"
	successRoutes := []mockRoute{
		{"api.openai.com", 200, `{"object":"list","data":[]}`},
	}

	t.Run("gate_off", func(t *testing.T) {
		a := scannerWithSingleFeature(t, "openai", false)
		// disable ai_all too (it's false by default in single-feature scanner)
		setMockClient(t, successRoutes)
		resetValidatorState(a)
		if got := a.CheckOpenAI(key, "http://example.com"); got != false {
			t.Error("expected false when gate is off")
		}
		assertValidated(t, 0)
	})

	t.Run("valid_key", func(t *testing.T) {
		a := validatorScanner(t)
		setMockClient(t, successRoutes)
		resetValidatorState(a)
		if got := a.CheckOpenAI(key, "http://example.com"); got != true {
			t.Error("expected true for valid key")
		}
		assertValidated(t, 1)
	})

	t.Run("invalid_key", func(t *testing.T) {
		a := validatorScanner(t)
		setMockClient(t, []mockRoute{{"api.openai.com", 401, `{"error":"invalid"}`}})
		resetValidatorState(a)
		if got := a.CheckOpenAI(key+"x", "http://example.com"); got != false {
			t.Error("expected false for invalid key")
		}
		assertValidated(t, 0)
	})
}

// ---------------------------------------------------------------------------
// TestValidator_Anthropic
// ---------------------------------------------------------------------------

func TestValidator_Anthropic(t *testing.T) {
	chdirTemp(t)
	const key = "sk-ant-api03-ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz1234-ABCDEFGHIJKLMNOPQRSTUVWXYZ1234"
	successRoutes := []mockRoute{
		{"api.anthropic.com", 200, `{"data":[]}`},
	}

	t.Run("gate_off", func(t *testing.T) {
		a := scannerWithSingleFeature(t, "anthropic", false)
		setMockClient(t, successRoutes)
		resetValidatorState(a)
		if got := a.CheckAnthropic(key, "http://example.com"); got != false {
			t.Error("expected false when gate is off")
		}
	})

	t.Run("valid_key", func(t *testing.T) {
		a := validatorScanner(t)
		setMockClient(t, successRoutes)
		resetValidatorState(a)
		if got := a.CheckAnthropic(key, "http://example.com"); got != true {
			t.Error("expected true for valid key")
		}
		assertValidated(t, 1)
	})

	t.Run("invalid_key", func(t *testing.T) {
		a := validatorScanner(t)
		setMockClient(t, []mockRoute{{"api.anthropic.com", 401, `{"error":"invalid"}`}})
		resetValidatorState(a)
		if got := a.CheckAnthropic(key+"x", "http://example.com"); got != false {
			t.Error("expected false for invalid key")
		}
		assertValidated(t, 0)
	})
}

// ---------------------------------------------------------------------------
// TestValidator_Twilio
// ---------------------------------------------------------------------------

func TestValidator_Twilio(t *testing.T) {
	chdirTemp(t)
	const (
		sid  = "ACabcdef1234567890abcdef1234567890"
		auth = "abcdef1234567890abcdef1234567890"
	)
	successRoutes := []mockRoute{
		{"api.twilio.com", 200, `{"status":"active","friendly_name":"Test"}`},
	}

	t.Run("gate_off", func(t *testing.T) {
		a := scannerWithSingleFeature(t, "twilio", false)
		setMockClient(t, successRoutes)
		resetValidatorState(a)
		if got := a.CheckTwilio(sid, auth, "http://example.com"); got != false {
			t.Error("expected false when gate is off")
		}
	})

	t.Run("invalid_sid_format", func(t *testing.T) {
		// Pre-flight regex rejects this before any HTTP call
		a := validatorScanner(t)
		setMockClient(t, successRoutes)
		resetValidatorState(a)
		if got := a.CheckTwilio("ACXXXXXXXXXX", auth, "http://example.com"); got != false {
			t.Error("expected false for malformed SID")
		}
		assertValidated(t, 0)
	})

	t.Run("valid_key", func(t *testing.T) {
		a := validatorScanner(t)
		setMockClient(t, successRoutes)
		resetValidatorState(a)
		if got := a.CheckTwilio(sid, auth, "http://example.com"); got != true {
			t.Error("expected true for valid SID+auth")
		}
		assertValidated(t, 1)
	})

	t.Run("invalid_key", func(t *testing.T) {
		a := validatorScanner(t)
		setMockClient(t, []mockRoute{{"api.twilio.com", 401, `{"code":20003}`}})
		resetValidatorState(a)
		// Use a different SID so KnownKeys dedup doesn't trigger
		if got := a.CheckTwilio("ACabcdef1234567890abcdef1234567891", auth, "http://example.com"); got != false {
			t.Error("expected false for 401")
		}
		assertValidated(t, 0)
	})
}

// ---------------------------------------------------------------------------
// TestValidator_SendGrid
// ---------------------------------------------------------------------------

func TestValidator_SendGrid(t *testing.T) {
	chdirTemp(t)
	const key = "SG.testkey123456789012345.testkeysecret1234567890123456789012"
	// Route all sendgrid requests: credits→200, senders→200, mail/send→202
	successRoutes := []mockRoute{
		{"api.sendgrid.com/v3/mail/send", 202, `{}`},
		{"api.sendgrid.com/v3/verified_senders", 200, `{"results":[]}`},
		{"api.sendgrid.com/v3/user/credits", 200, `{"total":100,"remain":50}`},
		{"api.sendgrid.com", 200, `{"total":100}`},
	}

	t.Run("gate_off", func(t *testing.T) {
		a := scannerWithSingleFeature(t, "sendgrid", false)
		setMockClient(t, successRoutes)
		resetValidatorState(a)
		if got := a.CheckSendGrid(key, "http://example.com"); got != false {
			t.Error("expected false when gate is off")
		}
	})

	t.Run("valid_key", func(t *testing.T) {
		a := validatorScanner(t)
		setMockClient(t, successRoutes)
		resetValidatorState(a)
		if got := a.CheckSendGrid(key, "http://example.com"); got != true {
			t.Error("expected true for valid key")
		}
		assertValidated(t, 1)
	})

	t.Run("invalid_key", func(t *testing.T) {
		a := validatorScanner(t)
		setMockClient(t, []mockRoute{{"api.sendgrid.com", 401, `{"errors":[{"message":"The provided authorization grant is invalid"}]}`}})
		resetValidatorState(a)
		if got := a.CheckSendGrid(key+"x", "http://example.com"); got != false {
			t.Error("expected false for 401")
		}
		assertValidated(t, 0)
	})
}

// ---------------------------------------------------------------------------
// TestValidator_Stripe
// ---------------------------------------------------------------------------

func TestValidator_Stripe(t *testing.T) {
	chdirTemp(t)
	const key = "sk_live_51ABCDEFGHIJKLMNOPQRSTUVWXYZabcdef1234567890"
	successRoutes := []mockRoute{
		{"api.stripe.com", 200, `{"livemode":false,"available":[]}`},
	}

	t.Run("gate_off", func(t *testing.T) {
		a := scannerWithSingleFeature(t, "stripe", false)
		setMockClient(t, successRoutes)
		resetValidatorState(a)
		if got := a.CheckStripe(key, "http://example.com"); got != false {
			t.Error("expected false when gate is off")
		}
	})

	t.Run("valid_key", func(t *testing.T) {
		a := validatorScanner(t)
		setMockClient(t, successRoutes)
		resetValidatorState(a)
		if got := a.CheckStripe(key, "http://example.com"); got != true {
			t.Error("expected true for valid key")
		}
		assertValidated(t, 1)
	})

	t.Run("invalid_key", func(t *testing.T) {
		a := validatorScanner(t)
		setMockClient(t, []mockRoute{{"api.stripe.com", 401, `{"error":{"type":"invalid_request_error"}}`}})
		resetValidatorState(a)
		if got := a.CheckStripe(key+"x", "http://example.com"); got != false {
			t.Error("expected false for 401")
		}
		assertValidated(t, 0)
	})
}

// ---------------------------------------------------------------------------
// TestValidator_Mailgun
// ---------------------------------------------------------------------------

func TestValidator_Mailgun(t *testing.T) {
	chdirTemp(t)
	const key = "key-abcdef1234567890abcdef1234567890"
	successRoutes := []mockRoute{
		{"api.mailgun.net", 200, `{"total_count":1,"items":[{"name":"mg.example.com"}]}`},
	}

	t.Run("gate_off", func(t *testing.T) {
		a := scannerWithSingleFeature(t, "mailgun", false)
		setMockClient(t, successRoutes)
		resetValidatorState(a)
		if got := a.CheckMailgun(key, "http://example.com"); got != false {
			t.Error("expected false when gate is off")
		}
	})

	t.Run("valid_key", func(t *testing.T) {
		a := validatorScanner(t)
		setMockClient(t, successRoutes)
		resetValidatorState(a)
		if got := a.CheckMailgun(key, "http://example.com"); got != true {
			t.Error("expected true for valid key")
		}
		assertValidated(t, 1)
	})

	t.Run("invalid_key", func(t *testing.T) {
		a := validatorScanner(t)
		setMockClient(t, []mockRoute{{"api.mailgun.net", 401, `{"message":"Forbidden"}`}})
		resetValidatorState(a)
		if got := a.CheckMailgun(key+"x", "http://example.com"); got != false {
			t.Error("expected false for 401")
		}
		assertValidated(t, 0)
	})
}

// ---------------------------------------------------------------------------
// TestValidator_Telnyx
// ---------------------------------------------------------------------------

func TestValidator_Telnyx(t *testing.T) {
	chdirTemp(t)
	const key = "KEY017c1e20f0e14d7c977a5a64a6c79f02_SomePaddingABCD"
	successRoutes := []mockRoute{
		{"api.telnyx.com", 200, `{"data":{"balance":"25.00","currency":"USD"}}`},
	}

	t.Run("gate_off", func(t *testing.T) {
		a := scannerWithSingleFeature(t, "telnyx", false)
		setMockClient(t, successRoutes)
		resetValidatorState(a)
		if got := a.CheckTelnyx(key, "http://example.com"); got != false {
			t.Error("expected false when gate is off")
		}
	})

	t.Run("valid_key", func(t *testing.T) {
		a := validatorScanner(t)
		setMockClient(t, successRoutes)
		resetValidatorState(a)
		if got := a.CheckTelnyx(key, "http://example.com"); got != true {
			t.Error("expected true for valid key")
		}
		assertValidated(t, 1)
	})

	t.Run("invalid_key", func(t *testing.T) {
		a := validatorScanner(t)
		setMockClient(t, []mockRoute{{"api.telnyx.com", 401, `{"errors":[]}`}})
		resetValidatorState(a)
		if got := a.CheckTelnyx(key+"x", "http://example.com"); got != false {
			t.Error("expected false for 401")
		}
		assertValidated(t, 0)
	})
}

// ---------------------------------------------------------------------------
// TestValidator_MessageBird
// ---------------------------------------------------------------------------

func TestValidator_MessageBird(t *testing.T) {
	chdirTemp(t)
	const key = "live_abcdefghijklmnopqrstuvwxy"
	successRoutes := []mockRoute{
		{"rest.messagebird.com", 200, `{"amount":10.5,"currency":"EUR","type":"credits"}`},
	}

	t.Run("gate_off", func(t *testing.T) {
		a := scannerWithSingleFeature(t, "messagebird", false)
		setMockClient(t, successRoutes)
		resetValidatorState(a)
		if got := a.CheckMessageBird(key, "http://example.com"); got != false {
			t.Error("expected false when gate is off")
		}
	})

	t.Run("valid_key", func(t *testing.T) {
		a := validatorScanner(t)
		setMockClient(t, successRoutes)
		resetValidatorState(a)
		if got := a.CheckMessageBird(key, "http://example.com"); got != true {
			t.Error("expected true for valid key")
		}
		assertValidated(t, 1)
	})

	t.Run("invalid_key", func(t *testing.T) {
		a := validatorScanner(t)
		setMockClient(t, []mockRoute{{"rest.messagebird.com", 401, `{"errors":[{"code":2,"description":"Request not allowed"}]}`}})
		resetValidatorState(a)
		if got := a.CheckMessageBird(key+"x", "http://example.com"); got != false {
			t.Error("expected false for 401")
		}
		assertValidated(t, 0)
	})
}

// ---------------------------------------------------------------------------
// TestValidator_Brevo
// ---------------------------------------------------------------------------

func TestValidator_Brevo(t *testing.T) {
	chdirTemp(t)
	const key = "xkeysib-abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ1234-abcdefghijklmnop"
	// Route all brevo requests to 200; the Send function needs v3/smtp/email
	successRoutes := []mockRoute{
		{"api.brevo.com/v3/smtp/email", 201, `{"messageId":"<test@brevo>"}`},
		{"api.brevo.com", 200, `{"email":"test@example.com","companyName":"TestCo"}`},
	}

	t.Run("gate_off", func(t *testing.T) {
		a := scannerWithSingleFeature(t, "brevo", false)
		setMockClient(t, successRoutes)
		resetValidatorState(a)
		if got := a.CheckBrevo(key, "http://example.com"); got != false {
			t.Error("expected false when gate is off")
		}
	})

	t.Run("valid_key", func(t *testing.T) {
		a := validatorScanner(t)
		setMockClient(t, successRoutes)
		resetValidatorState(a)
		if got := a.CheckBrevo(key, "http://example.com"); got != true {
			t.Error("expected true for valid key")
		}
		assertValidated(t, 1)
	})

	t.Run("invalid_key", func(t *testing.T) {
		a := validatorScanner(t)
		setMockClient(t, []mockRoute{{"api.brevo.com", 401, `{"message":"Key not found"}`}})
		resetValidatorState(a)
		if got := a.CheckBrevo(key+"x", "http://example.com"); got != false {
			t.Error("expected false for 401")
		}
		assertValidated(t, 0)
	})
}

// ---------------------------------------------------------------------------
// TestValidator_XSMTP
// ---------------------------------------------------------------------------

func TestValidator_XSMTP(t *testing.T) {
	chdirTemp(t)
	const key = "xsmtp-test-key-12345678"
	successRoutes := []mockRoute{
		{"api.xsmtp.com", 200, `{"status":"active"}`},
	}

	t.Run("gate_off", func(t *testing.T) {
		a := scannerWithSingleFeature(t, "xsmtp", false)
		setMockClient(t, successRoutes)
		resetValidatorState(a)
		if got := a.CheckXSMTP(key, "http://example.com"); got != false {
			t.Error("expected false when gate is off")
		}
	})

	t.Run("valid_key", func(t *testing.T) {
		a := validatorScanner(t)
		setMockClient(t, successRoutes)
		resetValidatorState(a)
		if got := a.CheckXSMTP(key, "http://example.com"); got != true {
			t.Error("expected true for valid key")
		}
		assertValidated(t, 1)
	})

	t.Run("invalid_key", func(t *testing.T) {
		a := validatorScanner(t)
		setMockClient(t, []mockRoute{{"api.xsmtp.com", 401, `{"error":"invalid key"}`}})
		resetValidatorState(a)
		if got := a.CheckXSMTP(key+"x", "http://example.com"); got != false {
			t.Error("expected false for 401")
		}
		assertValidated(t, 0)
	})
}

// ---------------------------------------------------------------------------
// TestValidator_Mandrill
// ---------------------------------------------------------------------------

func TestValidator_Mandrill(t *testing.T) {
	chdirTemp(t)
	const key = "md-ABCDEFabcdef0123456789"
	// Route all mandrillapp.com to 200; the Send function needs messages/send.json
	successRoutes := []mockRoute{
		{"mandrillapp.com", 200, `{"username":"testuser"}`},
	}

	t.Run("gate_off", func(t *testing.T) {
		a := scannerWithSingleFeature(t, "mandrill", false)
		setMockClient(t, successRoutes)
		resetValidatorState(a)
		if got := a.CheckMandrill(key, "http://example.com"); got != false {
			t.Error("expected false when gate is off")
		}
	})

	t.Run("valid_key", func(t *testing.T) {
		a := validatorScanner(t)
		setMockClient(t, successRoutes)
		resetValidatorState(a)
		if got := a.CheckMandrill(key, "http://example.com"); got != true {
			t.Error("expected true for valid key")
		}
		assertValidated(t, 1)
	})

	t.Run("invalid_key", func(t *testing.T) {
		a := validatorScanner(t)
		setMockClient(t, []mockRoute{{"mandrillapp.com", 401, `{"status":"error","code":-1}`}})
		resetValidatorState(a)
		if got := a.CheckMandrill(key+"x", "http://example.com"); got != false {
			t.Error("expected false for 401")
		}
		assertValidated(t, 0)
	})
}

// ---------------------------------------------------------------------------
// TestValidator_MailerSend
// ---------------------------------------------------------------------------

func TestValidator_MailerSend(t *testing.T) {
	chdirTemp(t)
	const key = "mlsn.abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ1234567890ab"
	// Route all api.mailersend.com: domains → 200, email → 202
	successRoutes := []mockRoute{
		{"api.mailersend.com/v1/email", 202, `{}`},
		{"api.mailersend.com", 200, `{"data":[{"name":"example.com"}]}`},
	}

	t.Run("gate_off", func(t *testing.T) {
		a := scannerWithSingleFeature(t, "mailersend", false)
		setMockClient(t, successRoutes)
		resetValidatorState(a)
		if got := a.CheckMailerSend(key, "http://example.com"); got != false {
			t.Error("expected false when gate is off")
		}
	})

	t.Run("valid_key", func(t *testing.T) {
		a := validatorScanner(t)
		setMockClient(t, successRoutes)
		resetValidatorState(a)
		if got := a.CheckMailerSend(key, "http://example.com"); got != true {
			t.Error("expected true for valid key")
		}
		assertValidated(t, 1)
	})

	t.Run("invalid_key", func(t *testing.T) {
		a := validatorScanner(t)
		setMockClient(t, []mockRoute{{"api.mailersend.com", 401, `{"message":"Unauthenticated."}`}})
		resetValidatorState(a)
		if got := a.CheckMailerSend(key+"x", "http://example.com"); got != false {
			t.Error("expected false for 401")
		}
		assertValidated(t, 0)
	})
}

// ---------------------------------------------------------------------------
// TestValidator_Nexmo
// ---------------------------------------------------------------------------

func TestValidator_Nexmo(t *testing.T) {
	chdirTemp(t)
	const (
		nexmoKey    = "a1b2c3d4"
		nexmoSecret = "a1b2c3d4e5f6a1b2"
	)
	successRoutes := []mockRoute{
		{"rest.nexmo.com", 200, `{"value":14.50}`},
	}

	t.Run("gate_off", func(t *testing.T) {
		a := scannerWithSingleFeature(t, "nexmo", false)
		setMockClient(t, successRoutes)
		resetValidatorState(a)
		if got := a.CheckNexmo(nexmoKey, nexmoSecret, "http://example.com"); got != false {
			t.Error("expected false when gate is off")
		}
	})

	t.Run("valid_key", func(t *testing.T) {
		a := validatorScanner(t)
		setMockClient(t, successRoutes)
		resetValidatorState(a)
		if got := a.CheckNexmo(nexmoKey, nexmoSecret, "http://example.com"); got != true {
			t.Error("expected true for valid key/secret")
		}
		assertValidated(t, 1)
	})

	t.Run("invalid_key", func(t *testing.T) {
		a := validatorScanner(t)
		setMockClient(t, []mockRoute{{"rest.nexmo.com", 401, `{"error-code":"401","error-code-label":"authentication failed"}`}})
		resetValidatorState(a)
		if got := a.CheckNexmo(nexmoKey+"x", nexmoSecret, "http://example.com"); got != false {
			t.Error("expected false for 401")
		}
		assertValidated(t, 0)
	})
}

// ---------------------------------------------------------------------------
// TestValidator_GitHubToken
// ---------------------------------------------------------------------------

func TestValidator_GitHubToken(t *testing.T) {
	chdirTemp(t)
	const key = "ghp_ABCDEFGHIJKLMNOPQRSTUVWXYZabcdef1234"
	successRoutes := []mockRoute{
		{"api.github.com", 200, `{"login":"testuser","id":12345}`},
	}

	t.Run("gate_off", func(t *testing.T) {
		a := scannerWithSingleFeature(t, "github", false)
		setMockClient(t, successRoutes)
		resetValidatorState(a)
		if got := a.CheckGitHubToken(key, "http://example.com"); got != false {
			t.Error("expected false when gate is off")
		}
	})

	t.Run("valid_key", func(t *testing.T) {
		a := validatorScanner(t)
		setMockClient(t, successRoutes)
		resetValidatorState(a)
		if got := a.CheckGitHubToken(key, "http://example.com"); got != true {
			t.Error("expected true for valid token")
		}
		assertValidated(t, 1)
	})

	t.Run("invalid_key", func(t *testing.T) {
		a := validatorScanner(t)
		setMockClient(t, []mockRoute{{"api.github.com", 401, `{"message":"Bad credentials"}`}})
		resetValidatorState(a)
		if got := a.CheckGitHubToken(key+"x", "http://example.com"); got != false {
			t.Error("expected false for 401")
		}
		assertValidated(t, 0)
	})
}

// ---------------------------------------------------------------------------
// TestValidator_GCPKey
// ---------------------------------------------------------------------------

func TestValidator_GCPKey(t *testing.T) {
	chdirTemp(t)
	const key = "AIzaSyABCDEFGHIJKLMNOPQRSTUVWXYZ0123456"
	validBody := `{"status":"ZERO_RESULTS","results":[]}`
	invalidBody := `{"status":"INVALID_KEY"}`

	t.Run("gate_off", func(t *testing.T) {
		a := scannerWithSingleFeature(t, "gcp_api_key", false)
		setMockClient(t, []mockRoute{{"maps.googleapis.com", 200, validBody}})
		resetValidatorState(a)
		if got := a.CheckGCPKey(key, "http://example.com"); got != false {
			t.Error("expected false when gate is off")
		}
	})

	t.Run("valid_key", func(t *testing.T) {
		a := validatorScanner(t)
		setMockClient(t, []mockRoute{{"maps.googleapis.com", 200, validBody}})
		resetValidatorState(a)
		if got := a.CheckGCPKey(key, "http://example.com"); got != true {
			t.Error("expected true for ZERO_RESULTS status")
		}
		assertValidated(t, 1)
	})

	t.Run("invalid_key", func(t *testing.T) {
		a := validatorScanner(t)
		setMockClient(t, []mockRoute{{"maps.googleapis.com", 401, `{"error":"invalid"}`}})
		resetValidatorState(a)
		if got := a.CheckGCPKey(key+"x", "http://example.com"); got != false {
			t.Error("expected false for 401")
		}
		assertValidated(t, 0)
	})

	t.Run("valid_status_200_wrong_body", func(t *testing.T) {
		// INVALID_KEY status in body should return false even on HTTP 200
		a := validatorScanner(t)
		setMockClient(t, []mockRoute{{"maps.googleapis.com", 200, invalidBody}})
		resetValidatorState(a)
		// Need a fresh key to avoid KnownKeys dedup from previous subtests
		if got := a.CheckGCPKey(key+"diff", "http://example.com"); got != false {
			t.Error("expected false when status==INVALID_KEY in body")
		}
		assertValidated(t, 0)
	})
}

// ---------------------------------------------------------------------------
// TestValidator_CryptoWallet
// ---------------------------------------------------------------------------

func TestValidator_CryptoWallet(t *testing.T) {
	chdirTemp(t)
	const validKey = "1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef"

	t.Run("gate_off", func(t *testing.T) {
		a := scannerWithSingleFeature(t, "crypto_wallet", false)
		resetValidatorState(a)
		if got := a.CheckCryptoWallet(validKey, "http://example.com"); got != false {
			t.Error("expected false when gate is off")
		}
	})

	t.Run("valid_key", func(t *testing.T) {
		a := validatorScanner(t)
		resetValidatorState(a)
		if got := a.CheckCryptoWallet(validKey, "http://example.com"); got != true {
			t.Error("expected true for valid 64-char hex key")
		}
		assertValidated(t, 1)
	})

	t.Run("invalid_key_all_zeros", func(t *testing.T) {
		a := validatorScanner(t)
		resetValidatorState(a)
		if got := a.CheckCryptoWallet(strings.Repeat("0", 64), "http://example.com"); got != false {
			t.Error("expected false for all-zero key")
		}
		assertValidated(t, 0)
	})

	t.Run("invalid_key_all_ff", func(t *testing.T) {
		a := validatorScanner(t)
		resetValidatorState(a)
		if got := a.CheckCryptoWallet(strings.Repeat("f", 64), "http://example.com"); got != false {
			t.Error("expected false for all-ff key")
		}
		assertValidated(t, 0)
	})

	t.Run("invalid_key_too_short", func(t *testing.T) {
		a := validatorScanner(t)
		resetValidatorState(a)
		if got := a.CheckCryptoWallet(strings.Repeat("a", 63), "http://example.com"); got != false {
			t.Error("expected false for 63-char key")
		}
		assertValidated(t, 0)
	})

	t.Run("invalid_key_too_long", func(t *testing.T) {
		a := validatorScanner(t)
		resetValidatorState(a)
		if got := a.CheckCryptoWallet(strings.Repeat("b", 65), "http://example.com"); got != false {
			t.Error("expected false for 65-char key")
		}
		assertValidated(t, 0)
	})
}

// ---------------------------------------------------------------------------
// TestValidator_Slack
// ---------------------------------------------------------------------------

func TestValidator_Slack(t *testing.T) {
	chdirTemp(t)
	const key = "xoxb-test-token-abcdefghijklmnopqrstuvwxyz"
	successBody := `{"ok":true,"team":"TestTeam","user":"slackbot"}`
	failBody := `{"ok":false,"error":"invalid_auth"}`

	t.Run("gate_off", func(t *testing.T) {
		a := scannerWithSingleFeature(t, "slack", false)
		setMockClient(t, []mockRoute{{"slack.com", 200, successBody}})
		resetValidatorState(a)
		if got := a.CheckSlack(key, "http://example.com"); got != false {
			t.Error("expected false when gate is off")
		}
	})

	t.Run("valid_key", func(t *testing.T) {
		a := validatorScanner(t)
		setMockClient(t, []mockRoute{{"slack.com", 200, successBody}})
		resetValidatorState(a)
		if got := a.CheckSlack(key, "http://example.com"); got != true {
			t.Error("expected true for ok:true response")
		}
		assertValidated(t, 1)
	})

	t.Run("invalid_key", func(t *testing.T) {
		a := validatorScanner(t)
		setMockClient(t, []mockRoute{{"slack.com", 401, `{"ok":false}`}})
		resetValidatorState(a)
		if got := a.CheckSlack(key+"x", "http://example.com"); got != false {
			t.Error("expected false for 401")
		}
		assertValidated(t, 0)
	})

	t.Run("valid_status_200_wrong_body", func(t *testing.T) {
		// HTTP 200 but ok:false — should return false
		a := validatorScanner(t)
		setMockClient(t, []mockRoute{{"slack.com", 200, failBody}})
		resetValidatorState(a)
		if got := a.CheckSlack(key+"diff2", "http://example.com"); got != false {
			t.Error("expected false when ok:false in body")
		}
		assertValidated(t, 0)
	})
}

// ---------------------------------------------------------------------------
// TestValidator_Discord
// ---------------------------------------------------------------------------

func TestValidator_Discord(t *testing.T) {
	chdirTemp(t)
	const key = "MTIzNDU2Nzg5MDEyMzQ1Njc4.GTestToken.abcdefghijklmnopqrstuvwxyz"
	successBody := `{"id":"123456789","username":"TestBot"}`
	failBody := `{"id":"","username":""}`

	t.Run("gate_off", func(t *testing.T) {
		a := scannerWithSingleFeature(t, "discord", false)
		setMockClient(t, []mockRoute{{"discord.com", 200, successBody}})
		resetValidatorState(a)
		if got := a.CheckDiscord(key, "http://example.com"); got != false {
			t.Error("expected false when gate is off")
		}
	})

	t.Run("valid_key", func(t *testing.T) {
		a := validatorScanner(t)
		setMockClient(t, []mockRoute{{"discord.com", 200, successBody}})
		resetValidatorState(a)
		if got := a.CheckDiscord(key, "http://example.com"); got != true {
			t.Error("expected true for non-empty id in response")
		}
		assertValidated(t, 1)
	})

	t.Run("invalid_key", func(t *testing.T) {
		a := validatorScanner(t)
		setMockClient(t, []mockRoute{{"discord.com", 401, `{"message":"401: Unauthorized"}`}})
		resetValidatorState(a)
		if got := a.CheckDiscord(key+"x", "http://example.com"); got != false {
			t.Error("expected false for 401")
		}
		assertValidated(t, 0)
	})

	t.Run("valid_status_200_wrong_body", func(t *testing.T) {
		// HTTP 200 but id is empty — should return false
		a := validatorScanner(t)
		setMockClient(t, []mockRoute{{"discord.com", 200, failBody}})
		resetValidatorState(a)
		if got := a.CheckDiscord(key+"diff2", "http://example.com"); got != false {
			t.Error("expected false when id is empty in body")
		}
		assertValidated(t, 0)
	})
}

// ---------------------------------------------------------------------------
// TestValidator_Cloudflare
// ---------------------------------------------------------------------------

func TestValidator_Cloudflare(t *testing.T) {
	chdirTemp(t)
	const key = "cloudflare-test-token-abcdefghijklmnopqrstuvwxyz"
	successBody := `{"result":{"status":"active","id":"abc123"}}`
	failBody := `{"result":{"status":"disabled"}}`

	t.Run("gate_off", func(t *testing.T) {
		a := scannerWithSingleFeature(t, "cloudflare", false)
		setMockClient(t, []mockRoute{{"api.cloudflare.com", 200, successBody}})
		resetValidatorState(a)
		if got := a.CheckCloudflare(key, "http://example.com"); got != false {
			t.Error("expected false when gate is off")
		}
	})

	t.Run("valid_key", func(t *testing.T) {
		a := validatorScanner(t)
		setMockClient(t, []mockRoute{{"api.cloudflare.com", 200, successBody}})
		resetValidatorState(a)
		if got := a.CheckCloudflare(key, "http://example.com"); got != true {
			t.Error("expected true for active status")
		}
		assertValidated(t, 1)
	})

	t.Run("invalid_key", func(t *testing.T) {
		a := validatorScanner(t)
		setMockClient(t, []mockRoute{{"api.cloudflare.com", 401, `{"errors":[{"code":1000}]}`}})
		resetValidatorState(a)
		if got := a.CheckCloudflare(key+"x", "http://example.com"); got != false {
			t.Error("expected false for 401")
		}
		assertValidated(t, 0)
	})

	t.Run("valid_status_200_wrong_body", func(t *testing.T) {
		// HTTP 200 but status != "active"
		a := validatorScanner(t)
		setMockClient(t, []mockRoute{{"api.cloudflare.com", 200, failBody}})
		resetValidatorState(a)
		if got := a.CheckCloudflare(key+"diff2", "http://example.com"); got != false {
			t.Error("expected false when status!=active in body")
		}
		assertValidated(t, 0)
	})
}

// ---------------------------------------------------------------------------
// TestValidator_DigitalOcean
// ---------------------------------------------------------------------------

func TestValidator_DigitalOcean(t *testing.T) {
	chdirTemp(t)
	key := "dop_v1_" + strings.Repeat("a", 64)
	successRoutes := []mockRoute{
		{"api.digitalocean.com", 200, `{"account":{"email":"user@example.com","status":"active"}}`},
	}

	t.Run("gate_off", func(t *testing.T) {
		a := scannerWithSingleFeature(t, "digitalocean", false)
		setMockClient(t, successRoutes)
		resetValidatorState(a)
		if got := a.CheckDigitalOcean(key, "http://example.com"); got != false {
			t.Error("expected false when gate is off")
		}
	})

	t.Run("valid_key", func(t *testing.T) {
		a := validatorScanner(t)
		setMockClient(t, successRoutes)
		resetValidatorState(a)
		if got := a.CheckDigitalOcean(key, "http://example.com"); got != true {
			t.Error("expected true for valid token")
		}
		assertValidated(t, 1)
	})

	t.Run("invalid_key", func(t *testing.T) {
		a := validatorScanner(t)
		setMockClient(t, []mockRoute{{"api.digitalocean.com", 401, `{"id":"unauthorized"}`}})
		resetValidatorState(a)
		if got := a.CheckDigitalOcean(key+"x", "http://example.com"); got != false {
			t.Error("expected false for 401")
		}
		assertValidated(t, 0)
	})
}

// ---------------------------------------------------------------------------
// TestValidator_Shopify
// ---------------------------------------------------------------------------

func TestValidator_Shopify(t *testing.T) {
	chdirTemp(t)
	const key = "shppa_abcdef1234567890abcdef1234567890"

	t.Run("gate_off", func(t *testing.T) {
		a := scannerWithSingleFeature(t, "shopify", false)
		resetValidatorState(a)
		if got := a.CheckShopify(key, "http://example.com"); got != false {
			t.Error("expected false when gate is off")
		}
		assertFoundTotal(t, 0)
	})

	t.Run("always_false_but_counts_found", func(t *testing.T) {
		// Shopify never validates — always returns false but increments APIsFoundTotal
		a := validatorScanner(t)
		resetValidatorState(a)
		if got := a.CheckShopify(key, "http://example.com"); got != false {
			t.Error("CheckShopify must always return false (record-only)")
		}
		assertFoundTotal(t, 1)
		assertValidated(t, 0)
	})
}

// ---------------------------------------------------------------------------
// TestValidator_HubSpot
// ---------------------------------------------------------------------------

func TestValidator_HubSpot(t *testing.T) {
	chdirTemp(t)
	const key = "pat-na1-12345678-1234-1234-1234-123456789012"
	successRoutes := []mockRoute{
		{"api.hubapi.com", 200, `{"results":[]}`},
	}

	t.Run("gate_off", func(t *testing.T) {
		a := scannerWithSingleFeature(t, "hubspot", false)
		setMockClient(t, successRoutes)
		resetValidatorState(a)
		if got := a.CheckHubSpot(key, "http://example.com"); got != false {
			t.Error("expected false when gate is off")
		}
	})

	t.Run("valid_key", func(t *testing.T) {
		a := validatorScanner(t)
		setMockClient(t, successRoutes)
		resetValidatorState(a)
		if got := a.CheckHubSpot(key, "http://example.com"); got != true {
			t.Error("expected true for valid token")
		}
		assertValidated(t, 1)
	})

	t.Run("invalid_key", func(t *testing.T) {
		a := validatorScanner(t)
		setMockClient(t, []mockRoute{{"api.hubapi.com", 401, `{"status":"error","message":"Invalid auth"}`}})
		resetValidatorState(a)
		if got := a.CheckHubSpot(key+"x", "http://example.com"); got != false {
			t.Error("expected false for 401")
		}
		assertValidated(t, 0)
	})
}

// ---------------------------------------------------------------------------
// TestValidator_Heroku
// ---------------------------------------------------------------------------

func TestValidator_Heroku(t *testing.T) {
	chdirTemp(t)
	const key = "12345678-1234-1234-1234-123456789012"
	successRoutes := []mockRoute{
		{"api.heroku.com", 200, `{"email":"user@heroku.com","id":"abc123"}`},
	}

	t.Run("gate_off", func(t *testing.T) {
		a := scannerWithSingleFeature(t, "heroku", false)
		setMockClient(t, successRoutes)
		resetValidatorState(a)
		if got := a.CheckHeroku(key, "http://example.com"); got != false {
			t.Error("expected false when gate is off")
		}
	})

	t.Run("valid_key", func(t *testing.T) {
		a := validatorScanner(t)
		setMockClient(t, successRoutes)
		resetValidatorState(a)
		if got := a.CheckHeroku(key, "http://example.com"); got != true {
			t.Error("expected true for valid key")
		}
		assertValidated(t, 1)
	})

	t.Run("invalid_key", func(t *testing.T) {
		a := validatorScanner(t)
		setMockClient(t, []mockRoute{{"api.heroku.com", 401, `{"id":"unauthorized"}`}})
		resetValidatorState(a)
		if got := a.CheckHeroku(key+"x", "http://example.com"); got != false {
			t.Error("expected false for 401")
		}
		assertValidated(t, 0)
	})
}

// ---------------------------------------------------------------------------
// TestValidator_Datadog
// ---------------------------------------------------------------------------

func TestValidator_Datadog(t *testing.T) {
	chdirTemp(t)
	const key = "abcdef1234567890abcdef1234567890"
	successBody := `{"valid":true}`
	failBody := `{"valid":false}`

	t.Run("gate_off", func(t *testing.T) {
		a := scannerWithSingleFeature(t, "datadog", false)
		setMockClient(t, []mockRoute{{"api.datadoghq.com", 200, successBody}})
		resetValidatorState(a)
		if got := a.CheckDatadog(key, "http://example.com"); got != false {
			t.Error("expected false when gate is off")
		}
	})

	t.Run("valid_key", func(t *testing.T) {
		a := validatorScanner(t)
		setMockClient(t, []mockRoute{{"api.datadoghq.com", 200, successBody}})
		resetValidatorState(a)
		if got := a.CheckDatadog(key, "http://example.com"); got != true {
			t.Error("expected true for valid:true response")
		}
		assertValidated(t, 1)
	})

	t.Run("invalid_key", func(t *testing.T) {
		a := validatorScanner(t)
		setMockClient(t, []mockRoute{{"api.datadoghq.com", 403, `{"errors":["Forbidden"]}`}})
		resetValidatorState(a)
		if got := a.CheckDatadog(key+"x", "http://example.com"); got != false {
			t.Error("expected false for 403")
		}
		assertValidated(t, 0)
	})

	t.Run("valid_status_200_wrong_body", func(t *testing.T) {
		// HTTP 200 but valid:false in body
		a := validatorScanner(t)
		setMockClient(t, []mockRoute{{"api.datadoghq.com", 200, failBody}})
		resetValidatorState(a)
		if got := a.CheckDatadog(key+"diff2", "http://example.com"); got != false {
			t.Error("expected false when valid:false in body")
		}
		assertValidated(t, 0)
	})
}

// ---------------------------------------------------------------------------
// TestValidator_Postmark
// ---------------------------------------------------------------------------

func TestValidator_Postmark(t *testing.T) {
	chdirTemp(t)
	const key = "a1b2c3d4-e5f6-a1b2-c3d4-e5f6a1b2c3d4"
	successRoutes := []mockRoute{
		{"api.postmarkapp.com", 200, `{"Name":"Test Server"}`},
	}

	t.Run("gate_off", func(t *testing.T) {
		a := scannerWithSingleFeature(t, "postmark", false)
		setMockClient(t, successRoutes)
		resetValidatorState(a)
		if got := a.CheckPostmark(key, "http://example.com"); got != false {
			t.Error("expected false when gate is off")
		}
	})

	t.Run("valid_key", func(t *testing.T) {
		a := validatorScanner(t)
		setMockClient(t, successRoutes)
		resetValidatorState(a)
		if got := a.CheckPostmark(key, "http://example.com"); got != true {
			t.Error("expected true for valid token")
		}
		assertValidated(t, 1)
	})

	t.Run("invalid_key", func(t *testing.T) {
		a := validatorScanner(t)
		setMockClient(t, []mockRoute{{"api.postmarkapp.com", 401, `{"ErrorCode":10,"Message":"Invalid API token"}`}})
		resetValidatorState(a)
		if got := a.CheckPostmark(key+"x", "http://example.com"); got != false {
			t.Error("expected false for 401")
		}
		assertValidated(t, 0)
	})
}

// ---------------------------------------------------------------------------
// TestValidator_SparkPost
// ---------------------------------------------------------------------------

func TestValidator_SparkPost(t *testing.T) {
	chdirTemp(t)
	const key = "abcdef1234567890abcdef1234567890abcdef12"
	successRoutes := []mockRoute{
		{"api.sparkpost.com", 200, `{"results":{"company_id":1}}`},
	}

	t.Run("gate_off", func(t *testing.T) {
		a := scannerWithSingleFeature(t, "sparkpost", false)
		setMockClient(t, successRoutes)
		resetValidatorState(a)
		if got := a.CheckSparkPost(key, "http://example.com"); got != false {
			t.Error("expected false when gate is off")
		}
	})

	t.Run("valid_key", func(t *testing.T) {
		a := validatorScanner(t)
		setMockClient(t, successRoutes)
		resetValidatorState(a)
		if got := a.CheckSparkPost(key, "http://example.com"); got != true {
			t.Error("expected true for valid key")
		}
		assertValidated(t, 1)
	})

	t.Run("invalid_key", func(t *testing.T) {
		a := validatorScanner(t)
		setMockClient(t, []mockRoute{{"api.sparkpost.com", 401, `{"errors":[{"message":"Unauthorized"}]}`}})
		resetValidatorState(a)
		if got := a.CheckSparkPost(key+"x", "http://example.com"); got != false {
			t.Error("expected false for 401")
		}
		assertValidated(t, 0)
	})
}

// ---------------------------------------------------------------------------
// TestValidator_Mailtrap
// ---------------------------------------------------------------------------

func TestValidator_Mailtrap(t *testing.T) {
	chdirTemp(t)
	const key = "abcdef1234567890abcdef1234567890"
	successRoutes := []mockRoute{
		{"mailtrap.io", 200, `[{"id":1,"name":"Test Account"}]`},
	}

	t.Run("gate_off", func(t *testing.T) {
		a := scannerWithSingleFeature(t, "mailtrap", false)
		setMockClient(t, successRoutes)
		resetValidatorState(a)
		if got := a.CheckMailtrap(key, "http://example.com"); got != false {
			t.Error("expected false when gate is off")
		}
	})

	t.Run("valid_key", func(t *testing.T) {
		a := validatorScanner(t)
		setMockClient(t, successRoutes)
		resetValidatorState(a)
		if got := a.CheckMailtrap(key, "http://example.com"); got != true {
			t.Error("expected true for valid token")
		}
		assertValidated(t, 1)
	})

	t.Run("invalid_key", func(t *testing.T) {
		a := validatorScanner(t)
		setMockClient(t, []mockRoute{{"mailtrap.io", 401, `{"error":"Unauthorized"}`}})
		resetValidatorState(a)
		if got := a.CheckMailtrap(key+"x", "http://example.com"); got != false {
			t.Error("expected false for 401")
		}
		assertValidated(t, 0)
	})
}

// ---------------------------------------------------------------------------
// TestValidator_Mailjet
// ---------------------------------------------------------------------------

func TestValidator_Mailjet(t *testing.T) {
	chdirTemp(t)
	const key = "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4"
	successRoutes := []mockRoute{
		{"api.mailjet.com", 200, `{"Count":1,"Data":[],"Total":1}`},
	}

	t.Run("gate_off", func(t *testing.T) {
		a := scannerWithSingleFeature(t, "mailjet", false)
		setMockClient(t, successRoutes)
		resetValidatorState(a)
		if got := a.CheckMailjet(key, "http://example.com"); got != false {
			t.Error("expected false when gate is off")
		}
	})

	t.Run("valid_key", func(t *testing.T) {
		a := validatorScanner(t)
		setMockClient(t, successRoutes)
		resetValidatorState(a)
		if got := a.CheckMailjet(key, "http://example.com"); got != true {
			t.Error("expected true for valid key")
		}
		assertValidated(t, 1)
	})

	t.Run("invalid_key", func(t *testing.T) {
		a := validatorScanner(t)
		setMockClient(t, []mockRoute{{"api.mailjet.com", 401, `{"StatusCode":401,"ErrorMessage":"API key not found"}`}})
		resetValidatorState(a)
		if got := a.CheckMailjet(key+"x", "http://example.com"); got != false {
			t.Error("expected false for 401")
		}
		assertValidated(t, 0)
	})
}

// ---------------------------------------------------------------------------
// TestValidator_Plivo
// ---------------------------------------------------------------------------

func TestValidator_Plivo(t *testing.T) {
	chdirTemp(t)
	const key = "MAa1b2c3d4e5f6a1b2c3"
	successRoutes := []mockRoute{
		{"api.plivo.com", 200, `{"account_type":"standard","auth_id":"MAa1b2c3d4e5f6a1b2c3"}`},
	}

	t.Run("gate_off", func(t *testing.T) {
		a := scannerWithSingleFeature(t, "plivo", false)
		setMockClient(t, successRoutes)
		resetValidatorState(a)
		if got := a.CheckPlivo(key, "http://example.com"); got != false {
			t.Error("expected false when gate is off")
		}
	})

	t.Run("valid_key", func(t *testing.T) {
		a := validatorScanner(t)
		setMockClient(t, successRoutes)
		resetValidatorState(a)
		if got := a.CheckPlivo(key, "http://example.com"); got != true {
			t.Error("expected true for valid auth ID")
		}
		assertValidated(t, 1)
	})

	t.Run("invalid_key", func(t *testing.T) {
		a := validatorScanner(t)
		setMockClient(t, []mockRoute{{"api.plivo.com", 401, `{"error":"Authentication credentials were not provided."}`}})
		resetValidatorState(a)
		// Use a different key to avoid KnownKeys dedup
		if got := a.CheckPlivo("MAa1b2c3d4e5f6a1b2c4", "http://example.com"); got != false {
			t.Error("expected false for 401")
		}
		assertValidated(t, 0)
	})
}

// ---------------------------------------------------------------------------
// TestValidator_Tencent
// ---------------------------------------------------------------------------

func TestValidator_Tencent(t *testing.T) {
	chdirTemp(t)
	const key = "AKID1234567890abcdef1234567890abcdef"
	successBody := `{"Response":{"InstanceSet":[]}}`
	failBody := `{"Response":{"Error":{"Code":"AuthFailure.InvalidSecretKey","Message":"The SecretKey is invalid."}}}`

	t.Run("gate_off", func(t *testing.T) {
		a := scannerWithSingleFeature(t, "tencent", false)
		setMockClient(t, []mockRoute{{"cvm.tencentcloudapi.com", 200, successBody}})
		resetValidatorState(a)
		if got := a.CheckTencent(key, "http://example.com"); got != false {
			t.Error("expected false when gate is off")
		}
	})

	t.Run("valid_key", func(t *testing.T) {
		// 200 with no Error field in Response → valid
		a := validatorScanner(t)
		setMockClient(t, []mockRoute{{"cvm.tencentcloudapi.com", 200, successBody}})
		resetValidatorState(a)
		if got := a.CheckTencent(key, "http://example.com"); got != true {
			t.Error("expected true for 200 without Error field")
		}
		assertValidated(t, 1)
	})

	t.Run("invalid_key", func(t *testing.T) {
		a := validatorScanner(t)
		setMockClient(t, []mockRoute{{"cvm.tencentcloudapi.com", 401, `{"Response":{"Error":{"Code":"AuthFailure"}}}`}})
		resetValidatorState(a)
		if got := a.CheckTencent(key+"x", "http://example.com"); got != false {
			t.Error("expected false for 401")
		}
		assertValidated(t, 0)
	})

	t.Run("valid_status_200_wrong_body", func(t *testing.T) {
		// HTTP 200 but body contains Error field → false
		a := validatorScanner(t)
		setMockClient(t, []mockRoute{{"cvm.tencentcloudapi.com", 200, failBody}})
		resetValidatorState(a)
		if got := a.CheckTencent(key+"diff2", "http://example.com"); got != false {
			t.Error("expected false when Response.Error present in body")
		}
		assertValidated(t, 0)
	})
}
