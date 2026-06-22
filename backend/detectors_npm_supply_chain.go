package main

// npm supply chain analysis — four checks run when package.json is fetched:
//
//  1. License compliance  — flags copyleft/restrictive licenses in the manifest
//  2. Typosquat detection — Levenshtein ≤2 from top-100 npm packages (local, no API)
//  3. Registry metadata   — age, maintainer count, missing repository for suspects
//  4. R2S passive         — verifies the stated repository URL is reachable
//
// Output files: license_issues.txt, npm_supply_chain.txt
// Telegram alert on: typosquat hit, package < 30 days old, or zero maintainers

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// ── License compliance ────────────────────────────────────────────────────────

var restrictedLicenses = map[string]string{
	"agpl-3.0":          "Copyleft — use forces open-sourcing your entire application",
	"agpl-3.0-only":     "Copyleft — use forces open-sourcing your entire application",
	"agpl-3.0-or-later": "Copyleft — use forces open-sourcing your entire application",
	"gpl-2.0":           "Copyleft — may require open-sourcing your application",
	"gpl-2.0-only":      "Copyleft — may require open-sourcing your application",
	"gpl-3.0":           "Copyleft — may require open-sourcing your application",
	"gpl-3.0-only":      "Copyleft — may require open-sourcing your application",
	"gpl-3.0-or-later":  "Copyleft — may require open-sourcing your application",
	"lgpl-2.0":          "Weak copyleft — library use usually safe, modifications are not",
	"lgpl-2.1":          "Weak copyleft — library use usually safe, modifications are not",
	"lgpl-3.0":          "Weak copyleft — library use usually safe, modifications are not",
	"sspl-1.0":          "SSPL — offering the software as a service triggers open-source obligation",
	"busl-1.1":          "Business Source License — production use restricted until change date",
	"cc-by-nc-4.0":      "Non-commercial — commercial use prohibited",
	"cc-by-nc-sa-4.0":   "Non-commercial ShareAlike — commercial use prohibited",
	"commons clause":    "Commons Clause rider — selling the software is prohibited",
	"elastic-2.0":       "Elastic License 2.0 — SaaS/managed service use prohibited",
}

func (a *AWSScanner) checkLicenseCompliance(content, sourceURL string) {
	var pkg struct {
		Name    string      `json:"name"`
		Version string      `json:"version"`
		License interface{} `json:"license"` // string OR {type, url}
	}
	if err := json.Unmarshal([]byte(content), &pkg); err != nil {
		return
	}

	var licenseStr string
	switch v := pkg.License.(type) {
	case string:
		licenseStr = v
	case map[string]interface{}:
		if t, ok := v["type"].(string); ok {
			licenseStr = t
		}
	}
	if licenseStr == "" {
		return
	}

	lower := strings.ToLower(licenseStr)
	for restricted, reason := range restrictedLicenses {
		if strings.Contains(lower, restricted) {
			name := pkg.Name
			if name == "" {
				name = sanitizeSource(sourceURL)
			}
			line := fmt.Sprintf("%s | package=%s@%s | license=%s | %s",
				sanitizeSource(sourceURL), name, pkg.Version, licenseStr, reason)
			a.saveIntoFile(line, "license_issues.txt")
			a.logValid("License", fmt.Sprintf("%s → %s", name, licenseStr))
			return
		}
	}
}

// ── Typosquat detection ───────────────────────────────────────────────────────

