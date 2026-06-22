package main

// OSV CVE integration — queries api.osv.dev/v1/querybatch for known
// vulnerabilities against package versions found in manifest files fetched
// by the LIB scanner (package.json, requirements.txt, go.mod, Gemfile.lock,
// composer.json, yarn.lock, package-lock.json, Pipfile.lock, pyproject.toml).
//
// Findings are saved to cve_found.txt; CRITICAL/HIGH severity findings also
// trigger a Telegram alert.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

// ── Types ─────────────────────────────────────────────────────────────────────

type packageDep struct {
	Name      string
	Version   string
	Ecosystem string
}

type cveFinding struct {
	Dep   packageDep
	Vulns []osvVuln
}

// ── OSV API wire types ────────────────────────────────────────────────────────

type osvPkg struct {
	Name      string `json:"name"`
	Ecosystem string `json:"ecosystem"`
}

type osvQuery struct {
	Package osvPkg `json:"package"`
	Version string `json:"version"`
}

type osvBatchRequest struct {
	Queries []osvQuery `json:"queries"`
}

type osvSeverity struct {
	Type  string `json:"type"`
	Score string `json:"score"` // CVSS vector string e.g. "CVSS:3.1/AV:N/..."
}

type osvVuln struct {
	ID       string        `json:"id"`
	Aliases  []string      `json:"aliases"`
	Summary  string        `json:"summary"`
	Severity []osvSeverity `json:"severity"`
}

type osvQueryResult struct {
	Vulns []osvVuln `json:"vulns"`
}

type osvBatchResponse struct {
	Results []osvQueryResult `json:"results"`
}

// ── Manifest detection ────────────────────────────────────────────────────────

func isPackageManifest(pathLower string) bool {
	for _, suffix := range []string{
		"package.json", "package-lock.json", "yarn.lock",
		"requirements.txt", "pipfile.lock", "pyproject.toml",
		"composer.json", "composer.lock",
		"gemfile.lock",
		"go.mod",
	} {
		if strings.HasSuffix(pathLower, suffix) {
			return true
		}
	}
	return false
}

// ── Version normalisation ─────────────────────────────────────────────────────

var semverRe = regexp.MustCompile(`\d+\.\d+(?:\.\d+)?`)

func extractVersion(raw string) string {
	return semverRe.FindString(raw)
}

// ── Per-manifest parsers ──────────────────────────────────────────────────────

func parsePackageJSON(content string) []packageDep {
	var pkg struct {
		Dependencies    map[string]string `json:"dependencies"`
		DevDependencies map[string]string `json:"devDependencies"`
	}
	if err := json.Unmarshal([]byte(content), &pkg); err != nil {
		return nil
	}
	all := make(map[string]string)
	for k, v := range pkg.Dependencies {
		all[k] = v
	}
	for k, v := range pkg.DevDependencies {
		all[k] = v
	}
	var deps []packageDep
	for name, ver := range all {
		if v := extractVersion(ver); v != "" {
			deps = append(deps, packageDep{Name: name, Version: v, Ecosystem: "npm"})
		}
	}
	return deps
}

func parsePackageLockJSON(content string) []packageDep {
	var lock struct {
		Packages     map[string]struct{ Version string `json:"version"` } `json:"packages"`
		Dependencies map[string]struct{ Version string `json:"version"` } `json:"dependencies"`
	}
	if err := json.Unmarshal([]byte(content), &lock); err != nil {
		return nil
	}
	var deps []packageDep
	// npm v7+ — packages keyed as "node_modules/name"
	for path, pkg := range lock.Packages {
		if path == "" || pkg.Version == "" {
			continue
		}
		parts := strings.Split(path, "/")
		name := parts[len(parts)-1]
		// handle scoped packages "node_modules/@scope/name"
		if len(parts) >= 2 && strings.HasPrefix(parts[len(parts)-2], "@") {
			name = parts[len(parts)-2] + "/" + parts[len(parts)-1]
		}
		deps = append(deps, packageDep{Name: name, Version: pkg.Version, Ecosystem: "npm"})
	}
	// npm v6 fallback
	if len(deps) == 0 {
		for name, pkg := range lock.Dependencies {
			if pkg.Version != "" {
				deps = append(deps, packageDep{Name: name, Version: pkg.Version, Ecosystem: "npm"})
			}
		}
	}
	return deps
}

