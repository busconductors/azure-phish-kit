package core

import (
	"os"
	"path/filepath"
	"testing"
)

func TestVerifyLeadsEmptyCSV(t *testing.T) {
	dir := t.TempDir()
	csvPath := filepath.Join(dir, "empty.csv")
	os.WriteFile(csvPath, []byte("email,name\n"), 0644)

	total, valid, invalid, catchAll, err := VerifyLeads(csvPath, false, "")
	if err != nil {
		t.Fatalf("VerifyLeads failed: %v", err)
	}
	if total != 0 {
		t.Errorf("expected total=0 for empty CSV, got %d", total)
	}
	if valid != 0 {
		t.Errorf("expected valid=0, got %d", valid)
	}
	if invalid != 0 {
		t.Errorf("expected invalid=0, got %d", invalid)
	}
	if catchAll != 0 {
		t.Errorf("expected catchAll=0, got %d", catchAll)
	}
}

func TestVerifyLeadsMissingFile(t *testing.T) {
	_, _, _, _, err := VerifyLeads("/nonexistent/path/leads.csv", false, "")
	if err == nil {
		t.Error("expected error for missing file")
	}
}

func TestExtractDomain(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"user@example.com", "example.com"},
		{"no-at-sign", "no-at-sign"},
		{"", ""},
		{"admin@sub.example.co.uk", "sub.example.co.uk"},
		{"@leading.com", "leading.com"},
		{"trailing@", ""},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			got := extractDomain(tc.input)
			if got != tc.expected {
				t.Errorf("extractDomain(%q) = %q, want %q", tc.input, got, tc.expected)
			}
		})
	}
}

func TestDetectEmailColumn(t *testing.T) {
	header := []string{"name", "email", "company"}
	rows := [][]string{
		{"Alice", "alice@example.com", "Acme"},
		{"Bob", "bob@example.com", "Beta"},
	}

	idx := detectEmailColumn(header, rows)
	if idx != 1 {
		t.Errorf("expected email column index 1, got %d", idx)
	}
}
