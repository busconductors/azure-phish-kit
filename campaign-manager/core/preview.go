package core

import (
	"fmt"
	"os"
	"strings"
)

// PreviewLure reads a lure HTML file, replaces {LINK} and ##victimemail##
// placeholders, and returns the filled HTML string.
//
// lurePath is the full path to the HTML file (e.g., lures/attachments/sharepoint-doc.html).
// link is the phishing link to embed.
// recipientEmail is the target email address.
func PreviewLure(lurePath string, link string, recipientEmail string) (string, error) {
	data, err := os.ReadFile(lurePath)
	if err != nil {
		return "", fmt.Errorf("read lure %s: %w", lurePath, err)
	}

	html := string(data)

	// Replace the link placeholder.
	html = strings.ReplaceAll(html, "{LINK}", link)

	// Replace the victim email placeholder.
	html = strings.ReplaceAll(html, "##victimemail##", recipientEmail)

	return html, nil
}
