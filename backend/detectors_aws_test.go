package main

import (
	"regexp"
	"testing"
)

// Patterns compiled directly from main.go initialization (NewAWSScanner)

var (
	// AWS patterns
	patAWSSESUser      = regexp.MustCompile(`\b(AKIA|ASIA)[A-Z0-9]{16}\b`)
	patAWSSecretKeyInfo = regexp.MustCompile(`\b[A-Za-z0-9/+=]{40}\b`)
	patAWSSessionToken  = regexp.MustCompile(`['"]([A-Za-z0-9/+=]{100,})['"]`)
	patAWSAccessKey     = regexp.MustCompile(`['"](AKIA[0-9A-Z]{16})['"]`)

	// Email / API patterns
	patSendGrid    = regexp.MustCompile(`SG\.[0-9A-Za-z\-_]{22}\.[0-9A-Za-z\-_]{43}`)
	patStripe      = regexp.MustCompile(`(?:sk_live_|sk_test_|rk_live_|rk_test_)[0-9a-zA-Z]{16,99}`)
	patMailgun     = regexp.MustCompile(`(?:key-[0-9a-zA-Z]{32}|(?i)(?:MAILGUN[_\-]?(?:API[_\-]?)?(?:KEY|SECRET|TOKEN)|MG_API_KEY)["'\s:=]+([a-zA-Z0-9\-]{20,50}))`)
	patNewMailgun  = regexp.MustCompile(`[a-f0-9]{32}-[0-9a-f]{8}-[a-f0-9]{8}`)
	patOpenAI      = regexp.MustCompile(`sk-(?:proj-|o1-|svcacct-)?[a-zA-Z0-9]{20,}(?:T3BlbkFJ[a-zA-Z0-9]{20,}|[a-zA-Z0-9_-]{28,})`)
	patAnthropic   = regexp.MustCompile(`sk-ant-(?:api\d+-[A-Za-z0-9_-]{86,}|[A-Za-z0-9_-]{92,})`)
)

// Test fixture credentials (safe/fake values for unit testing only)
const (
	akiaKey      = "AKIA0000000000000000"  // AKIA + 16 zeros = valid format, obviously fake
	asiaKey      = "ASIA0000000000000000"  // ASIA + 16 zeros = valid format, obviously fake
	awsSecret    = "0000000000000000000000000000000000000000" // 40 zeros
	// 102 char session token of zeros
	awsToken     = "0000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000"
	// SendGrid: SG.0000.0000
	sendGridKey  = "SG.00000000000000000000.000000000000000000000000000000000000000000"
	stripeLive   = "sk_live_0000000000000000"
	stripeTest   = "sk_test_0000000000000000"
	// Mailgun legacy: key-00000000000000000000000000000000
	mailgunLegacy = "key-00000000000000000000000000000000"
	mailgunNew   = "00000000000000000000000000000000-00000000-00000000"
	// OpenAI: sk-proj-0000...
	openAIKey    = "sk-proj-00000000000000000000000000000000000000"
	anthropicKey = "sk-ant-api03-000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000"
)

// ---------------------------------------------------------------------------
// AWSSESUserPattern: \b(AKIA|ASIA)[A-Z0-9]{16}\b — no-quote match, group 1 = prefix
// ---------------------------------------------------------------------------

