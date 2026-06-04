package main

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// Helper — build a scanner with specific web-panel validation flags enabled.
// ---------------------------------------------------------------------------

func makeWebPanelScanner(t *testing.T, cpanel, ftp, wordpress bool) *AWSScanner {
	t.Helper()
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	cfgJSON := fmt.Sprintf(`{
		"api_validation": {
			"cpanel":    %v,
			"ftp":       %v,
			"wordpress": %v
		},
		"scanning_features": {"aws_main_scan": false, "smtp_credentials_scan": false}
	}`, cpanel, ftp, wordpress)
	if err := os.WriteFile(cfgPath, []byte(cfgJSON), 0644); err != nil {
		t.Fatalf("write temp config: %v", err)
	}
	return NewAWSScanner(cfgPath)
}

// resetWebPanelState clears counters and KnownKeys for a clean slate.
func resetWebPanelState(a *AWSScanner) {
	globalCounters.mu.Lock()
	globalCounters.APIsFoundTotal = 0
	globalCounters.APIsValidated = 0
	globalCounters.mu.Unlock()
	a.KnownKeys = sync.Map{}
}

// resultJSContains checks whether ResultJS/<filename> contains <needle>.
// It does NOT fail the test if the file is absent — it returns false.
func resultJSContains(filename, needle string) bool {
	data, err := os.ReadFile(filepath.Join("ResultJS", filename))
	if err != nil {
		return false
	}
	return strings.Contains(string(data), needle)
}

// ---------------------------------------------------------------------------
// 1. extractWebPanelCredsFromText — FTP cluster → ftp_found.txt + counter
// ---------------------------------------------------------------------------

func TestExtractWebPanelCreds_FTP(t *testing.T) {
	a := makeWebPanelScanner(t, false, true, false)
	resetWebPanelState(a)

	text := `
APP_NAME=MyApp
FTP_HOST=ftp.example.com
FTP_PORT=21
FTP_USER=ftpuser
FTP_PASS=S3cr3tPass
DB_HOST=localhost
`
	a.extractWebPanelCredsFromText(text, "http://example.com/.env")

	// Give the async counter increment a moment to land
	time.Sleep(20 * time.Millisecond)

	globalCounters.mu.Lock()
	total := globalCounters.APIsFoundTotal
	globalCounters.mu.Unlock()

	if total == 0 {
		t.Error("APIsFoundTotal should be > 0 after FTP cluster detection")
	}

	// ftp_found.txt should contain the host and user
	if !resultJSContains("ftp_found.txt", "ftp.example.com") {
		t.Error("ftp_found.txt should contain ftp.example.com")
	}
	if !resultJSContains("ftp_found.txt", "ftpuser") {
		t.Error("ftp_found.txt should contain ftpuser")
	}
}

// ---------------------------------------------------------------------------
// 2. extractWebPanelCredsFromText — WordPress cluster → wordpress_found.txt
// ---------------------------------------------------------------------------

func TestExtractWebPanelCreds_WordPress(t *testing.T) {
	a := makeWebPanelScanner(t, false, false, true)
	resetWebPanelState(a)

	text := `
WP_HOME=https://mysite.example.com
WP_ADMIN_USER=admin
WP_ADMIN_PASSWORD=SuperSecret123
SOME_OTHER_VAR=ignored
`
	a.extractWebPanelCredsFromText(text, "http://mysite.example.com/.env")

	time.Sleep(20 * time.Millisecond)

	globalCounters.mu.Lock()
	total := globalCounters.APIsFoundTotal
	globalCounters.mu.Unlock()

	if total == 0 {
		t.Error("APIsFoundTotal should be > 0 after WordPress cluster detection")
	}

	if !resultJSContains("wordpress_found.txt", "mysite.example.com") {
		t.Error("wordpress_found.txt should contain mysite.example.com")
	}
	if !resultJSContains("wordpress_found.txt", "admin") {
		t.Error("wordpress_found.txt should contain admin")
	}
}

// ---------------------------------------------------------------------------
// 3. extractWebPanelCredsFromText — all gates off → nothing written / no count
// ---------------------------------------------------------------------------

func TestExtractWebPanelCreds_GatesOff(t *testing.T) {
	a := makeWebPanelScanner(t, false, false, false)
	resetWebPanelState(a)

	text := `
FTP_HOST=ftp.example.com
FTP_USER=ftpuser
FTP_PASS=S3cr3tPass
WP_HOME=https://mysite.example.com
WP_ADMIN_USER=admin
WP_ADMIN_PASSWORD=SuperSecret123
CPANEL_HOST=cpanel.example.com
CPANEL_USER=cpuser
CPANEL_PASS=cppass1234
`
	a.extractWebPanelCredsFromText(text, "http://example.com/.env")

	time.Sleep(20 * time.Millisecond)

	globalCounters.mu.Lock()
	total := globalCounters.APIsFoundTotal
	globalCounters.mu.Unlock()

	if total != 0 {
		t.Errorf("APIsFoundTotal should be 0 when all gates are off, got %d", total)
	}
}

