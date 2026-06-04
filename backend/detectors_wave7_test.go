package main

import (
	"regexp"
	"testing"
)

// Wave-7 regex patterns compiled directly from main.go (NewAWSScanner).
var (
	wave7GitHubTokenPattern    = regexp.MustCompile(`(?:ghp_|gho_|ghs_|ghr_|github_pat_)[A-Za-z0-9_]{36,255}`)
	wave7GCPAPIKeyPattern      = regexp.MustCompile(`AIza[0-9A-Za-z\-_]{35}`)
	wave7SlackBotTokenPattern  = regexp.MustCompile(`xoxb-[0-9]{10,13}-[0-9]{10,13}-[A-Za-z0-9]{24,32}`)
	wave7DiscordBotPattern     = regexp.MustCompile(`[MN][A-Za-z0-9]{23,25}\.[A-Za-z0-9_-]{6}\.[A-Za-z0-9_-]{27,38}`)
	wave7CloudflarePattern     = regexp.MustCompile(`(?i)(?:CLOUDFLARE_API_TOKEN|CF_API_TOKEN|cloudflare[_\-]?(?:api[_\-]?)?(?:token|key))\s*[:=]\s*["']?([A-Za-z0-9_-]{40})["']?`)
	wave7DigitalOceanPattern   = regexp.MustCompile(`dop_v1_[a-f0-9]{64}`)
	wave7ShopifyPattern        = regexp.MustCompile(`shp(?:pa|ca|at)_[a-f0-9]{32}`)
	wave7HubSpotPattern        = regexp.MustCompile(`(?:pat-[a-z]{2}\d+-[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}|(?i)(?:HUBSPOT_API_KEY|hubspot[_\-]?(?:api[_\-]?)?key)\s*[:=]\s*["']?([0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12})["']?)`)
	wave7HerokuPattern         = regexp.MustCompile(`(?i)(?:HEROKU_API_KEY|heroku[_\-]?(?:api[_\-]?)?(?:key|token))\s*[:=]\s*["']?([0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12})["']?`)
	wave7DatadogPattern        = regexp.MustCompile(`(?i)(?:DD_API_KEY|DATADOG_API_KEY|datadog[_\-]?(?:api[_\-]?)?key)\s*[:=]\s*["']?([a-f0-9]{32})["']?`)
)

type wave7Case struct {
	name       string
	input      string
	wantMatch  bool
	wantGroup1 string // only checked when wantMatch==true and pattern has a capture group
}

func runWave7Cases(t *testing.T, re *regexp.Regexp, cases []wave7Case) {
	t.Helper()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := re.FindStringSubmatch(tc.input)
			matched := len(m) > 0
			if matched != tc.wantMatch {
				t.Errorf("input %q: wantMatch=%v got matched=%v (full match=%q)", tc.input, tc.wantMatch, matched, m)
				return
			}
			if tc.wantMatch && tc.wantGroup1 != "" {
				if len(m) < 2 {
					t.Errorf("input %q: expected capture group 1 but submatches=%v", tc.input, m)
					return
				}
				if m[1] != tc.wantGroup1 {
					t.Errorf("input %q: group1 want %q got %q", tc.input, tc.wantGroup1, m[1])
				}
			}
		})
	}
}

// helper: repeat char n times
func rep(ch byte, n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = ch
	}
	return string(b)
}

// ---------- GitHubTokenPattern ----------

func TestWave7_GitHubTokenPattern(t *testing.T) {
	suffix36 := rep('a', 36)
	suffix35 := rep('a', 35)

	cases := []wave7Case{
		{
			name:      "ghp_ prefix 36 chars match",
			input:     "ghp_" + suffix36,
			wantMatch: true,
		},
		{
			name:      "gho_ prefix 36 chars match",
			input:     "gho_" + suffix36,
			wantMatch: true,
		},
		{
			name:      "ghs_ prefix 36 chars match",
			input:     "ghs_" + suffix36,
			wantMatch: true,
		},
		{
			name:      "ghr_ prefix 36 chars match",
			input:     "ghr_" + suffix36,
			wantMatch: true,
		},
		{
			name:      "github_pat_ prefix 36 chars match",
			input:     "github_pat_" + suffix36,
			wantMatch: true,
		},
		{
			name:      "mixed alphanumeric underscore body 36 chars",
			input:     "ghp_ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghij",
			wantMatch: true,
		},
		{
			name:      "ghp_ only 35 chars too short",
			input:     "ghp_" + suffix35,
			wantMatch: false,
		},
		{
			name:      "glp_ wrong prefix no match",
			input:     "glp_" + suffix36,
			wantMatch: false,
		},
		{
			name:      "no prefix bare alphanumeric",
			input:     rep('a', 40),
			wantMatch: false,
		},
		{
			name:      "github_pat_ in env var format",
			input:     "GITHUB_TOKEN=github_pat_" + suffix36,
			wantMatch: true,
		},
		{
			name:      "ghp_ in JSON format",
			input:     `"token": "ghp_` + suffix36 + `"`,
			wantMatch: true,
		},
	}
	runWave7Cases(t, wave7GitHubTokenPattern, cases)
}

