package main

import (
	"fmt"
	"os"
	"sync/atomic"
	"time"
)

// Progress tracks verification throughput and status distribution.
// All counters are updated atomically so they are safe for concurrent use.
type Progress struct {
	total     int64         // total emails to verify
	done      atomic.Int64  // emails processed so far
	delivered atomic.Int64
	invalid   atomic.Int64
	catchAll  atomic.Int64
	risky     atomic.Int64
	errors    atomic.Int64

	start   time.Time
	stopped atomic.Bool
}

// NewProgress creates a progress tracker for total emails.
func NewProgress(total int) *Progress {
	return &Progress{
		total: int64(total),
		start: time.Now(),
	}
}

// Record updates the counters for a single result.
func (p *Progress) Record(r VerifyResult) {
	p.done.Add(1)
	switch r.Status {
	case "delivered":
		p.delivered.Add(1)
	case "invalid":
		p.invalid.Add(1)
	case "catch_all":
		p.catchAll.Add(1)
	case "risky":
		p.risky.Add(1)
	case "error":
		p.errors.Add(1)
	}
}

// Stop marks the tracker as finished so the render loop exits.
func (p *Progress) Stop() { p.stopped.Store(true) }

// RunRender starts a goroutine that writes progress to stderr every second.
// Call Stop() to halt it.
func (p *Progress) RunRender() {
	go func() {
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()

		for range ticker.C {
			if p.stopped.Load() {
				return
			}
			p.render(false)
		}
	}()
}

// Finalize prints the final line (newline-terminated) and summary.
func (p *Progress) Finalize() {
	p.render(true)
	fmt.Fprintf(os.Stderr, "\nDone. %d emails verified in %v.\n",
		p.done.Load(), time.Since(p.start).Round(time.Second))
}

// render writes a single progress line (or final summary) to stderr.
func (p *Progress) render(final bool) {
	done := p.done.Load()
	del := p.delivered.Load()
	inv := p.invalid.Load()
	ca := p.catchAll.Load()
	ri := p.risky.Load()
	er := p.errors.Load()
	total := p.total

	pct := func(n int64) float64 {
		if done == 0 {
			return 0
		}
		return float64(n) / float64(done) * 100
	}

	line := fmt.Sprintf(
		"Verified %d/%d | %.0f%% delivered | %.0f%% invalid | %.0f%% catch-all | %.0f%% risky | %d errors",
		done, total,
		pct(del), pct(inv), pct(ca), pct(ri), er,
	)

	// ETA
	if done > 0 && done < total {
		elapsed := time.Since(p.start)
		rate := float64(elapsed) / float64(done)
		eta := time.Duration(float64(total-done) * rate).Round(time.Second)
		line += fmt.Sprintf(" | ETA %v", eta)
	}

	if final {
		fmt.Fprintf(os.Stderr, "%s\n", line)
	} else {
		// Carriage-return for in-place update (no newline).
		fmt.Fprintf(os.Stderr, "\r%-100s", line)
	}
}
