package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"
)

func main() {
	inputPath := flag.String("input", "", "Path to input CSV file (required)")
	outputPath := flag.String("output", "", "Path to output CSV file (default: input with _verified suffix)")
	emailColName := flag.String("email-column", "", "Email column name (auto-detected from data if omitted)")
	enableSMTP := flag.Bool("smtp", false, "Enable SMTP verification (slow; requires outbound port 25)")
	smtpProxy := flag.String("smtp-proxy", "", "SOCKS5 proxy for SMTP (e.g. socks5://host:port)")
	batchSize := flag.Int("batch-size", 0, "Rows per batch (default: 10 for SMTP, 100 for DNS-only)")
	timeoutSec := flag.Int("timeout", 10, "DNS/SMTP timeout in seconds")
	concurrency := flag.Int("concurrency", 5, "Max concurrent verifications")
	skipDisposable := flag.Bool("skip-disposable", false, "Skip disposable-domain check")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: email-verifier --input <file.csv> [flags]\n\nFlags:\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	if *inputPath == "" {
		fmt.Fprintln(os.Stderr, "Error: --input is required")
		flag.Usage()
		os.Exit(1)
	}

	if *outputPath == "" {
		*outputPath = deriveOutputPath(*inputPath)
	}

	if *batchSize == 0 {
		if *enableSMTP {
			*batchSize = 10
		} else {
			*batchSize = 100
		}
	}

	// --- Open CSV reader ---------------------------------------------------
	reader, err := OpenCSVReader(*inputPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error opening input: %v\n", err)
		os.Exit(1)
	}
	defer reader.Close()

	// Read all rows upfront so we know total count for progress.
	allRows, err := reader.ReadAll()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading CSV: %v\n", err)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "Loaded %d rows from %s\n", len(allRows), *inputPath)

	// Detect email column.
	emailIdx := resolveEmailColumn(*emailColName, reader.Header(), allRows)

	// --- Open CSV writer ---------------------------------------------------
	writer, err := OpenCSVWriter(*outputPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error opening output: %v\n", err)
		os.Exit(1)
	}
	defer writer.Close()

	outHeader := OutputHeader(reader.Header())
	if err := writer.WriteRow(outHeader); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing header: %v\n", err)
		os.Exit(1)
	}

	// --- Build verifier ----------------------------------------------------
	cfg := VerifierConfig{
		EnableSMTP:     *enableSMTP,
		SMTPProxy:      *smtpProxy,
		TimeoutSeconds: *timeoutSec,
		SkipDisposable: *skipDisposable,
	}
	verifier := NewVerifier(cfg)

	// --- Process -----------------------------------------------------------
	progress := NewProgress(len(allRows))
	progress.RunRender()

	sem := make(chan struct{}, *concurrency)

	for start := 0; start < len(allRows); start += *batchSize {
		end := start + *batchSize
		if end > len(allRows) {
			end = len(allRows)
		}
		batch := allRows[start:end]

		results := make([]VerifyResult, len(batch))
		var wg sync.WaitGroup

		for i, row := range batch {
			email := ""
			if emailIdx < len(row) {
				email = row[emailIdx]
			}

			wg.Add(1)
			sem <- struct{}{} // acquire slot

			go func(idx int, emailAddr string) {
				defer wg.Done()
				defer func() { <-sem }() // release slot

				results[idx] = verifier.VerifyEmail(emailAddr)
				progress.Record(results[idx])
			}(i, email)
		}

		wg.Wait()

		// Write batch rows in original order.
		for i, row := range batch {
			r := results[i]
			outRow := appendVerificationColumns(row, r)
			if err := writer.WriteRow(outRow); err != nil {
				fmt.Fprintf(os.Stderr, "\nError writing row: %v\n", err)
				os.Exit(1)
			}
		}
	}

	progress.Stop()
	progress.Finalize()
}

// --- Helpers -----------------------------------------------------------------

// deriveOutputPath replaces the .csv extension with _verified.csv.
func deriveOutputPath(in string) string {
	if strings.HasSuffix(in, ".csv") {
		return in[:len(in)-4] + "_verified.csv"
	}
	return in + "_verified.csv"
}

// resolveEmailColumn returns the column index to use for email addresses.
// If name is provided, it is looked up in the header.  Otherwise the first
// data row is scanned for a column that looks like an email address.
func resolveEmailColumn(name string, header []string, rows [][]string) int {
	if name != "" {
		for i, h := range header {
			if strings.EqualFold(h, name) {
				return i
			}
		}
		fmt.Fprintf(os.Stderr, "Warning: column %q not found; falling back to auto-detect\n", name)
	}

	// Check the first few data rows for a column that looks like an email.
	// This is more robust than relying on the header name alone.
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

// appendVerificationColumns returns the input row with the five verification
// columns appended.
func appendVerificationColumns(row []string, r VerifyResult) []string {
	now := time.Now().UTC().Format(time.RFC3339)
	return append(row,
		r.Status,
		now,
		r.MXHost,
		r.CatchAll,
		fmt.Sprintf("%d", r.ResponseMs),
	)
}