// ---------- GCPAPIKeyPattern ----------

func TestWave7_GCPAPIKeyPattern(t *testing.T) {
	suffix35 := rep('a', 35)
	suffix34 := rep('a', 34)

	cases := []wave7Case{
		{
			name:      "AIza + 35 lowercase alpha match",
			input:     "AIza" + suffix35,
			wantMatch: true,
		},
		{
			name:      "AIza + 35 mixed alphanumeric match",
			input:     "AIzaSyD-9tSrke72FkZeQyfM123456789012345",
			wantMatch: true,
		},
		{
			name:      "AIza + hyphens and underscores match",
			input:     "AIza" + rep('-', 17) + rep('_', 18),
			wantMatch: true,
		},
		{
			name:      "AIza + 34 chars too short",
			input:     "AIza" + suffix34,
			wantMatch: false,
		},
		{
			name:      "aiza lowercase prefix no match (case-sensitive)",
			input:     "aiza" + suffix35,
			wantMatch: false,
		},
		{
			name:      "AIZA uppercase prefix no match",
			input:     "AIZA" + suffix35,
			wantMatch: false,
		},
		{
			name:      "AIza in env var format",
			input:     "GCP_API_KEY=AIza" + suffix35,
			wantMatch: true,
		},
		{
			name:      "AIza in JSON format",
			input:     `"api_key": "AIza` + suffix35 + `"`,
			wantMatch: true,
		},
	}
	runWave7Cases(t, wave7GCPAPIKeyPattern, cases)
}

// ---------- SlackBotTokenPattern ----------

func TestWave7_SlackBotTokenPattern(t *testing.T) {
	// valid: xoxb-{10-13 digits}-{10-13 digits}-{24-32 alphanum}
	suffix24 := rep('A', 24)
	suffix23 := rep('A', 23)

	cases := []wave7Case{
		{
			name:      "10-digit segments 24-char suffix match",
			input:     "xoxb-1234567890-1234567890-" + suffix24,
			wantMatch: true,
		},
		{
			name:      "13-digit segments 32-char suffix match",
			input:     "xoxb-1234567890123-1234567890123-" + rep('z', 32),
			wantMatch: true,
		},
		{
			name:      "mixed alphanum suffix match",
			input:     "xoxb-1234567890-1234567890-ABCDEFabcdef123456789012",
			wantMatch: true,
		},
		{
			name:      "xoxp prefix no match",
			input:     "xoxp-1234567890-1234567890-" + suffix24,
			wantMatch: false,
		},
		{
			name:      "first segment 9 digits too short",
			input:     "xoxb-123456789-1234567890-" + suffix24,
			wantMatch: false,
		},
		{
			name:      "second segment 9 digits too short",
			input:     "xoxb-1234567890-123456789-" + suffix24,
			wantMatch: false,
		},
		{
			name:      "suffix 23 chars too short",
			input:     "xoxb-1234567890-1234567890-" + suffix23,
			wantMatch: false,
		},
		{
			name:      "xoxb token in env var format",
			input:     "SLACK_BOT_TOKEN=xoxb-1234567890-1234567890-" + suffix24,
			wantMatch: true,
		},
		{
			name:      "xoxb token in JSON format",
			input:     `"token": "xoxb-1234567890-1234567890-` + suffix24 + `"`,
			wantMatch: true,
		},
	}
	runWave7Cases(t, wave7SlackBotTokenPattern, cases)
}

// ---------- DiscordBotTokenPattern ----------

