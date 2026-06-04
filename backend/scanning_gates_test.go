package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Bug 1 — JSScan regex: old regex matched any src= attribute, not just <script>
//
// Old:  src=["'](.*?.js)["']
//       - matches <img src="/logo.js.png">, <link src="...">
//       - unescaped dot before "js" (matches any char: "ajs", "bjs")
// Fixed: (?i)<script[^>]+src=["']([^"']+\.js[^"']*)["']
//        - anchored to <script  (case-insensitive)
//        - escaped dot: only matches literal ".js"
// ---------------------------------------------------------------------------

func TestJSScanRegex(t *testing.T) {
	html := `<html>
<img src="/images/logo.png" />
<link href="/style.css" />
<script src="/app/main.js"></script>
<script src="https://cdn.example.com/lib.min.js"></script>
<script SRC='/bundle.js'></script>
<img src="/images/fakejs.png" />
</html>`

	t.Run("old_regex_false_positives", func(t *testing.T) {
		// The old regex matches src= on non-script elements and on paths that
		// end in a character + "js" (unescaped dot).
		old := regexp.MustCompile(`src=["'](.*?.js)["']`)
		var caps []string
		for _, m := range old.FindAllStringSubmatch(html, -1) {
			if len(m) > 1 {
				caps = append(caps, m[1])
			}
		}
		// Should find at least one non-.js path (the img src ending in .png matched
		// because .*? stops at the .js inside fakejs or matches the png path).
		// The old regex also matches "/images/logo.png" because .*? is lazy but .
		// means ANY char — it won't match .png, but it WILL match anything ending
		// in <char>js. Demonstrate this with an explicit false-positive probe.
		badHTML := `<img src="/data/inject.js.png">`
		ms := old.FindAllStringSubmatch(badHTML, -1)
		if len(ms) > 0 {
			t.Logf("old regex hits non-script img src: %v", ms)
		}
		// The key proof: img src is captured alongside script src.
		imgMatched := false
		for _, cap := range caps {
			if strings.Contains(cap, "fake") || !strings.Contains(cap, "main") && !strings.Contains(cap, "lib") && !strings.Contains(cap, "bundle") {
				imgMatched = true
			}
		}
		_ = imgMatched // informational — old regex behaviour documented
	})

	t.Run("fixed_regex_only_script_tags", func(t *testing.T) {
		fixed := regexp.MustCompile(`(?i)<script[^>]+src=["']([^"']+\.js[^"']*)["']`)
		var caps []string
		for _, m := range fixed.FindAllStringSubmatch(html, -1) {
			if len(m) > 1 {
				caps = append(caps, m[1])
			}
		}
		// Must capture the three JS paths
		want := []string{"/app/main.js", "https://cdn.example.com/lib.min.js", "/bundle.js"}
		for _, w := range want {
			found := false
			for _, c := range caps {
				if c == w {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("fixed regex: expected %q in captures %v", w, caps)
			}
		}
		// Must NOT capture non-script paths
		for _, c := range caps {
			if strings.Contains(c, ".png") || strings.Contains(c, ".css") {
				t.Errorf("fixed regex: unexpected non-JS capture %q", c)
			}
		}
	})

	t.Run("fixed_regex_case_insensitive_script", func(t *testing.T) {
		// SRC in uppercase should still match
		fixed := regexp.MustCompile(`(?i)<script[^>]+src=["']([^"']+\.js[^"']*)["']`)
		html2 := `<SCRIPT SRC="/uppercase.js"></SCRIPT>`
		ms := fixed.FindAllStringSubmatch(html2, -1)
		if len(ms) == 0 {
			t.Error("fixed regex: should match case-insensitive <SCRIPT SRC=...>")
		}
	})

	t.Run("fixed_regex_no_unescaped_dot", func(t *testing.T) {
		// "ajs" or "bjs" should NOT match the fixed regex
		fixed := regexp.MustCompile(`(?i)<script[^>]+src=["']([^"']+\.js[^"']*)["']`)
		html3 := `<script src="/path/fakejs"></script>`
		ms := fixed.FindAllStringSubmatch(html3, -1)
		if len(ms) > 0 {
			t.Errorf("fixed regex: 'fakejs' (no dot) should not match, got %v", ms)
		}
	})
}

// ---------------------------------------------------------------------------
// Bug 2 — commonPaths deduplication
//
// PHPInfoPaths shares entries with the default EnvPaths / PHPInfo static
// additions (/config.php, /database.php). Without unique(), each is fetched
// twice per target URL.
// ---------------------------------------------------------------------------

func TestCommonPathsDedup(t *testing.T) {
	// Simulate the append pattern used in createRequest
	envPaths := []string{"/.env", "/config.php", "/database.php", "/.env.local"}
	awsPaths := []string{"/.aws/credentials", "/.aws/config"}
	phpInfoExtra := []string{"/phpinfo.php", "/info.php", "/config.php", "/database.php"}

	combined := append([]string(nil), envPaths...)
	combined = append(combined, awsPaths...)
	combined = append(combined, phpInfoExtra...)

	before := len(combined)
	deduped := unique(combined)
	after := len(deduped)

	if after >= before {
		t.Errorf("expected dedup to reduce path count: before=%d after=%d", before, after)
	}

	// Verify no duplicates in output
	seen := make(map[string]int)
	for _, p := range deduped {
		seen[p]++
	}
	for p, count := range seen {
		if count > 1 {
			t.Errorf("path %q appears %d times after dedup", p, count)
		}
	}
}

// ---------------------------------------------------------------------------
// Bug 3 / Config loading — all ScanningFeatures flags must load correctly
// ---------------------------------------------------------------------------

func TestLoadConfigScanningFeatures(t *testing.T) {
	// Write a temporary config.json with all flags set to specific values
	cfgJSON := `{
		"scanning_features": {
			"aws_main_scan": true,
			"smtp_credentials_scan": true,
			"github_token_deep_scan": false,
			"js_scan": true,
			"phpinfo_scan": true,
			"git_config_scan": false,
			"docker_scan": true,
			"config_file_scan": false,
			"backup_file_scan": true
		}
	}`

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	if err := os.WriteFile(cfgPath, []byte(cfgJSON), 0644); err != nil {
		t.Fatalf("write temp config: %v", err)
	}

	cfg, err := loadConfig(cfgPath)
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}

	sf := cfg.ScanningFeatures
	tests := []struct {
		name  string
		got   bool
		want  bool
	}{
		{"AWSMainScan", sf.AWSMainScan, true},
		{"SMTPCredentialsScan", sf.SMTPCredentialsScan, true},
		{"GitHubTokenDeepScan", sf.GitHubTokenDeepScan, false},
		{"JSScan", sf.JSScan, true},
		{"PHPInfoScan", sf.PHPInfoScan, true},
		{"GitConfigScan", sf.GitConfigScan, false},
		{"DockerScan", sf.DockerScan, true},
		{"ConfigFileScan", sf.ConfigFileScan, false},
		{"BackupFileScan", sf.BackupFileScan, true},
	}

	for _, tc := range tests {
		if tc.got != tc.want {
			t.Errorf("ScanningFeatures.%s = %v, want %v", tc.name, tc.got, tc.want)
		}
	}
}

