// Package main — TUI campaign manager for the Azure Phish Kit.
//
// Shared types mirror ../core/ — once that package lands, import from there instead.
// Until then, these definitions keep the TUI self-contained and compilable.

package main

// Campaign represents a phishing campaign managed by the system.
// Mirrors: ../core/campaign.go
type Campaign struct {
	ID        string // unique identifier: "c001"
	Name      string // human-readable name: "q3-phishing"
	Lure      string // lure filename: "fake-mfa.html"
	Phishlet  string // phishlet key: "microsoft", "okta"
	Link      string // generated phishing URL (base64-encoded target)
	LeadFile  string // path to target CSV: "leads/q3-targets.csv"
	LeadCount int    // number of leads loaded
	Status    string // "draft" | "ready" | "active" | "archived"
	CreatedAt string // ISO 8601 date: "2026-06-15"
}