func TestWave7_DiscordBotTokenPattern(t *testing.T) {
	// [MN][A-Za-z0-9]{23,25} . [A-Za-z0-9_-]{6} . [A-Za-z0-9_-]{27,38}
	mid6 := rep('A', 6)
	tail27 := rep('B', 27)
	tail26 := rep('B', 26)
	body24 := rep('a', 24) // 24 chars → total prefix len = 25 (1+24)

	cases := []wave7Case{
		{
			name:      "M prefix 24 body 6 mid 27 tail match",
			input:     "M" + body24 + "." + mid6 + "." + tail27,
			wantMatch: true,
		},
		{
			name:      "N prefix 25 body 6 mid 38 tail match",
			input:     "N" + rep('a', 25) + "." + mid6 + "." + rep('C', 38),
			wantMatch: true,
		},
		{
			name:      "underscores and hyphens in tail",
			input:     "M" + body24 + "." + mid6 + "." + rep('_', 14) + rep('-', 13),
			wantMatch: true,
		},
		{
			name:      "P prefix no match",
			input:     "P" + body24 + "." + mid6 + "." + tail27,
			wantMatch: false,
		},
		{
			name:      "middle segment 5 chars no match",
			input:     "M" + body24 + "." + rep('A', 5) + "." + tail27,
			wantMatch: false,
		},
		{
			name:      "tail 26 chars no match",
			input:     "M" + body24 + "." + mid6 + "." + tail26,
			wantMatch: false,
		},
		{
			name:      "body only 22 chars (total prefix 23) no match",
			input:     "M" + rep('a', 22) + "." + mid6 + "." + tail27,
			wantMatch: false,
		},
		{
			name:      "discord token in env var",
			input:     "DISCORD_TOKEN=M" + body24 + "." + mid6 + "." + tail27,
			wantMatch: true,
		},
		{
			name:      "discord token in JSON",
			input:     `"bot_token": "M` + body24 + "." + mid6 + "." + tail27 + `"`,
			wantMatch: true,
		},
	}
	runWave7Cases(t, wave7DiscordBotPattern, cases)
}

// ---------- CloudflareTokenPattern ----------

func TestWave7_CloudflareTokenPattern(t *testing.T) {
	token40 := "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMN" // exactly 40 chars
	token39 := "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLM"  // 39 chars

	cases := []wave7Case{
		{
			name:       "CLOUDFLARE_API_TOKEN= bare token",
			input:      "CLOUDFLARE_API_TOKEN=" + token40,
			wantMatch:  true,
			wantGroup1: token40,
		},
		{
			name:       "CF_API_TOKEN= quoted token",
			input:      `CF_API_TOKEN="` + token40 + `"`,
			wantMatch:  true,
			wantGroup1: token40,
		},
		{
			name:       "CF_API_TOKEN= single-quoted token",
			input:      "CF_API_TOKEN='" + token40 + "'",
			wantMatch:  true,
			wantGroup1: token40,
		},
		{
			name:       "cloudflare_api_token= lowercase match",
			input:      "cloudflare_api_token=" + token40,
			wantMatch:  true,
			wantGroup1: token40,
		},
		{
			name:       "cloudflare-api-key= hyphenated match",
			input:      "cloudflare-api-key=" + token40,
			wantMatch:  true,
			wantGroup1: token40,
		},
		{
			name:       "cloudflare_token colon separator",
			input:      "cloudflare_token: " + token40,
			wantMatch:  true,
			wantGroup1: token40,
		},
		{
			name:      "CF_API_TOKEN= 39 chars no match",
			input:     "CF_API_TOKEN=" + token39,
			wantMatch: false,
		},
		{
			name:      "bare 40-char token no context keyword no match",
			input:     token40,
			wantMatch: false,
		},
		{
			// JSON wraps the key name in quotes which blocks key=value matching;
			// use YAML/config style (unquoted key name) instead.
			name:       "YAML cloudflare_api_token: value format",
			input:      "cloudflare_api_token: " + token40,
			wantMatch:  true,
			wantGroup1: token40,
		},
	}
	runWave7Cases(t, wave7CloudflarePattern, cases)
}

// ---------- DigitalOceanTokenPattern ----------

