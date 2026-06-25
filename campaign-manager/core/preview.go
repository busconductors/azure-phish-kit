package core

import (
	"fmt"
	"os"
	"strings"
)

// PreviewLure reads a lure HTML file, replaces the {LINK} placeholder,
// and returns the filled HTML string.
//
// lurePath is the full path to the HTML file (e.g., lures/attachments/sharepoint-doc.html).
// link is the phishing link to embed.
func PreviewLure(lurePath string, link string) (string, error) {
	data, err := os.ReadFile(lurePath)
	if err != nil {
		return "", fmt.Errorf("read lure %s: %w", lurePath, err)
	}

	html := string(data)

	// Replace the link placeholder.
	html = strings.ReplaceAll(html, "{LINK}", link)

	return html, nil
}
