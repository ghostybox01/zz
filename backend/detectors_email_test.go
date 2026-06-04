package main

import (
	"regexp"
	"testing"
)

// ---------------------------------------------------------------------------
// Shared helpers
// ---------------------------------------------------------------------------

// mustGroup1 returns the first capture group from the first match, or "".
func mustGroup1(re *regexp.Regexp, input string) string {
	m := re.FindStringSubmatch(input)
	if len(m) < 2 {
		return ""
	}
	return m[1]
}

// ---------------------------------------------------------------------------
// Mailjet
// ---------------------------------------------------------------------------

const (
	mjPub  = "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4"
	mjPriv = "b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5"
)

func TestMailjet(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string // expected group 1
	}{
		// .env style
		{"env_MJ_APIKEY_PUBLIC", `MJ_APIKEY_PUBLIC=` + mjPub, mjPub},
		{"env_MJ_APIKEY_PRIVATE", `MJ_APIKEY_PRIVATE=` + mjPriv, mjPriv},
		{"env_MAILJET_API_KEY", `MAILJET_API_KEY=` + mjPub, mjPub},
		// quoted .env
		{"env_MJ_APIKEY_PUBLIC_quoted", `MJ_APIKEY_PUBLIC="` + mjPub + `"`, mjPub},
		{"env_MAILJET_API_KEY_quoted", `MAILJET_API_KEY="` + mjPub + `"`, mjPub},
		// JSON
		{"json_mailjet_api_key", `"mailjet_api_key": "` + mjPub + `"`, mjPub},
		// YAML
		{"yaml_mailjet_key", `mailjet_key: ` + mjPub, mjPub},
		// PHP
		{"php_mailjet", `$key = '` + mjPub + `';`, ""},         // no mailjet context → no match (correct)
		{"php_mailjet_ctx", `$mailjet_key = '` + mjPub + `';`, mjPub},
		// JS config
		{"js_mailjetApiKey", `mailjetApiKey: '` + mjPub + `'`, mjPub},
		// MJ_APIKEY (bare, without PUBLIC/PRIVATE suffix)
		{"env_MJ_APIKEY_bare", `MJ_APIKEY=` + mjPub, mjPub},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := mustGroup1(mailjetPattern, tc.input)
			if got != tc.want {
				t.Errorf("mailjetPattern on %q\n  got  %q\n  want %q", tc.input, got, tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Mailtrap
// ---------------------------------------------------------------------------

const mtToken = "c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6"

func TestMailtrap(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{"env_MAILTRAP_API_TOKEN", `MAILTRAP_API_TOKEN=` + mtToken, mtToken},
		{"env_MAILTRAP_TOKEN", `MAILTRAP_TOKEN=` + mtToken, mtToken},
		{"env_MAILTRAP_API_KEY", `MAILTRAP_API_KEY=` + mtToken, mtToken},
		{"env_MT_TOKEN", `MT_TOKEN=` + mtToken, mtToken},
		{"env_quoted", `MAILTRAP_API_TOKEN="` + mtToken + `"`, mtToken},
		{"json_mailtrap_api_token", `"mailtrap_api_token": "` + mtToken + `"`, mtToken},
		{"yaml_mailtrap_token", `mailtrap_token: ` + mtToken, mtToken},
		{"php_mailtrap", `$mailtrap_key = '` + mtToken + `';`, mtToken},
		{"js_mailtrapToken", `mailtrapToken: '` + mtToken + `'`, mtToken},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := mustGroup1(mailtrapPattern, tc.input)
			if got != tc.want {
				t.Errorf("mailtrapPattern on %q\n  got  %q\n  want %q", tc.input, got, tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Postmark
// ---------------------------------------------------------------------------

const pmToken = "a1b2c3d4-e5f6-a1b2-c3d4-e5f6a1b2c3d4"

func TestPostmark(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{"env_POSTMARK_SERVER_TOKEN", `POSTMARK_SERVER_TOKEN=` + pmToken, pmToken},
		{"env_POSTMARK_API_TOKEN", `POSTMARK_API_TOKEN=` + pmToken, pmToken},
		{"env_PM_SERVER_TOKEN", `PM_SERVER_TOKEN=` + pmToken, pmToken},
		{"env_postmark_token", `postmark_token=` + pmToken, pmToken},
		{"env_quoted", `POSTMARK_SERVER_TOKEN="` + pmToken + `"`, pmToken},
		{"json_postmark_server_token", `"postmark_server_token": "` + pmToken + `"`, pmToken},
		{"yaml_postmark_token", `postmark_token: ` + pmToken, pmToken},
		{"php_postmark", `$postmark_token = '` + pmToken + `';`, pmToken},
		{"js_postmarkToken", `postmarkToken: '` + pmToken + `'`, pmToken},
		// group1 must be just the UUID, not the full key=uuid string
		{"group1_is_uuid_only", `POSTMARK_SERVER_TOKEN=` + pmToken, pmToken},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := mustGroup1(postmarkPattern, tc.input)
			if got != tc.want {
				t.Errorf("postmarkPattern on %q\n  got  %q\n  want %q", tc.input, got, tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// SparkPost
// ---------------------------------------------------------------------------

const spKey = "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2"

func TestSparkPost(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{"env_SPARKPOST_API_KEY", `SPARKPOST_API_KEY=` + spKey, spKey},
		{"env_SP_API_KEY", `SP_API_KEY=` + spKey, spKey},
		{"env_SPARKPOST_KEY", `SPARKPOST_KEY=` + spKey, spKey},
		{"env_sparkpost_key", `sparkpost_key=` + spKey, spKey},
		{"env_quoted", `SPARKPOST_API_KEY="` + spKey + `"`, spKey},
		{"json_sparkpost_api_key", `"sparkpost_api_key": "` + spKey + `"`, spKey},
		{"yaml_sparkpost_key", `sparkpost_key: ` + spKey, spKey},
		{"php_sparkpost", `$sparkpost_api_key = '` + spKey + `';`, spKey},
		{"js_sparkpostApiKey", `sparkpostApiKey: '` + spKey + `'`, spKey},
		// group1 must be just the 40-char hex, not the full key=hex string
		{"group1_is_hex_only", `SPARKPOST_API_KEY=` + spKey, spKey},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := mustGroup1(sparkpostPattern, tc.input)
			if got != tc.want {
				t.Errorf("sparkpostPattern on %q\n  got  %q\n  want %q", tc.input, got, tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Brevo  (no capture group — FindAllString returns the full key)
// ---------------------------------------------------------------------------

// 64 alphanum chars: 26 lower + 26 upper + 10 digits + 2 lower = 64
const brevoKey = "xkeysib-abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789ab-abcdefghijklmnop"

// brevoPattern mirrors the regexp compiled in main.go NewAWSScanner.
var brevoPattern = regexp.MustCompile(`xkeysib-[a-zA-Z0-9]{64}-[a-zA-Z0-9]{16}`)

func TestBrevo(t *testing.T) {
	cases := []struct {
		name    string
		input   string
		wantKey string // expected value in FindAllString result
	}{
		{"env_BREVO_API_KEY", `BREVO_API_KEY=` + brevoKey, brevoKey},
		{"env_quoted", `BREVO_API_KEY="` + brevoKey + `"`, brevoKey},
		{"json_brevo_key", `"api_key": "` + brevoKey + `"`, brevoKey},
		{"yaml_brevo_key", `api_key: ` + brevoKey, brevoKey},
		{"php_brevo", `$key = '` + brevoKey + `';`, brevoKey},
		{"js_brevo", `apiKey: '` + brevoKey + `'`, brevoKey},
		// negative: truncated key must not match
		{"neg_short_key", `xkeysib-abc-abc`, ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			matches := brevoPattern.FindAllString(tc.input, -1)
			if tc.wantKey == "" {
				if len(matches) != 0 {
					t.Errorf("brevoPattern on %q: expected no match, got %v", tc.input, matches)
				}
				return
			}
			if len(matches) == 0 {
				t.Errorf("brevoPattern on %q: expected match %q, got none", tc.input, tc.wantKey)
				return
			}
			if matches[0] != tc.wantKey {
				t.Errorf("brevoPattern on %q\n  got  %q\n  want %q", tc.input, matches[0], tc.wantKey)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Mandrill  (group 1 = md-<22alphanum>, no surrounding quotes)
// ---------------------------------------------------------------------------

// md- prefix + exactly 22 alphanum chars = valid Mandrill key
const mandrillKey = "md-ABCDEFabcdef0123456789"

// mandrillPattern mirrors the regexp compiled in main.go NewAWSScanner.
var mandrillPattern = regexp.MustCompile(`['"]?(md-[0-9a-zA-Z]{22})['"]?`)

func TestMandrill(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string // expected group 1 (no quotes)
	}{
		{"bare", mandrillKey, mandrillKey},
		{"single_quoted", `'` + mandrillKey + `'`, mandrillKey},
		{"double_quoted", `"` + mandrillKey + `"`, mandrillKey},
		{"env_MANDRILL_API_KEY", `MANDRILL_API_KEY=` + mandrillKey, mandrillKey},
		{"json_mandrill_key", `"api_key": "` + mandrillKey + `"`, mandrillKey},
		{"yaml_mandrill_key", `api_key: ` + mandrillKey, mandrillKey},
		{"php_mandrill", `$key = '` + mandrillKey + `';`, mandrillKey},
		{"js_mandrill", `apiKey: '` + mandrillKey + `'`, mandrillKey},
		// group1 must NOT include surrounding quotes
		{"group1_no_quotes_single", `'` + mandrillKey + `'`, mandrillKey},
		{"group1_no_quotes_double", `"` + mandrillKey + `"`, mandrillKey},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := mustGroup1(mandrillPattern, tc.input)
			if got != tc.want {
				t.Errorf("mandrillPattern on %q\n  got  %q\n  want %q", tc.input, got, tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// MailerSend  (no capture group — FindAllString returns the full token)
// ---------------------------------------------------------------------------

const mailerSendToken = "mlsn.abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789ab"

// mailerSendPattern mirrors the regexp compiled in main.go NewAWSScanner.
var mailerSendPattern = regexp.MustCompile(`mlsn\.[a-zA-Z0-9_\-]{40,100}`)

func TestMailerSend(t *testing.T) {
	cases := []struct {
		name    string
		input   string
		wantKey string
	}{
		{"env_MAILERSEND_API_KEY", `MAILERSEND_API_KEY=` + mailerSendToken, mailerSendToken},
		{"env_quoted", `MAILERSEND_API_KEY="` + mailerSendToken + `"`, mailerSendToken},
		{"json_mailersend", `"api_token": "` + mailerSendToken + `"`, mailerSendToken},
		{"yaml_mailersend", `api_token: ` + mailerSendToken, mailerSendToken},
		{"php_mailersend", `$token = '` + mailerSendToken + `';`, mailerSendToken},
		{"js_mailersend", `apiToken: '` + mailerSendToken + `'`, mailerSendToken},
		// negative: too short (mlsn. + 39 chars)
		{"neg_too_short", `mlsn.` + "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLM", ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			matches := mailerSendPattern.FindAllString(tc.input, -1)
			if tc.wantKey == "" {
				if len(matches) != 0 {
					t.Errorf("mailerSendPattern on %q: expected no match, got %v", tc.input, matches)
				}
				return
			}
			if len(matches) == 0 {
				t.Errorf("mailerSendPattern on %q: expected match %q, got none", tc.input, tc.wantKey)
				return
			}
			if matches[0] != tc.wantKey {
				t.Errorf("mailerSendPattern on %q\n  got  %q\n  want %q", tc.input, matches[0], tc.wantKey)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Mailgun dual-pattern  (legacy key- branch vs. context-based branch)
// ---------------------------------------------------------------------------

// mailgunDualPattern mirrors the regexp compiled in main.go NewAWSScanner.
var mailgunDualPattern = regexp.MustCompile(`(?:key-[0-9a-zA-Z]{32}|(?i)(?:MAILGUN[_\-]?(?:API[_\-]?)?(?:KEY|SECRET|TOKEN)|MG_API_KEY)["'\s:=]+([a-zA-Z0-9\-]{20,50}))`)

// dispatchKey simulates the dispatch loop logic for a single submatch slice.
func dispatchKey(m []string) string {
	if len(m) > 1 && m[1] != "" {
		return m[1]
	}
	if len(m) > 0 {
		return m[0]
	}
	return ""
}

func TestMailgunDualPattern(t *testing.T) {
	legacyKey := "key-" + "abcdefghijklmnopqrstuvwxyz123456" // exactly key- + 32 chars
	newKey    := "abcdef1234567890abcdef1234567890"          // 32-char hex

	cases := []struct {
		name    string
		input   string
		wantKey string // what the dispatch loop should hand to CheckMailgun
	}{
		// Legacy key- bare — no capture group, so m[0] is the full match
		{"legacy_bare", legacyKey, legacyKey},
		// Legacy key- in env context — context alt fires (captures up to 50 chars), m[1]
		// should be the key-xxx portion (or the full value including key- prefix)
		{"legacy_in_env", "MAILGUN_API_KEY=" + legacyKey, legacyKey},
		// Context new key
		{"context_new_key", "MAILGUN_API_KEY=" + newKey, newKey},
		{"context_new_key_single_quoted", "MAILGUN_API_KEY='" + newKey + "'", newKey},
		{"mg_api_key", "MG_API_KEY=" + newKey, newKey},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ms := mailgunDualPattern.FindAllStringSubmatch(tc.input, -1)
			if len(ms) == 0 {
				t.Fatalf("mailgunDualPattern no match on %q", tc.input)
			}
			got := dispatchKey(ms[0])
			if got != tc.wantKey {
				t.Errorf("mailgunDualPattern dispatch on %q\n  got  %q\n  want %q", tc.input, got, tc.wantKey)
			}
		})
	}
}

// TestMailgunLegacyInEnvDoesNotReturnContextString verifies that when a legacy
// "key-XXX" value appears after MAILGUN_API_KEY=, the dispatched key is exactly
// the "key-XXX" token, NOT the entire "MAILGUN_API_KEY=key-XXX" string.
func TestMailgunLegacyInEnvDispatchedKeyIsJustToken(t *testing.T) {
	legacyKey := "key-" + "abcdefghijklmnopqrstuvwxyz123456"
	input := "MAILGUN_API_KEY=" + legacyKey

	ms := mailgunDualPattern.FindAllStringSubmatch(input, -1)
	if len(ms) == 0 {
		t.Fatalf("no match")
	}
	got := dispatchKey(ms[0])
	// The context branch captures group 1 = "key-abcdef..." (value after =)
	// That is the full legacy key, which is fine — CheckMailgun receives it.
	if got != legacyKey {
		t.Errorf("dispatched key = %q, want %q", got, legacyKey)
	}
}

// TestMailgunNewKeyPattern tests the NewMailgunAPIKeyPattern (UUID-style).
var newMailgunPattern = regexp.MustCompile(`[a-f0-9]{32}-[0-9a-f]{8}-[a-f0-9]{8}`)

func TestNewMailgunPatternDispatch(t *testing.T) {
	newKey := "abcdef1234567890abcdef1234567890-12345678-abcdef12"
	cases := []struct {
		name    string
		input   string
		wantKey string
	}{
		{"bare_new_key", newKey, newKey},
		{"env_new_key", "MAILGUN_API_KEY=" + newKey, newKey},
		// negative: too short
		{"neg_too_short", "abc123-12345678-abcdef12", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ms := newMailgunPattern.FindAllStringSubmatch(tc.input, -1)
			if tc.wantKey == "" {
				if len(ms) != 0 {
					t.Errorf("expected no match, got %v", ms)
				}
				return
			}
			if len(ms) == 0 {
				t.Fatalf("no match on %q", tc.input)
			}
			got := dispatchKey(ms[0])
			if got != tc.wantKey {
				t.Errorf("got %q, want %q", got, tc.wantKey)
			}
		})
	}
}

// TestSendGridPattern tests that the SendGrid pattern (no capture group) returns
// the full SG.xxx token via m[0] in the dispatch loop.
var sendGridPattern = regexp.MustCompile(`SG\.[0-9A-Za-z\-_]{22}\.[0-9A-Za-z\-_]{43}`)

func TestSendGridPatternDispatch(t *testing.T) {
	// seg1 = exactly 22 alphanum chars, seg2 = exactly 43 alphanum chars
	sgKey := "SG.abcdefghijklmnopqrstuv.abcdefghijklmnopqrstuvwxyzabcdefghijklmnopq"
	cases := []struct {
		name    string
		input   string
		wantKey string
	}{
		{"bare_sg_key", sgKey, sgKey},
		{"env_sg_key", "SENDGRID_API_KEY=" + sgKey, sgKey},
		{"json_sg_key", `{"api_key":"` + sgKey + `"}`, sgKey},
		// negative: too short middle segment
		{"neg_short_middle", "SG.abc.abcdefghijklmnopqrstuvwxyzabcdefghijklmnopqr", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ms := sendGridPattern.FindAllStringSubmatch(tc.input, -1)
			if tc.wantKey == "" {
				if len(ms) != 0 {
					t.Errorf("expected no match, got %v", ms)
				}
				return
			}
			if len(ms) == 0 {
				t.Fatalf("no match on %q", tc.input)
			}
			got := dispatchKey(ms[0])
			if got != tc.wantKey {
				t.Errorf("got %q, want %q", got, tc.wantKey)
			}
		})
	}
}

// TestXSMTPPattern tests the XSMTP API key pattern (no capture group).
var xsmtpPattern = regexp.MustCompile(`xsmtpsib-[a-fA-F0-9]{64}-[a-zA-Z0-9]{16}`)

func TestXSMTPPattern(t *testing.T) {
	// hex part = exactly 64 lowercase hex chars, suffix = 16 alphanum
	xKey := "xsmtpsib-aabbccddeeff0011223344556677889900aabbccddeeff001122334455667788-AbCdEfGhIjKlMnOp"
	cases := []struct {
		name    string
		input   string
		wantKey string
	}{
		{"bare_xsmtp_key", xKey, xKey},
		{"env_xsmtp_key", "XSMTP_API_KEY=" + xKey, xKey},
		{"json_xsmtp_key", `{"api_key":"` + xKey + `"}`, xKey},
		{"neg_short", "xsmtpsib-aabbcc-AbCdEfGhIjKlMnOp", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ms := xsmtpPattern.FindAllStringSubmatch(tc.input, -1)
			if tc.wantKey == "" {
				if len(ms) != 0 {
					t.Errorf("expected no match, got %v", ms)
				}
				return
			}
			if len(ms) == 0 {
				t.Fatalf("no match on %q", tc.input)
			}
			got := dispatchKey(ms[0])
			if got != tc.wantKey {
				t.Errorf("got %q, want %q", got, tc.wantKey)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Tencent  (no capture group — FindAllString returns the full AKID token)
// ---------------------------------------------------------------------------

const tencentKey = "AKIDabcdefghijklmnopqrstuvwxyz123456"

// tencentPattern mirrors the regexp compiled in main.go NewAWSScanner.
var tencentPattern = regexp.MustCompile(`AKID[a-zA-Z0-9]{32}`)

func TestTencent(t *testing.T) {
	cases := []struct {
		name    string
		input   string
		wantKey string
	}{
		{"env_TENCENT_SECRET_ID", `TENCENT_SECRET_ID=` + tencentKey, tencentKey},
		{"env_quoted", `TENCENT_SECRET_ID="` + tencentKey + `"`, tencentKey},
		{"json_tencent", `"secret_id": "` + tencentKey + `"`, tencentKey},
		{"yaml_tencent", `secret_id: ` + tencentKey, tencentKey},
		{"php_tencent", `$secretId = '` + tencentKey + `';`, tencentKey},
		{"js_tencent", `secretId: '` + tencentKey + `'`, tencentKey},
		// full match must be just AKID+32, no surrounding quotes
		{"no_quotes_in_match", `"` + tencentKey + `"`, tencentKey},
		// negative: too short (AKID + 31 chars)
		{"neg_too_short", `AKID` + "abcdefghijklmnopqrstuvwxyz12345", ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			matches := tencentPattern.FindAllString(tc.input, -1)
			if tc.wantKey == "" {
				if len(matches) != 0 {
					t.Errorf("tencentPattern on %q: expected no match, got %v", tc.input, matches)
				}
				return
			}
			if len(matches) == 0 {
				t.Errorf("tencentPattern on %q: expected match %q, got none", tc.input, tc.wantKey)
				return
			}
			if matches[0] != tc.wantKey {
				t.Errorf("tencentPattern on %q\n  got  %q\n  want %q", tc.input, matches[0], tc.wantKey)
			}
		})
	}
}
