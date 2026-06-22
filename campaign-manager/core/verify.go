package core

import (
	"encoding/csv"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	emailverifier "github.com/AfterShip/email-verifier"
)

// VerifierConfig holds the configuration for email verification.
type VerifierConfig struct {
	EnableSMTP     bool
	SMTPProxy      string
	TimeoutSeconds int
}

// VerifyResult is the structured output for a single email verification.
type VerifyResult struct {
	Status     string // delivered, invalid, catch_all, risky, error
	MXHost     string // primary MX hostname (empty if none)
	CatchAll   string // "true", "false", or "" when not checked
	ResponseMs int64  // milliseconds spent on verification
	Error      string // error message when status is "error"
}

// verifier wraps the AfterShip email-verifier library with caching and
// domain-level serialization to avoid flooding SMTP servers.
type verifier struct {
	engine *emailverifier.Verifier

	// MX record cache keyed by domain.
	mxCache   map[string]mxEntry
	mxCacheMu sync.RWMutex
}

type mxEntry struct {
	hasMX    bool
	hostname string
}

// newVerifier creates a configured verifier.
func newVerifier(cfg VerifierConfig) *verifier {
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

	engine.EnableAutoUpdateDisposable()

	return &verifier{
		engine:  engine,
		mxCache: make(map[string]mxEntry),
	}
}

// getMXHost returns the primary MX hostname for a domain, using an
// in-memory cache to avoid repeated DNS lookups.
func (v *verifier) getMXHost(domain string) string {
	v.mxCacheMu.RLock()
	entry, ok := v.mxCache[domain]
	v.mxCacheMu.RUnlock()
	if ok {
		return entry.hostname
	}

	v.mxCacheMu.Lock()
	defer v.mxCacheMu.Unlock()

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

// verifyEmail runs the full verification pipeline for a single address.
func (v *verifier) verifyEmail(email string) VerifyResult {
	start := time.Now()
	domain := extractDomain(email)

	result, err := v.engine.Verify(email)
	elapsed := time.Since(start).Milliseconds()

	res := VerifyResult{
		ResponseMs: elapsed,
	}

	if err != nil {
		if le := emailverifier.ParseSMTPError(err); le != nil {
			switch le.Message {
			case emailverifier.ErrNoSuchHost, emailverifier.ErrServerUnavailable:
				res.Status = "invalid"
			default:
				res.Status = "error"
			}
		} else {
			res.Status = "error"
		}
		res.Error = err.Error()
		return res
	}

	// Determine status.
	switch {
	case !result.Syntax.Valid:
		res.Status = "invalid"

	case result.Disposable:
		res.Status = "invalid"

	case !result.HasMxRecords:
		res.Status = "invalid"

	case result.SMTP != nil:
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
		// DNS-only mode.
		res.Status = "risky"
	}

	res.MXHost = v.getMXHost(domain)
	return res
}

// extractDomain returns the domain portion of an email address.
func extractDomain(email string) string {
	for i := len(email) - 1; i >= 0; i-- {
		if email[i] == '@' {
			return email[i+1:]
		}
	}
	return email
}

// VerifyLeads reads a CSV file, verifies each email address, and returns
// summary counts.  If smtp is true, SMTP verification is enabled (slow).
// smtpProxy is an optional SOCKS5 proxy for SMTP connections.
//
// Returns total, valid (delivered), invalid, catchAll counts.
func VerifyLeads(csvPath string, smtp bool, smtpProxy string) (int, int, int, int, error) {
	// Open CSV.
	f, err := os.Open(csvPath)
	if err != nil {
		return 0, 0, 0, 0, fmt.Errorf("open %s: %w", csvPath, err)
	}
	defer f.Close()

	r := csv.NewReader(f)
	r.LazyQuotes = true
	r.TrimLeadingSpace = true

	header, err := r.Read()
	if err != nil {
		return 0, 0, 0, 0, fmt.Errorf("read header: %w", err)
	}

	rows, err := r.ReadAll()
	if err != nil {
		return 0, 0, 0, 0, fmt.Errorf("read records: %w", err)
	}

	if len(rows) == 0 {
		return 0, 0, 0, 0, nil
	}

	// Detect email column.
	emailIdx := detectEmailColumn(header, rows)

	// Build verifier.
	cfg := VerifierConfig{
		EnableSMTP:     smtp,
		SMTPProxy:      smtpProxy,
		TimeoutSeconds: 10,
	}
	v := newVerifier(cfg)

	// Process all rows concurrently (max 5).
	total := len(rows)
	var valid, invalid, catchAll int
	var mu sync.Mutex
	sem := make(chan struct{}, 5)
	var wg sync.WaitGroup

	for _, row := range rows {
		email := ""
		if emailIdx < len(row) {
			email = row[emailIdx]
		}

		wg.Add(1)
		sem <- struct{}{}

		go func(emailAddr string) {
			defer wg.Done()
			defer func() { <-sem }()

			res := v.verifyEmail(emailAddr)

			mu.Lock()
			defer mu.Unlock()
			switch res.Status {
			case "delivered":
				valid++
			case "invalid":
				invalid++
			case "catch_all":
				catchAll++
			}
		}(email)
	}

	wg.Wait()
	return total, valid, invalid, catchAll, nil
}

// detectEmailColumn finds the column containing email addresses by scanning
// the first few data rows for values that look like emails.
func detectEmailColumn(header []string, rows [][]string) int {
	lookback := len(rows)
	if lookback > 5 {
		lookback = 5
	}
	scores := make([]int, len(header))
	for r := 0; r < lookback; r++ {
		for c, val := range rows[r] {
			if strings.Contains(val, "@") && strings.Contains(val, ".") {
				scores[c]++
			}
		}
	}
	bestIdx, bestScore := 0, 0
	for i, s := range scores {
		if s > bestScore {
			bestScore = s
			bestIdx = i
		}
	}
	return bestIdx
}
