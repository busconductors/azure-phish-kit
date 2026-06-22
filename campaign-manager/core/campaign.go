package core

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Campaign represents a phishing campaign stored on disk.
type Campaign struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Lure      string `json:"lure"`
	Phishlet  string `json:"phishlet"`
	Link      string `json:"link"`
	LeadFile  string `json:"lead_file"`
	LeadCount int    `json:"lead_count"`
	Status    string `json:"status"` // "draft", "ready", "active", "archived"
	CreatedAt string `json:"created_at"`
}

// campaignFile returns the full path for a campaign JSON file.
func campaignFile(dir, id string) string {
	return filepath.Join(dir, id+".json")
}

// ListCampaigns returns all campaigns stored as JSON files in the given
// directory.  Missing directory is treated as an empty list.
func ListCampaigns(dir string) ([]Campaign, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read campaign dir %s: %w", dir, err)
	}

	var campaigns []Campaign
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		c, err := LoadCampaign(filepath.Join(dir, e.Name()))
		if err != nil {
			continue // skip corrupt files
		}
		campaigns = append(campaigns, c)
	}
	return campaigns, nil
}

// SaveCampaign writes a campaign to its JSON file under the given directory.
// The directory is created if it does not exist.
func SaveCampaign(dir string, c Campaign) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create campaign dir: %w", err)
	}

	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal campaign: %w", err)
	}

	path := campaignFile(dir, c.ID)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write campaign file: %w", err)
	}
	return nil
}

// LoadCampaign reads a single campaign JSON file.
func LoadCampaign(path string) (Campaign, error) {
	var c Campaign
	data, err := os.ReadFile(path)
	if err != nil {
		return c, fmt.Errorf("read campaign file %s: %w", path, err)
	}
	if err := json.Unmarshal(data, &c); err != nil {
		return c, fmt.Errorf("unmarshal campaign: %w", err)
	}
	return c, nil
}
