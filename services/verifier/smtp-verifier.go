package main

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/smtp"
	"regexp"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

// ============================================================================
// TYPES AND CONSTANTS
// ============================================================================

type ValidationStatus string

const (
	StatusValid    ValidationStatus = "valid"
	StatusInvalid  ValidationStatus = "invalid"
	StatusCatchAll ValidationStatus = "catch-all"
	StatusUnknown  ValidationStatus = "unknown"
	StatusRisky    ValidationStatus = "risky"
)

type ValidationResult struct {
	Email            string           `json:"email"`
	EmailHash        string           `json:"email_hash"`
	Domain           string           `json:"domain"`
	Status           ValidationStatus `json:"status"`
	Reason           string           `json:"reason"`
	Confidence       float64          `json:"confidence"`
	SMTPCode         int              `json:"smtp_code,omitempty"`
	SMTPResponse     string           `json:"smtp_response,omitempty"`
	MXHost           string           `json:"mx_host,omitempty"`
	MXRecords        []MXRecord       `json:"mx_records,omitempty"`
	IsCatchAll       bool             `json:"is_catch_all"`
	IsDisposable     bool             `json:"is_disposable"`
	ValidationTimeMs int64            `json:"validation_duration_ms"`
	CheckedAt        time.Time        `json:"checked_at"`
}

type MXRecord struct {
	Exchange string `json:"exchange"`
	Priority uint16 `json:"priority"`
}

type DomainMetadata struct {
	IsCatchAll       *bool      `json:"is_catch_all,omitempty"`
	CatchAllChecked  *time.Time `json:"catch_all_checked_at,omitempty"`
	IsDisposable     bool       `json:"is_disposable"`
	MXRecords        []MXRecord `json:"mx_records,omitempty"`
	LastValidation   time.Time  `json:"last_validation,omitempty"`
}

// Configuration
type Config struct {
	// SMTP Timeouts
	SMTPConnectTimeout time.Duration
	SMTPReadTimeout    time.Duration
	SMTPWriteTimeout   time.Duration

	// SMTP Identity
	EHLOHostname string
	MailFrom     string

	// Rate Limiting
	MaxConcurrentPerDomain int
	MaxConcurrentPerMX     int
	DomainRateLimit        time.Duration // Min delay between requests to same domain

	// Retry Policy
	MaxRetries         int
	RetryBackoff       time.Duration
	RetryBackoffFactor float64

	// Catch-all Detection
	EnableCatchAllDetection bool
	CatchAllProbeCount      int

	// Cache TTLs
	MXCacheTTL         time.Duration
	ResultCacheTTL     time.Duration
	DomainMetaCacheTTL time.Duration
}

// Default configuration
func DefaultConfig() *Config {
	return &Config{
		SMTPConnectTimeout:      10 * time.Second,
		SMTPReadTimeout:         15 * time.Second,
		SMTPWriteTimeout:        15 * time.Second,
		EHLOHostname:            "mail-validator.yourdomain.com",
		MailFrom:                "verify@mail-validator.yourdomain.com",
		MaxConcurrentPerDomain:  5,
		MaxConcurrentPerMX:      50,
		DomainRateLimit:         1 * time.Second,
		MaxRetries:              3,
		RetryBackoff:            2 * time.Second,
		RetryBackoffFactor:      2.0,
		EnableCatchAllDetection: true,
		CatchAllProbeCount:      2,
		MXCacheTTL:              1 * time.Hour,
		ResultCacheTTL:          7 * 24 * time.Hour,
		DomainMetaCacheTTL:      24 * time.Hour,
	}
}

// ============================================================================
// SMTP VERIFIER
// ============================================================================

type SMTPVerifier struct {
	config *Config
	redis  *redis.Client
}

func NewSMTPVerifier(config *Config, redisClient *redis.Client) *SMTPVerifier {
	if config == nil {
		config = DefaultConfig()
	}
	return &SMTPVerifier{
		config: config,
		redis:  redisClient,
	}
}