func parseYarnLock(content string) []packageDep {
	pkgLineRe := regexp.MustCompile(`^"?([^@\s"]+)@`)
	// handles both classic "version "1.2.3"" and berry "version: 1.2.3"
	verLineRe := regexp.MustCompile(`^\s+version[: ]+"?(\d+\.\d+(?:\.\d+)?)"?`)
	var deps []packageDep
	lastPkg := ""
	for _, line := range strings.Split(content, "\n") {
		if m := pkgLineRe.FindStringSubmatch(line); len(m) == 2 {
			lastPkg = strings.TrimSpace(m[1])
		} else if m := verLineRe.FindStringSubmatch(line); len(m) == 2 && lastPkg != "" {
			deps = append(deps, packageDep{Name: lastPkg, Version: m[1], Ecosystem: "npm"})
			lastPkg = ""
		}
	}
	return deps
}

func parseRequirementsTxt(content string) []packageDep {
	splitRe := regexp.MustCompile(`[=<>!~]+`)
	var deps []packageDep
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "-") {
			continue
		}
		// strip inline comments
		if idx := strings.Index(line, " #"); idx != -1 {
			line = line[:idx]
		}
		parts := splitRe.Split(line, 2)
		if len(parts) < 2 {
			continue
		}
		// strip extras e.g. "requests[security]"
		name := strings.TrimSpace(strings.Split(parts[0], "[")[0])
		if name == "" {
			continue
		}
		if v := extractVersion(parts[1]); v != "" {
			deps = append(deps, packageDep{Name: name, Version: v, Ecosystem: "PyPI"})
		}
	}
	return deps
}

func parsePipfileLock(content string) []packageDep {
	var lock struct {
		Default map[string]struct{ Version string `json:"version"` } `json:"default"`
		Develop map[string]struct{ Version string `json:"version"` } `json:"develop"`
	}
	if err := json.Unmarshal([]byte(content), &lock); err != nil {
		return nil
	}
	var deps []packageDep
	for name, pkg := range lock.Default {
		if v := extractVersion(pkg.Version); v != "" {
			deps = append(deps, packageDep{Name: name, Version: v, Ecosystem: "PyPI"})
		}
	}
	for name, pkg := range lock.Develop {
		if v := extractVersion(pkg.Version); v != "" {
			deps = append(deps, packageDep{Name: name, Version: v, Ecosystem: "PyPI"})
		}
	}
	return deps
}

func parsePyprojectToml(content string) []packageDep {
	lineRe := regexp.MustCompile(`^(\S+)\s*=\s*["'^~>=<*]*(\d[^"'\s,]*)`)
	var deps []packageDep
	inDeps := false
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "[tool.poetry.dependencies]") ||
			strings.HasPrefix(trimmed, "[project]") {
			inDeps = true
			continue
		}
		if inDeps && strings.HasPrefix(trimmed, "[") {
			inDeps = false
			continue
		}
		if !inDeps {
			continue
		}
		m := lineRe.FindStringSubmatch(trimmed)
		if len(m) != 3 {
			continue
		}
		name := m[1]
		if name == "python" || name == "name" || name == "version" || name == "description" {
			continue
		}
		if v := extractVersion(m[2]); v != "" {
			deps = append(deps, packageDep{Name: name, Version: v, Ecosystem: "PyPI"})
		}
	}
	return deps
}

func parseComposerJSON(content string) []packageDep {
	var pkg struct {
		Require    map[string]string `json:"require"`
		RequireDev map[string]string `json:"require-dev"`
	}
	if err := json.Unmarshal([]byte(content), &pkg); err != nil {
		return nil
	}
	all := make(map[string]string)
	for k, v := range pkg.Require {
		all[k] = v
	}
	for k, v := range pkg.RequireDev {
		all[k] = v
	}
	var deps []packageDep
	for name, ver := range all {
		if name == "php" || strings.HasPrefix(name, "ext-") {
			continue
		}
		if v := extractVersion(ver); v != "" {
			deps = append(deps, packageDep{Name: name, Version: v, Ecosystem: "Packagist"})
		}
	}
	return deps
}

