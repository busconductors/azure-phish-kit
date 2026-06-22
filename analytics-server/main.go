package main

import (
	"embed"
	"encoding/json"
	"flag"
	"html/template"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
)

//go:embed dashboard.html
var tmplFS embed.FS

// allowedNets holds parsed CIDRs from ALLOWED_IPS env var for IP-based access control.
var allowedNets []net.IPNet

func main() {
	port := flag.String("port", "9091", "listen port")
	dataPath := flag.String("data", "../data/captures.jsonl", "path to captures.jsonl")
	token := flag.String("token", "", "auth token for dashboard access (empty = no auth)")
	flag.Parse()

	// OPSEC env vars
	maxAge := os.Getenv("MAX_AGE_HOURS")
	allowedIPs := os.Getenv("ALLOWED_IPS")

	// Set the package-level max-age for parseJSONL filtering.
	if maxAge != "" {
		MaxAgeHours = maxAge
		log.Printf("MAX_AGE_HOURS=%s — events older than %s hours will be filtered", maxAge, maxAge)
	}

	// Parse ALLOWED_IPS into CIDRs for authMiddleware.
	if allowedIPs != "" {
		for _, cidr := range strings.Split(allowedIPs, ",") {
			cidr = strings.TrimSpace(cidr)
			_, n, err := net.ParseCIDR(cidr)
			if err != nil {
				log.Fatalf("FATAL: invalid CIDR in ALLOWED_IPS: %s (%v)", cidr, err)
			}
			allowedNets = append(allowedNets, *n)
			log.Printf("ALLOWED_IPS: granting access to %s", cidr)
		}
	}

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

	mux.HandleFunc("GET /api/events", func(w http.ResponseWriter, r *http.Request) {
		data, err := cache.Get()
		if err != nil {
			http.Error(w, `{"error":"service unavailable"}`, http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		body, _ := json.Marshal(data); w.Write(body)
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

// authMiddleware requires ?token=<value> if a token is configured,
// and checks remote IP against ALLOWED_IPS CIDRs when configured.
func authMiddleware(token string, next http.Handler) http.Handler {
	if token == "" && len(allowedNets) == 0 {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// IP check: if ALLOWED_IPS is set, remote IP must match one of the CIDRs.
		if len(allowedNets) > 0 {
			ip := extractIP(r)
			ok := false
			for _, n := range allowedNets {
				if n.Contains(ip) {
					ok = true
					break
				}
			}
			if !ok {
				w.WriteHeader(http.StatusNotFound)
				w.Write([]byte("<html><body></body></html>"))
				return
			}
		}
		// Token check: if a token is configured, one of query/auth header must match.
		if token != "" {
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
			return
		}
		next.ServeHTTP(w, r)
	})
}

// extractIP returns the remote IP from the request, preferring X-Forwarded-For.
func extractIP(r *http.Request) net.IP {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		ips := strings.Split(xff, ",")
		if ip := net.ParseIP(strings.TrimSpace(ips[0])); ip != nil {
			return ip
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return net.ParseIP(r.RemoteAddr)
	}
	return net.ParseIP(host)
}

// securityHeaders adds defensive HTTP headers to avoid fingerprinting.
func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		next.ServeHTTP(w, r)
	})
}