func TestAWSKeyPatterns(t *testing.T) {
	t.Run("AKIA_in_env_file", func(t *testing.T) {
		input := "AWS_ACCESS_KEY_ID=" + akiaKey
		ms := patAWSSESUser.FindAllStringSubmatch(input, -1)
		if len(ms) == 0 {
			t.Fatalf("no match in: %q", input)
		}
		if ms[0][0] != akiaKey {
			t.Errorf("full match = %q, want %q", ms[0][0], akiaKey)
		}
		if ms[0][1] != "AKIA" {
			t.Errorf("group 1 = %q, want AKIA", ms[0][1])
		}
	})

	t.Run("ASIA_in_env_file", func(t *testing.T) {
		input := "AWS_ACCESS_KEY_ID=" + asiaKey
		ms := patAWSSESUser.FindAllStringSubmatch(input, -1)
		if len(ms) == 0 {
			t.Fatalf("no match in: %q", input)
		}
		if ms[0][0] != asiaKey {
			t.Errorf("full match = %q, want %q", ms[0][0], asiaKey)
		}
		if ms[0][1] != "ASIA" {
			t.Errorf("group 1 = %q, want ASIA", ms[0][1])
		}
	})

	t.Run("AKIA_in_JSON", func(t *testing.T) {
		input := `{"access_key": "` + akiaKey + `"}`
		ms := patAWSSESUser.FindAllStringSubmatch(input, -1)
		if len(ms) == 0 {
			t.Fatalf("no match in: %q", input)
		}
		if ms[0][0] != akiaKey {
			t.Errorf("got %q, want %q", ms[0][0], akiaKey)
		}
	})

	t.Run("AKIA_in_PHP_define", func(t *testing.T) {
		input := `define('AWS_KEY', '` + akiaKey + `');`
		ms := patAWSSESUser.FindAllStringSubmatch(input, -1)
		if len(ms) == 0 {
			t.Fatalf("no match in: %q", input)
		}
		if ms[0][0] != akiaKey {
			t.Errorf("got %q, want %q", ms[0][0], akiaKey)
		}
	})

	t.Run("AKIA_in_YAML", func(t *testing.T) {
		input := "aws_access_key_id: " + akiaKey
		ms := patAWSSESUser.FindAllStringSubmatch(input, -1)
		if len(ms) == 0 {
			t.Fatalf("no match in: %q", input)
		}
		if ms[0][0] != akiaKey {
			t.Errorf("got %q, want %q", ms[0][0], akiaKey)
		}
	})

	t.Run("no_match_on_short_key", func(t *testing.T) {
		// Only 10 alphanum after prefix — must not match
		input := "AKIASHORT1234"
		ms := patAWSSESUser.FindAllString(input, -1)
		if len(ms) != 0 {
			t.Errorf("unexpected match: %v", ms)
		}
	})

	t.Run("no_match_on_lowercase_prefix", func(t *testing.T) {
		input := "akiaiosfodnn7example"
		ms := patAWSSESUser.FindAllString(input, -1)
		if len(ms) != 0 {
			t.Errorf("unexpected match (pattern is case-sensitive): %v", ms)
		}
	})
}

// ---------------------------------------------------------------------------
// AWSSecretKeyPatternInfo: \b[A-Za-z0-9/+=]{40}\b
// ---------------------------------------------------------------------------

func TestAWSSecretKeyPattern(t *testing.T) {
	t.Run("secret_in_env_file", func(t *testing.T) {
		input := "AWS_SECRET_ACCESS_KEY=" + awsSecret
		ms := patAWSSecretKeyInfo.FindAllString(input, -1)
		found := false
		for _, m := range ms {
			if m == awsSecret {
				found = true
			}
		}
		if !found {
			t.Errorf("secret not found in matches %v for input %q", ms, input)
		}
	})

	t.Run("secret_in_JSON", func(t *testing.T) {
		input := `{"secret_key": "` + awsSecret + `"}`
		ms := patAWSSecretKeyInfo.FindAllString(input, -1)
		found := false
		for _, m := range ms {
			if m == awsSecret {
				found = true
			}
		}
		if !found {
			t.Errorf("secret not found in matches %v", ms)
		}
	})

	t.Run("secret_in_PHP_define", func(t *testing.T) {
		input := `define('AWS_SECRET', '` + awsSecret + `');`
		ms := patAWSSecretKeyInfo.FindAllString(input, -1)
		found := false
		for _, m := range ms {
			if m == awsSecret {
				found = true
			}
		}
		if !found {
			t.Errorf("secret not found in matches %v", ms)
		}
	})

	t.Run("no_match_on_39_chars", func(t *testing.T) {
		// 39 chars — one short
		input := "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEK"
		ms := patAWSSecretKeyInfo.FindAllString(input, -1)
		for _, m := range ms {
			if len(m) == 39 {
				t.Errorf("should not match 39-char string, got %q", m)
			}
		}
	})
}

// ---------------------------------------------------------------------------
// AWSSessionTokenPattern: ['"]([A-Za-z0-9/+=]{100,})['"]
// group 1 = token without quotes
// ---------------------------------------------------------------------------