func TestWave7_DigitalOceanTokenPattern(t *testing.T) {
	hex64 := rep('a', 32) + rep('0', 32) // 64 lowercase hex chars
	hex63 := rep('a', 32) + rep('0', 31) // 63 chars

	cases := []wave7Case{
		{
			name:      "dop_v1_ + 64 lowercase hex match",
			input:     "dop_v1_" + hex64,
			wantMatch: true,
		},
		{
			name:      "dop_v1_ + all f hex chars match",
			input:     "dop_v1_" + rep('f', 64),
			wantMatch: true,
		},
		{
			name:      "dop_v1_ + 63 hex no match",
			input:     "dop_v1_" + hex63,
			wantMatch: false,
		},
		{
			name:      "dop_v2_ wrong version no match",
			input:     "dop_v2_" + hex64,
			wantMatch: false,
		},
		{
			name:      "dop_v1_ uppercase hex no match",
			input:     "dop_v1_" + rep('A', 64),
			wantMatch: false,
		},
		{
			name:      "dop_v1_ hex with g char no match",
			input:     "dop_v1_" + rep('g', 64),
			wantMatch: false,
		},
		{
			name:      "token in env var",
			input:     "DIGITALOCEAN_ACCESS_TOKEN=dop_v1_" + hex64,
			wantMatch: true,
		},
		{
			name:      "token in JSON",
			input:     `"access_token": "dop_v1_` + hex64 + `"`,
			wantMatch: true,
		},
	}
	runWave7Cases(t, wave7DigitalOceanPattern, cases)
}

// ---------- ShopifyTokenPattern ----------

func TestWave7_ShopifyTokenPattern(t *testing.T) {
	hex32 := rep('a', 16) + rep('0', 16) // 32 lowercase hex
	hex31 := rep('a', 16) + rep('0', 15) // 31 chars

	cases := []wave7Case{
		{
			name:      "shppa_ + 32 hex match",
			input:     "shppa_" + hex32,
			wantMatch: true,
		},
		{
			name:      "shpca_ + 32 hex match",
			input:     "shpca_" + hex32,
			wantMatch: true,
		},
		{
			name:      "shpat_ + 32 hex match",
			input:     "shpat_" + hex32,
			wantMatch: true,
		},
		{
			name:      "shpxx_ wrong middle no match",
			input:     "shpxx_" + hex32,
			wantMatch: false,
		},
		{
			name:      "shppa_ + 31 hex no match",
			input:     "shppa_" + hex31,
			wantMatch: false,
		},
		{
			name:      "shppa_ uppercase hex no match",
			input:     "shppa_" + rep('A', 32),
			wantMatch: false,
		},
		{
			name:      "shpat_ in env var",
			input:     "SHOPIFY_TOKEN=shpat_" + hex32,
			wantMatch: true,
		},
		{
			name:      "shpca_ in JSON",
			input:     `"shopify_token": "shpca_` + hex32 + `"`,
			wantMatch: true,
		},
	}
	runWave7Cases(t, wave7ShopifyPattern, cases)
}

// ---------- HubSpotTokenPattern ----------

func TestWave7_HubSpotTokenPattern(t *testing.T) {
	uuid := "12345678-1234-1234-1234-123456789012"

	cases := []wave7Case{
		{
			name:      "pat- format na1 match",
			input:     "pat-na1-12345678-1234-1234-1234-123456789012",
			wantMatch: true,
		},
		{
			name:      "pat- format eu1 match",
			input:     "pat-eu1-abcdef01-abcd-abcd-abcd-abcdef012345",
			wantMatch: true,
		},
		{
			name:       "HUBSPOT_API_KEY= UUID match group1",
			input:      "HUBSPOT_API_KEY=" + uuid,
			wantMatch:  true,
			wantGroup1: uuid,
		},
		{
			name:       "hubspot_api_key= quoted UUID match group1",
			input:      `hubspot_api_key="` + uuid + `"`,
			wantMatch:  true,
			wantGroup1: uuid,
		},
		{
			name:       "hubspot-api-key= hyphen variant match group1",
			input:      "hubspot-api-key=" + uuid,
			wantMatch:  true,
			wantGroup1: uuid,
		},
		{
			name:       "HUBSPOT_API_KEY colon separator match group1",
			input:      "HUBSPOT_API_KEY: " + uuid,
			wantMatch:  true,
			wantGroup1: uuid,
		},
		{
			name:      "bare UUID no context no match",
			input:     uuid,
			wantMatch: false,
		},
		{
			name:      "pat- format missing region digits no match",
			input:     "pat-na-12345678-1234-1234-1234-123456789012",
			wantMatch: false,
		},
		{
			name:       "YAML hubspot_key: value format",
			input:      "hubspot_key: " + uuid,
			wantMatch:  true,
			wantGroup1: uuid,
		},
	}
	runWave7Cases(t, wave7HubSpotPattern, cases)
}

// ---------- HerokuAPIKeyPattern ----------

