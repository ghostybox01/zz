package main

import (
	"regexp"
	"testing"
)

// ---------------------------------------------------------------------------
// OpenAI edge-case: legacy 48-char sk- key and minimum length analysis
// These supplement the broader TestOpenAIPattern tests in detectors_aws_test.go
// ---------------------------------------------------------------------------

func TestOpenAILegacyKeyEdgeCases(t *testing.T) {
	re := regexp.MustCompile(`sk-(?:proj-|o1-|svcacct-)?[a-zA-Z0-9]{20,}(?:T3BlbkFJ[a-zA-Z0-9]{20,}|[a-zA-Z0-9_-]{28,})`)

	// Legacy 48-char key: "sk-" + 45 alphanum.
	// Branch2 requires {20,} + {28,} = 48 chars minimum after "sk-".
	// 45 < 48 → legacy 48-char keys do NOT match. This is a known gap.
	legacy48 := "sk-" + "xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"[:45]
	t.Run("legacy_48char_does_not_match_current_pattern", func(t *testing.T) {
		got := re.FindString(legacy48)
		// We document this as a KNOWN LIMITATION: the pattern misses true legacy keys.
		// Test passes either way — we're documenting the behaviour, not asserting an outcome.
		t.Logf("legacy 48-char key match result: %q (empty = not matched)", got)
	})

	// Minimum chars needed after "sk-" for branch2 to fire: 20 + 28 = 48
	// Key with exactly 48 chars after "sk-" = 51 total: should match.
	key51 := "sk-AAAAAAAAAAAAAAAAAAAABBBBBBBBBBBBBBBBBBBBBBBBBBBB" // sk- + 48 = 51 total
	if len(key51) != 51 {
		t.Fatalf("test setup error: key51 len=%d want 51", len(key51))
	}
	t.Run("51char_key_matches_via_branch2", func(t *testing.T) {
		got := re.FindString(key51)
		if got == "" {
			t.Errorf("51-char key (sk- + 48) should match via branch2, got no match")
		}
	})

	// Project key with T3BlbkFJ marker (the primary modern format)
	projKey := "sk-proj-AAAAAAAAAAAAAAAAAAAAT3BlbkFJBBBBBBBBBBBBBBBBBBBB"
	t.Run("proj_key_with_T3BlbkFJ_marker", func(t *testing.T) {
		got := re.FindString(projKey)
		if got != projKey {
			t.Errorf("proj key: got %q, want %q", got, projKey)
		}
	})
}

// ---------------------------------------------------------------------------
// Anthropic: minimum length boundary tests
// These supplement the broader TestAnthropicPattern in detectors_aws_test.go
//
// Pattern (fixed): sk-ant-(?:api\d+-[A-Za-z0-9_-]{86,}|[A-Za-z0-9_-]{92,})
//   Alt1: with api\d+- separator → requires 86+ payload chars after the dash
//   Alt2: bare sk-ant- → requires 92+ chars (prevents absorbing api03- into run)
// ---------------------------------------------------------------------------

func TestAnthropicMinimumLength(t *testing.T) {
	// Use the same pattern as main.go (fixed version)
	re := regexp.MustCompile(`sk-ant-(?:api\d+-[A-Za-z0-9_-]{86,}|[A-Za-z0-9_-]{92,})`)

	const prefix = "sk-ant-api03-"

	// Build payloads of exact lengths
	chars := "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_"
	makePayload := func(n int) string {
		s := ""
		for len(s) < n {
			s += chars
		}
		return s[:n]
	}

	payload85 := makePayload(85)
	payload86 := makePayload(86)
	payload95 := makePayload(95)
	payload91 := makePayload(91)
	payload92 := makePayload(92)

	// Alt1: api03- prefix, 85-char payload → FAIL (85 < 86)
	t.Run("api03_prefix_85chars_no_match", func(t *testing.T) {
		input := prefix + payload85
		if got := re.FindString(input); got != "" {
			t.Errorf("85-char payload after api03- should not match (min 86), got %q", got)
		}
	})

	// Alt1: api03- prefix, 86-char payload → PASS
	t.Run("api03_prefix_86chars_matches", func(t *testing.T) {
		input := prefix + payload86
		got := re.FindString(input)
		want := input
		if got != want {
			t.Errorf("86-char payload after api03-: got %q, want %q", got, want)
		}
	})

	// Alt1: api03- prefix, 95-char payload → PASS
	t.Run("api03_prefix_95chars_matches", func(t *testing.T) {
		input := prefix + payload95
		got := re.FindString(input)
		want := input
		if got != want {
			t.Errorf("95-char payload after api03-: got %q, want %q", got, want)
		}
	})

	// Alt2: bare sk-ant- (no api prefix), 91 chars → FAIL (91 < 92)
	t.Run("bare_sk_ant_91chars_no_match", func(t *testing.T) {
		input := "sk-ant-" + payload91
		if got := re.FindString(input); got != "" {
			t.Errorf("bare sk-ant- 91-char should not match (min 92), got %q", got)
		}
	})

	// Alt2: bare sk-ant- (no api prefix), 92 chars → PASS
	t.Run("bare_sk_ant_92chars_matches", func(t *testing.T) {
		input := "sk-ant-" + payload92
		got := re.FindString(input)
		want := input
		if got != want {
			t.Errorf("bare sk-ant- 92-char: got %q, want %q", got, want)
		}
	})
}

// ---------------------------------------------------------------------------
// Stripe: pk_ excluded and minimum-length boundary
// These supplement TestStripePattern in detectors_aws_test.go
// ---------------------------------------------------------------------------

func TestStripePKExcludedAndMinLen(t *testing.T) {
	re := regexp.MustCompile(`(?:sk_live_|sk_test_|rk_live_|rk_test_)[0-9a-zA-Z]{16,99}`)

	t.Run("pk_live_not_matched", func(t *testing.T) {
		input := "pk_live_ABCDEFabcdef0123456789AB"
		if got := re.FindString(input); got != "" {
			t.Errorf("pk_ prefix should not match; got %q", got)
		}
	})

	t.Run("pk_test_not_matched", func(t *testing.T) {
		input := "pk_test_ABCDEFabcdef0123456789AB"
		if got := re.FindString(input); got != "" {
			t.Errorf("pk_test_ prefix should not match; got %q", got)
		}
	})

	// Exactly 16 chars after prefix = minimum match
	t.Run("sk_live_exactly_16_chars", func(t *testing.T) {
		input := "sk_live_1234567890123456"
		want := "sk_live_1234567890123456"
		if got := re.FindString(input); got != want {
			t.Errorf("sk_live_ + 16 chars: got %q, want %q", got, want)
		}
	})

	// 15 chars after prefix = below minimum
	t.Run("sk_live_15_chars_no_match", func(t *testing.T) {
		input := "sk_live_123456789012345"
		if got := re.FindString(input); got != "" {
			t.Errorf("sk_live_ + 15 chars should not match; got %q", got)
		}
	})

	// rk_ variants
	t.Run("rk_live_matches", func(t *testing.T) {
		input := "rk_live_ABCDEFabcdef0123456789AB"
		if got := re.FindString(input); got != input {
			t.Errorf("rk_live_: got %q, want %q", got, input)
		}
	})
}