func TestAWSSessionTokenPattern(t *testing.T) {
	t.Run("token_in_single_quotes", func(t *testing.T) {
		input := "session_token='" + awsToken + "'"
		ms := patAWSSessionToken.FindAllStringSubmatch(input, -1)
		if len(ms) == 0 {
			t.Fatalf("no match in: %q", input)
		}
		if ms[0][1] != awsToken {
			t.Errorf("group 1 = %q, want %q", ms[0][1], awsToken)
		}
	})

	t.Run("token_in_double_quotes", func(t *testing.T) {
		input := `"session_token": "` + awsToken + `"`
		ms := patAWSSessionToken.FindAllStringSubmatch(input, -1)
		if len(ms) == 0 {
			t.Fatalf("no match in: %q", input)
		}
		if ms[0][1] != awsToken {
			t.Errorf("group 1 = %q, want %q", ms[0][1], awsToken)
		}
	})

	t.Run("token_in_JS_assignment", func(t *testing.T) {
		input := `const sessionToken = "` + awsToken + `";`
		ms := patAWSSessionToken.FindAllStringSubmatch(input, -1)
		if len(ms) == 0 {
			t.Fatalf("no match in: %q", input)
		}
		if ms[0][1] != awsToken {
			t.Errorf("group 1 = %q, want %q", ms[0][1], awsToken)
		}
	})

	t.Run("no_match_without_quotes", func(t *testing.T) {
		// Token not wrapped in quotes — should not match
		input := "session_token=" + awsToken
		ms := patAWSSessionToken.FindAllString(input, -1)
		if len(ms) != 0 {
			t.Errorf("unexpected match without quotes: %v", ms)
		}
	})

	t.Run("no_match_on_short_token", func(t *testing.T) {
		// Only 50 chars — below the 100-char minimum
		short := "AQoXnyc4lcK4ZIAAAAAAAAABBBBvkFk/dpKyUGSRc4qr1TFH"
		input := "'" + short + "'"
		ms := patAWSSessionToken.FindAllString(input, -1)
		if len(ms) != 0 {
			t.Errorf("unexpected match on short token: %v", ms)
		}
	})
}

// ---------------------------------------------------------------------------
// AWSAccessKeyPattern: ['"](AKIA[0-9A-Z]{16})['"]
// group 1 = AKIA key without quotes
// ---------------------------------------------------------------------------

func TestAWSAccessKeyPattern(t *testing.T) {
	t.Run("AKIA_in_single_quotes", func(t *testing.T) {
		input := "define('AWS_KEY', '" + akiaKey + "');"
		ms := patAWSAccessKey.FindAllStringSubmatch(input, -1)
		if len(ms) == 0 {
			t.Fatalf("no match in: %q", input)
		}
		if ms[0][1] != akiaKey {
			t.Errorf("group 1 = %q, want %q", ms[0][1], akiaKey)
		}
	})

	t.Run("AKIA_in_double_quotes", func(t *testing.T) {
		input := `"access_key": "` + akiaKey + `"`
		ms := patAWSAccessKey.FindAllStringSubmatch(input, -1)
		if len(ms) == 0 {
			t.Fatalf("no match in: %q", input)
		}
		if ms[0][1] != akiaKey {
			t.Errorf("group 1 = %q, want %q", ms[0][1], akiaKey)
		}
	})

	t.Run("AKIA_in_JS_assignment", func(t *testing.T) {
		input := `const accessKey = '` + akiaKey + `';`
		ms := patAWSAccessKey.FindAllStringSubmatch(input, -1)
		if len(ms) == 0 {
			t.Fatalf("no match in: %q", input)
		}
		if ms[0][1] != akiaKey {
			t.Errorf("group 1 = %q, want %q", ms[0][1], akiaKey)
		}
	})

	t.Run("ASIA_not_matched_by_AKIA_pattern", func(t *testing.T) {
		// AWSAccessKeyPattern only matches AKIA, not ASIA
		input := `"key": "` + asiaKey + `"`
		ms := patAWSAccessKey.FindAllString(input, -1)
		if len(ms) != 0 {
			t.Errorf("ASIA key should not match AWSAccessKeyPattern, got: %v", ms)
		}
	})

	t.Run("no_match_without_quotes", func(t *testing.T) {
		input := "AWS_ACCESS_KEY_ID=" + akiaKey
		ms := patAWSAccessKey.FindAllString(input, -1)
		if len(ms) != 0 {
			t.Errorf("expected no match without surrounding quotes, got: %v", ms)
		}
	})
}

