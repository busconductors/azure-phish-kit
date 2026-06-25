package core

// Campaign represents a phishing campaign.
type Campaign struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Lure      string `json:"lure"`
	Phishlet  string `json:"phishlet"`
	Link      string `json:"link"`
	LeadFile  string `json:"lead_file"`
	LeadCount int    `json:"lead_count"`
	Status    string `json:"status"` // "draft", "active", "verified", "deployed"
	CreatedAt string `json:"created_at"`
}

const (
	StatusDraft    = "draft"
	StatusActive   = "active"
	StatusVerified = "verified"
	StatusDeployed = "deployed"
)