// ============================================================================
// PUBLIC API
// ============================================================================

// Verify validates a single email address
func (v *SMTPVerifier) Verify(ctx context.Context, email string) (*ValidationResult, error) {
	startTime := time.Now()

	// Normalize email
	email = strings.ToLower(strings.TrimSpace(email))

	// Generate email hash for caching
	emailHash := hashEmail(email)

	// Check cache first
	if cached, err := v.getCachedResult(ctx, emailHash); err == nil && cached != nil {
		return cached, nil
	}

	// Step 1: Syntax validation
	if !isValidEmailSyntax(email) {
		return v.createResult(email, emailHash, "", StatusInvalid, "syntax_error", 1.0, 0, "", "", nil, startTime), nil
	}

	// Extract domain
	parts := strings.Split(email, "@")
	if len(parts) != 2 {
		return v.createResult(email, emailHash, "", StatusInvalid, "invalid_format", 1.0, 0, "", "", nil, startTime), nil
	}
	domain := parts[1]

	// Step 2: DNS MX lookup
	mxRecords, err := v.getMXRecords(ctx, domain)
	if err != nil || len(mxRecords) == 0 {
		return v.createResult(email, emailHash, domain, StatusInvalid, "no_mx_records", 0.95, 0, "", "", nil, startTime), nil
	}

	// Step 3: Check domain metadata (disposable, catch-all cache)
	domainMeta, _ := v.getDomainMetadata(ctx, domain)
	if domainMeta != nil && domainMeta.IsDisposable {
		return v.createResult(email, emailHash, domain, StatusRisky, "disposable_domain", 0.9, 0, "", "", mxRecords, startTime), nil
	}

	// Step 4: SMTP verification
	result, err := v.performSMTPVerification(ctx, email, domain, mxRecords)
	if err != nil {
		return v.createResult(email, emailHash, domain, StatusUnknown, fmt.Sprintf("smtp_error: %v", err), 0.2, 0, "", "", mxRecords, startTime), nil
	}

	// Step 5: Cache result
	v.cacheResult(ctx, emailHash, result)

	return result, nil
}

// ============================================================================
// SMTP VERIFICATION LOGIC
// ============================================================================

func (v *SMTPVerifier) performSMTPVerification(ctx context.Context, email, domain string, mxRecords []MXRecord) (*ValidationResult, error) {
	startTime := time.Now()
	emailHash := hashEmail(email)

	// Try each MX record in priority order
	var lastErr error
	for _, mx := range mxRecords {
		result, err := v.verifySMTPWithMX(ctx, email, domain, mx, startTime)
		if err == nil {
			// Successful verification
			if result.Status == StatusValid || result.Status == StatusInvalid {
				return result, nil
			}
		}
		lastErr = err
	}

	// All MX records failed
	return v.createResult(email, emailHash, domain, StatusUnknown, "all_mx_failed", 0.2, 0, "", "", mxRecords, startTime), lastErr
}

