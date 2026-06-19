package main

import (
	"embed"
	"flag"
	"html/template"
	"log"
	"net/http"
	"strings"
)

//go:embed dashboard.html
var tmplFS embed.FS

func main() {
	port := flag.String("port", "9091", "listen port")
	dataPath := flag.String("data", "../data/captures.jsonl", "path to captures.jsonl")
	token := flag.String("token", "", "auth token for dashboard access (empty = no auth)")
	flag.Parse()

	tmpl, err := template.ParseFS(tmplFS, "dashboard.html")
	if err != nil {
		log.Fatalf("FATAL: cannot parse template: %v", err)
	}

	cache := NewCache(*dataPath)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		data, err := cache.Get()
		if err != nil {
			log.Printf("[ERROR] cache get: %v", err)
			http.Error(w, "service unavailable", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := tmpl.Execute(w, data); err != nil {
			log.Printf("[ERROR] template execute: %v", err)
		}
	})

	// Catch-all 404 handler — avoids Go's default fingerprint
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte("<html><body></body></html>"))
	})

	addr := ":" + *port
	log.Printf("Analytics dashboard listening on http://localhost%s (data: %s)", addr, *dataPath)
	log.Fatal(http.ListenAndServe(addr, authMiddleware(*token, securityHeaders(mux))))
}

// authMiddleware requires ?token=<value> if a token is configured.
func authMiddleware(token string, next http.Handler) http.Handler {
	if token == "" {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("token") == token {
			next.ServeHTTP(w, r)
			return
		}
		if ah := r.Header.Get("Authorization"); strings.HasPrefix(ah, "Bearer ") {
			if strings.TrimPrefix(ah, "Bearer ") == token {
				next.ServeHTTP(w, r)
				return
			}
		}
		w.Header().Set("WWW-Authenticate", "Basic")
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	})
}

// securityHeaders adds defensive HTTP headers to avoid fingerprinting.
func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		next.ServeHTTP(w, r)
	})
}