func TestWave7_HerokuAPIKeyPattern(t *testing.T) {
	uuid := "12345678-1234-1234-1234-123456789012"

	cases := []wave7Case{
		{
			name:       "HEROKU_API_KEY= UUID match group1",
			input:      "HEROKU_API_KEY=" + uuid,
			wantMatch:  true,
			wantGroup1: uuid,
		},
		{
			name:       "heroku_api_token= quoted UUID match group1",
			input:      `heroku_api_token="` + uuid + `"`,
			wantMatch:  true,
			wantGroup1: uuid,
		},
		{
			name:       "heroku_api_key= single-quoted UUID match group1",
			input:      "heroku_api_key='" + uuid + "'",
			wantMatch:  true,
			wantGroup1: uuid,
		},
		{
			name:       "HEROKU_API_KEY colon separator match group1",
			input:      "HEROKU_API_KEY: " + uuid,
			wantMatch:  true,
			wantGroup1: uuid,
		},
		{
			name:       "heroku-api-key hyphen variant match group1",
			input:      "heroku-api-key=" + uuid,
			wantMatch:  true,
			wantGroup1: uuid,
		},
		{
			name:      "bare UUID without context no match",
			input:     uuid,
			wantMatch: false,
		},
		{
			name:      "wrong keyword prefix no match",
			input:     "AWS_API_KEY=" + uuid,
			wantMatch: false,
		},
		{
			// JSON wraps the key name in quotes which blocks key=value matching.
			// Use YAML/config style (unquoted key name) instead.
			name:       "YAML heroku_api_key: value format",
			input:      "heroku_api_key: " + uuid,
			wantMatch:  true,
			wantGroup1: uuid,
		},
		{
			// (?i) governs the whole pattern including [0-9a-f], making it
			// case-insensitive, so uppercase hex digits match.
			name:       "HEROKU_API_KEY= uppercase hex matches due to (?i)",
			input:      "HEROKU_API_KEY=ABCDEF01-ABCD-ABCD-ABCD-ABCDEF012345",
			wantMatch:  true,
			wantGroup1: "ABCDEF01-ABCD-ABCD-ABCD-ABCDEF012345",
		},
	}
	runWave7Cases(t, wave7HerokuPattern, cases)
}

// ---------- DatadogAPIKeyPattern ----------

func TestWave7_DatadogAPIKeyPattern(t *testing.T) {
	hex32 := rep('a', 16) + rep('0', 16)
	hex31 := rep('a', 16) + rep('0', 15)

	cases := []wave7Case{
		{
			name:       "DD_API_KEY= 32 hex match group1",
			input:      "DD_API_KEY=" + hex32,
			wantMatch:  true,
			wantGroup1: hex32,
		},
		{
			name:       "DATADOG_API_KEY= quoted 32 hex match group1",
			input:      `DATADOG_API_KEY="` + hex32 + `"`,
			wantMatch:  true,
			wantGroup1: hex32,
		},
		{
			name:       "datadog_api_key= lowercase match group1",
			input:      "datadog_api_key=" + hex32,
			wantMatch:  true,
			wantGroup1: hex32,
		},
		{
			name:       "datadog-api-key= hyphen variant match group1",
			input:      "datadog-api-key=" + hex32,
			wantMatch:  true,
			wantGroup1: hex32,
		},
		{
			name:       "DD_API_KEY colon separator match group1",
			input:      "DD_API_KEY: " + hex32,
			wantMatch:  true,
			wantGroup1: hex32,
		},
		{
			name:      "bare 32 hex without context no match",
			input:     hex32,
			wantMatch: false,
		},
		{
			name:      "DD_API_KEY= 31 hex no match",
			input:     "DD_API_KEY=" + hex31,
			wantMatch: false,
		},
		{
			name:      "wrong keyword prefix no match",
			input:     "AWS_API_KEY=" + hex32,
			wantMatch: false,
		},
		{
			// JSON wraps the key name in quotes — the regex keyword match fails
			// because "DD_API_KEY" has a closing quote before the separator.
			// Use YAML/config style (unquoted key name) instead.
			name:       "YAML datadog_api_key: value format",
			input:      "datadog_api_key: " + hex32,
			wantMatch:  true,
			wantGroup1: hex32,
		},
		{
			// (?i) governs the whole pattern including [a-f0-9], making it
			// case-insensitive, so uppercase hex digits match.
			name:       "DD_API_KEY= uppercase hex matches due to (?i)",
			input:      "DD_API_KEY=" + rep('A', 32),
			wantMatch:  true,
			wantGroup1: rep('A', 32),
		},
	}
	runWave7Cases(t, wave7DatadogPattern, cases)
}
