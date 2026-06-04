package main

// detectors_db_test.go
//
// Tests for DB credential extraction and DB validator gates.
//
// Run with: go test -v -run TestDB ./...

import (
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// ---------------------------------------------------------------------------
// Scanner constructor helpers
// ---------------------------------------------------------------------------

func dbTestScanner(t *testing.T, mysql, postgresql, redis bool) *AWSScanner {
	t.Helper()
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")

	b := func(v bool) string {
		if v {
			return "true"
		}
		return "false"
	}

	cfgJSON := `{
		"api_validation": {
			"mysql":      ` + b(mysql) + `,
			"postgresql": ` + b(postgresql) + `,
			"redis":      ` + b(redis) + `
		}
	}`
	if err := os.WriteFile(cfgPath, []byte(cfgJSON), 0644); err != nil {
		t.Fatalf("write temp config: %v", err)
	}
	return NewAWSScanner(cfgPath)
}

func resetDBState(a *AWSScanner) {
	globalCounters.mu.Lock()
	globalCounters.APIsFoundTotal = 0
	globalCounters.APIsValidated = 0
	globalCounters.mu.Unlock()
	a.KnownKeys = sync.Map{}
}

// ---------------------------------------------------------------------------
// extractDBCredsFromText — MySQL cluster
// ---------------------------------------------------------------------------

func TestExtractDBCreds_MySQLCluster(t *testing.T) {
	chdirTemp(t)
	a := dbTestScanner(t, true, false, false)
	resetDBState(a)

	text := strings.Join([]string{
		"APP_NAME=myapp",
		"DB_HOST=db.internal",
		"DB_PORT=3306",
		"DB_USER=appuser",
		"DB_PASSWORD=s3cr3tPass!",
		"DB_NAME=production",
		"DB_CONNECTION=mysql",
	}, "\n")

	a.extractDBCredsFromText(text, "http://example.com/.env")

	globalCounters.mu.Lock()
	found := globalCounters.APIsFoundTotal
	globalCounters.mu.Unlock()

	if found != 1 {
		t.Errorf("APIsFoundTotal = %d, want 1", found)
	}

	data, err := os.ReadFile("ResultJS/database_found.txt")
	if err != nil {
		t.Fatalf("ResultJS/database_found.txt not created: %v", err)
	}
	if !strings.Contains(string(data), "db.internal") {
		t.Errorf("database_found.txt missing host, got: %s", string(data))
	}
	if !strings.Contains(string(data), "appuser") {
		t.Errorf("database_found.txt missing user, got: %s", string(data))
	}
}

// ---------------------------------------------------------------------------
// extractDBCredsFromText — DATABASE_URL connection string
// ---------------------------------------------------------------------------

func TestExtractDBCreds_DatabaseURL_MySQL(t *testing.T) {
	chdirTemp(t)
	a := dbTestScanner(t, true, false, false)
	resetDBState(a)

	text := "DATABASE_URL=mysql://dbuser:mypassword@mysql.host.com:3306/mydb"

	a.extractDBCredsFromText(text, "http://example.com/.env")

	globalCounters.mu.Lock()
	found := globalCounters.APIsFoundTotal
	globalCounters.mu.Unlock()

	if found != 1 {
		t.Errorf("APIsFoundTotal = %d, want 1 (DATABASE_URL mysql)", found)
	}

	data, err := os.ReadFile("ResultJS/database_found.txt")
	if err != nil {
		t.Fatalf("ResultJS/database_found.txt not created: %v", err)
	}
	if !strings.Contains(string(data), "mysql.host.com") {
		t.Errorf("database_found.txt missing host from DATABASE_URL, got: %s", string(data))
	}
	if !strings.Contains(string(data), "dbuser") {
		t.Errorf("database_found.txt missing user from DATABASE_URL, got: %s", string(data))
	}
}

// ---------------------------------------------------------------------------
// extractDBCredsFromText — flag off, nothing extracted
// ---------------------------------------------------------------------------

func TestExtractDBCreds_FlagOff(t *testing.T) {
	chdirTemp(t)
	a := dbTestScanner(t, false, false, false)
	resetDBState(a)

	text := strings.Join([]string{
		"DB_HOST=db.internal",
		"DB_USER=appuser",
		"DB_PASSWORD=s3cr3tPass!",
		"DB_NAME=production",
		"DATABASE_URL=mysql://dbuser:mypassword@mysql.host.com:3306/mydb",
	}, "\n")

	a.extractDBCredsFromText(text, "http://example.com/.env")

	globalCounters.mu.Lock()
	found := globalCounters.APIsFoundTotal
	globalCounters.mu.Unlock()

	if found != 0 {
		t.Errorf("APIsFoundTotal = %d, want 0 (all DB flags off)", found)
	}
}

// ---------------------------------------------------------------------------
// extractDBCredsFromText — Postgres DATABASE_URL
// ---------------------------------------------------------------------------

func TestExtractDBCreds_DatabaseURL_Postgres(t *testing.T) {
	chdirTemp(t)
	a := dbTestScanner(t, false, true, false)
	resetDBState(a)

	text := "DATABASE_URL=postgres://pguser:pgpass@pg.host.com:5432/pgdb"

	a.extractDBCredsFromText(text, "http://example.com/.env")

	globalCounters.mu.Lock()
	found := globalCounters.APIsFoundTotal
	globalCounters.mu.Unlock()

	if found != 1 {
		t.Errorf("APIsFoundTotal = %d, want 1 (DATABASE_URL postgres)", found)
	}
}

// ---------------------------------------------------------------------------
// extractDBCredsFromText — Redis cluster
// ---------------------------------------------------------------------------

func TestExtractDBCreds_RedisCluster(t *testing.T) {
	chdirTemp(t)
	a := dbTestScanner(t, false, false, true)
	resetDBState(a)

	text := strings.Join([]string{
		"REDIS_HOST=redis.internal",
		"REDIS_PORT=6379",
		"REDIS_PASSWORD=redisp4ss",
	}, "\n")

	a.extractDBCredsFromText(text, "http://example.com/.env")

	globalCounters.mu.Lock()
	found := globalCounters.APIsFoundTotal
	globalCounters.mu.Unlock()

	if found != 1 {
		t.Errorf("APIsFoundTotal = %d, want 1 (redis cluster)", found)
	}
}

// ---------------------------------------------------------------------------
// CheckMySQL — gate off
// ---------------------------------------------------------------------------

func TestCheckMySQL_GateOff(t *testing.T) {
	a := dbTestScanner(t, false, false, false)
	got := a.CheckMySQL("127.0.0.1", "3306", "root", "pass", "mydb", "http://example.com")
	if got != false {
		t.Error("expected false when MySQL gate is off")
	}
}

// ---------------------------------------------------------------------------
// CheckMySQL — connection refused, no panic
// ---------------------------------------------------------------------------

func TestCheckMySQL_ConnectionRefused(t *testing.T) {
	chdirTemp(t)
	a := dbTestScanner(t, true, false, false)
	resetDBState(a)

	// Port 1 on loopback is reliably refused.
	got := a.CheckMySQL("127.0.0.1", "1", "root", "password123", "testdb", "http://127.0.0.1/.env")
	if got != false {
		t.Error("expected false when connection is refused")
	}

	globalCounters.mu.Lock()
	validated := globalCounters.APIsValidated
	globalCounters.mu.Unlock()
	if validated != 0 {
		t.Errorf("APIsValidated = %d, want 0", validated)
	}
}

// ---------------------------------------------------------------------------
// CheckRedis — gate off
// ---------------------------------------------------------------------------

func TestCheckRedis_GateOff(t *testing.T) {
	a := dbTestScanner(t, false, false, false)
	got := a.CheckRedis("127.0.0.1", "6379", "", "http://example.com")
	if got != false {
		t.Error("expected false when Redis gate is off")
	}
}

// ---------------------------------------------------------------------------
// CheckRedis — open server mock (PING → +PONG)
// ---------------------------------------------------------------------------

func TestCheckRedis_OpenMock(t *testing.T) {
	chdirTemp(t)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		buf := make([]byte, 64)
		conn.Read(buf)
		conn.Write([]byte("+PONG\r\n"))
	}()

	_, portStr, _ := net.SplitHostPort(ln.Addr().String())

	a := dbTestScanner(t, false, false, true)
	resetDBState(a)

	got := a.CheckRedis("127.0.0.1", portStr, "", "http://127.0.0.1/.env")
	if got != true {
		t.Error("expected true for open Redis (PING→+PONG)")
	}

	globalCounters.mu.Lock()
	validated := globalCounters.APIsValidated
	globalCounters.mu.Unlock()
	if validated != 1 {
		t.Errorf("APIsValidated = %d, want 1", validated)
	}

	data, err := os.ReadFile("ResultJS/valid_redis.txt")
	if err != nil {
		t.Fatalf("ResultJS/valid_redis.txt not created: %v", err)
	}
	if !strings.Contains(string(data), "127.0.0.1") {
		t.Errorf("valid_redis.txt missing host, got: %s", string(data))
	}
}