// ---------------------------------------------------------------------------
// SendGrid: SG\.[0-9A-Za-z\-_]{22}\.[0-9A-Za-z\-_]{43}
// ---------------------------------------------------------------------------

func TestSendGridPattern(t *testing.T) {
	t.Run("sendgrid_in_env_file", func(t *testing.T) {
		input := "SENDGRID_API_KEY=" + sendGridKey
		ms := patSendGrid.FindAllString(input, -1)
		if len(ms) == 0 {
			t.Fatalf("no match in: %q", input)
		}
		if ms[0] != sendGridKey {
			t.Errorf("got %q, want %q", ms[0], sendGridKey)
		}
	})

	t.Run("sendgrid_in_JSON", func(t *testing.T) {
		input := `{"api_key": "` + sendGridKey + `"}`
		ms := patSendGrid.FindAllString(input, -1)
		if len(ms) == 0 {
			t.Fatalf("no match in: %q", input)
		}
		if ms[0] != sendGridKey {
			t.Errorf("got %q, want %q", ms[0], sendGridKey)
		}
	})

	t.Run("sendgrid_in_JS_assignment", func(t *testing.T) {
		input := "const apiKey = '" + sendGridKey + "'"
		ms := patSendGrid.FindAllString(input, -1)
		if len(ms) == 0 {
			t.Fatalf("no match in: %q", input)
		}
		if ms[0] != sendGridKey {
			t.Errorf("got %q, want %q", ms[0], sendGridKey)
		}
	})

	t.Run("sendgrid_in_YAML", func(t *testing.T) {
		input := "api_key: " + sendGridKey
		ms := patSendGrid.FindAllString(input, -1)
		if len(ms) == 0 {
			t.Fatalf("no match in: %q", input)
		}
		if ms[0] != sendGridKey {
			t.Errorf("got %q, want %q", ms[0], sendGridKey)
		}
	})

	t.Run("no_match_on_short_second_segment", func(t *testing.T) {
		// Second segment only 10 chars (needs 43)
		input := "SG.ngeVaQIVTJqNMK-MNLIT3g.short12345"
		ms := patSendGrid.FindAllString(input, -1)
		if len(ms) != 0 {
			t.Errorf("unexpected match: %v", ms)
		}
	})
}

// ---------------------------------------------------------------------------
// Stripe: (?:sk_live_|sk_test_|rk_live_|rk_test_)[0-9a-zA-Z]{16,99}
// ---------------------------------------------------------------------------

func TestStripePattern(t *testing.T) {
	t.Run("stripe_live_in_env_file", func(t *testing.T) {
		input := "STRIPE_SECRET_KEY=" + stripeLive
		ms := patStripe.FindAllString(input, -1)
		if len(ms) == 0 {
			t.Fatalf("no match in: %q", input)
		}
		if ms[0] != stripeLive {
			t.Errorf("got %q, want %q", ms[0], stripeLive)
		}
	})

	t.Run("stripe_test_in_JSON", func(t *testing.T) {
		input := `{"stripe_key": "` + stripeTest + `"}`
		ms := patStripe.FindAllString(input, -1)
		if len(ms) == 0 {
			t.Fatalf("no match in: %q", input)
		}
		if ms[0] != stripeTest {
			t.Errorf("got %q, want %q", ms[0], stripeTest)
		}
	})

	t.Run("stripe_live_in_PHP_define", func(t *testing.T) {
		input := "define('STRIPE_KEY', '" + stripeLive + "');"
		ms := patStripe.FindAllString(input, -1)
		if len(ms) == 0 {
			t.Fatalf("no match in: %q", input)
		}
		if ms[0] != stripeLive {
			t.Errorf("got %q, want %q", ms[0], stripeLive)
		}
	})

	t.Run("stripe_test_in_JS_assignment", func(t *testing.T) {
		input := "const stripe = '" + stripeTest + "'"
		ms := patStripe.FindAllString(input, -1)
		if len(ms) == 0 {
			t.Fatalf("no match in: %q", input)
		}
		if ms[0] != stripeTest {
			t.Errorf("got %q, want %q", ms[0], stripeTest)
		}
	})

	t.Run("no_match_on_wrong_prefix", func(t *testing.T) {
		input := "pk_live_51ABC123DEFGHIJKLMNOPQRSTuvwxyz"
		ms := patStripe.FindAllString(input, -1)
		if len(ms) != 0 {
			t.Errorf("pk_ prefix should not match: %v", ms)
		}
	})

	t.Run("no_match_on_short_suffix", func(t *testing.T) {
		// Only 10 chars after prefix (needs 16+)
		input := "sk_live_shortkey"
		ms := patStripe.FindAllString(input, -1)
		if len(ms) != 0 {
			t.Errorf("unexpected match on short key: %v", ms)
		}
	})
}

