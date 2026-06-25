package core

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"sort"
	"sync"
)

// Store holds campaigns in memory, persisting to a JSON file.
type Store struct {
	mu    sync.RWMutex
	path  string
	items map[string]Campaign
}

// NewStore creates a Store, loading existing campaigns from the given JSON
// file path if it exists.
func NewStore(path string) *Store {
	s := &Store{path: path, items: make(map[string]Campaign)}
	data, err := os.ReadFile(path)
	if err == nil {
		var campaigns []Campaign
		if json.Unmarshal(data, &campaigns) == nil {
			for _, c := range campaigns {
				s.items[c.ID] = c
			}
			log.Printf("Loaded %d campaigns from %s", len(s.items), path)
		}
	}
	return s
}

// List returns all campaigns sorted by CreatedAt descending.
func (s *Store) List() []Campaign {
	s.mu.RLock()
	defer s.mu.RUnlock()
	list := make([]Campaign, 0, len(s.items))
	for _, c := range s.items {
		list = append(list, c)
	}
	sort.Slice(list, func(i, j int) bool {
		return list[i].CreatedAt > list[j].CreatedAt
	})
	return list
}

// Get returns the campaign with the given id and a boolean indicating
// whether it was found.
func (s *Store) Get(id string) (Campaign, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	c, ok := s.items[id]
	return c, ok
}

// Put inserts or updates a campaign in the store and persists to disk.
func (s *Store) Put(c Campaign) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.items[c.ID] = c
	s.persist()
}

// persist writes the full campaign list to the JSON file.
func (s *Store) persist() {
	list := make([]Campaign, 0, len(s.items))
	for _, c := range s.items {
		list = append(list, c)
	}
	data, err := json.Marshal(list)
	if err != nil {
		log.Printf("[ERROR] marshal store: %v", err)
		return
	}
	os.MkdirAll(filepath.Dir(s.path), 0755)
	if err := os.WriteFile(s.path, data, 0644); err != nil {
		log.Printf("[ERROR] write store: %v", err)
	}
}
