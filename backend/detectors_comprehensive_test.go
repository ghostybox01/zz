package main

// detectors_comprehensive_test.go
//
// Comprehensive pattern coverage for every addon:
//   - Detection in .env, JSON, PHP define, JS bundle, YAML formats
//   - Group-1 extraction (no surrounding quotes, no env-var prefix)
//   - Correct value length
//   - Critical negative cases (wrong format / excluded prefix)
//
// Run with:  go test -v -run TestComprehensive ./...

import (
	"fmt"
	"regexp"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Dummy credentials — safe/fake values only
// ---------------------------------------------------------------------------

const (
	// AWS
	compAKIA   = "AKIA0000000000000000" // AKIA + 16 zeros
	compASIA   = "ASIA0000000000000000" // ASIA + 16 zeros
	compAWSSecret = "00000000000000000000000000000000000000" // 40 zeros

	// SendGrid: SG.<22>.<43>
	compSendGrid = "SG.00000000000000000000.0000000000000000000000000000000000000000000"

	// Stripe
	compStripeLive = "sk_live_0000000000000000"
	compStripeRK   = "rk_live_0000000000000000" // rk_live_ + zeros

	// Mailgun legacy: key- + exactly 32 alphanum
	compMailgunLegacy = "key-00000000000000000000000000000000"
	// Mailgun new UUID: [a-f0-9]{32}-[0-9a-f]{8}-[a-f0-9]{8}
	// All segments must be strict lowercase hex (a-f0-9 only)
	compMailgunNew = "00000000000000000000000000000000-00000000-00000000"

	// Brevo: xkeysib- + 64 alphanum + - + 16 alphanum
	compBrevo = "xkeysib-0000000000000000000000000000000000000000000000000000000000000000-0000000000000000"

	// Mandrill: md- + exactly 22 alphanum (22 chars after "md-")
	compMandrill = "md-0000000000000000000000"

	// MailerSend: mlsn. + 68 chars of [A-Za-z0-9_-]
	// 26 lower + 26 upper + 10 digits + 6 more = 68 chars
	compMailerSend = "mlsn.abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789abcdef"

	// Mailjet: 32-char lowercase hex
	compMJPub  = "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4"
	compMJPriv = "b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5"

	// Postmark: UUID format
	compPostmark = "a1b2c3d4-e5f6-a1b2-c3d4-e5f6a1b2c3d4"

	// SparkPost: 40 lowercase hex
	compSparkPost = "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2"

	// Mailtrap: 32 lowercase hex
	compMailtrap = "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4"

	// OpenAI (modern sk-proj- format).
	// Branch2 requires {20,} + {28,} = 48 chars minimum after "sk-proj-".
	// We supply 52 chars (26 upper + 26 lower = 52) to satisfy both quantifiers.
	compOpenAI = "sk-proj-ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

	// OpenAI legacy with T3BlbkFJ marker
	compOpenAILegacy = "sk-ABCDEFGHIJKLMNOPQRSTuvwxT3BlbkFJABCDEFGHIJKLMNOPQRSTUVWX"

	// Anthropic: sk-ant-api03- + 86+ base64url chars
	compAnthropic = "sk-ant-api03-ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZab"

	// Twilio
	compTwilioSID   = "ACa1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4" // AC + 32 lowercase hex
	compTwilioAuth  = "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4"  // 32 lowercase hex

	// Nexmo/Vonage
	compNexmoKey    = "a1b2c3d4"        // 8 alphanum
	compNexmoSecret = "a1b2c3d4e5f6a1b2" // 16 alphanum

	// Telnyx: KEY + lowercase hex (20–56 chars) + optional _suffix
	compTelnyx = "KEY017c1e20f0e14d7c977a5a64a6c79f02_SomePaddingAB"

	// MessageBird
	compMBLive = "live_abcdefghijklmnopqrstuvwxy" // live_ + 25 alphanum
	compMBTest = "test_abcdefghijklmnopqrstuvwxy"

	// Plivo: MA/SA + 18 uppercase alphanum
	compPlivo = "MAa1b2c3d4e5f6a1b2c3"
)

// ---------------------------------------------------------------------------
// Pattern mirrors (identical to main.go NewAWSScanner — kept here for
// self-contained testing without instantiating the full AWSScanner struct)
// ---------------------------------------------------------------------------

var (
	// AWS
	compPatAWSSESUser    = regexp.MustCompile(`\b(AKIA|ASIA)[A-Z0-9]{16}\b`)
	compPatAWSSecretInfo = regexp.MustCompile(`\b[A-Za-z0-9/+=]{40}\b`)

	// SendGrid (no capture group — full match is the key)
	compPatSendGrid = regexp.MustCompile(`SG\.[0-9A-Za-z\-_]{22}\.[0-9A-Za-z\-_]{43}`)

	// Stripe (no capture group)
	compPatStripe = regexp.MustCompile(`(?:sk_live_|sk_test_|rk_live_|rk_test_)[0-9a-zA-Z]{16,99}`)

	// Mailgun dual-branch
	compPatMailgun    = regexp.MustCompile(`(?:key-[0-9a-zA-Z]{32}|(?i)(?:MAILGUN[_\-]?(?:API[_\-]?)?(?:KEY|SECRET|TOKEN)|MG_API_KEY)["'\s:=]+([a-zA-Z0-9\-]{20,50}))`)
	compPatNewMailgun = regexp.MustCompile(`[a-f0-9]{32}-[0-9a-f]{8}-[a-f0-9]{8}`)

	// Brevo (no capture group)
	compPatBrevo = regexp.MustCompile(`xkeysib-[a-zA-Z0-9]{64}-[a-zA-Z0-9]{16}`)

	// Mandrill — group 1 strips surrounding optional quotes
	compPatMandrill = regexp.MustCompile(`['"]?(md-[0-9a-zA-Z]{22})['"]?`)

	// MailerSend (no capture group)
	compPatMailerSend = regexp.MustCompile(`mlsn\.[a-zA-Z0-9_\-]{40,100}`)

	// Mailjet — group 1 = 32-char hex (from detectors_mailjet.go)
	compPatMailjet = regexp.MustCompile(`(?i)(?:mailjet[_\-]?(?:api[_\-]?)?(?:key|public|secret)|MJ_APIKEY_(?:PUBLIC|PRIVATE)|MJ_APIKEY)["'\s:=]+([0-9a-f]{32})`)

	// Postmark — group 1 = UUID (from detectors_postmark.go)
	compPatPostmark = regexp.MustCompile(`(?i)(?:postmark[_\-]?(?:server[_\-]?|api[_\-]?)?token|POSTMARK_(?:SERVER_|API_)?TOKEN|PM_SERVER_TOKEN)["'\s:=]+([0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12})`)

	// SparkPost — group 1 = 40-char hex (from detectors_sparkpost.go)
	compPatSparkPost = regexp.MustCompile(`(?i)(?:sparkpost[_\-]?(?:api[_\-]?)?key|SP_API_KEY|SPARKPOST_KEY)["'\s:=]+([a-f0-9]{40})`)

	// Mailtrap — group 1 = 32-char hex (from detectors_mailtrap.go)
	compPatMailtrap = regexp.MustCompile(`(?i)(?:mailtrap[_\-]?(?:api[_\-]?)?(?:token|key)|MAILTRAP_(?:API_)?(?:TOKEN|KEY)|MT_TOKEN)["'\s:=]+([a-f0-9]{32})`)

	// OpenAI (no capture group)
	compPatOpenAI = regexp.MustCompile(`sk-(?:proj-|o1-|svcacct-)?[a-zA-Z0-9]{20,}(?:T3BlbkFJ[a-zA-Z0-9]{20,}|[a-zA-Z0-9_-]{28,})`)

	// Anthropic (no capture group)
	compPatAnthropic = regexp.MustCompile(`sk-ant-(?:api\d+-[A-Za-z0-9_-]{86,}|[A-Za-z0-9_-]{92,})`)

	// Twilio SID (no capture group)
	compPatTwilioSID = regexp.MustCompile(`AC[a-f0-9]{32}`)

	// Twilio auth — group 1 = 32 lowercase hex (quotes optional)
	compPatTwilioAuth = regexp.MustCompile(`(?i)['"']?([0-9a-f]{32})['"']?`)

	// Nexmo/Vonage key — group 2 = 8 alphanum
	compPatNexmoKey = regexp.MustCompile(`(?i)(NEXMO_API_KEY|VONAGE_API_KEY)\s*[:=]\s*["']?([a-zA-Z0-9]{8})["\']?`)

	// Nexmo/Vonage secret — group 2 = 16 alphanum
	compPatNexmoSecret = regexp.MustCompile(`(?i)(NEXMO_API_SECRET|VONAGE_API_SECRET)\s*[:=]\s*["\']?([a-zA-Z0-9]{16})["\']?`)

	// Telnyx (no capture group)
	compPatTelnyx = regexp.MustCompile(`KEY[0-9a-f]{20,56}(?:_[A-Za-z0-9_\-]{10,30})?`)

	// MessageBird (no capture group)
	compPatMessageBird = regexp.MustCompile(`(?:live|test)_[a-zA-Z0-9]{25,40}`)

	// Plivo — group 1 = MA/SA + 18 uppercase alphanum (from detectors_plivo.go)
	compPatPlivo = regexp.MustCompile(`(?i)(?:plivo[_-]?(?:auth[_-]?)?(?:id|sid))["'\s:=]+([MS]A[A-Z0-9]{18})`)
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// g1 returns group 1 of the first match (or "" if no match / no group).
func g1(re *regexp.Regexp, s string) string {
	m := re.FindStringSubmatch(s)
	if len(m) < 2 {
		return ""
	}
	return m[1]
}

// g2 returns group 2 of the first match.
func g2(re *regexp.Regexp, s string) string {
	m := re.FindStringSubmatch(s)
	if len(m) < 3 {
		return ""
	}
	return m[2]
}

// m0 returns the full first match (m[0]).
func m0(re *regexp.Regexp, s string) string {
	return re.FindString(s)
}

// ---------------------------------------------------------------------------
// Test result table entry (printed at end for the caller's summary table)
// ---------------------------------------------------------------------------

type compResult struct {
	addon    string
	format   string
	want     string
	got      string
}

var allResults []compResult

func recordResult(t *testing.T, addon, format, want, got string) {
	t.Helper()
	allResults = append(allResults, compResult{addon, format, want, got})
	if got != want {
		t.Errorf("[%s/%s] got=%q want=%q", addon, format, got, want)
	}
}

// ---------------------------------------------------------------------------
// TestComprehensive — master test
// ---------------------------------------------------------------------------

func TestComprehensive(t *testing.T) {
	// ------------------------------------------------------------------ AWS
	t.Run("AWS_AKIA", func(t *testing.T) {
		formats := []struct{ name, input string }{
			{"env",  "AWS_ACCESS_KEY_ID=" + compAKIA},
			{"json", `{"access_key": "` + compAKIA + `"}`},
			{"php",  `define('AWS_KEY', '` + compAKIA + `');`},
			{"js",   `var k="` + compAKIA + `"`},
			{"yaml", "aws_access_key_id: " + compAKIA},
		}
		for _, f := range formats {
			got := m0(compPatAWSSESUser, f.input)
			recordResult(t, "AWS_AKIA", f.name, compAKIA, got)
		}
	})

	t.Run("AWS_ASIA", func(t *testing.T) {
		formats := []struct{ name, input string }{
			{"env",  "AWS_ACCESS_KEY_ID=" + compASIA},
			{"json", `{"access_key": "` + compASIA + `"}`},
			{"php",  `define('AWS_KEY', '` + compASIA + `');`},
			{"js",   `var k="` + compASIA + `"`},
			{"yaml", "aws_access_key_id: " + compASIA},
		}
		for _, f := range formats {
			got := m0(compPatAWSSESUser, f.input)
			recordResult(t, "AWS_ASIA", f.name, compASIA, got)
		}
	})

	t.Run("AWS_SecretKey", func(t *testing.T) {
		// AWSSecretKeyPatternInfo matches any 40-char word; we confirm our value appears
		formats := []struct{ name, input string }{
			{"env",  "AWS_SECRET_ACCESS_KEY=" + compAWSSecret},
			{"json", `{"secret_key": "` + compAWSSecret + `"}`},
			{"php",  `define('AWS_SECRET', '` + compAWSSecret + `');`},
			{"yaml", "aws_secret_access_key: " + compAWSSecret},
		}
		for _, f := range formats {
			found := false
			for _, m := range compPatAWSSecretInfo.FindAllString(f.input, -1) {
				if m == compAWSSecret {
					found = true
				}
			}
			got := ""
			if found {
				got = compAWSSecret
			}
			recordResult(t, "AWS_SecretKey", f.name, compAWSSecret, got)
		}
	})

	// ------------------------------------------------------------------ SendGrid
	t.Run("SendGrid", func(t *testing.T) {
		formats := []struct{ name, input string }{
			{"env",  "SENDGRID_API_KEY=" + compSendGrid},
			{"json", `{"api_key": "` + compSendGrid + `"}`},
			{"php",  `define('SG_KEY', '` + compSendGrid + `');`},
			{"js",   `var k="` + compSendGrid + `"`},
			{"yaml", "sendgrid_api_key: " + compSendGrid},
		}
		for _, f := range formats {
			got := m0(compPatSendGrid, f.input)
			recordResult(t, "SendGrid", f.name, compSendGrid, got)
		}
		// Negative: pk_ excluded (SendGrid pattern has no pk_ but confirm our value has SG.)
		t.Run("negative_short_segment", func(t *testing.T) {
			bad := "SG.short.tooshort"
			if m0(compPatSendGrid, bad) != "" {
				t.Errorf("SendGrid: short segment must not match")
			}
		})
	})

	// ------------------------------------------------------------------ Stripe
	t.Run("Stripe", func(t *testing.T) {
		formats := []struct{ name, input, want string }{
			{"env_sk_live",  "STRIPE_SECRET_KEY=" + compStripeLive, compStripeLive},
			{"json_sk_live", `{"stripe_key": "` + compStripeLive + `"}`, compStripeLive},
			{"php_sk_live",  `define('STRIPE', '` + compStripeLive + `');`, compStripeLive},
			{"js_rk_live",   `var k="` + compStripeRK + `"`, compStripeRK},
			{"yaml_sk_live", "stripe_secret_key: " + compStripeLive, compStripeLive},
		}
		for _, f := range formats {
			got := m0(compPatStripe, f.input)
			recordResult(t, "Stripe", f.name, f.want, got)
		}
		// Negative: pk_ excluded
		t.Run("negative_pk", func(t *testing.T) {
			pk := "pk_live_51ABCDEFGHIJKLMNOPQRSTUVWXYZabcdef1234567890"
			if m0(compPatStripe, pk) != "" {
				t.Errorf("Stripe: pk_ prefix must not match; got match")
			}
		})
	})

	// ------------------------------------------------------------------ Mailgun legacy
	t.Run("Mailgun_Legacy", func(t *testing.T) {
		formats := []struct{ name, input, want string }{
			// In .env with context keyword → context branch fires, group 1 = key value
			{"env_ctx",  "MAILGUN_API_KEY=" + compMailgunLegacy, compMailgunLegacy},
			// Bare key- (no context) → key- branch fires, m[0] = full key
			{"bare",     compMailgunLegacy, compMailgunLegacy},
			{"json",     `{"api_key": "` + compMailgunLegacy + `"}`, compMailgunLegacy},
			{"yaml",     "api_key: " + compMailgunLegacy, compMailgunLegacy},
		}
		for _, f := range formats {
			ms := compPatMailgun.FindAllStringSubmatch(f.input, -1)
			var got string
			if len(ms) > 0 {
				if len(ms[0]) > 1 && ms[0][1] != "" {
					got = ms[0][1]
				} else {
					got = ms[0][0]
				}
			}
			recordResult(t, "Mailgun_Legacy", f.name, f.want, got)
		}
	})

	// ------------------------------------------------------------------ Mailgun New UUID
	t.Run("Mailgun_New", func(t *testing.T) {
		formats := []struct{ name, input string }{
			{"env",  "MAILGUN_API_KEY=" + compMailgunNew},
			{"json", `{"api_key": "` + compMailgunNew + `"}`},
			{"php",  `define('MG_KEY', '` + compMailgunNew + `');`},
			{"yaml", "mailgun_api_key: " + compMailgunNew},
		}
		for _, f := range formats {
			got := m0(compPatNewMailgun, f.input)
			recordResult(t, "Mailgun_New", f.name, compMailgunNew, got)
		}
	})

	// ------------------------------------------------------------------ Brevo
	t.Run("Brevo", func(t *testing.T) {
		formats := []struct{ name, input string }{
			{"env",  "BREVO_API_KEY=" + compBrevo},
			{"json", `{"api_key": "` + compBrevo + `"}`},
			{"php",  `define('BREVO', '` + compBrevo + `');`},
			{"js",   `var k="` + compBrevo + `"`},
			{"yaml", "brevo_api_key: " + compBrevo},
		}
		for _, f := range formats {
			got := m0(compPatBrevo, f.input)
			recordResult(t, "Brevo", f.name, compBrevo, got)
		}
	})

	// ------------------------------------------------------------------ Mandrill
	t.Run("Mandrill", func(t *testing.T) {
		formats := []struct{ name, input string }{
			{"env",  `MANDRILL_API_KEY="` + compMandrill + `"`},
			{"json", `{"api_key": "` + compMandrill + `"}`},
			{"php",  `define('MANDRILL', '` + compMandrill + `');`},
			{"js",   `var k='` + compMandrill + `'`},
			{"yaml", "mandrill_key: " + compMandrill},
		}
		for _, f := range formats {
			got := g1(compPatMandrill, f.input)
			recordResult(t, "Mandrill", f.name, compMandrill, got)
		}
		// Confirm quotes stripped from group 1
		t.Run("group1_no_quotes_double", func(t *testing.T) {
			got := g1(compPatMandrill, `"`+compMandrill+`"`)
			if got != compMandrill {
				t.Errorf("Mandrill: double-quoted input group1=%q want=%q", got, compMandrill)
			}
		})
		t.Run("group1_no_quotes_single", func(t *testing.T) {
			got := g1(compPatMandrill, `'`+compMandrill+`'`)
			if got != compMandrill {
				t.Errorf("Mandrill: single-quoted input group1=%q want=%q", got, compMandrill)
			}
		})
	})

	// ------------------------------------------------------------------ MailerSend
	t.Run("MailerSend", func(t *testing.T) {
		formats := []struct{ name, input string }{
			{"env",  "MAILERSEND_API_KEY=" + compMailerSend},
			{"json", `{"api_token": "` + compMailerSend + `"}`},
			{"php",  `define('MS_TOKEN', '` + compMailerSend + `');`},
			{"js",   `var k="` + compMailerSend + `"`},
			{"yaml", "mailersend_token: " + compMailerSend},
		}
		for _, f := range formats {
			got := m0(compPatMailerSend, f.input)
			recordResult(t, "MailerSend", f.name, compMailerSend, got)
		}
		// Length check: mlsn.(5) + 68 alphanum chars = 73 total
		if len(compMailerSend) != 73 {
			t.Errorf("MailerSend: dummy credential length=%d want 73", len(compMailerSend))
		}
	})

	// ------------------------------------------------------------------ Mailjet
	t.Run("Mailjet_Public", func(t *testing.T) {
		formats := []struct{ name, input string }{
			{"env_MJ_APIKEY_PUBLIC",   "MJ_APIKEY_PUBLIC=" + compMJPub},
			{"env_MAILJET_API_KEY",    "MAILJET_API_KEY=" + compMJPub},
			{"env_MJ_APIKEY",          "MJ_APIKEY=" + compMJPub},
			{"json",                   `"mailjet_api_key": "` + compMJPub + `"`},
			{"yaml",                   "mailjet_key: " + compMJPub},
		}
		for _, f := range formats {
			got := g1(compPatMailjet, f.input)
			recordResult(t, "Mailjet_Public", f.name, compMJPub, got)
		}
	})

	t.Run("Mailjet_Private", func(t *testing.T) {
		formats := []struct{ name, input string }{
			{"env_MJ_APIKEY_PRIVATE", "MJ_APIKEY_PRIVATE=" + compMJPriv},
			{"json",                  `"mailjet_api_secret": "` + compMJPriv + `"`},
		}
		for _, f := range formats {
			got := g1(compPatMailjet, f.input)
			recordResult(t, "Mailjet_Private", f.name, compMJPriv, got)
		}
		// group1 = bare 32-hex value, no prefix
		t.Run("group1_is_32hex_only", func(t *testing.T) {
			got := g1(compPatMailjet, "MJ_APIKEY_PUBLIC="+compMJPub)
			if len(got) != 32 {
				t.Errorf("Mailjet group1 length=%d want 32", len(got))
			}
			if strings.ContainsAny(got, `'"=`) {
				t.Errorf("Mailjet group1 contains separator char: %q", got)
			}
		})
	})

	// ------------------------------------------------------------------ Postmark
	t.Run("Postmark", func(t *testing.T) {
		formats := []struct{ name, input string }{
			{"env_POSTMARK_SERVER_TOKEN", "POSTMARK_SERVER_TOKEN=" + compPostmark},
			{"env_PM_SERVER_TOKEN",       "PM_SERVER_TOKEN=" + compPostmark},
			{"json",                      `"postmark_server_token": "` + compPostmark + `"`},
			{"php",                       `$postmark_token = '` + compPostmark + `';`},
			{"yaml",                      "postmark_token: " + compPostmark},
		}
		for _, f := range formats {
			got := g1(compPatPostmark, f.input)
			recordResult(t, "Postmark", f.name, compPostmark, got)
		}
		// group1 must be UUID only — not the full POSTMARK_SERVER_TOKEN=uuid string
		t.Run("group1_is_uuid_only", func(t *testing.T) {
			got := g1(compPatPostmark, "POSTMARK_SERVER_TOKEN="+compPostmark)
			if got != compPostmark {
				t.Errorf("Postmark: group1=%q want UUID=%q", got, compPostmark)
			}
			if strings.HasPrefix(got, "POSTMARK") {
				t.Errorf("Postmark: group1 must not include the env-var prefix")
			}
		})
	})

	// ------------------------------------------------------------------ SparkPost
	t.Run("SparkPost", func(t *testing.T) {
		formats := []struct{ name, input string }{
			{"env_SPARKPOST_API_KEY", "SPARKPOST_API_KEY=" + compSparkPost},
			{"env_SP_API_KEY",        "SP_API_KEY=" + compSparkPost},
			{"json",                  `"sparkpost_api_key": "` + compSparkPost + `"`},
			{"php",                   `$sparkpost_api_key = '` + compSparkPost + `';`},
			{"yaml",                  "sparkpost_key: " + compSparkPost},
		}
		for _, f := range formats {
			got := g1(compPatSparkPost, f.input)
			recordResult(t, "SparkPost", f.name, compSparkPost, got)
		}
		// group1 must be just the 40-char hex
		t.Run("group1_is_hex_only", func(t *testing.T) {
			got := g1(compPatSparkPost, "SPARKPOST_API_KEY="+compSparkPost)
			if len(got) != 40 {
				t.Errorf("SparkPost group1 length=%d want 40", len(got))
			}
		})
	})

	// ------------------------------------------------------------------ Mailtrap
	t.Run("Mailtrap", func(t *testing.T) {
		formats := []struct{ name, input string }{
			{"env_MAILTRAP_API_TOKEN", "MAILTRAP_API_TOKEN=" + compMailtrap},
			{"env_MT_TOKEN",           "MT_TOKEN=" + compMailtrap},
			{"json",                   `"mailtrap_api_token": "` + compMailtrap + `"`},
			{"php",                    `$mailtrap_key = '` + compMailtrap + `';`},
			{"yaml",                   "mailtrap_token: " + compMailtrap},
		}
		for _, f := range formats {
			got := g1(compPatMailtrap, f.input)
			recordResult(t, "Mailtrap", f.name, compMailtrap, got)
		}
	})

	// ------------------------------------------------------------------ OpenAI
	t.Run("OpenAI_Modern", func(t *testing.T) {
		formats := []struct{ name, input string }{
			{"env",  "OPENAI_API_KEY=" + compOpenAI},
			{"json", `{"openai_key": "` + compOpenAI + `"}`},
			{"php",  `define('OPENAI_KEY', '` + compOpenAI + `');`},
			{"js",   `var k="` + compOpenAI + `"`},
			{"yaml", "openai_api_key: " + compOpenAI},
		}
		for _, f := range formats {
			got := m0(compPatOpenAI, f.input)
			recordResult(t, "OpenAI_Modern", f.name, compOpenAI, got)
		}
	})

	t.Run("OpenAI_Legacy_T3BlbkFJ", func(t *testing.T) {
		formats := []struct{ name, input string }{
			{"env",  "OPENAI_API_KEY=" + compOpenAILegacy},
			{"json", `{"api_key": "` + compOpenAILegacy + `"}`},
		}
		for _, f := range formats {
			got := m0(compPatOpenAI, f.input)
			recordResult(t, "OpenAI_Legacy", f.name, compOpenAILegacy, got)
		}
	})

	// ------------------------------------------------------------------ Anthropic
	t.Run("Anthropic", func(t *testing.T) {
		formats := []struct{ name, input string }{
			{"env",  "ANTHROPIC_API_KEY=" + compAnthropic},
			{"json", `{"api_key": "` + compAnthropic + `"}`},
			{"php",  `define('ANTHROPIC_KEY', '` + compAnthropic + `');`},
			{"js",   `var k="` + compAnthropic + `"`},
			{"yaml", "anthropic_api_key: " + compAnthropic},
		}
		for _, f := range formats {
			got := m0(compPatAnthropic, f.input)
			recordResult(t, "Anthropic", f.name, compAnthropic, got)
		}
		// Negative: short key (< 86 chars after api03-)
		t.Run("negative_short_key", func(t *testing.T) {
			short := "sk-ant-api03-ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopq"
			if m0(compPatAnthropic, short) != "" {
				t.Errorf("Anthropic: short key (50 chars after api03-) must not match")
			}
		})
	})

	// ------------------------------------------------------------------ Twilio
	t.Run("Twilio_SID", func(t *testing.T) {
		formats := []struct{ name, input string }{
			{"env",  "TWILIO_ACCOUNT_SID=" + compTwilioSID},
			{"json", `"account_sid": "` + compTwilioSID + `"`},
			{"php",  `$sid = '` + compTwilioSID + `';`},
			{"js",   `var sid="` + compTwilioSID + `"`},
			{"yaml", "twilio_account_sid: " + compTwilioSID},
		}
		for _, f := range formats {
			got := m0(compPatTwilioSID, f.input)
			recordResult(t, "Twilio_SID", f.name, compTwilioSID, got)
		}
	})

	t.Run("Twilio_AuthToken", func(t *testing.T) {
		formats := []struct{ name, input string }{
			{"env",  "TWILIO_AUTH_TOKEN=" + compTwilioAuth},
			{"json", `"auth_token": "` + compTwilioAuth + `"`},
			{"php",  `$auth = '` + compTwilioAuth + `';`},
			{"js_dq", `auth_token = "` + compTwilioAuth + `"`},
			{"yaml", "auth_token: " + compTwilioAuth},
			// Quoted — confirm extraction strips quotes
			{"quoted_single", `auth_token = '` + compTwilioAuth + `'`},
			{"quoted_double", `"auth_token": "` + compTwilioAuth + `"`},
		}
		for _, f := range formats {
			got := g1(compPatTwilioAuth, f.input)
			recordResult(t, "Twilio_AuthToken", f.name, compTwilioAuth, got)
		}
		// Verify no quote chars in group 1
		t.Run("group1_no_quotes", func(t *testing.T) {
			got := g1(compPatTwilioAuth, `auth_token = '`+compTwilioAuth+`'`)
			if len(got) > 0 && (got[0] == '\'' || got[0] == '"') {
				t.Errorf("Twilio auth: group1 starts with quote: %q", got)
			}
		})
	})

	// ------------------------------------------------------------------ Nexmo / Vonage
	t.Run("Nexmo_Key", func(t *testing.T) {
		formats := []struct{ name, input string }{
			{"env_NEXMO",  "NEXMO_API_KEY=" + compNexmoKey},
			{"env_VONAGE", "VONAGE_API_KEY=" + compNexmoKey},
			{"json_NEXMO", `NEXMO_API_KEY: "` + compNexmoKey + `"`},
			{"yaml_VONAGE", "VONAGE_API_KEY: " + compNexmoKey},
			{"php",        `NEXMO_API_KEY='` + compNexmoKey + `'`},
		}
		for _, f := range formats {
			got := g2(compPatNexmoKey, f.input)
			recordResult(t, "Nexmo_Key", f.name, compNexmoKey, got)
		}
	})

	t.Run("Nexmo_Secret", func(t *testing.T) {
		formats := []struct{ name, input string }{
			{"env_NEXMO",  "NEXMO_API_SECRET=" + compNexmoSecret},
			{"env_VONAGE", "VONAGE_API_SECRET=" + compNexmoSecret},
			{"json_NEXMO", `NEXMO_API_SECRET: "` + compNexmoSecret + `"`},
			{"yaml_VONAGE", "VONAGE_API_SECRET: " + compNexmoSecret},
		}
		for _, f := range formats {
			got := g2(compPatNexmoSecret, f.input)
			recordResult(t, "Nexmo_Secret", f.name, compNexmoSecret, got)
		}
	})

	// ------------------------------------------------------------------ Telnyx
	t.Run("Telnyx", func(t *testing.T) {
		formats := []struct{ name, input string }{
			{"env",  "TELNYX_API_KEY=" + compTelnyx},
			{"json", `{"api_key": "` + compTelnyx + `"}`},
			{"php",  `$key = '` + compTelnyx + `';`},
			{"js",   `var k="` + compTelnyx + `"`},
			{"yaml", "telnyx_api_key: " + compTelnyx},
		}
		for _, f := range formats {
			got := m0(compPatTelnyx, f.input)
			recordResult(t, "Telnyx", f.name, compTelnyx, got)
		}
		// CRITICAL: uppercase hex after KEY must NOT match (pattern requires [0-9a-f])
		t.Run("negative_uppercase_hex", func(t *testing.T) {
			upper := "KEYABCDEF1234567890ABCDEF12345678"
			if m0(compPatTelnyx, upper) != "" {
				t.Errorf("Telnyx: UPPERCASE hex after KEY must not match; got match")
			}
		})
	})

	// ------------------------------------------------------------------ MessageBird
	t.Run("MessageBird", func(t *testing.T) {
		formats := []struct{ name, input, want string }{
			{"env_live",   "MESSAGEBIRD_API_KEY=" + compMBLive, compMBLive},
			{"env_test",   "MESSAGEBIRD_API_KEY=" + compMBTest, compMBTest},
			{"json_live",  `{"api_key": "` + compMBLive + `"}`, compMBLive},
			{"php_live",   `$key = '` + compMBLive + `';`, compMBLive},
			{"js_test",    `var k="` + compMBTest + `"`, compMBTest},
			{"yaml_live",  "mb_key: " + compMBLive, compMBLive},
		}
		for _, f := range formats {
			got := m0(compPatMessageBird, f.input)
			recordResult(t, "MessageBird", f.name, f.want, got)
		}
	})

	// ------------------------------------------------------------------ Plivo
	t.Run("Plivo", func(t *testing.T) {
		formats := []struct{ name, input string }{
			{"env_MA",  "PLIVO_AUTH_ID=" + compPlivo},
			{"json",    `"plivo_auth_id": "` + compPlivo + `"`},
			{"php",     `$id = 'plivo_auth_id=` + compPlivo + `';`},
			{"yaml",    "plivo_auth_id: " + compPlivo},
		}
		for _, f := range formats {
			got := g1(compPatPlivo, f.input)
			recordResult(t, "Plivo", f.name, compPlivo, got)
		}
		// SA prefix variant
		t.Run("SA_prefix", func(t *testing.T) {
			sa := "SA1B2C3D4E5F1B2C3D4E"
			input := "PLIVO_AUTH_SID=" + sa
			got := g1(compPatPlivo, input)
			if got != sa {
				t.Errorf("Plivo SA prefix: got=%q want=%q", got, sa)
			}
		})
		// group1 = Auth ID only, no env-var prefix
		t.Run("group1_no_prefix", func(t *testing.T) {
			got := g1(compPatPlivo, "PLIVO_AUTH_ID="+compPlivo)
			if got != compPlivo {
				t.Errorf("Plivo group1=%q want=%q", got, compPlivo)
			}
			if strings.HasPrefix(got, "PLIVO") || strings.HasPrefix(got, "plivo") {
				t.Errorf("Plivo group1 leaked keyword: %q", got)
			}
		})
	})

	// ------------------------------------------------------------------ SMTP (contextual detection)
	t.Run("SMTP_env_file", func(t *testing.T) {
		// SMTP detection uses separate patterns for host/user/pass/port.
		// Verify a realistic .env file returns correct values for each.
		dotenv := `MAIL_HOST=smtp.sendgrid.net
MAIL_USERNAME=apikey
MAIL_PASSWORD=` + compSendGrid + `
MAIL_PORT=587`

		hostRe := regexp.MustCompile(`(?i)MAIL_HOST\s*[:=]\s*([^\s'"]+)`)
		userRe := regexp.MustCompile(`(?i)MAIL_USERNAME\s*[:=]\s*([^\s'"]+)`)
		passRe := regexp.MustCompile(`(?i)MAIL_PASSWORD\s*[:=]\s*([^\s'"]+)`)
		portRe := regexp.MustCompile(`(?i)MAIL_PORT\s*[:=]\s*([0-9]+)`)

		type smtpCheck struct {
			name, want string
			re         *regexp.Regexp
		}
		checks := []smtpCheck{
			{"MAIL_HOST",     "smtp.sendgrid.net", hostRe},
			{"MAIL_USERNAME", "apikey",            userRe},
			{"MAIL_PASSWORD", compSendGrid,        passRe},
			{"MAIL_PORT",     "587",               portRe},
		}
		for _, c := range checks {
			got := g1(c.re, dotenv)
			recordResult(t, "SMTP", c.name, c.want, got)
		}
	})

	// ------------------------------------------------------------------ Print summary table
	t.Run("SUMMARY_TABLE", func(t *testing.T) {
		t.Log("\n" + summaryTable())
	})
}

// summaryTable formats allResults into a human-readable table.
func summaryTable() string {
	var sb strings.Builder
	hdr := "%-25s | %-22s | %-8s | %s"
	row := "%-25s | %-22s | %-8s | %s"
	sb.WriteString("Comprehensive detector audit results\n")
	sb.WriteString(strings.Repeat("=", 90) + "\n")
	sb.WriteString(fmt.Sprintf(hdr+"\n", "ADDON", "FORMAT", "STATUS", "EXTRACTED VALUE"))
	sb.WriteString(strings.Repeat("-", 90) + "\n")
	for _, r := range allResults {
		status := "PASS"
		if r.got != r.want {
			status = "FAIL"
		}
		// Truncate long values for readability
		display := r.got
		if len(display) > 50 {
			display = display[:47] + "..."
		}
		if display == "" {
			display = "(no match)"
		}
		sb.WriteString(fmt.Sprintf(row+"\n", r.addon, r.format, status, display))
	}
	sb.WriteString(strings.Repeat("=", 90) + "\n")
	return sb.String()
}
