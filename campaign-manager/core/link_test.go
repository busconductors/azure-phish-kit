package core

import (
	"crypto/rand"
	"encoding/base64"
	"testing"
)

// validKey32 returns a base64-encoded 32-byte key (44-char standard base64).
func validKey32(t *testing.T) string {
	t.Helper()
	keyBytes := make([]byte, 32)
	if _, err := rand.Read(keyBytes); err != nil {
		t.Fatalf("rand.Read: %v", err)
	}
	return base64.StdEncoding.EncodeToString(keyBytes)
}

func TestGenerateLinkValid(t *testing.T) {
	key := validKey32(t)
	link, err := GenerateLink(key, "https://example.com", "camp-1", "microsoft", "shared-doc", "")
	if err != nil {
		t.Fatalf("GenerateLink failed: %v", err)
	}
	if link == "" {
		t.Error("expected non-empty link")
	}
}

func TestGenerateLinkInvalidKey(t *testing.T) {
	_, err := GenerateLink("short", "https://example.com", "camp-1", "", "", "")
	if err == nil {
		t.Error("expected error for short key")
	}
}

func TestGenerateLinkDefaults(t *testing.T) {
	key := validKey32(t)
	link, err := GenerateLink(key, "https://example.com", "camp-1", "", "", "")
	if err != nil {
		t.Fatalf("GenerateLink with empty brand/template failed: %v", err)
	}
	if link == "" {
		t.Error("expected non-empty link with default brand and template")
	}
}

func TestGenerateLinkStdEncoding(t *testing.T) {
	// Standard base64 key (contains + and /, no - or _).
	keyBytes := make([]byte, 32)
	if _, err := rand.Read(keyBytes); err != nil {
		t.Fatalf("rand.Read: %v", err)
	}
	key := base64.StdEncoding.EncodeToString(keyBytes)

	link, err := GenerateLink(key, "https://example.com", "camp-1", "", "", "")
	if err != nil {
		t.Fatalf("GenerateLink with std encoding key failed: %v", err)
	}
	if link == "" {
		t.Error("expected non-empty link")
	}
}

func TestGenerateLinkURLSafeEncoding(t *testing.T) {
	// URL-safe base64 key (uses - and _ instead of + and /).
	keyBytes := make([]byte, 32)
	if _, err := rand.Read(keyBytes); err != nil {
		t.Fatalf("rand.Read: %v", err)
	}
	key := base64.URLEncoding.WithPadding(base64.NoPadding).EncodeToString(keyBytes)

	link, err := GenerateLink(key, "https://example.com", "camp-1", "", "", "")
	if err != nil {
		t.Fatalf("GenerateLink with URL-safe key failed: %v", err)
	}
	if link == "" {
		t.Error("expected non-empty link")
	}
}