// ---------------------------------------------------------------------------
// 4. CheckFTP — gate off → false
// ---------------------------------------------------------------------------

func TestCheckFTP_GateOff(t *testing.T) {
	a := makeWebPanelScanner(t, false, false, false)
	got := a.CheckFTP("127.0.0.1", "21", "user", "pass", "http://example.com")
	if got {
		t.Error("CheckFTP should return false when FTP gate is disabled")
	}
}

// ---------------------------------------------------------------------------
// 5. CheckFTP — mock server returns 230 (login success) → returns true
// ---------------------------------------------------------------------------

func TestCheckFTP_MockSuccess(t *testing.T) {
	// Start a mock FTP server that accepts one connection.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	_, portStr, _ := net.SplitHostPort(ln.Addr().String())

	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()

		// Banner
		fmt.Fprintf(conn, "220 Mock FTP Server\r\n")

		buf := make([]byte, 256)
		// USER command
		conn.Read(buf) //nolint:errcheck
		fmt.Fprintf(conn, "331 Please specify the password.\r\n")

		// PASS command
		conn.Read(buf) //nolint:errcheck
		fmt.Fprintf(conn, "230 Login successful.\r\n")
	}()

	a := makeWebPanelScanner(t, false, true, false)
	a.KnownKeys = sync.Map{} // ensure fresh dedup map

	got := a.CheckFTP("127.0.0.1", portStr, "ftpuser", "ftppass", "http://example.com")
	if !got {
		t.Error("CheckFTP should return true when server responds 230")
	}
}

// ---------------------------------------------------------------------------
// 6. CheckFTP — mock server returns 530 (bad credentials) → returns false
// ---------------------------------------------------------------------------

func TestCheckFTP_MockFailure(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	_, portStr, _ := net.SplitHostPort(ln.Addr().String())

	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()

		fmt.Fprintf(conn, "220 Mock FTP Server\r\n")

		buf := make([]byte, 256)
		conn.Read(buf) //nolint:errcheck
		fmt.Fprintf(conn, "331 Please specify the password.\r\n")

		conn.Read(buf) //nolint:errcheck
		fmt.Fprintf(conn, "530 Login incorrect.\r\n")
	}()

	a := makeWebPanelScanner(t, false, true, false)
	a.KnownKeys = sync.Map{}

	got := a.CheckFTP("127.0.0.1", portStr, "baduser", "badpass", "http://example.com")
	if got {
		t.Error("CheckFTP should return false when server responds 530")
	}
}

// ---------------------------------------------------------------------------
// 7. CheckCPanel — gate off → false
// ---------------------------------------------------------------------------

func TestCheckCPanel_GateOff(t *testing.T) {
	a := makeWebPanelScanner(t, false, false, false)
	got := a.CheckCPanel("example.com", "user", "pass", "http://example.com")
	if got {
		t.Error("CheckCPanel should return false when cPanel gate is disabled")
	}
}

// ---------------------------------------------------------------------------
// 8. CheckWordPress — gate off → false
// ---------------------------------------------------------------------------

func TestCheckWordPress_GateOff(t *testing.T) {
	a := makeWebPanelScanner(t, false, false, false)
	got := a.CheckWordPress("https://example.com", "admin", "password", "http://example.com")
	if got {
		t.Error("CheckWordPress should return false when WordPress gate is disabled")
	}
}

// ---------------------------------------------------------------------------
// 9. extractFTPFromEnv — raw ftp:// URL extraction
// ---------------------------------------------------------------------------

func TestExtractFTPFromEnv_RawURL(t *testing.T) {
	a := makeWebPanelScanner(t, false, true, false)
	resetWebPanelState(a)

	text := `DATABASE_URL=mysql://db:3306
FTP_URL=ftp://ftpuser:ftppass@ftp.example.com:2121/uploads
REDIS_URL=redis://localhost:6379`

	a.extractFTPFromEnv(text, "http://example.com/.env")

	time.Sleep(20 * time.Millisecond)

	globalCounters.mu.Lock()
	total := globalCounters.APIsFoundTotal
	globalCounters.mu.Unlock()

	if total == 0 {
		t.Error("APIsFoundTotal should be > 0 after raw ftp:// URL extraction")
	}
}

// ---------------------------------------------------------------------------
// 10. extractFTPFromEnv — gate off → nothing counted
// ---------------------------------------------------------------------------

func TestExtractFTPFromEnv_GateOff(t *testing.T) {
	a := makeWebPanelScanner(t, false, false, false)
	resetWebPanelState(a)

	text := `FTP_URL=ftp://ftpuser:ftppass@ftp.example.com:21/`
	a.extractFTPFromEnv(text, "http://example.com/.env")

	time.Sleep(20 * time.Millisecond)

	globalCounters.mu.Lock()
	total := globalCounters.APIsFoundTotal
	globalCounters.mu.Unlock()

	if total != 0 {
		t.Errorf("APIsFoundTotal should be 0 when FTP gate is off, got %d", total)
	}
}
