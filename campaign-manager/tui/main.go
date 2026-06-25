// Package main — Bubble Tea TUI for the GLNT Campaign Manager.
//
// Entry point.  Parses flags, reads PHISH_KEY from the environment, seeds a
// random dev key if none is set, and launches the Bubble Tea program.
package main

import (
	"crypto/rand"
	"encoding/base64"
	"flag"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
)

func main() {
	storePath := flag.String("store", "campaigns.json", "path to campaign store JSON file")
	luresPath := flag.String("lures", "../../lures", "path to lures directory")
	smtpProxy := flag.String("smtp-proxy", "", "SOCKS5 proxy address for SMTP verification (e.g. socks5://127.0.0.1:9050)")
	flag.Parse()

	phishKey := os.Getenv("PHISH_KEY")
	if phishKey == "" {
		key := make([]byte, 32)
		if _, err := rand.Read(key); err != nil {
			fmt.Fprintf(os.Stderr, "tui: failed to generate random dev key: %v\n", err)
			os.Exit(1)
		}
		phishKey = base64.StdEncoding.EncodeToString(key)
		fmt.Fprintf(os.Stderr, "tui: PHISH_KEY not set — using random key for development\n")
	}

	model := newModel(*storePath, *luresPath, phishKey, *smtpProxy)
	p := tea.NewProgram(model, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "tui: %v\n", err)
		os.Exit(1)
	}
}