func parseGemfileLock(content string) []packageDep {
	// Spec lines look like: "    gem_name (1.2.3)"
	specRe := regexp.MustCompile(`^ {4}(\S+) \((\d+\.\d+(?:\.\d+)?)\)$`)
	var deps []packageDep
	inSpecs := false
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "specs:" {
			inSpecs = true
			continue
		}
		// Empty line or a new section header ends specs
		if inSpecs && (trimmed == "" || (len(trimmed) > 0 && trimmed[0] != ' ' && !strings.HasPrefix(line, " "))) {
			if trimmed != "" && trimmed != "specs:" {
				inSpecs = false
			}
			continue
		}
		if !inSpecs {
			continue
		}
		m := specRe.FindStringSubmatch(line)
		if len(m) == 3 {
			deps = append(deps, packageDep{Name: m[1], Version: m[2], Ecosystem: "RubyGems"})
		}
	}
	return deps
}

func parseGoMod(content string) []packageDep {
	blockLineRe := regexp.MustCompile(`^\s+(\S+)\s+(v\d+\.\d+\.\d+(?:-[^\s]+)?)`)
	singleRe := regexp.MustCompile(`^require\s+(\S+)\s+(v\d+\.\d+\.\d+(?:-[^\s]+)?)`)
	var deps []packageDep
	inRequire := false
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "require (" {
			inRequire = true
			continue
		}
		if inRequire && trimmed == ")" {
			inRequire = false
			continue
		}
		if m := singleRe.FindStringSubmatch(trimmed); len(m) == 3 {
			deps = append(deps, packageDep{Name: m[1], Version: m[2], Ecosystem: "Go"})
			continue
		}
		if inRequire {
			m := blockLineRe.FindStringSubmatch(line)
			if len(m) == 3 {
				// skip indirect stdlib replacements
				if !strings.Contains(line, "// indirect") || strings.HasPrefix(m[1], "github.com") || strings.HasPrefix(m[1], "golang.org") {
					deps = append(deps, packageDep{Name: m[1], Version: m[2], Ecosystem: "Go"})
				}
			}
		}
	}
	return deps
}

// ── Manifest dispatcher ───────────────────────────────────────────────────────

func parseManifestDeps(content, pathLower string) []packageDep {
	switch {
	case strings.HasSuffix(pathLower, "package-lock.json"):
		return parsePackageLockJSON(content)
	case strings.HasSuffix(pathLower, "yarn.lock"):
		return parseYarnLock(content)
	case strings.HasSuffix(pathLower, "package.json"):
		return parsePackageJSON(content)
	case strings.HasSuffix(pathLower, "requirements.txt"):
		return parseRequirementsTxt(content)
	case strings.HasSuffix(pathLower, "pipfile.lock"):
		return parsePipfileLock(content)
	case strings.HasSuffix(pathLower, "pyproject.toml"):
		return parsePyprojectToml(content)
	case strings.HasSuffix(pathLower, "composer.lock"), strings.HasSuffix(pathLower, "composer.json"):
		return parseComposerJSON(content)
	case strings.HasSuffix(pathLower, "gemfile.lock"):
		return parseGemfileLock(content)
	case strings.HasSuffix(pathLower, "go.mod"):
		return parseGoMod(content)
	}
	return nil
}

// ── OSV batch query ───────────────────────────────────────────────────────────

var osvHTTPClient = &http.Client{Timeout: 20 * time.Second}

