package main

// detectors_ssh_test.go
//
// Tests for SSH credential extraction, host URL parsing, and SSH validator stubs.
//
// Run with: go test -v -run TestSSH ./...

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// ---------------------------------------------------------------------------
// Scanner constructor helpers
// ---------------------------------------------------------------------------

// sshTestScanner builds an AWSScanner with SSHScan and APIValidation.SSH
// set to the provided values, and every other flag off.
func sshTestScanner(t *testing.T, sshScan bool, sshValidate bool) *AWSScanner {
	t.Helper()
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")

	sshScanStr := "false"
	if sshScan {
		sshScanStr = "true"
	}
	sshValidateStr := "false"
	if sshValidate {
		sshValidateStr = "true"
	}

	cfgJSON := `{
		"scanning_features": {
			"aws_main_scan": false,
			"smtp_credentials_scan": false,
			"ssh_scan": ` + sshScanStr + `
		},
		"api_validation": {
			"ssh": ` + sshValidateStr + `
		}
	}`
	if err := os.WriteFile(cfgPath, []byte(cfgJSON), 0644); err != nil {
		t.Fatalf("write temp config: %v", err)
	}
	return NewAWSScanner(cfgPath)
}

func resetSSHState(a *AWSScanner) {
	globalCounters.mu.Lock()
	globalCounters.APIsFoundTotal = 0
	globalCounters.APIsValidated = 0
	globalCounters.mu.Unlock()
	a.KnownKeys = sync.Map{}
}

// generateTestPrivateKey produces a valid RSA 2048-bit private key in PEM format.
func generateTestPrivateKey(t *testing.T) string {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	block := &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(key),
	}
	return string(pem.EncodeToMemory(block))
}

// ---------------------------------------------------------------------------
// extractHostFromURL
// ---------------------------------------------------------------------------

