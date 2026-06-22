package main

import (
	"fmt"
	"sync"
	"time"

	emailverifier "github.com/AfterShip/email-verifier"
)

// VerifierConfig holds the configuration for email verification.
type VerifierConfig struct {
	EnableSMTP     bool
	SMTPProxy      string
	TimeoutSeconds int
	SkipDisposable bool
}

// VerifyResult is the structured output for a single email verification.
type VerifyResult struct {
	Status     string // delivered, invalid, catch_all, risky, error
	MXHost     string // primary MX hostname (empty if none)
	CatchAll   string // "true", "false", or "" when not checked
	ResponseMs int64  // milliseconds spent on verification
	Error      string // error message when status is "error"
}

// Verifier wraps the AfterShip email-verifier library with caching and
// domain-level serialization to avoid flooding SMTP servers.
type Verifier struct {
	engine   *emailverifier.Verifier
	skipDisp bool

	// MX record cache keyed by domain.
	mxCache   map[string]mxEntry
	mxCacheMu sync.RWMutex

}

type mxEntry struct {
	hasMX    bool
	hostname string
}

// NewVerifier creates a configured verifier.
func NewVerifier(cfg VerifierConfig) *Verifier {
	engine := emailverifier.NewVerifier()

	if cfg.EnableSMTP {
		engine.EnableSMTPCheck().EnableCatchAllCheck()
		if cfg.SMTPProxy != "" {
			engine.Proxy(cfg.SMTPProxy)
		}
	}

	if cfg.TimeoutSeconds > 0 {
		d := time.Duration(cfg.TimeoutSeconds) * time.Second
		engine.ConnectTimeout(d).OperationTimeout(d)
	}

	if !cfg.SkipDisposable {
		engine.EnableAutoUpdateDisposable()
	}

	return &Verifier{
		engine:   engine,
		skipDisp: cfg.SkipDisposable,
		mxCache:  make(map[string]mxEntry),
	}
}

// getMXHost returns the primary MX hostname for a domain, using an
// in-memory cache to avoid repeated DNS lookups.
func (v *Verifier) getMXHost(domain string) string {
	// Fast-path: check cache under read lock.
	v.mxCacheMu.RLock()
	entry, ok := v.mxCache[domain]
	v.mxCacheMu.RUnlock()
	if ok {
		return entry.hostname
	}

	v.mxCacheMu.Lock()
	defer v.mxCacheMu.Unlock()

	// Double-check: another goroutine may have populated the cache while
	// we were waiting for the write lock.
	if entry, ok = v.mxCache[domain]; ok {
		return entry.hostname
	}

	mx, err := v.engine.CheckMX(domain)
	if err != nil || !mx.HasMXRecord || len(mx.Records) == 0 {
		v.mxCache[domain] = mxEntry{hasMX: false}
		return ""
	}

	hostname := mx.Records[0].Host
	v.mxCache[domain] = mxEntry{hasMX: true, hostname: hostname}
	return hostname
}

// VerifyEmail runs the full verification pipeline for a single address.
func (v *Verifier) VerifyEmail(email string) VerifyResult {
	start := time.Now()
	domain := extractDomain(email)

	result, err := v.engine.Verify(email)
	elapsed := time.Since(start).Milliseconds()

	res := VerifyResult{
		ResponseMs: elapsed,
	}

	if err != nil {
		// Map well-known library errors to appropriate statuses.
		if le := emailverifier.ParseSMTPError(err); le != nil {
			switch le.Message {
			case emailverifier.ErrNoSuchHost, emailverifier.ErrServerUnavailable:
				res.Status = "invalid"
			default:
				res.Status = "error"
			}
		} else {
			// Generic error from Verify() — could be a DNS timeout, network
			// issue, or other infrastructure problem.
			res.Status = "error"
		}
		res.Error = err.Error()
		return res
	}

	// --- Determine status ---------------------------------------------------
	switch {
	case !result.Syntax.Valid:
		res.Status = "invalid"

	case !v.skipDisp && result.Disposable:
		res.Status = "invalid"

	case !result.HasMxRecords:
		res.Status = "invalid"

	case result.SMTP != nil:
		// SMTP was performed.  Map the library's SMTP flags to our statuses.
		res.CatchAll = fmt.Sprintf("%t", result.SMTP.CatchAll)

		switch {
		case result.SMTP.Deliverable:
			res.Status = "delivered"
		case result.SMTP.CatchAll:
			res.Status = "catch_all"
		case result.SMTP.Disabled:
			res.Status = "invalid"
		default:
			res.Status = "risky"
		}

	default:
		// DNS-only mode — infrastructure looks fine but we cannot confirm
		// deliverability without an SMTP handshake.
		res.Status = "risky"
	}

	// Resolve primary MX hostname (cached).
	res.MXHost = v.getMXHost(domain)

	return res
}

// extractDomain returns the domain portion of an email address.
// Assumes the address has already been syntax-checked.
func extractDomain(email string) string {
	for i := len(email) - 1; i >= 0; i-- {
		if email[i] == '@' {
			return email[i+1:]
		}
	}
	return email
}
