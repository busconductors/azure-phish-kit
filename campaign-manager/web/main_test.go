package main

import (
	"bytes"
	"encoding/csv"
	"html/template"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/strasser-lab/azure-phish-kit/campaign-manager/core"
)

// newTestServer creates an httptest.Server wired up the same way as main(),
// but using temp directories so tests are isolated.
func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()

	tmpDir := t.TempDir()

	// Create lures directory with a test lure file.
	luresDir := filepath.Join(tmpDir, "lures")
	os.MkdirAll(luresDir, 0755)
	os.WriteFile(
		filepath.Join(luresDir, "sharepoint-doc.html"),
		[]byte(`<html><body><a href="{LINK}">Open Document</a></body></html>`),
		0644,
	)

	// Create phishlets directory with a test phishlet JSON file.
	phishletsDir := filepath.Join(tmpDir, "phishlets")
	os.MkdirAll(phishletsDir, 0755)
	os.WriteFile(
		filepath.Join(phishletsDir, "microsoft365.json"),
		[]byte(`{"name":"microsoft365","label":"Microsoft 365"}`),
		0644,
	)

	// Create store.
	storePath := filepath.Join(tmpDir, "campaigns.json")
	store := core.NewStore(storePath)

	// Scan lures and phishlets from the temp dirs.
	lures := scanLures(luresDir)
	phishlets := scanPhishlets(phishletsDir)

	// Parse templates from the embedded filesystem.
	tmpl, err := template.ParseFS(templateFS, "templates/*.html")
	if err != nil {
		t.Fatalf("parse templates: %v", err)
	}

	mux := http.NewServeMux()

	// Replicate the route registrations from main().
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		campaigns := store.List()
		data := listPageData{
			Campaigns: campaigns,
			Summary:   buildSummary(campaigns),
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := tmpl.ExecuteTemplate(w, "list.html", data); err != nil {
			t.Logf("[ERROR] template list: %v", err)
		}
	})

	mux.HandleFunc("GET /campaigns/new", func(w http.ResponseWriter, r *http.Request) {
		data := newCampaignData{
			Lures:     lures,
			Phishlets: phishlets,
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := tmpl.ExecuteTemplate(w, "new.html", data); err != nil {
			t.Logf("[ERROR] template new: %v", err)
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
			t.Logf("[ERROR] template detail: %v", err)
		}
	})

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
			CreatedAt: "2025-01-01T00:00:00Z", // deterministic for test
		}
		store.Put(c)
		http.Redirect(w, r, "/campaigns/"+c.ID, http.StatusSeeOther)
	})

	mux.HandleFunc("POST /api/campaigns/{id}/link", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		_, ok := store.Get(id)
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "campaign not found"})
			return
		}
		if err := r.ParseForm(); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid form"})
			return
		}
		keyB64 := strings.TrimSpace(r.FormValue("key"))
		if keyB64 == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "encryption key is required"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "skipped in test"})
	})

	mux.HandleFunc("POST /api/campaigns/{id}/verify", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		c, ok := store.Get(id)
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "campaign not found"})
			return
		}
		_ = c
		file, _, err := r.FormFile("leads")
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "CSV file upload required (field: leads)"})
			return
		}
		defer file.Close()

		// Save uploaded CSV to disk (in tmpDir).
		leadsDir := filepath.Join(tmpDir, "leads")
		os.MkdirAll(leadsDir, 0755)
		dstPath := filepath.Join(leadsDir, id+".csv")
		dst, err := os.Create(dstPath)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to save file"})
			return
		}
		defer dst.Close()
		io.Copy(dst, file)

		// Re-read and count lines.
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
		lureFile := filepath.Join(luresDir, c.Lure)
		preview, err := core.PreviewLure(lureFile, c.Link)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "lure file not found"})
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(preview))
	})

	mux.HandleFunc("GET /api/events", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{}`))
	})

	mux.HandleFunc("POST /api/campaigns/{id}/deploy", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		c, ok := store.Get(id)
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "campaign not found"})
			return
		}
		c.Status = core.StatusDeployed
		store.Put(c)
		writeJSON(w, http.StatusOK, map[string]string{"status": "deployed"})
	})

	// Catch-all 404 handler.
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte("<html><body></body></html>"))
	})

	return httptest.NewServer(securityHeaders(mux))
}

func TestListPage(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	resp, err := srv.Client().Get(srv.URL + "/")
	if err != nil {
		t.Fatalf("GET /: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "<html") {
		t.Error("expected HTML response")
	}
}

func TestNewCampaignPage(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	resp, err := srv.Client().Get(srv.URL + "/campaigns/new")
	if err != nil {
		t.Fatalf("GET /campaigns/new: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
}

func TestCreateCampaign(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	// The client should not follow redirects so we can check the 303.
	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	form := url.Values{}
	form.Set("name", "Test Campaign")
	form.Set("lure", "sharepoint-doc.html")
	form.Set("phishlet", "microsoft365")

	resp, err := client.PostForm(srv.URL+"/api/campaigns", form)
	if err != nil {
		t.Fatalf("POST /api/campaigns: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusSeeOther {
		t.Errorf("expected 303 redirect, got %d", resp.StatusCode)
	}

	loc := resp.Header.Get("Location")
	if !strings.HasPrefix(loc, "/campaigns/") {
		t.Errorf("expected redirect to /campaigns/{id}, got %q", loc)
	}
}

func TestCreateCampaignMissingFields(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	form := url.Values{}
	form.Set("name", "")
	form.Set("lure", "")
	form.Set("phishlet", "")

	resp, err := http.PostForm(srv.URL+"/api/campaigns", form)
	if err != nil {
		t.Fatalf("POST /api/campaigns: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400 for missing fields, got %d", resp.StatusCode)
	}
}

func TestCampaignNotFound(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	resp, err := srv.Client().Get(srv.URL + "/campaigns/nonexistent")
	if err != nil {
		t.Fatalf("GET /campaigns/nonexistent: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404, got %d", resp.StatusCode)
	}
}

func TestVerifyCSVUpload(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	// First, create a campaign to verify.
	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	form := url.Values{}
	form.Set("name", "Verify Test")
	form.Set("lure", "sharepoint-doc.html")
	form.Set("phishlet", "microsoft365")
	resp, err := client.PostForm(srv.URL+"/api/campaigns", form)
	if err != nil {
		t.Fatalf("create campaign: %v", err)
	}
	resp.Body.Close()

	loc := resp.Header.Get("Location")
	if loc == "" {
		t.Fatal("expected Location header in redirect")
	}
	// Extract campaign ID from /campaigns/{id}
	campaignID := filepath.Base(loc)

	// Build a multipart form with a CSV file.
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, err := mw.CreateFormFile("leads", "leads.csv")
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	fw.Write([]byte("email,name\nalice@example.com,Alice\nbob@example.com,Bob\n"))
	mw.Close()

	req, err := http.NewRequest("POST", srv.URL+"/api/campaigns/"+campaignID+"/verify", &buf)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())

	resp2, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("POST verify: %v", err)
	}
	defer resp2.Body.Close()

	if resp2.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp2.Body)
		t.Errorf("expected 200, got %d: %s", resp2.StatusCode, string(body))
	}
}