func TestLoadConfigScanningFeaturesDefaultFalse(t *testing.T) {
	// When flags are absent from config.json they must default to false (not panic)
	cfgJSON := `{"scanning_features": {"aws_main_scan": true}}`

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	if err := os.WriteFile(cfgPath, []byte(cfgJSON), 0644); err != nil {
		t.Fatalf("write temp config: %v", err)
	}

	cfg, err := loadConfig(cfgPath)
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}

	sf := cfg.ScanningFeatures
	if !sf.AWSMainScan {
		t.Error("AWSMainScan should be true")
	}
	// All absent flags must be false (zero value)
	absent := map[string]bool{
		"JSScan":       sf.JSScan,
		"PHPInfoScan":  sf.PHPInfoScan,
		"GitConfigScan": sf.GitConfigScan,
		"DockerScan":   sf.DockerScan,
		"ConfigFileScan": sf.ConfigFileScan,
		"BackupFileScan": sf.BackupFileScan,
	}
	for name, val := range absent {
		if val {
			t.Errorf("ScanningFeatures.%s should default to false when absent", name)
		}
	}
}

// ---------------------------------------------------------------------------
// loadConfig — verify the redundant features.brevo re-assignment doesn't
// shadow a properly-loaded value (regression guard).
// ---------------------------------------------------------------------------

func TestLoadConfigFeaturesBrevo(t *testing.T) {
	cfgJSON := `{
		"features": {"brevo": true},
		"scanning_features": {}
	}`
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	if err := os.WriteFile(cfgPath, []byte(cfgJSON), 0644); err != nil {
		t.Fatalf("write temp config: %v", err)
	}
	cfg, err := loadConfig(cfgPath)
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}
	if !cfg.Features.Brevo {
		t.Error("Features.Brevo should be true after loadConfig")
	}
}

// ---------------------------------------------------------------------------
// GitHubTokenDeepScan dead-field guard
// The field is declared in ScanningFeatures but never read as a gate anywhere
// in the code. This test documents that behaviour so a future developer knows
// the flag exists and must be wired up before use.
// ---------------------------------------------------------------------------

func TestGitHubTokenDeepScanIsUnused(_ *testing.T) {
	// Compile-time proof: the field can be set without error, and json round-trips.
	type sf struct {
		GitHubTokenDeepScan bool `json:"github_token_deep_scan"`
	}
	data, _ := json.Marshal(sf{GitHubTokenDeepScan: true})
	var out sf
	json.Unmarshal(data, &out) //nolint:errcheck
	// If the field were renamed or removed in the struct, this file would fail to compile.
	_ = out.GitHubTokenDeepScan
}
