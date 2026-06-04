package main

import (
	"regexp"
	"testing"
)

// ---------------------------------------------------------------------------
// Shared dummy credentials (safe/fake values only)
// ---------------------------------------------------------------------------

const (
	twilioSID       = "ACa1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4"
	twilioAuthToken = "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4"
	nexmoKey        = "a1b2c3d4"
	nexmoSecret     = "a1b2c3d4e5f6a1b2"
	telnyxKey       = "KEY017c1e20f0e14d7c977a5a64a6c79f02_SomePadding"
	mbLiveKey       = "live_abcdefghijklmnopqrstuvwxy"
	mbTestKey       = "test_abcdefghijklmnopqrstuvwxy"
	plivoAuthID     = "MAa1b2c3d4e5f6a1b2c3"
)

// ---------------------------------------------------------------------------
// Helper
// ---------------------------------------------------------------------------

func firstGroup1(re *regexp.Regexp, s string) string {
	m := re.FindStringSubmatch(s)
	if len(m) < 2 {
		return ""
	}
	return m[1]
}

// ---------------------------------------------------------------------------
// Twilio SID
// ---------------------------------------------------------------------------

func TestTwilioSIDPattern(t *testing.T) {
	re := regexp.MustCompile(`AC[a-f0-9]{32}`)

	cases := []struct {
		name  string
		input string
		want  string
	}{
		{"env style", "TWILIO_ACCOUNT_SID=" + twilioSID, twilioSID},
		{"json config", `"account_sid": "` + twilioSID + `"`, twilioSID},
		{"php", "$sid = '" + twilioSID + "';", twilioSID},
		{"js", `const sid = "` + twilioSID + `"`, twilioSID},
		{"bare", twilioSID, twilioSID},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got := re.FindString(tc.input)
			if got != tc.want {
				t.Errorf("input=%q: got %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

// No-match cases for SID
func TestTwilioSIDPatternNoMatch(t *testing.T) {
	re := regexp.MustCompile(`AC[a-f0-9]{32}`)
	noMatches := []string{
		"ACZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZ",  // uppercase hex
		"ACa1b2c3",                            // too short
		"a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4",  // missing AC prefix
	}
	for _, s := range noMatches {
		if got := re.FindString(s); got != "" {
			t.Errorf("expected no match for %q, got %q", s, got)
		}
	}
}

// ---------------------------------------------------------------------------
// Twilio Auth Token
// ---------------------------------------------------------------------------

func TestTwilioAuthTokenPattern(t *testing.T) {
	re := regexp.MustCompile(`(?i)['"']?([0-9a-f]{32})['"']?`)

	cases := []struct {
		name  string
		input string
		want  string
	}{
		{"env style", "TWILIO_AUTH_TOKEN=" + twilioAuthToken, twilioAuthToken},
		{"json config", `"auth_token": "` + twilioAuthToken + `"`, twilioAuthToken},
		{"php", "$auth = '" + twilioAuthToken + "';", twilioAuthToken},
		{"js", `const auth = "` + twilioAuthToken + `"`, twilioAuthToken},
		{"bare", twilioAuthToken, twilioAuthToken},
		{"single-quoted token", "auth_token='" + twilioAuthToken + "'", twilioAuthToken},
		{"double-quoted token", `"auth_token": "` + twilioAuthToken + `"`, twilioAuthToken},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got := firstGroup1(re, tc.input)
			if got != tc.want {
				t.Errorf("input=%q: got %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

// Verify quotes are NOT included in group 1
func TestTwilioAuthTokenStripQuotes(t *testing.T) {
	re := regexp.MustCompile(`(?i)['"']?([0-9a-f]{32})['"']?`)

	cases := []struct {
		name  string
		input string
	}{
		{"single-quoted value", "auth_token='" + twilioAuthToken + "'"},
		{"double-quoted json", `"auth_token": "` + twilioAuthToken + `"`},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got := firstGroup1(re, tc.input)
			if got != twilioAuthToken {
				t.Errorf("group 1 must be bare hex; got %q", got)
			}
			// Group 1 must not start or end with a quote
			if len(got) > 0 && (got[0] == '\'' || got[0] == '"') {
				t.Errorf("group 1 starts with a quote: %q", got)
			}
			if len(got) > 0 && (got[len(got)-1] == '\'' || got[len(got)-1] == '"') {
				t.Errorf("group 1 ends with a quote: %q", got)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Twilio Encoded (base64 SID:token)
// ---------------------------------------------------------------------------

func TestTwilioEncodedPattern(t *testing.T) {
	re := regexp.MustCompile(`QU[MN][A-Za-z0-9]{87}==`)

	// A valid encoded token is exactly QU[MN] + 87 alphanum + == (92 chars total)
	// Build a plausible fake: "QUM" + 87 'A's + "=="
	const aRun87 = "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA" // 87 A's
	encoded := "QUM" + aRun87 + "=="
	if len(encoded) != 92 {
		t.Fatalf("test setup error: encoded len=%d, want 92", len(encoded))
	}

	cases := []struct {
		name  string
		input string
		want  bool
	}{
		{"bare token", encoded, true},
		{"in env", "TWILIO_TOKEN=" + encoded, true},
		{"in JSON", `"token":"` + encoded + `"`, true},
		{"QUN variant", "QUN" + aRun87 + "==", true},
		{"wrong prefix", "QUX" + aRun87 + "==", false},
		{"too short", "QUM" + "AAAAAAAAA" + "==", false},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got := re.FindString(tc.input)
			matched := got != ""
			if matched != tc.want {
				t.Errorf("input=%q: matched=%v, want %v (got %q)", tc.input, matched, tc.want, got)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Nexmo / Vonage API Key
// ---------------------------------------------------------------------------

func TestNexmoKeyPattern(t *testing.T) {
	re := regexp.MustCompile(`(?i)(NEXMO_API_KEY|VONAGE_API_KEY)\s*[:=]\s*["']?([a-zA-Z0-9]{8})["\']?`)

	cases := []struct {
		name  string
		input string
		want  string
	}{
		{"nexmo env", "NEXMO_API_KEY=" + nexmoKey, nexmoKey},
		{"vonage env", "VONAGE_API_KEY=" + nexmoKey, nexmoKey},
		// JSON-like: key name unquoted (regex requires keyword directly before [:=])
		{"nexmo json", `NEXMO_API_KEY: "` + nexmoKey + `"`, nexmoKey},
		{"vonage json", `VONAGE_API_KEY: "` + nexmoKey + `"`, nexmoKey},
		{"nexmo php", "NEXMO_API_KEY='" + nexmoKey + "'", nexmoKey},
		{"vonage js", `const VONAGE_API_KEY = "` + nexmoKey + `"`, nexmoKey},
		{"bare key var", "NEXMO_API_KEY:" + nexmoKey, nexmoKey},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			m := re.FindStringSubmatch(tc.input)
			var got string
			if len(m) >= 3 {
				got = m[2]
			}
			if got != tc.want {
				t.Errorf("input=%q: got %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Nexmo / Vonage API Secret
// ---------------------------------------------------------------------------

func TestNexmoSecretPattern(t *testing.T) {
	re := regexp.MustCompile(`(?i)(NEXMO_API_SECRET|VONAGE_API_SECRET)\s*[:=]\s*["\']?([a-zA-Z0-9]{16})["\']?`)

	cases := []struct {
		name  string
		input string
		want  string
	}{
		{"nexmo env", "NEXMO_API_SECRET=" + nexmoSecret, nexmoSecret},
		{"vonage env", "VONAGE_API_SECRET=" + nexmoSecret, nexmoSecret},
		// JSON-like: key name unquoted (regex requires keyword directly before [:=])
		{"nexmo json", `NEXMO_API_SECRET: "` + nexmoSecret + `"`, nexmoSecret},
		{"vonage json", `VONAGE_API_SECRET: "` + nexmoSecret + `"`, nexmoSecret},
		{"nexmo php", "NEXMO_API_SECRET='" + nexmoSecret + "'", nexmoSecret},
		{"vonage js", `const VONAGE_API_SECRET = "` + nexmoSecret + `"`, nexmoSecret},
		{"bare var colon", "NEXMO_API_SECRET:" + nexmoSecret, nexmoSecret},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			m := re.FindStringSubmatch(tc.input)
			var got string
			if len(m) >= 3 {
				got = m[2]
			}
			if got != tc.want {
				t.Errorf("input=%q: got %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Telnyx
// ---------------------------------------------------------------------------

func TestTelnyxPattern(t *testing.T) {
	re := regexp.MustCompile(`KEY[0-9a-f]{20,56}(?:_[A-Za-z0-9_\-]{10,30})?`)

	cases := []struct {
		name  string
		input string
		want  bool
	}{
		{"bare key", telnyxKey, true},
		{"env style", "TELNYX_API_KEY=" + telnyxKey, true},
		{"json config", `"api_key": "` + telnyxKey + `"`, true},
		{"php", "$key = '" + telnyxKey + "';", true},
		{"js", `const key = "` + telnyxKey + `"`, true},
		{"lowercase hex digits", "KEY017c1e20f0e14d7c977a5a64a6c79f02_SomePadding", true},
		// No suffix variant (minimum 20 hex after KEY)
		{"no suffix", "KEY017c1e20f0e14d7c977a5a", true},
		// Negative cases
		{"uppercase hex KEY", "KEYABCDEF1234567890ABCDEF", false}, // uppercase not matched by [0-9a-f]
		{"too short", "KEY01234567890", false},                    // only 11 hex chars after KEY
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got := re.FindString(tc.input)
			matched := got != ""
			if matched != tc.want {
				t.Errorf("input=%q: matched=%v, want %v (got %q)", tc.input, matched, tc.want, got)
			}
		})
	}
}

// Specifically verify lowercase hex is required (the "FIXED" behaviour)
func TestTelnyxLowercaseHexRequired(t *testing.T) {
	re := regexp.MustCompile(`KEY[0-9a-f]{20,56}(?:_[A-Za-z0-9_\-]{10,30})?`)

	lowercaseKey := "KEY017c1e20f0e14d7c977a5a64a6c79f0272b94af5f5f940_SoMeBase64PAdding"
	if got := re.FindString(lowercaseKey); got == "" {
		t.Errorf("expected match for lowercase key %q, got no match", lowercaseKey)
	}

	uppercaseKey := "KEYABCDEF1234567890ABCDEF12345678901234567890AB"
	if got := re.FindString(uppercaseKey); got != "" {
		t.Errorf("expected NO match for uppercase key %q, got %q", uppercaseKey, got)
	}
}

// ---------------------------------------------------------------------------
// MessageBird
// ---------------------------------------------------------------------------

func TestMessageBirdPattern(t *testing.T) {
	re := regexp.MustCompile(`(?:live|test)_[a-zA-Z0-9]{25,40}`)

	cases := []struct {
		name  string
		input string
		want  bool
	}{
		{"live bare", mbLiveKey, true},
		{"test bare", mbTestKey, true},
		{"live env", "MESSAGEBIRD_API_KEY=" + mbLiveKey, true},
		{"test env", "MESSAGEBIRD_API_KEY=" + mbTestKey, true},
		{"live json", `"api_key": "` + mbLiveKey + `"`, true},
		{"test json", `"api_key": "` + mbTestKey + `"`, true},
		{"live php", "$key = '" + mbLiveKey + "';", true},
		{"test js", `const key = "` + mbTestKey + `"`, true},
		// Negative cases
		{"wrong prefix", "prod_abcdefghijklmnopqrstuvwxy", false},
		{"no prefix", "abcdefghijklmnopqrstuvwxy", false},
		{"too short", "live_abc", false},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got := re.FindString(tc.input)
			matched := got != ""
			if matched != tc.want {
				t.Errorf("input=%q: matched=%v, want %v (got %q)", tc.input, matched, tc.want, got)
			}
		})
	}
}

// Verify both live_ and test_ prefixes are explicitly matched
func TestMessageBirdPrefixes(t *testing.T) {
	re := regexp.MustCompile(`(?:live|test)_[a-zA-Z0-9]{25,40}`)

	if got := re.FindString(mbLiveKey); got == "" {
		t.Errorf("live_ key must match: %q", mbLiveKey)
	}
	if got := re.FindString(mbTestKey); got == "" {
		t.Errorf("test_ key must match: %q", mbTestKey)
	}
}

// ---------------------------------------------------------------------------
// Plivo
// ---------------------------------------------------------------------------

func TestPlivoPattern(t *testing.T) {
	re := regexp.MustCompile(`(?i)(?:plivo[_-]?(?:auth[_-]?)?(?:id|sid))["'\s:=]+([MS]A[A-Z0-9]{18})`)

	cases := []struct {
		name  string
		input string
		want  string
	}{
		{"env style", "PLIVO_AUTH_ID=" + plivoAuthID, plivoAuthID},
		{"json config", `"plivo_auth_id": "` + plivoAuthID + `"`, plivoAuthID},
		{"php", "$id = 'plivo_auth_id=" + plivoAuthID + "';", plivoAuthID},
		{"bare context", "plivo_auth_id " + plivoAuthID, plivoAuthID},
		// SA prefix variant — SA + 18 uppercase alphanum = 20 chars total
		{"sa prefix env", "PLIVO_AUTH_SID=SA1B2C3D4E5F1B2C3D4E", "SA1B2C3D4E5F1B2C3D4E"},
		// Case-insensitive keyword
		{"lowercase key", "plivo_auth_id=" + plivoAuthID, plivoAuthID},
		{"mixed case", "Plivo_Auth_Id=" + plivoAuthID, plivoAuthID},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got := firstGroup1(re, tc.input)
			if got != tc.want {
				t.Errorf("input=%q: got %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

// Verify group 1 = only the Auth ID, no surrounding context
func TestPlivoGroup1Extraction(t *testing.T) {
	re := regexp.MustCompile(`(?i)(?:plivo[_-]?(?:auth[_-]?)?(?:id|sid))["'\s:=]+([MS]A[A-Z0-9]{18})`)

	input := "PLIVO_AUTH_ID=" + plivoAuthID
	got := firstGroup1(re, input)
	if got != plivoAuthID {
		t.Errorf("group 1 must be Auth ID only; got %q, want %q", got, plivoAuthID)
	}
	// Must not contain the keyword prefix
	if len(got) > 0 && (got[0] == 'P' || got[0] == 'p') {
		t.Errorf("group 1 must not start with P (keyword leaked into capture): %q", got)
	}
}

// Negative cases: pattern must NOT match unrelated contexts
func TestPlivoNoMatch(t *testing.T) {
	re := regexp.MustCompile(`(?i)(?:plivo[_-]?(?:auth[_-]?)?(?:id|sid))["'\s:=]+([MS]A[A-Z0-9]{18})`)

	noMatches := []string{
		"AUTH_ID=MAa1b2c3d4e5f6a1b2c3",        // no "plivo" keyword
		"PLIVO_AUTH_ID=BAa1b2c3d4e5f6a1b2c3",  // wrong prefix (BA)
		"PLIVO_AUTH_ID=MAshort",                // too short
	}
	for _, s := range noMatches {
		if got := firstGroup1(re, s); got != "" {
			t.Errorf("expected no match for %q, got %q", s, got)
		}
	}
}