func TestExtractHostFromURL(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"https://1.2.3.4/.ssh/id_rsa", "1.2.3.4"},
		{"http://example.com:8080/path", "example.com"},
		{"1.2.3.4", "1.2.3.4"},
		{"https://hostname.example.com/foo/bar", "hostname.example.com"},
		{"http://1.2.3.4/", "1.2.3.4"},
	}

	for _, tc := range cases {
		got := extractHostFromURL(tc.input)
		if got != tc.want {
			t.Errorf("extractHostFromURL(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

// ---------------------------------------------------------------------------
// extractSSHCredsFromText
// ---------------------------------------------------------------------------

// TestExtractSSHCreds_CompleteCluster verifies that a HOST+USER+PASS block
// within 8 lines is extracted and APIsFoundTotal is incremented.
func TestExtractSSHCreds_CompleteCluster(t *testing.T) {
	chdirTemp(t)
	a := sshTestScanner(t, true, false)
	resetSSHState(a)

	text := strings.Join([]string{
		"APP_NAME=myapp",
		"SSH_HOST=192.168.1.100",
		"SSH_USER=deployer",
		"SSH_PASS=s3cr3tPass!",
		"DB_HOST=localhost",
	}, "\n")

	a.extractSSHCredsFromText(text, "http://192.168.1.100/.env")

	globalCounters.mu.Lock()
	found := globalCounters.APIsFoundTotal
	globalCounters.mu.Unlock()

	if found != 1 {
		t.Errorf("APIsFoundTotal = %d, want 1", found)
	}

	// Confirm the file was written (saveIntoFile writes into ResultJS/)
	data, err := os.ReadFile("ResultJS/ssh_found.txt")
	if err != nil {
		t.Fatalf("ResultJS/ssh_found.txt not created: %v", err)
	}
	if !strings.Contains(string(data), "192.168.1.100") {
		t.Errorf("ResultJS/ssh_found.txt missing host, got: %s", string(data))
	}
}

// TestExtractSSHCreds_MissingPass verifies no extraction when SSH_PASS is absent.
func TestExtractSSHCreds_MissingPass(t *testing.T) {
	chdirTemp(t)
	a := sshTestScanner(t, true, false)
	resetSSHState(a)

	text := strings.Join([]string{
		"SSH_HOST=192.168.1.101",
		"SSH_USER=admin",
		// No SSH_PASS
	}, "\n")

	a.extractSSHCredsFromText(text, "http://192.168.1.101/.env")

	globalCounters.mu.Lock()
	found := globalCounters.APIsFoundTotal
	globalCounters.mu.Unlock()

	if found != 0 {
		t.Errorf("APIsFoundTotal = %d, want 0 (no password present)", found)
	}
}

// TestExtractSSHCreds_FieldsTooFarApart verifies no extraction when fields
// are more than 8 lines apart from the host line.
func TestExtractSSHCreds_FieldsTooFarApart(t *testing.T) {
	chdirTemp(t)
	a := sshTestScanner(t, true, false)
	resetSSHState(a)

	lines := make([]string, 30)
	lines[0] = "SSH_HOST=10.0.0.1"
	// USER and PASS more than 8 lines away from HOST
	lines[20] = "SSH_USER=ops"
	lines[25] = "SSH_PASS=faraway999"

	text := strings.Join(lines, "\n")
	a.extractSSHCredsFromText(text, "http://10.0.0.1/.env")

	globalCounters.mu.Lock()
	found := globalCounters.APIsFoundTotal
	globalCounters.mu.Unlock()

	if found != 0 {
		t.Errorf("APIsFoundTotal = %d, want 0 (fields too far apart)", found)
	}
}

// TestExtractSSHCreds_SSHScanFlagOff verifies nothing is extracted when the
// SSHScan feature flag is disabled.
func TestExtractSSHCreds_SSHScanFlagOff(t *testing.T) {
	chdirTemp(t)
	a := sshTestScanner(t, false, false)
	resetSSHState(a)

	text := strings.Join([]string{
		"SSH_HOST=10.0.0.2",
		"SSH_USER=root",
		"SSH_PASS=password123",
	}, "\n")

	a.extractSSHCredsFromText(text, "http://10.0.0.2/.env")

	globalCounters.mu.Lock()
	found := globalCounters.APIsFoundTotal
	globalCounters.mu.Unlock()

	if found != 0 {
		t.Errorf("APIsFoundTotal = %d, want 0 (SSHScan is off)", found)
	}
}

// TestExtractSSHCreds_CustomPort verifies that an explicit SSH_PORT is captured.
func TestExtractSSHCreds_CustomPort(t *testing.T) {
	chdirTemp(t)
	a := sshTestScanner(t, true, false)
	resetSSHState(a)

	text := strings.Join([]string{
		"SSH_HOST=10.0.0.3",
		"SSH_PORT=2222",
		"SSH_USER=ci",
		"SSH_PASS=cipass456",
	}, "\n")

	a.extractSSHCredsFromText(text, "http://10.0.0.3/.env")

	globalCounters.mu.Lock()
	found := globalCounters.APIsFoundTotal
	globalCounters.mu.Unlock()

	if found != 1 {
		t.Errorf("APIsFoundTotal = %d, want 1", found)
	}

	data, err := os.ReadFile("ResultJS/ssh_found.txt")
	if err != nil {
		t.Fatalf("ResultJS/ssh_found.txt not created: %v", err)
	}
	if !strings.Contains(string(data), "2222") {
		t.Errorf("ResultJS/ssh_found.txt missing custom port 2222, got: %s", string(data))
	}
}

// ---------------------------------------------------------------------------
// CheckSSHPrivateKey
// ---------------------------------------------------------------------------

// TestCheckSSHPrivateKey_GateOff verifies that when APIValidation.SSH is false
// the function returns false immediately without attempting a connection.
func TestCheckSSHPrivateKey_GateOff(t *testing.T) {
	a := sshTestScanner(t, true, false) // ssh validate = false
	pemKey := generateTestPrivateKey(t)

	got := a.CheckSSHPrivateKey(pemKey, "https://1.2.3.4/.ssh/id_rsa")
	if got != false {
		t.Error("expected false when APIValidation.SSH is off")
	}
}

// TestCheckSSHPrivateKey_InvalidPEM verifies that a malformed PEM block
// returns false (parse error path).
func TestCheckSSHPrivateKey_InvalidPEM(t *testing.T) {
	a := sshTestScanner(t, true, true) // ssh validate = true

	got := a.CheckSSHPrivateKey("-----BEGIN RSA PRIVATE KEY-----\nNOTVALIDB64\n-----END RSA PRIVATE KEY-----", "https://1.2.3.4/.ssh/id_rsa")
	if got != false {
		t.Error("expected false for invalid PEM block")
	}
}

// TestCheckSSHPrivateKey_ValidKeyNoHost verifies that a valid PEM key with an
// empty/unparseable URL host returns false without panicking.
func TestCheckSSHPrivateKey_ValidKeyNoHost(t *testing.T) {
	a := sshTestScanner(t, true, true)
	pemKey := generateTestPrivateKey(t)

	// Empty source URL → extractHostFromURL returns ""
	got := a.CheckSSHPrivateKey(pemKey, "")
	if got != false {
		t.Error("expected false when host cannot be extracted from sourceURL")
	}
}

// TestCheckSSHPrivateKey_ValidKeyConnectionRefused verifies that a valid RSA
// key with a live-unreachable address (guaranteed refused) returns false and
// does not panic.
func TestCheckSSHPrivateKey_ValidKeyConnectionRefused(t *testing.T) {
	chdirTemp(t)
	a := sshTestScanner(t, true, true)
	pemKey := generateTestPrivateKey(t)

	// Port 1 on loopback is reliably refused (not firewalled, just closed).
	got := a.CheckSSHPrivateKey(pemKey, "http://127.0.0.1:1/.ssh/id_rsa")
	if got != false {
		t.Error("expected false when connection is refused (no SSH server)")
	}
}

// ---------------------------------------------------------------------------
// CheckSSH
// ---------------------------------------------------------------------------

// TestCheckSSH_GateOff verifies that the password validator gate returns false
// immediately when APIValidation.SSH is disabled.
func TestCheckSSH_GateOff(t *testing.T) {
	a := sshTestScanner(t, true, false) // ssh validate = false

	got := a.CheckSSH("127.0.0.1", "22", "root", "password", "http://example.com")
	if got != false {
		t.Error("expected false when APIValidation.SSH is off")
	}
}

// TestCheckSSH_ConnectionRefused verifies that an SSH password attempt to a
// port that is guaranteed refused returns false without panicking.
func TestCheckSSH_ConnectionRefused(t *testing.T) {
	chdirTemp(t)
	a := sshTestScanner(t, true, true)
	resetSSHState(a)

	// Port 1 on loopback is reliably connection-refused.
	got := a.CheckSSH("127.0.0.1", "1", "root", "password123", "http://127.0.0.1/.env")
	if got != false {
		t.Error("expected false when connection is refused")
	}

	globalCounters.mu.Lock()
	validated := globalCounters.APIsValidated
	globalCounters.mu.Unlock()
	if validated != 0 {
		t.Errorf("APIsValidated = %d, want 0 (no successful connection)", validated)
	}
}