// topNPMPackages is the set most frequently impersonated in supply-chain attacks.
var topNPMPackages = []string{
	"lodash", "react", "react-dom", "chalk", "commander", "yargs", "express",
	"moment", "axios", "webpack", "babel-runtime", "bluebird", "request",
	"underscore", "async", "vue", "jquery", "typescript", "jest", "mocha",
	"eslint", "prettier", "nodemon", "dotenv", "cors", "mongoose", "sequelize",
	"passport", "jsonwebtoken", "bcrypt", "uuid", "ramda", "rxjs", "redux",
	"socket.io", "next", "nuxt", "svelte", "gatsby", "vite", "rollup",
	"parcel", "esbuild", "tslib", "semver", "minimist", "mkdirp", "rimraf",
	"cross-env", "body-parser", "helmet", "morgan", "debug", "colors",
	"inquirer", "ora", "winston", "pino", "zod", "joi", "ajv",
	"date-fns", "dayjs", "typeorm", "prisma", "pg", "mysql2", "redis",
	"ioredis", "bull", "bullmq", "node-fetch", "got", "superagent",
	"fs-extra", "glob", "chokidar", "crypto-js", "bcryptjs", "argon2",
	"aws-sdk", "stripe", "twilio", "firebase-admin", "googleapis",
	"sharp", "multer", "formidable", "mime", "file-type",
}

type typosquatHit struct {
	Suspect  string
	Imitates string
	Distance int
}

func levenshtein(a, b string) int {
	la, lb := len(a), len(b)
	if la == 0 {
		return lb
	}
	if lb == 0 {
		return la
	}
	prev := make([]int, lb+1)
	curr := make([]int, lb+1)
	for j := 0; j <= lb; j++ {
		prev[j] = j
	}
	for i := 1; i <= la; i++ {
		curr[0] = i
		for j := 1; j <= lb; j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			del := curr[j-1] + 1
			ins := prev[j] + 1
			sub := prev[j-1] + cost
			if del <= ins && del <= sub {
				curr[j] = del
			} else if ins <= sub {
				curr[j] = ins
			} else {
				curr[j] = sub
			}
		}
		prev, curr = curr, prev
	}
	return prev[lb]
}

func detectTyposquats(deps []packageDep) []typosquatHit {
	var hits []typosquatHit
	for _, dep := range deps {
		name := dep.Name
		// Strip npm scope — @scope/pkg → compare only pkg portion
		if idx := strings.LastIndex(name, "/"); idx >= 0 {
			name = name[idx+1:]
		}
		for _, popular := range topNPMPackages {
			if strings.EqualFold(name, popular) {
				break // exact match — not a typosquat
			}
			dist := levenshtein(strings.ToLower(name), popular)
			// Distance 1 = very likely typosquat; 2 = suspicious for short names
			if dist == 1 || (dist == 2 && len(name) <= 8) {
				hits = append(hits, typosquatHit{
					Suspect:  dep.Name,
					Imitates: popular,
					Distance: dist,
				})
				break
			}
		}
	}
	return hits
}

// ── npm registry metadata + R2S passive ──────────────────────────────────────

type npmRegistryMeta struct {
	Time struct {
		Created  string `json:"created"`
		Modified string `json:"modified"`
	} `json:"time"`
	Maintainers []struct {
		Name string `json:"name"`
	} `json:"maintainers"`
	Repository struct {
		Type string `json:"type"`
		URL  string `json:"url"`
	} `json:"repository"`
}

var npmHTTPClient = &http.Client{Timeout: 10 * time.Second}