// ---------------------------------------------------------------------------
// Mailgun legacy: (?:key-[0-9a-zA-Z]{32}|...) — two match branches with different capture behavior
//
// Branch 1 (key- prefix): full match IS the key, group 1 = ""
// Branch 2 (context keyword): full match includes keyword+separator, group 1 = credential
//
// When `MAILGUN_API_KEY=key-...` appears in input, the context-keyword branch fires
// (because MAILGUN_API_KEY appears before =), so group 1 contains the key value.
// When `key-...` appears without a MAILGUN context prefix, branch 1 fires and
// the full match IS the key with group 1 = "".
// ---------------------------------------------------------------------------

func TestMailgunPattern(t *testing.T) {
	t.Run("legacy_key_in_env_file", func(t *testing.T) {
		// MAILGUN_API_KEY=key-... triggers the context-keyword branch.
		// group 1 captures the credential (key-...) without the surrounding quotes/spaces.
		input := "MAILGUN_API_KEY=" + mailgunLegacy
		ms := patMailgun.FindAllStringSubmatch(input, -1)
		if len(ms) == 0 {
			t.Fatalf("no match in: %q", input)
		}
		if ms[0][1] != mailgunLegacy {
			t.Errorf("group 1 = %q, want %q", ms[0][1], mailgunLegacy)
		}
	})

	t.Run("legacy_key_bare", func(t *testing.T) {
		// A bare key- value (no MAILGUN_ context) triggers the key- branch.
		// Full match IS the key; group 1 = "".
		input := "some_var=" + mailgunLegacy
		ms := patMailgun.FindAllStringSubmatch(input, -1)
		if len(ms) == 0 {
			t.Fatalf("no match in: %q", input)
		}
		if ms[0][0] != mailgunLegacy {
			t.Errorf("full match = %q, want %q", ms[0][0], mailgunLegacy)
		}
		if ms[0][1] != "" {
			t.Errorf("group 1 should be empty for key- branch, got %q", ms[0][1])
		}
	})

	t.Run("legacy_key_in_JSON", func(t *testing.T) {
		// In JSON there is no MAILGUN_ keyword context, so the key- branch fires.
		input := `{"api_key": "` + mailgunLegacy + `"}`
		ms := patMailgun.FindAllStringSubmatch(input, -1)
		if len(ms) == 0 {
			t.Fatalf("no match in: %q", input)
		}
		if ms[0][0] != mailgunLegacy {
			t.Errorf("full match = %q, want %q", ms[0][0], mailgunLegacy)
		}
	})

	t.Run("context_based_MAILGUN_KEY_env", func(t *testing.T) {
		// Context keyword form: MAILGUN_API_KEY='somevalue'
		val := "somevalidmailgunvalue1234"
		input := "MAILGUN_API_KEY='" + val + "'"
		ms := patMailgun.FindAllStringSubmatch(input, -1)
		if len(ms) == 0 {
			t.Fatalf("no match in: %q", input)
		}
		// group 1 should be the value without quotes
		if ms[0][1] != val {
			t.Errorf("group 1 = %q, want %q", ms[0][1], val)
		}
	})

	t.Run("context_based_MG_API_KEY_env", func(t *testing.T) {
		val := "anothervalidmailgunkey56"
		input := "MG_API_KEY='" + val + "'"
		ms := patMailgun.FindAllStringSubmatch(input, -1)
		if len(ms) == 0 {
			t.Fatalf("no match in: %q", input)
		}
		if ms[0][1] != val {
			t.Errorf("group 1 = %q, want %q", ms[0][1], val)
		}
	})

	t.Run("no_match_on_wrong_prefix", func(t *testing.T) {
		// Not key- prefix and no context keyword
		input := "abc-3ax6xnjp29d4qt0mqr3bqp97e27d9"
		ms := patMailgun.FindAllString(input, -1)
		if len(ms) != 0 {
			t.Errorf("unexpected match: %v", ms)
		}
	})
}

