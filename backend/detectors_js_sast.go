package main

// Advanced JS pattern analysis — detects dangerous code patterns in fetched
// JavaScript beyond credential hunting. Not a full AST (that requires Semgrep);
// these are high-signal regex patterns that fire at very low false-positive rates.
//
// Patterns detected:
//   eval() with non-literal argument             — remote code execution risk
//   new Function() constructor                   — dynamic code construction
//   innerHTML / outerHTML assignment             — DOM XSS sink
//   document.write() / document.writeln()        — legacy DOM XSS sink
//   __proto__ mutation                           — prototype pollution
//   Object.defineProperty(Object.prototype       — prototype pollution
//   setTimeout / setInterval with string arg     — string-eval code injection
//   require() with variable argument (Node.js)   — dynamic module loading
//   child_process.exec() / execSync()            — command injection sink
//   process.env access in client bundle          — server secret exposure
//   postMessage without origin check             — cross-origin data leak
//   Hardcoded RSA / EC private key preamble      — cryptographic material in JS
//
// Output: js_sast_findings.txt
// Only runs when JSScan is enabled and sourceURL ends in .js

import (
	"fmt"
	"regexp"
	"strings"
)

// ── Pattern registry ──────────────────────────────────────────────────────────

type jsSASTPattern struct {
	re       *regexp.Regexp
	label    string
	severity string // CRITICAL | HIGH | MEDIUM
}

var jsSASTPatterns = []jsSASTPattern{
	// eval() — RCE sink. Excludes eval(string_literal) which is a no-op.
	{
		re:       regexp.MustCompile(`\beval\s*\(\s*(?:[^"']|"[^"]*"|'[^']*')*?\b(?:req|res|param|input|data|body|query|user|msg|cmd|str|val|src|buf)\b`),
		label:    "eval() with dynamic input",
		severity: "CRITICAL",
	},
	// new Function() constructor with variable args — RCE
	{
		re:       regexp.MustCompile(`new\s+Function\s*\([^)]*(?:req|res|param|input|data|query|user|cmd|str)\b`),
		label:    "new Function() with dynamic input",
		severity: "CRITICAL",
	},
	// setTimeout / setInterval with string first arg (code injection)
	{
		re:       regexp.MustCompile(`(?:setTimeout|setInterval)\s*\(\s*(?:[^"'][^,)]+),`),
		label:    "setTimeout/setInterval with non-literal string (code injection)",
		severity: "HIGH",
	},
	// innerHTML / outerHTML XSS sinks — assignment only
	{
		re:       regexp.MustCompile(`\.(?:inner|outer)HTML\s*[+]?=\s*(?!["'` + "`" + `])`),
		label:    "innerHTML/outerHTML assignment from non-literal",
		severity: "HIGH",
	},
	// document.write / document.writeln
	{
		re:       regexp.MustCompile(`document\.write(?:ln)?\s*\(\s*(?:[^"'][^)]*)\)`),
		label:    "document.write() with non-literal argument",
		severity: "HIGH",
	},
	// window.location / location.href open redirect
	{
		re:       regexp.MustCompile(`(?:window\.location|location\.href)\s*=\s*(?:[^"'][^;]+)`),
		label:    "window.location assignment from variable (open redirect)",
		severity: "MEDIUM",
	},
	// Prototype pollution — __proto__ mutation
	{
		re:       regexp.MustCompile(`\[["']__proto__["']\]\s*=|__proto__\s*:\s*\{`),
		label:    "Prototype pollution — __proto__ mutation",
		severity: "HIGH",
	},
	// Prototype pollution — Object.defineProperty(Object.prototype
	{
		re:       regexp.MustCompile(`Object\.defineProperty\s*\(\s*Object\.prototype`),
		label:    "Prototype pollution — Object.defineProperty(Object.prototype)",
		severity: "HIGH",
	},
	// require() with variable — dynamic module loading
	{
		re:       regexp.MustCompile(`\brequire\s*\(\s*(?:[^"'` + "`" + `][^)]*)\)`),
		label:    "Dynamic require() — variable module path",
		severity: "MEDIUM",
	},
	// child_process exec / execSync — command injection
	{
		re:       regexp.MustCompile(`(?:child_process\.|require\(["']child_process["']\)\.)(?:exec|execSync|spawn|spawnSync|execFile)\s*\(`),
		label:    "child_process exec/spawn — command injection risk",
		severity: "CRITICAL",
	},
	// postMessage without targetOrigin check — data leak
	{
		re:       regexp.MustCompile(`\.postMessage\s*\([^,)]+,\s*["']\*["']`),
		label:    "postMessage with wildcard origin (*)",
		severity: "MEDIUM",
	},
	// process.env in client-side bundle — server secrets potentially compiled in
	{
		re:       regexp.MustCompile(`process\.env\.[A-Z_]{4,}`),
		label:    "process.env access in JS bundle — may expose server secrets",
		severity: "MEDIUM",
	},
	// Hardcoded RSA/EC private key
	{
		re:       regexp.MustCompile(`-----BEGIN (?:RSA |EC )?PRIVATE KEY-----`),
		label:    "Hardcoded private key preamble in JS bundle",
		severity: "CRITICAL",
	},
}