// checkNPMRegistryPackage fetches the registry record for a package and reports:
// - age < 30 days (brand-new, suspicious for a dependency)
// - zero declared maintainers
// - no repository field (anonymous package)
// - repository URL returns 4xx/5xx (R2S passive)
func (a *AWSScanner) checkNPMRegistryPackage(pkgName, sourceURL string) {
	url := "https://registry.npmjs.org/" + pkgName
	resp, err := npmHTTPClient.Get(url)
	if err != nil || resp.StatusCode != 200 {
		return
	}
	defer resp.Body.Close()
	rb, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if err != nil {
		return
	}
	var meta npmRegistryMeta
	if err := json.Unmarshal(rb, &meta); err != nil {
		return
	}

	var alerts []string

	// Age check
	if meta.Time.Created != "" {
		created, err := time.Parse(time.RFC3339, meta.Time.Created)
		if err == nil && time.Since(created) < 30*24*time.Hour {
			ageHours := int(time.Since(created).Hours())
			alerts = append(alerts, fmt.Sprintf("⏰ NEW package — first published %dh ago", ageHours))
		}
	}

	// Maintainer count
	if len(meta.Maintainers) == 0 {
		alerts = append(alerts, "👻 ZERO maintainers listed on registry")
	}

	// Missing repository
	if meta.Repository.URL == "" {
		alerts = append(alerts, "🔍 No repository URL — cannot verify source")
	} else {
		// R2S passive: clean up git+ prefix and .git suffix, do HEAD check
		repoURL := meta.Repository.URL
		repoURL = strings.TrimPrefix(repoURL, "git+")
		repoURL = strings.TrimSuffix(repoURL, ".git")
		if strings.HasPrefix(repoURL, "https://") || strings.HasPrefix(repoURL, "http://") {
			req, err := http.NewRequest("HEAD", repoURL, nil)
			if err == nil {
				req.Header.Set("User-Agent", "RavenX/2.0")
				r2sClient := &http.Client{Timeout: 8 * time.Second}
				r2sResp, r2sErr := r2sClient.Do(req)
				if r2sErr == nil {
					r2sResp.Body.Close()
					if r2sResp.StatusCode == 404 || r2sResp.StatusCode == 410 {
						alerts = append(alerts, fmt.Sprintf("🔗 R2S: repository URL returns %d — source no longer exists", r2sResp.StatusCode))
					}
				} else {
					alerts = append(alerts, "🔗 R2S: repository URL unreachable")
				}
			}
		}
	}

	if len(alerts) == 0 {
		return
	}

	for _, alert := range alerts {
		line := fmt.Sprintf("%s | pkg=%s | %s", sanitizeSource(sourceURL), pkgName, alert)
		a.saveIntoFile(line, "npm_supply_chain.txt")
		a.logValid("Supply Chain", fmt.Sprintf("%s → %s", pkgName, alert))
	}

	msg := fmt.Sprintf("⚠️ *npm Supply Chain Alert*\nSource: `%s`\nPackage: `%s`\n\n%s",
		sanitizeSource(sourceURL), pkgName, strings.Join(alerts, "\n"))
	go a.sendTelegram(msg)
}

// ── Main entry point ──────────────────────────────────────────────────────────

// checkNPMSupplyChain is called from the LIB path-scan loop when package.json
// is fetched. Runs license check and typosquat detection synchronously, then
// dispatches registry checks async for each typosquat hit.
func (a *AWSScanner) checkNPMSupplyChain(content, sourceURL, pathLower string) {
	// Only operates on package.json, not lock files
	if !strings.HasSuffix(pathLower, "package.json") ||
		strings.HasSuffix(pathLower, "package-lock.json") {
		return
	}

	a.checkLicenseCompliance(content, sourceURL)

	deps := parsePackageJSON(content)
	if len(deps) == 0 {
		return
	}

	hits := detectTyposquats(deps)
	for _, hit := range hits {
		line := fmt.Sprintf("%s | suspect=%s | imitates=%s | edit_distance=%d",
			sanitizeSource(sourceURL), hit.Suspect, hit.Imitates, hit.Distance)
		a.saveIntoFile(line, "npm_supply_chain.txt")
		a.logValid("Typosquat", fmt.Sprintf("%s → looks like %s (dist=%d)", hit.Suspect, hit.Imitates, hit.Distance))

		msg := fmt.Sprintf("⚠️ *npm Typosquat*\nSource: `%s`\n`%s` may be impersonating `%s` (edit distance %d)",
			sanitizeSource(sourceURL), hit.Suspect, hit.Imitates, hit.Distance)
		go a.sendTelegram(msg)
		go a.checkNPMRegistryPackage(hit.Suspect, sourceURL)
	}
}