// ---------------------------------------------------------------------------
// NewMailgunAPIKeyPattern: [a-f0-9]{32}-[0-9a-f]{8}-[a-f0-9]{8}
// ---------------------------------------------------------------------------

func TestNewMailgunPattern(t *testing.T) {
	t.Run("new_mailgun_in_env_file", func(t *testing.T) {
		input := "MAILGUN_API_KEY=" + mailgunNew
		ms := patNewMailgun.FindAllString(input, -1)
		if len(ms) == 0 {
			t.Fatalf("no match in: %q", input)
		}
		if ms[0] != mailgunNew {
			t.Errorf("got %q, want %q", ms[0], mailgunNew)
		}
	})

	t.Run("new_mailgun_in_JSON", func(t *testing.T) {
		input := `{"api_key": "` + mailgunNew + `"}`
		ms := patNewMailgun.FindAllString(input, -1)
		if len(ms) == 0 {
			t.Fatalf("no match in: %q", input)
		}
		if ms[0] != mailgunNew {
			t.Errorf("got %q, want %q", ms[0], mailgunNew)
		}
	})

	t.Run("new_mailgun_in_YAML", func(t *testing.T) {
		input := "api_key: " + mailgunNew
		ms := patNewMailgun.FindAllString(input, -1)
		if len(ms) == 0 {
			t.Fatalf("no match in: %q", input)
		}
		if ms[0] != mailgunNew {
			t.Errorf("got %q, want %q", ms[0], mailgunNew)
		}
	})

	t.Run("no_match_on_uppercase", func(t *testing.T) {
		// Pattern only accepts lowercase hex
		input := "A1B2C3D4E5F6A1B2C3D4E5F6A1B2C3D4-12345678-ABCDEF12"
		ms := patNewMailgun.FindAllString(input, -1)
		if len(ms) != 0 {
			t.Errorf("unexpected match on uppercase: %v", ms)
		}
	})

	t.Run("no_match_on_missing_segment", func(t *testing.T) {
		input := "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4-12345678"
		ms := patNewMailgun.FindAllString(input, -1)
		if len(ms) != 0 {
			t.Errorf("unexpected match on incomplete key: %v", ms)
		}
	})
}

// ---------------------------------------------------------------------------
// OpenAI: sk-(?:proj-|o1-|svcacct-)?[a-zA-Z0-9]{20,}(?:T3BlbkFJ[a-zA-Z0-9]{20,}|[a-zA-Z0-9_-]{28,})
// ---------------------------------------------------------------------------

