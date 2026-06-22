package main

import (
	"encoding/csv"
	"fmt"
	"os"
	"strings"
)

// --- CSV Reader --------------------------------------------------------------

// CSVReader wraps encoding/csv.Reader with helper methods for streaming.
type CSVReader struct {
	file   *os.File
	reader *csv.Reader
	header []string
}

// OpenCSVReader opens a CSV file and reads the header row.
func OpenCSVReader(path string) (*CSVReader, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}

	r := csv.NewReader(f)
	r.LazyQuotes = true
	r.TrimLeadingSpace = true

	header, err := r.Read()
	if err != nil {
		f.Close()
		return nil, fmt.Errorf("read header from %s: %w", path, err)
	}

	return &CSVReader{file: f, reader: r, header: header}, nil
}

// Header returns the column names from the first row.
func (r *CSVReader) Header() []string { return r.header }

// ReadAll reads every remaining record into memory.
func (r *CSVReader) ReadAll() ([][]string, error) {
	recs, err := r.reader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("read records: %w", err)
	}
	return recs, nil
}

// Close releases the underlying file handle.
func (r *CSVReader) Close() error { return r.file.Close() }

// --- CSV Writer --------------------------------------------------------------

// CSVWriter writes records to a CSV file.
type CSVWriter struct {
	file   *os.File
	writer *csv.Writer
}

// OpenCSVWriter creates a new CSV file for writing.
func OpenCSVWriter(path string) (*CSVWriter, error) {
	f, err := os.Create(path)
	if err != nil {
		return nil, fmt.Errorf("create %s: %w", path, err)
	}
	return &CSVWriter{file: f, writer: csv.NewWriter(f)}, nil
}

// WriteRow writes a single record (flushed immediately for crash-safety).
func (w *CSVWriter) WriteRow(row []string) error {
	if err := w.writer.Write(row); err != nil {
		return fmt.Errorf("write row: %w", err)
	}
	w.writer.Flush()
	return w.writer.Error()
}

// Close flushes and closes the file.
func (w *CSVWriter) Close() error {
	w.writer.Flush()
	if err := w.writer.Error(); err != nil {
		w.file.Close()
		return err
	}
	return w.file.Close()
}

// --- Helpers -----------------------------------------------------------------

// DetectEmailColumn returns the index of the first header whose value
// contains '@' and is not one of the output columns appended by this tool.
// Falls back to 0 if no such column is found.
func DetectEmailColumn(header []string) int {
	outputCols := map[string]bool{
		"_verified_status": true,
		"_verified_at":     true,
		"_mx_host":         true,
		"_catch_all":       true,
		"_response_ms":     true,
	}

	for i, h := range header {
		if outputCols[h] {
			continue
		}
		if strings.Contains(h, "@") {
			return i
		}
	}

	// Fallback: first column
	return 0
}

// OutputHeader returns the header to be written, which is the original
// header plus the verification columns appended.
func OutputHeader(original []string) []string {
	out := make([]string, len(original), len(original)+5)
	copy(out, original)
	out = append(out, "_verified_status", "_verified_at", "_mx_host", "_catch_all", "_response_ms")
	return out
}