// ---------------------------------------------------------------------------
// CheckRedis — AUTH mock (+OK response)
// ---------------------------------------------------------------------------

func TestCheckRedis_AuthMock(t *testing.T) {
	chdirTemp(t)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		buf := make([]byte, 64)
		conn.Read(buf)
		conn.Write([]byte("+OK\r\n"))
	}()

	_, portStr, _ := net.SplitHostPort(ln.Addr().String())

	a := dbTestScanner(t, false, false, true)
	resetDBState(a)

	got := a.CheckRedis("127.0.0.1", portStr, "myredispass", "http://127.0.0.1/.env")
	if got != true {
		t.Error("expected true for authenticated Redis (AUTH→+OK)")
	}

	globalCounters.mu.Lock()
	validated := globalCounters.APIsValidated
	globalCounters.mu.Unlock()
	if validated != 1 {
		t.Errorf("APIsValidated = %d, want 1", validated)
	}
}

// ---------------------------------------------------------------------------
// CheckRedis — wrong password mock (-ERR response)
// ---------------------------------------------------------------------------

func TestCheckRedis_AuthFailed(t *testing.T) {
	chdirTemp(t)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		buf := make([]byte, 64)
		conn.Read(buf)
		conn.Write([]byte("-ERR invalid password\r\n"))
	}()

	_, portStr, _ := net.SplitHostPort(ln.Addr().String())

	a := dbTestScanner(t, false, false, true)
	resetDBState(a)

	got := a.CheckRedis("127.0.0.1", portStr, "wrongpass", "http://127.0.0.1/.env")
	if got != false {
		t.Error("expected false for wrong Redis password")
	}
}