func (v *SMTPVerifier) verifySMTPWithMX(ctx context.Context, email, domain string, mx MXRecord, startTime time.Time) (*ValidationResult, error) {
	emailHash := hashEmail(email)

	// Acquire rate limit
	if err := v.waitForRateLimit(ctx, domain, mx.Exchange); err != nil {
		return nil, err
	}

	// Perform SMTP handshake with retries
	var smtpCode int
	var smtpResponse string
	var err error

	for attempt := 0; attempt < v.config.MaxRetries; attempt++ {
		smtpCode, smtpResponse, err = v.smtpHandshake(ctx, email, mx.Exchange)
		if err == nil {
			break
		}

		// Check if error is retryable
		if !isRetryableError(err) {
			break
		}

		// Exponential backoff
		if attempt < v.config.MaxRetries-1 {
			backoff := time.Duration(float64(v.config.RetryBackoff) * float64(attempt+1) * v.config.RetryBackoffFactor)
			select {
			case <-time.After(backoff):
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
	}

	if err != nil {
		return nil, err
	}

	// Classify response
	status, reason, confidence := classifySMTPResponse(smtpCode, smtpResponse)

	// Check for catch-all if enabled and status is valid
	isCatchAll := false
	if status == StatusValid && v.config.EnableCatchAllDetection {
		isCatchAll, _ = v.detectCatchAll(ctx, domain, mx)
		if isCatchAll {
			status = StatusCatchAll
			reason = "catch_all_domain"
			confidence = 0.5
		}
	}

	result := v.createResult(email, emailHash, domain, status, reason, confidence, smtpCode, smtpResponse, mx.Exchange, []MXRecord{mx}, startTime)
	result.IsCatchAll = isCatchAll

	return result, nil
}

// smtpHandshake performs the SMTP handshake: EHLO -> MAIL FROM -> RCPT TO -> QUIT
func (v *SMTPVerifier) smtpHandshake(ctx context.Context, email, mxHost string) (int, string, error) {
	// Connect with timeout
	d := net.Dialer{
		Timeout: v.config.SMTPConnectTimeout,
	}

	conn, err := d.DialContext(ctx, "tcp", net.JoinHostPort(mxHost, "25"))
	if err != nil {
		return 0, "", fmt.Errorf("connection failed: %w", err)
	}
	defer conn.Close()

	// Set deadlines
	conn.SetDeadline(time.Now().Add(v.config.SMTPReadTimeout))

	// Create SMTP client
	client, err := smtp.NewClient(conn, mxHost)
	if err != nil {
		return 0, "", fmt.Errorf("smtp client creation failed: %w", err)
	}
	defer client.Close()

	// EHLO/HELO
	if err := client.Hello(v.config.EHLOHostname); err != nil {
		return 0, "", fmt.Errorf("EHLO failed: %w", err)
	}

	// Try STARTTLS if available (optional)
	if ok, _ := client.Extension("STARTTLS"); ok {
		tlsConfig := &tls.Config{
			ServerName:         mxHost,
			InsecureSkipVerify: true, // For verification purposes only
		}
		if err := client.StartTLS(tlsConfig); err == nil {
			// TLS upgraded successfully (ignore error if not supported)
		}
	}

	// MAIL FROM
	if err := client.Mail(v.config.MailFrom); err != nil {
		return 0, "", fmt.Errorf("MAIL FROM failed: %w", err)
	}

	// RCPT TO (this is the critical step)
	err = client.Rcpt(email)

	// Extract SMTP code and response
	smtpCode := 0
	smtpResponse := ""

	if err != nil {
		// Parse error to extract SMTP code
		smtpCode, smtpResponse = parseSMTPError(err)
	} else {
		// Success (250)
		smtpCode = 250
		smtpResponse = "Recipient OK"
	}

	// QUIT
	client.Quit()

	return smtpCode, smtpResponse, nil
}

// ============================================================================
// CATCH-ALL DETECTION
// ============================================================================

func (v *SMTPVerifier) detectCatchAll(ctx context.Context, domain string, mx MXRecord) (bool, error) {
	// Check cache first
	if cached, err := v.getCachedCatchAllStatus(ctx, domain); err == nil && cached != nil {
		return *cached, nil
	}

	// Generate random email addresses
	probeEmails := make([]string, v.config.CatchAllProbeCount)
	for i := 0; i < v.config.CatchAllProbeCount; i++ {
		randomLocal := fmt.Sprintf("probeverify%d%d", time.Now().UnixNano(), i)
		probeEmails[i] = randomLocal + "@" + domain
	}

	// Test random addresses
	acceptCount := 0
	for _, probeEmail := range probeEmails {
		smtpCode, _, err := v.smtpHandshake(ctx, probeEmail, mx.Exchange)
		if err == nil && (smtpCode == 250 || smtpCode == 251) {
			acceptCount++
		}

		// Small delay between probes
		time.Sleep(500 * time.Millisecond)
	}

	// If all or most probes are accepted, it's likely a catch-all
	isCatchAll := acceptCount >= (v.config.CatchAllProbeCount / 2)

	// Cache result
	v.cacheCatchAllStatus(ctx, domain, isCatchAll)

	return isCatchAll, nil
}

// ============================================================================
// DNS MX LOOKUP
// ============================================================================

func (v *SMTPVerifier) getMXRecords(ctx context.Context, domain string) ([]MXRecord, error) {
	// Check cache
	if cached, err := v.getCachedMXRecords(ctx, domain); err == nil && len(cached) > 0 {
		return cached, nil
	}

	// Query DNS
	mxs, err := net.LookupMX(domain)
	if err != nil {
		return nil, err
	}

	records := make([]MXRecord, len(mxs))
	for i, mx := range mxs {
		records[i] = MXRecord{
			Exchange: strings.TrimSuffix(mx.Host, "."),
			Priority: mx.Pref,
		}
	}

	// Sort by priority
	sortMXRecords(records)

	// Cache results
	v.cacheMXRecords(ctx, domain, records)

	return records, nil
}

func sortMXRecords(records []MXRecord) {
	// Simple bubble sort by priority
	for i := 0; i < len(records)-1; i++ {
		for j := i + 1; j < len(records); j++ {
			if records[i].Priority > records[j].Priority {
				records[i], records[j] = records[j], records[i]
			}
		}
	}
}

// ============================================================================
// CACHING LAYER
// ============================================================================

func (v *SMTPVerifier) getCachedResult(ctx context.Context, emailHash string) (*ValidationResult, error) {
	key := "validation:result:" + emailHash
	val, err := v.redis.Get(ctx, key).Result()
	if err != nil {
		return nil, err
	}

	var result ValidationResult
	if err := json.Unmarshal([]byte(val), &result); err != nil {
		return nil, err
	}

	return &result, nil
}

func (v *SMTPVerifier) cacheResult(ctx context.Context, emailHash string, result *ValidationResult) error {
	key := "validation:result:" + emailHash
	data, err := json.Marshal(result)
	if err != nil {
		return err
	}

	return v.redis.Set(ctx, key, data, v.config.ResultCacheTTL).Err()
}

func (v *SMTPVerifier) getCachedMXRecords(ctx context.Context, domain string) ([]MXRecord, error) {
	key := "mx:records:" + domain
	val, err := v.redis.Get(ctx, key).Result()
	if err != nil {
		return nil, err
	}

	var records []MXRecord
	if err := json.Unmarshal([]byte(val), &records); err != nil {
		return nil, err
	}

	return records, nil
}

func (v *SMTPVerifier) cacheMXRecords(ctx context.Context, domain string, records []MXRecord) error {
	key := "mx:records:" + domain
	data, err := json.Marshal(records)
	if err != nil {
		return err
	}

	return v.redis.Set(ctx, key, data, v.config.MXCacheTTL).Err()
}

func (v *SMTPVerifier) getDomainMetadata(ctx context.Context, domain string) (*DomainMetadata, error) {
	key := "domain:meta:" + domain
	val, err := v.redis.Get(ctx, key).Result()
	if err != nil {
		return nil, err
	}

	var meta DomainMetadata
	if err := json.Unmarshal([]byte(val), &meta); err != nil {
		return nil, err
	}

	return &meta, nil
}

func (v *SMTPVerifier) getCachedCatchAllStatus(ctx context.Context, domain string) (*bool, error) {
	key := "domain:catchall:" + domain
	val, err := v.redis.Get(ctx, key).Result()
	if err != nil {
		return nil, err
	}

	isCatchAll := val == "1"
	return &isCatchAll, nil
}

func (v *SMTPVerifier) cacheCatchAllStatus(ctx context.Context, domain string, isCatchAll bool) error {
	key := "domain:catchall:" + domain
	val := "0"
	if isCatchAll {
		val = "1"
	}

	return v.redis.Set(ctx, key, val, v.config.ResultCacheTTL).Err()
}

// ============================================================================
// RATE LIMITING
// ============================================================================

func (v *SMTPVerifier) waitForRateLimit(ctx context.Context, domain, mxHost string) error {
	// Domain-level rate limit
	domainKey := "ratelimit:domain:" + domain + ":last"
	lastCheck, err := v.redis.Get(ctx, domainKey).Result()
	if err == nil && lastCheck != "" {
		lastTime, _ := time.Parse(time.RFC3339, lastCheck)
		elapsed := time.Since(lastTime)
		if elapsed < v.config.DomainRateLimit {
			waitTime := v.config.DomainRateLimit - elapsed
			select {
			case <-time.After(waitTime):
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}

	// Update last check time
	v.redis.Set(ctx, domainKey, time.Now().Format(time.RFC3339), v.config.DomainRateLimit*2)

	return nil
}

// ============================================================================
// HELPER FUNCTIONS
// ============================================================================

func (v *SMTPVerifier) createResult(email, emailHash, domain string, status ValidationStatus, reason string, confidence float64, smtpCode int, smtpResponse, mxHost string, mxRecords []MXRecord, startTime time.Time) *ValidationResult {
	return &ValidationResult{
		Email:            email,
		EmailHash:        emailHash,
		Domain:           domain,
		Status:           status,
		Reason:           reason,
		Confidence:       confidence,
		SMTPCode:         smtpCode,
		SMTPResponse:     smtpResponse,
		MXHost:           mxHost,
		MXRecords:        mxRecords,
		ValidationTimeMs: time.Since(startTime).Milliseconds(),
		CheckedAt:        time.Now(),
	}
}

func hashEmail(email string) string {
	h := sha256.New()
	h.Write([]byte(strings.ToLower(email)))
	return hex.EncodeToString(h.Sum(nil))
}

var emailRegex = regexp.MustCompile(`^[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}$`)

func isValidEmailSyntax(email string) bool {
	return emailRegex.MatchString(email) && len(email) <= 320
}

func parseSMTPError(err error) (int, string) {
	errStr := err.Error()

	// Try to extract code from error string
	// Format is usually: "550 User not found"
	if len(errStr) >= 3 {
		code := 0
		fmt.Sscanf(errStr, "%d", &code)
		if code >= 100 && code < 600 {
			return code, errStr
		}
	}

	return 0, errStr
}

func classifySMTPResponse(code int, response string) (ValidationStatus, string, float64) {
	switch {
	case code == 250 || code == 251:
		return StatusValid, "mailbox_exists", 0.98

	case code == 550 || code == 551 || code == 553:
		return StatusInvalid, "mailbox_not_found", 0.95

	case code == 450 || code == 451 || code == 452:
		return StatusUnknown, "temporary_failure", 0.3

	case code == 421:
		return StatusUnknown, "rate_limited", 0.2

	case code >= 500:
		return StatusInvalid, fmt.Sprintf("smtp_error_%d", code), 0.7

	default:
		return StatusUnknown, "unknown_response", 0.1
	}
}

func isRetryableError(err error) bool {
	if err == nil {
		return false
	}

	errStr := err.Error()

	// Network errors are generally retryable
	if errors.Is(err, context.DeadlineExceeded) ||
		errors.Is(err, context.Canceled) ||
		strings.Contains(errStr, "timeout") ||
		strings.Contains(errStr, "connection refused") ||
		strings.Contains(errStr, "temporary failure") {
		return true
	}

	// 4xx SMTP codes are temporary
	code, _ := parseSMTPError(err)
	if code >= 400 && code < 500 {
		return true
	}

	return false
}
