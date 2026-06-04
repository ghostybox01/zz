package main

import (
	"os"
	"path/filepath"
	"testing"
)

// ────────────────────────────────────────────────────────────────────────────
// loadConfig
// ────────────────────────────────────────────────────────────────────────────

func TestLoadConfig(t *testing.T) {
	dir := t.TempDir()

	tests := []struct {
		name        string
		content     string     // written to file; empty string means "don't create file"
		wantErr     bool
		check       func(t *testing.T, cfg *Config)
	}{
		{
			name:    "valid minimal JSON sets SendGrid and Brevo",
			content: `{"api_validation":{"sendgrid":true},"features":{"brevo":true}}`,
			check: func(t *testing.T, cfg *Config) {
				if !cfg.APIValidation.SendGrid {
					t.Error("want APIValidation.SendGrid == true")
				}
				if !cfg.Features.Brevo {
					t.Error("want Features.Brevo == true (compat layer)")
				}
			},
		},
		{
			name:    "malformed JSON returns error with nil config",
			content: `{this is not json`,
			wantErr: true,
			check: func(t *testing.T, cfg *Config) {
				if cfg != nil {
					t.Error("want nil config on JSON error")
				}
			},
		},
		{
			name:    "empty JSON object returns zero-value config without error",
			content: `{}`,
			check: func(t *testing.T, cfg *Config) {
				if cfg == nil {
					t.Fatal("want non-nil config")
				}
				if cfg.APIValidation.SendGrid {
					t.Error("want SendGrid == false for empty config")
				}
				if cfg.Features.Brevo {
					t.Error("want Brevo == false for empty config")
				}
			},
		},
		{
			name: "features compat layer: features.brevo true sets cfg.Features.Brevo",
			// api_validation.brevo is left false to prove the compat layer alone sets it
			content: `{"features":{"brevo":true}}`,
			check: func(t *testing.T, cfg *Config) {
				if !cfg.Features.Brevo {
					t.Error("want Features.Brevo == true via compat layer")
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var path string
			if tc.content != "" {
				path = filepath.Join(dir, tc.name+".json")
				if err := os.WriteFile(path, []byte(tc.content), 0o644); err != nil {
					t.Fatalf("setup: WriteFile: %v", err)
				}
			} else {
				// non-existent path
				path = filepath.Join(dir, "does_not_exist.json")
			}

			cfg, err := loadConfig(path)
			if tc.wantErr {
				if err == nil {
					t.Error("want error, got nil")
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
			}
			if tc.check != nil {
				tc.check(t, cfg)
			}
		})
	}

	// Separate sub-test for missing file (no content to write)
	t.Run("missing file returns error", func(t *testing.T) {
		path := filepath.Join(dir, "missing_file_xyz.json")
		cfg, err := loadConfig(path)
		if err == nil {
			t.Error("want error for missing file, got nil")
		}
		if cfg != nil {
			t.Error("want nil config for missing file")
		}
	})
}

// ────────────────────────────────────────────────────────────────────────────
// isValidSMTPConfig
// ────────────────────────────────────────────────────────────────────────────

func TestIsValidSMTPConfig(t *testing.T) {
	a := &AWSScanner{}

	tests := []struct {
		name string
		host string
		port string
		user string
		pass string
		from string
		want bool
	}{
		{
			name: "valid gmail config",
			host: "smtp.gmail.com",
			port: "587",
			user: "user@gmail.com",
			pass: "mypassword123",
			from: "user@gmail.com",
			want: true,
		},
		{
			name: "host without dot is invalid",
			host: "localhost",
			port: "587",
			user: "user",
			pass: "pass1234",
			from: "user@x.com",
			want: false,
		},
		{
			name: "host with JS chars is invalid",
			host: "smtp.()evil.com",
			port: "587",
			user: "user",
			pass: "pass1234",
			from: "u@e.com",
			want: false,
		},
		{
			name: "invalid port 8080",
			host: "smtp.example.com",
			port: "8080",
			user: "user@example.com",
			pass: "mypassword123",
			from: "user@example.com",
			want: false,
		},
		{
			name: "valid port 25",
			host: "smtp.example.com",
			port: "25",
			user: "user@example.com",
			pass: "mypassword123",
			from: "user@example.com",
			want: true,
		},
		{
			name: "valid port 465",
			host: "smtp.example.com",
			port: "465",
			user: "user@example.com",
			pass: "mypassword123",
			from: "user@example.com",
			want: true,
		},
		{
			name: "valid port 587",
			host: "smtp.example.com",
			port: "587",
			user: "user@example.com",
			pass: "mypassword123",
			from: "user@example.com",
			want: true,
		},
		{
			name: "valid port 2525",
			host: "smtp.example.com",
			port: "2525",
			user: "user@example.com",
			pass: "mypassword123",
			from: "user@example.com",
			want: true,
		},
		{
			name: "valid port 2587",
			host: "smtp.example.com",
			port: "2587",
			user: "user@example.com",
			pass: "mypassword123",
			from: "user@example.com",
			want: true,
		},
		{
			name: "user too short (2 chars)",
			host: "smtp.example.com",
			port: "587",
			user: "ab",
			pass: "mypassword123",
			from: "user@example.com",
			want: false,
		},
		{
			name: "user with JS chars",
			host: "smtp.example.com",
			port: "587",
			user: "use()r@example.com",
			pass: "mypassword123",
			from: "user@example.com",
			want: false,
		},
		{
			name: "blacklisted password 'password'",
			host: "smtp.example.com",
			port: "587",
			user: "user@example.com",
			pass: "password",
			from: "user@example.com",
			want: false,
		},
		{
			name: "blacklisted password 'null'",
			host: "smtp.example.com",
			port: "587",
			user: "user@example.com",
			pass: "null",
			from: "user@example.com",
			want: false,
		},
		{
			name: "blacklisted password 'PASSWORD' (case-insensitive)",
			host: "smtp.example.com",
			port: "587",
			user: "user@example.com",
			pass: "PASSWORD",
			from: "user@example.com",
			want: false,
		},
		{
			name: "password too short (3 chars)",
			host: "smtp.example.com",
			port: "587",
			user: "user@example.com",
			pass: "abc",
			from: "user@example.com",
			want: false,
		},
		{
			name: "password with JS chars",
			host: "smtp.example.com",
			port: "587",
			user: "user@example.com",
			pass: "}{pass",
			from: "user@example.com",
			want: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := a.isValidSMTPConfig(tc.host, tc.port, tc.user, tc.pass, tc.from)
			if got != tc.want {
				t.Errorf("isValidSMTPConfig(%q, %q, %q, %q, %q) = %v, want %v",
					tc.host, tc.port, tc.user, tc.pass, tc.from, got, tc.want)
			}
		})
	}
}

// ────────────────────────────────────────────────────────────────────────────
// loadEnvPaths
// ────────────────────────────────────────────────────────────────────────────

func TestLoadEnvPaths(t *testing.T) {
	// Helper: cd to tmp dir, return original wd for cleanup.
	chdir := func(t *testing.T, dir string) {
		t.Helper()
		orig, err := os.Getwd()
		if err != nil {
			t.Fatalf("Getwd: %v", err)
		}
		if err := os.Chdir(dir); err != nil {
			t.Fatalf("Chdir(%s): %v", dir, err)
		}
		t.Cleanup(func() { _ = os.Chdir(orig) })
	}

	t.Run("no paths.txt returns built-in list containing /.env", func(t *testing.T) {
		tmp, err := os.MkdirTemp("", "loadenv-nofile-*")
		if err != nil {
			t.Fatalf("MkdirTemp: %v", err)
		}
		defer os.RemoveAll(tmp)
		chdir(t, tmp)

		paths := loadEnvPaths()
		if len(paths) == 0 {
			t.Fatal("want non-empty built-in list")
		}
		found := false
		for _, p := range paths {
			if p == "/.env" {
				found = true
				break
			}
		}
		if !found {
			t.Error("built-in list does not contain '/.env'")
		}
	})

	t.Run("valid paths.txt returns custom paths", func(t *testing.T) {
		tmp, err := os.MkdirTemp("", "loadenv-custom-*")
		if err != nil {
			t.Fatalf("MkdirTemp: %v", err)
		}
		defer os.RemoveAll(tmp)
		chdir(t, tmp)

		content := "/custom/.env\n/another/.env\n"
		if err := os.WriteFile(filepath.Join(tmp, "paths.txt"), []byte(content), 0o644); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}

		paths := loadEnvPaths()
		if len(paths) != 2 {
			t.Fatalf("want 2 paths, got %d: %v", len(paths), paths)
		}
		if paths[0] != "/custom/.env" || paths[1] != "/another/.env" {
			t.Errorf("unexpected paths: %v", paths)
		}
	})

	t.Run("paths.txt with only comments and blank lines falls back to built-in", func(t *testing.T) {
		tmp, err := os.MkdirTemp("", "loadenv-commentsonly-*")
		if err != nil {
			t.Fatalf("MkdirTemp: %v", err)
		}
		defer os.RemoveAll(tmp)
		chdir(t, tmp)

		content := "# this is a comment\n\n   \n# another comment\n"
		if err := os.WriteFile(filepath.Join(tmp, "paths.txt"), []byte(content), 0o644); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}

		paths := loadEnvPaths()
		found := false
		for _, p := range paths {
			if p == "/.env" {
				found = true
				break
			}
		}
		if !found {
			t.Error("expected fallback to built-in list containing '/.env'")
		}
	})
}

// ────────────────────────────────────────────────────────────────────────────
// countLines
// ────────────────────────────────────────────────────────────────────────────

func TestCountLines(t *testing.T) {
	dir := t.TempDir()

	writeFile := func(name, content string) string {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("WriteFile %s: %v", name, err)
		}
		return path
	}

	tests := []struct {
		name     string
		content  string
		wantN    int
		wantErr  bool
		usePath  string // overrides generated path when set
	}{
		{
			name:    "empty file returns 0",
			content: "",
			wantN:   0,
		},
		{
			name:    "3 lines of content with 2 newlines returns 2",
			content: "line1\nline2\nline3",
			wantN:   2,
		},
		{
			name:    "5 newline-terminated lines returns 5",
			content: "a\nb\nc\nd\ne\n",
			wantN:   5,
		},
		{
			name:    "non-existent file returns error",
			usePath: filepath.Join(dir, "ghost_file_xyz.txt"),
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			path := tc.usePath
			if path == "" {
				path = writeFile(tc.name+".txt", tc.content)
			}

			n, err := countLines(path)
			if tc.wantErr {
				if err == nil {
					t.Error("want error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if n != tc.wantN {
				t.Errorf("countLines = %d, want %d", n, tc.wantN)
			}
		})
	}
}