func TestOpenAIPattern(t *testing.T) {
	t.Run("openai_proj_in_env_file", func(t *testing.T) {
		input := "OPENAI_API_KEY=" + openAIKey
		ms := patOpenAI.FindAllString(input, -1)
		if len(ms) == 0 {
			t.Fatalf("no match in: %q", input)
		}
		if ms[0] != openAIKey {
			t.Errorf("got %q, want %q", ms[0], openAIKey)
		}
	})

	t.Run("openai_proj_in_JSON", func(t *testing.T) {
		input := `{"openai_key": "` + openAIKey + `"}`
		ms := patOpenAI.FindAllString(input, -1)
		if len(ms) == 0 {
			t.Fatalf("no match in: %q", input)
		}
		if ms[0] != openAIKey {
			t.Errorf("got %q, want %q", ms[0], openAIKey)
		}
	})

	t.Run("openai_proj_in_JS_assignment", func(t *testing.T) {
		input := "const apiKey = '" + openAIKey + "'"
		ms := patOpenAI.FindAllString(input, -1)
		if len(ms) == 0 {
			t.Fatalf("no match in: %q", input)
		}
		if ms[0] != openAIKey {
			t.Errorf("got %q, want %q", ms[0], openAIKey)
		}
	})

	t.Run("openai_proj_in_YAML", func(t *testing.T) {
		input := "openai_api_key: " + openAIKey
		ms := patOpenAI.FindAllString(input, -1)
		if len(ms) == 0 {
			t.Fatalf("no match in: %q", input)
		}
		if ms[0] != openAIKey {
			t.Errorf("got %q, want %q", ms[0], openAIKey)
		}
	})

	t.Run("no_match_on_wrong_prefix", func(t *testing.T) {
		input := "ak-proj-abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMN_0123456789012345678"
		ms := patOpenAI.FindAllString(input, -1)
		if len(ms) != 0 {
			t.Errorf("unexpected match: %v", ms)
		}
	})
}

// ---------------------------------------------------------------------------
// Anthropic: sk-ant-(?:api\d+-)?[A-Za-z0-9_-]{86,}
// ---------------------------------------------------------------------------

func TestAnthropicPattern(t *testing.T) {
	t.Run("anthropic_in_env_file", func(t *testing.T) {
		input := "ANTHROPIC_API_KEY=" + anthropicKey
		ms := patAnthropic.FindAllString(input, -1)
		if len(ms) == 0 {
			t.Fatalf("no match in: %q", input)
		}
		if ms[0] != anthropicKey {
			t.Errorf("got %q, want %q", ms[0], anthropicKey)
		}
	})

	t.Run("anthropic_in_JSON", func(t *testing.T) {
		input := `{"api_key": "` + anthropicKey + `"}`
		ms := patAnthropic.FindAllString(input, -1)
		if len(ms) == 0 {
			t.Fatalf("no match in: %q", input)
		}
		if ms[0] != anthropicKey {
			t.Errorf("got %q, want %q", ms[0], anthropicKey)
		}
	})

	t.Run("anthropic_in_PHP_define", func(t *testing.T) {
		input := "define('ANTHROPIC_KEY', '" + anthropicKey + "');"
		ms := patAnthropic.FindAllString(input, -1)
		if len(ms) == 0 {
			t.Fatalf("no match in: %q", input)
		}
		if ms[0] != anthropicKey {
			t.Errorf("got %q, want %q", ms[0], anthropicKey)
		}
	})

	t.Run("anthropic_in_JS_assignment", func(t *testing.T) {
		input := "const key = '" + anthropicKey + "'"
		ms := patAnthropic.FindAllString(input, -1)
		if len(ms) == 0 {
			t.Fatalf("no match in: %q", input)
		}
		if ms[0] != anthropicKey {
			t.Errorf("got %q, want %q", ms[0], anthropicKey)
		}
	})

	t.Run("anthropic_in_YAML", func(t *testing.T) {
		input := "anthropic_api_key: " + anthropicKey
		ms := patAnthropic.FindAllString(input, -1)
		if len(ms) == 0 {
			t.Fatalf("no match in: %q", input)
		}
		if ms[0] != anthropicKey {
			t.Errorf("got %q, want %q", ms[0], anthropicKey)
		}
	})

	t.Run("no_match_on_wrong_prefix", func(t *testing.T) {
		input := "sk-openai-api03-abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"
		ms := patAnthropic.FindAllString(input, -1)
		if len(ms) != 0 {
			t.Errorf("unexpected match: %v", ms)
		}
	})

	t.Run("no_match_on_short_key", func(t *testing.T) {
		// Only 50 chars after prefix — below the 86-char minimum
		input := "sk-ant-api03-abcdefghijklmnopqrstuvwxyzABCDEFGHIJ"
		ms := patAnthropic.FindAllString(input, -1)
		if len(ms) != 0 {
			t.Errorf("unexpected match on short key: %v", ms)
		}
	})
}