func queryOSVBatch(deps []packageDep) ([]osvQueryResult, error) {
	queries := make([]osvQuery, 0, len(deps))
	for _, d := range deps {
		queries = append(queries, osvQuery{
			Package: osvPkg{Name: d.Name, Ecosystem: d.Ecosystem},
			Version: d.Version,
		})
	}
	body, err := json.Marshal(osvBatchRequest{Queries: queries})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest("POST", "https://api.osv.dev/v1/querybatch", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := osvHTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	rb, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var batchResp osvBatchResponse
	if err := json.Unmarshal(rb, &batchResp); err != nil {
		return nil, err
	}
	return batchResp.Results, nil
}

// ── Severity classification ───────────────────────────────────────────────────

// cvssApproxSeverity classifies a CVSS vector string by counting high-impact
// metrics. It is an approximation — good enough for alert gating.
func cvssApproxSeverity(vector string) string {
	if vector == "" {
		return "UNKNOWN"
	}
	highCount := strings.Count(vector, ":H") + strings.Count(vector, ":C")
	critMetrics := strings.Contains(vector, "PR:N") && strings.Contains(vector, "AV:N")
	switch {
	case highCount >= 4 && critMetrics:
		return "CRITICAL"
	case highCount >= 3:
		return "HIGH"
	case highCount >= 1:
		return "MEDIUM"
	default:
		return "LOW"
	}
}

func vulnSeverity(vuln osvVuln) string {
	best := "UNKNOWN"
	for _, s := range vuln.Severity {
		if strings.HasPrefix(s.Type, "CVSS") {
			sev := cvssApproxSeverity(s.Score)
			// upgrade severity if we find something worse
			order := map[string]int{"UNKNOWN": 0, "LOW": 1, "MEDIUM": 2, "HIGH": 3, "CRITICAL": 4}
			if order[sev] > order[best] {
				best = sev
			}
		}
	}
	return best
}

// canonicalCVE returns the CVE alias if present, otherwise the OSV ID.
func canonicalCVE(vuln osvVuln) string {
	for _, a := range vuln.Aliases {
		if strings.HasPrefix(a, "CVE-") {
			return a
		}
	}
	return vuln.ID
}

// ── Main entry point ──────────────────────────────────────────────────────────

// checkPackageManifestCVEs parses a fetched package manifest, queries OSV for
// known vulnerabilities, and reports findings. Called from the path-scan loop
// when a manifest path is fetched and LibScan is enabled.
func (a *AWSScanner) checkPackageManifestCVEs(content, sourceURL, pathLower string) {
	deps := parseManifestDeps(content, pathLower)
	if len(deps) == 0 {
		return
	}

	// Deduplicate by ecosystem:name@version
	seen := make(map[string]bool)
	var unique []packageDep
	for _, d := range deps {
		k := d.Ecosystem + ":" + d.Name + "@" + d.Version
		if !seen[k] {
			seen[k] = true
			unique = append(unique, d)
		}
	}

	// Batch in chunks of 200 (well under the 1000-query API limit)
	const chunkSize = 200
	var findings []cveFinding
	for i := 0; i < len(unique); i += chunkSize {
		end := i + chunkSize
		if end > len(unique) {
			end = len(unique)
		}
		chunk := unique[i:end]
		results, err := queryOSVBatch(chunk)
		if err != nil {
			continue
		}
		for j, res := range results {
			if j >= len(chunk) || len(res.Vulns) == 0 {
				continue
			}
			findings = append(findings, cveFinding{Dep: chunk[j], Vulns: res.Vulns})
		}
	}

	if len(findings) == 0 {
		return
	}

	critCount := 0
	highCount := 0
	var telegramLines []string

	for _, f := range findings {
		for _, vuln := range f.Vulns {
			sev := vulnSeverity(vuln)
			cveID := canonicalCVE(vuln)
			summary := vuln.Summary
			if len(summary) > 120 {
				summary = summary[:117] + "..."
			}

			line := fmt.Sprintf("%s | pkg=%s@%s | ecosystem=%s | vuln=%s | severity=%s | %s",
				sanitizeSource(sourceURL),
				f.Dep.Name, f.Dep.Version,
				f.Dep.Ecosystem,
				cveID,
				sev,
				summary,
			)
			a.saveIntoFile(line, "cve_found.txt")
			a.logValid("CVE", fmt.Sprintf("%s@%s → %s (%s)", f.Dep.Name, f.Dep.Version, cveID, sev))

			if sev == "CRITICAL" {
				critCount++
				telegramLines = append(telegramLines, fmt.Sprintf("🔴 *%s* — `%s@%s` — %s", cveID, f.Dep.Name, f.Dep.Version, summary))
			} else if sev == "HIGH" {
				highCount++
				telegramLines = append(telegramLines, fmt.Sprintf("🟠 *%s* — `%s@%s` — %s", cveID, f.Dep.Name, f.Dep.Version, summary))
			}
		}
	}

	if len(telegramLines) > 0 {
		// Cap to 10 lines to avoid Telegram message length limit
		if len(telegramLines) > 10 {
			overflow := len(telegramLines) - 10
			telegramLines = telegramLines[:10]
			telegramLines = append(telegramLines, fmt.Sprintf("…and %d more", overflow))
		}
		msg := fmt.Sprintf(
			"⚠️ *CVE Alert* — %d critical, %d high\nSource: `%s`\n\n%s",
			critCount, highCount,
			sanitizeSource(sourceURL),
			strings.Join(telegramLines, "\n"),
		)
		go a.sendTelegram(msg)
	}
}
