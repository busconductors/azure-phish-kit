package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPreviewLureReplacesLink(t *testing.T) {
	dir := t.TempDir()
	lurePath := filepath.Join(dir, "test-lure.html")
	os.WriteFile(lurePath, []byte(`<html><a href="{LINK}">Click here</a></html>`), 0644)

	result, err := PreviewLure(lurePath, "https://phish.example.com/track")
	if err != nil {
		t.Fatalf("PreviewLure failed: %v", err)
	}
	if !strings.Contains(result, "https://phish.example.com/track") {
		t.Errorf("expected link to be in result, got: %s", result)
	}
	if strings.Contains(result, "{LINK}") {
		t.Error("expected {LINK} placeholder to be replaced")
	}
}

func TestPreviewLureNoPlaceholder(t *testing.T) {
	dir := t.TempDir()
	lurePath := filepath.Join(dir, "no-placeholder.html")
	content := `<html><body>No placeholder here</body></html>`
	os.WriteFile(lurePath, []byte(content), 0644)

	result, err := PreviewLure(lurePath, "https://phish.example.com/track")
	if err != nil {
		t.Fatalf("PreviewLure failed: %v", err)
	}
	if result != content {
		t.Errorf("expected unchanged content without placeholder, got: %s", result)
	}
}

func TestPreviewLureMissingFile(t *testing.T) {
	_, err := PreviewLure("/nonexistent/path/lure.html", "https://phish.example.com/track")
	if err == nil {
		t.Error("expected error for missing file")
	}
}

func TestPreviewLureMultiplePlaceholders(t *testing.T) {
	dir := t.TempDir()
	lurePath := filepath.Join(dir, "multi-lure.html")
	os.WriteFile(lurePath, []byte(`<a href="{LINK}">Link 1</a><a href="{LINK}">Link 2</a>`), 0644)

	result, err := PreviewLure(lurePath, "https://phish.example.com/track")
	if err != nil {
		t.Fatalf("PreviewLure failed: %v", err)
	}
	if strings.Contains(result, "{LINK}") {
		t.Error("expected all {LINK} placeholders to be replaced")
	}
	count := strings.Count(result, "https://phish.example.com/track")
	if count != 2 {
		t.Errorf("expected 2 replacements, got %d", count)
	}
}
