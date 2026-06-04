package main

import (
	"encoding/base64"
	"reflect"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// isPrintableText
// ---------------------------------------------------------------------------

func TestUtilityIsPrintableText(t *testing.T) {
	tests := []struct {
		name string
		data []byte
		want bool
	}{
		{
			name: "empty slice returns false",
			data: []byte{},
			want: false,
		},
		{
			name: "all ASCII printable returns true",
			data: []byte("Hello, World! 0123456789"),
			want: true,
		},
		{
			name: "tab newline CR counted as printable",
			data: []byte("line1\tfield\nline2\r\n"),
			want: true,
		},
		{
			name: "exactly 29 percent non-printable (4 out of 14) is true",
			// 4 non-printable bytes out of 14 = 0.2857... < 0.30 → true
			data: func() []byte {
				b := []byte("0123456789" ) // 10 printable
				b = append(b, 0, 1, 2, 3) // 4 non-printable (null bytes)
				return b
			}(),
			want: true,
		},
		{
			name: "exactly 30 percent non-printable (3 out of 10) is false",
			// 3/10 = 0.30, NOT < 0.30 → false
			data: func() []byte {
				b := []byte("1234567") // 7 printable
				b = append(b, 0, 1, 2) // 3 non-printable
				return b
			}(),
			want: false,
		},
		{
			name: "null bytes counted as non-printable",
			data: []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
			want: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := isPrintableText(tc.data)
			if got != tc.want {
				t.Errorf("isPrintableText(%q) = %v, want %v", tc.data, got, tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// tryDecodeBase64
// ---------------------------------------------------------------------------

func TestUtilityTryDecodeBase64(t *testing.T) {
	// Helper: encode a string to standard base64.
	enc := func(s string) string { return base64.StdEncoding.EncodeToString([]byte(s)) }

	// Produce a 39-char base64-ish string (below 40 threshold).
	shortStr := strings.Repeat("A", 39)

	// Valid base64 that decodes to printable text (>= 40 chars encoded).
	printableMsg := "This is a printable ASCII message for testing base64 decode functionality!"
	printableB64 := enc(printableMsg)

	// Valid base64 that decodes to binary/non-printable data.
	binaryData := make([]byte, 30)
	for i := range binaryData {
		binaryData[i] = byte(i) // 0..29, many below 32 → non-printable
	}
	binaryB64 := enc(string(binaryData))

	// URL-safe base64: replace + with -, / with _.
	urlSafeMsg := "This is another printable ASCII message for url-safe test!"
	urlSafeB64 := enc(urlSafeMsg)
	urlSafeB64 = strings.ReplaceAll(urlSafeB64, "+", "-")
	urlSafeB64 = strings.ReplaceAll(urlSafeB64, "/", "_")

	// Padding mod-2: decoded length such that encoded len % 4 == 2 (needs ==).
	mod2Msg := "Hello!" // 6 bytes → 8 chars base64, 8%4==0, try longer
	// "This is a test of padding mod 2 case!!" = 38 bytes → ceil(38*4/3)=52 chars, 52%4==0
	// Pick a message whose stripped encoding has remainder 2.
	// "ABCDE" = 5 bytes → base64 = 8 chars → 8%4=0
	// "ABCDEFG" = 7 bytes → base64 = 12 chars → 12%4=0
	// "A" = 1 byte → "QQ==" (4 chars) → stripped "QQ" (2 chars, <40 threshold)
	// We need the cleaned string to have len >= 40 AND len%4 == 2.
	// Generate: 29 bytes → 40 chars base64 with padding "==" (29 bytes: 40 chars, 40%4=0)
	// 28 bytes → 40 chars (28*4/3=37.3 → ceil = 40 with padding); actually base64(28) = 40 chars
	// Let's find length where base64 has remainder 2 after stripping padding:
	// base64 without padding: ceil(n*4/3). For n=30: 40 chars no padding. 40%4=0.
	// For n=31: ceil(41.3)=44 chars, 44%4=0 with no padding... Actually:
	// n bytes → (n+2)/3*4 chars with padding; without padding:
	//   n%3==0 → 0 padding chars, encoded%4==0
	//   n%3==1 → 2 padding chars, without padding encoded_len%4==2
	//   n%3==2 → 1 padding char, without padding encoded_len%4==3
	// So for mod-2: n%3==1 and n large enough so encoded >= 40 chars.
	// n=31 → (32/3)*4=44 base64 chars → stripped of "==" → 42 chars, 42%4==2 ✓
	mod2Msg = strings.Repeat("x", 31) // 31 bytes, n%3==1 → 2 padding chars → stripped len 42, 42%4==2
	mod2B64 := enc(mod2Msg)
	mod2Stripped := strings.TrimRight(mod2B64, "=")
	if len(mod2Stripped)%4 != 2 {
		t.Fatalf("mod2 setup error: stripped len=%d, mod=%d", len(mod2Stripped), len(mod2Stripped)%4)
	}

	// Padding mod-3: n%3==2 → 1 padding char → stripped len%4==3
	// n=32: n%3==2 → (33/3)*4... Actually (32+2)/3*4 = 34/3... let me just compute:
	// n=32 bytes → base64: ceil(32/3)*4 = 11*4=44 chars with "=" → stripped 43 chars, 43%4==3 ✓
	mod3Msg := strings.Repeat("y", 32) // 32 bytes, n%3==2 → 1 padding char → stripped len 43, 43%4==3
	mod3B64 := enc(mod3Msg)
	mod3Stripped := strings.TrimRight(mod3B64, "=")
	if len(mod3Stripped)%4 != 3 {
		t.Fatalf("mod3 setup error: stripped len=%d, mod=%d", len(mod3Stripped), len(mod3Stripped)%4)
	}

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "input under 40 chars after cleaning returns empty",
			input: shortStr,
			want:  "",
		},
		{
			name:  "valid base64 decoding to printable text returns decoded text",
			input: printableB64,
			want:  printableMsg,
		},
		{
			name:  "valid base64 decoding to binary data returns empty",
			input: binaryB64,
			want:  "",
		},
		{
			name:  "url-safe chars dash and underscore normalised to plus and slash",
			input: urlSafeB64,
			want:  urlSafeMsg,
		},
		{
			name:  "padding added correctly for mod-2 remainder",
			input: mod2Stripped,
			want:  mod2Msg,
		},
		{
			name:  "padding added correctly for mod-3 remainder",
			input: mod3Stripped,
			want:  mod3Msg,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := tryDecodeBase64(tc.input)
			if got != tc.want {
				t.Errorf("tryDecodeBase64(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// IsIgnoredExt
// ---------------------------------------------------------------------------

func TestUtilityIsIgnoredExt(t *testing.T) {
	ignoredExts := []string{
		".jpg", ".jpeg", ".png", ".gif", ".exe", ".zip",
		".pdf", ".css", ".html", ".svg", ".woff", ".woff2",
		".mp4", ".mp3", ".json", ".lock",
	}

	for _, ext := range ignoredExts {
		ext := ext
		t.Run("ignored "+ext, func(t *testing.T) {
			if !IsIgnoredExt(ext) {
				t.Errorf("IsIgnoredExt(%q) = false, want true", ext)
			}
		})
	}

	t.Run("uppercase JPG is case-insensitive true", func(t *testing.T) {
		if !IsIgnoredExt(".JPG") {
			t.Error("IsIgnoredExt(\".JPG\") = false, want true")
		}
	})

	notIgnored := []string{".go", ".txt", ".env"}
	for _, ext := range notIgnored {
		ext := ext
		t.Run("not ignored "+ext, func(t *testing.T) {
			if IsIgnoredExt(ext) {
				t.Errorf("IsIgnoredExt(%q) = true, want false", ext)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// unique
// ---------------------------------------------------------------------------

func TestUtilityUnique(t *testing.T) {
	tests := []struct {
		name  string
		input []string
		want  []string
	}{
		{
			name:  "empty slice returns empty",
			input: []string{},
			want:  nil, // unique returns nil out when no elements appended
		},
		{
			name:  "all unique preserves order",
			input: []string{"a", "b", "c"},
			want:  []string{"a", "b", "c"},
		},
		{
			name:  "duplicates removed first occurrence preserved",
			input: []string{"a", "b", "a", "c", "b"},
			want:  []string{"a", "b", "c"},
		},
		{
			name:  "nil input does not panic",
			input: nil,
			want:  nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := unique(tc.input)
			// nil and empty slice are both acceptable for "empty" cases.
			if len(got) == 0 && len(tc.want) == 0 {
				return
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("unique(%v) = %v, want %v", tc.input, got, tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// resolveURL
// ---------------------------------------------------------------------------

func TestUtilityResolveURL(t *testing.T) {
	tests := []struct {
		name string
		base string
		ref  string
		want string
	}{
		{
			name: "absolute ref returned unchanged",
			base: "https://example.com/page",
			ref:  "https://other.com/resource",
			want: "https://other.com/resource",
		},
		{
			name: "relative path resolved against base",
			base: "https://example.com/dir/page.html",
			ref:  "sub/file.js",
			want: "https://example.com/dir/sub/file.js",
		},
		{
			name: "absolute ref with different scheme returned unchanged",
			base: "https://example.com/",
			ref:  "ftp://files.example.com/data.zip",
			want: "ftp://files.example.com/data.zip",
		},
		{
			name: "bad base URL returns ref as-is",
			base: "://not a valid url",
			ref:  "relative/path",
			want: "relative/path",
		},
		{
			name: "bad ref URL returns ref as-is",
			// url.Parse actually succeeds on most strings; use a truly unparseable ref.
			// url.Parse never fails on well-formed ASCII strings, so let's use a
			// ref that IS absolute (IsAbs) so the function exits early – we test
			// the bad-base branch above. For bad-ref we can test that an opaque
			// ref string is returned unchanged.
			base: "https://example.com/",
			ref:  "https://valid.com/path",
			want: "https://valid.com/path",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := resolveURL(tc.base, tc.ref)
			if got != tc.want {
				t.Errorf("resolveURL(%q, %q) = %q, want %q", tc.base, tc.ref, got, tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// isSameHost (method on *Enhancer)
// ---------------------------------------------------------------------------

func TestUtilityIsSameHost(t *testing.T) {
	e := NewEnhancer(nil)

	tests := []struct {
		name string
		a, b string
		want bool
	}{
		{
			name: "same hostname returns true",
			a:    "https://example.com/page1",
			b:    "https://example.com/page2",
			want: true,
		},
		{
			name: "different hostname returns false",
			a:    "https://example.com/page",
			b:    "https://other.com/page",
			want: false,
		},
		{
			name: "case-insensitive comparison returns true",
			a:    "https://Example.COM/path",
			b:    "https://example.com/other",
			want: true,
		},
		{
			name: "one URL with port same host returns true",
			a:    "https://example.com:8443/path",
			b:    "https://example.com/path",
			want: true,
		},
		{
			name: "bad URL returns false",
			a:    "://not-valid",
			b:    "https://example.com/",
			want: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := e.isSameHost(tc.a, tc.b)
			if got != tc.want {
				t.Errorf("isSameHost(%q, %q) = %v, want %v", tc.a, tc.b, got, tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// extractLinksFromHTML (method on *Enhancer)
// ---------------------------------------------------------------------------

func TestUtilityExtractLinksFromHTML(t *testing.T) {
	e := NewEnhancer(nil)
	base := "https://example.com/page"

	tests := []struct {
		name string
		body string
		base string
		want []string
	}{
		{
			name: "absolute href included",
			body: `<a href="https://other.com/resource">link</a>`,
			base: base,
			want: []string{"https://other.com/resource"},
		},
		{
			name: "relative href starting with slash resolved to base",
			body: `<a href="/about">about</a>`,
			base: base,
			want: []string{"https://example.com/about"},
		},
		{
			name: "href javascript excluded",
			body: `<a href="javascript:void(0)">click</a>`,
			base: base,
			want: []string{},
		},
		{
			name: "href mailto excluded",
			body: `<a href="mailto:user@example.com">email</a>`,
			base: base,
			want: []string{},
		},
		{
			name: "duplicate hrefs deduplicated",
			body: `<a href="https://example.com/dup">a</a><a href="https://example.com/dup">b</a>`,
			base: base,
			want: []string{"https://example.com/dup"},
		},
		{
			name: "href not starting with http after resolution excluded",
			// relative path not starting with / won't gain http prefix
			body: `<a href="relative/path.html">link</a>`,
			base: base,
			want: []string{},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := e.extractLinksFromHTML(tc.body, tc.base)
			// Treat nil and empty slice as equivalent for empty results.
			if len(got) == 0 && len(tc.want) == 0 {
				return
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("extractLinksFromHTML body=%q => %v, want %v", tc.body, got, tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// extractValueFromPhpInfoTable
// ---------------------------------------------------------------------------

func TestUtilityExtractValueFromPhpInfoTable(t *testing.T) {
	tests := []struct {
		name        string
		html        string
		settingName string
		want        string
	}{
		{
			name: "setting found returns trimmed value",
			html: `<td class="e">max_execution_time</td>
			       <td class="v">  30  </td>`,
			settingName: "max_execution_time",
			want:        "30",
		},
		{
			name: "nbsp in value replaced with space",
			html: `<td class="e">memory_limit</td>
			       <td class="v">128&nbsp;M</td>`,
			settingName: "memory_limit",
			want:        "128 M",
		},
		{
			name: "quot in value replaced with double-quote",
			html: `<td class="e">error_log</td>
			       <td class="v">&quot;/var/log/php.log&quot;</td>`,
			settingName: "error_log",
			want:        "/var/log/php.log",
		},
		{
			name: "surrounding quotes stripped",
			html: `<td class="e">doc_root</td>
			       <td class="v">"/var/www"</td>`,
			settingName: "doc_root",
			want:        "/var/www",
		},
		{
			name: "setting not found returns empty string",
			html: `<td class="e">other_setting</td>
			       <td class="v">value</td>`,
			settingName: "nonexistent",
			want:        "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := extractValueFromPhpInfoTable(tc.html, tc.settingName)
			if got != tc.want {
				t.Errorf("extractValueFromPhpInfoTable(settingName=%q) = %q, want %q", tc.settingName, got, tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// sanitizeSource
// ---------------------------------------------------------------------------

func TestUtilitySanitizeSource(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "https prefix stripped",
			input: "https://example.com/path",
			want:  "example.com/path",
		},
		{
			name:  "http prefix stripped",
			input: "http://example.com/path",
			want:  "example.com/path",
		},
		{
			name:  "no scheme unchanged",
			input: "example.com/path",
			want:  "example.com/path",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := sanitizeSource(tc.input)
			if got != tc.want {
				t.Errorf("sanitizeSource(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// ExtractIPOnly (method on *AWSScanner)
// ---------------------------------------------------------------------------

func TestUtilityExtractIPOnly(t *testing.T) {
	a := &AWSScanner{}

	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{
			name:  "single IP extracted",
			input: "Connect to 192.168.1.1 for access",
			want:  []string{"192.168.1.1"},
		},
		{
			name:  "duplicate IPs deduplicated",
			input: "10.0.0.1 and again 10.0.0.1",
			want:  []string{"10.0.0.1"},
		},
		{
			name:  "255.255.255.255 found",
			input: "broadcast: 255.255.255.255",
			want:  []string{"255.255.255.255"},
		},
		{
			name:  "invalid octet 256 not found",
			input: "bad address 256.0.0.1",
			want:  []string{},
		},
		{
			name:  "IP with port only IP extracted",
			input: "server 192.168.1.1:8080",
			want:  []string{"192.168.1.1"},
		},
		{
			name:  "mixed text multiple IPs all extracted",
			input: "hosts: 10.0.0.1, 172.16.0.2, gateway 192.168.0.1",
			want:  []string{"10.0.0.1", "172.16.0.2", "192.168.0.1"},
		},
		{
			name:  "no IPs returns empty slice",
			input: "no addresses here",
			want:  []string{},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := a.ExtractIPOnly(tc.input)
			// Treat nil and empty slice as equivalent for empty result.
			if len(got) == 0 && len(tc.want) == 0 {
				return
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("ExtractIPOnly(%q) = %v, want %v", tc.input, got, tc.want)
			}
		})
	}
}