// ── Source map follow ─────────────────────────────────────────────────────────

var sourceMapURLRe = regexp.MustCompile(`//# sourceMappingURL=([^\s]+\.map)`)

// extractSourceMapURL returns the source map URL from a JS file if present.
func extractSourceMapURL(content, jsURL string) string {
	m := sourceMapURLRe.FindStringSubmatch(content)
	if len(m) < 2 {
		return ""
	}
	return resolveURL(jsURL, m[1])
}

// ── Main scanner ──────────────────────────────────────────────────────────────

// checkJSSAST scans a fetched JS file for dangerous patterns. Called from the
// JS scanner section of main.go after checkAndSaveKeys. Gated on JSScan flag.
func (a *AWSScanner) checkJSSAST(content, sourceURL string) {
	if len(content) == 0 {
		return
	}
	// Skip minified files over 256 KB — pattern matching against a 500 KB
	// one-liner produces enormous match sets and high false-positive rates.
	if len(content) > 256*1024 && strings.Count(content, "\n") < 50 {
		return
	}

	var alerts []string
	seen := map[string]bool{}

	for _, pat := range jsSASTPatterns {
		matches := pat.re.FindAllString(content, 5) // cap at 5 per pattern per file
		for _, m := range matches {
			snippet := m
			if len(snippet) > 80 {
				snippet = snippet[:77] + "..."
			}
			key := pat.label + "|" + snippet
			if seen[key] {
				continue
			}
			seen[key] = true
			alerts = append(alerts, fmt.Sprintf("[%s] %s — `%s`", pat.severity, pat.label, snippet))
		}
	}

	if len(alerts) == 0 {
		return
	}

	for _, alert := range alerts {
		line := fmt.Sprintf("%s | %s", sanitizeSource(sourceURL), alert)
		a.saveIntoFile(line, "js_sast_findings.txt")
		a.logValid("JS-SAST", fmt.Sprintf("%s → %s", sanitizeSource(sourceURL), alert))
	}

	// Only Telegram on CRITICAL findings
	var critical []string
	for _, al := range alerts {
		if strings.HasPrefix(al, "[CRITICAL]") {
			critical = append(critical, al)
		}
	}
	if len(critical) > 0 {
		if len(critical) > 5 {
			critical = critical[:5]
		}
		msg := fmt.Sprintf("⚠️ *JS SAST — %d critical pattern(s)*\nSource: `%s`\n\n%s",
			len(critical),
			sanitizeSource(sourceURL),
			strings.Join(critical, "\n"),
		)
		go a.sendTelegram(msg)
	}
}
