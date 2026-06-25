package core

import (
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestNewStore(t *testing.T) {
	path := filepath.Join(t.TempDir(), "store.json")
	s := NewStore(path)
	list := s.List()
	if len(list) != 0 {
		t.Fatalf("expected empty store, got %d campaigns", len(list))
	}
}

func TestStorePutAndGet(t *testing.T) {
	path := filepath.Join(t.TempDir(), "store.json")
	s := NewStore(path)
	c := Campaign{
		ID:        "test-1",
		Name:      "Test Campaign",
		Lure:      "sharepoint-doc.html",
		Phishlet:  "microsoft",
		Status:    StatusDraft,
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}
	s.Put(c)

	got, ok := s.Get("test-1")
	if !ok {
		t.Fatal("expected campaign to be found")
	}
	if got.ID != c.ID {
		t.Errorf("ID = %q, want %q", got.ID, c.ID)
	}
	if got.Name != c.Name {
		t.Errorf("Name = %q, want %q", got.Name, c.Name)
	}
	if got.Lure != c.Lure {
		t.Errorf("Lure = %q, want %q", got.Lure, c.Lure)
	}
	if got.Phishlet != c.Phishlet {
		t.Errorf("Phishlet = %q, want %q", got.Phishlet, c.Phishlet)
	}
	if got.Status != c.Status {
		t.Errorf("Status = %q, want %q", got.Status, c.Status)
	}
	if got.CreatedAt != c.CreatedAt {
		t.Errorf("CreatedAt = %q, want %q", got.CreatedAt, c.CreatedAt)
	}
}

func TestStoreList(t *testing.T) {
	path := filepath.Join(t.TempDir(), "store.json")
	s := NewStore(path)

	now := time.Now().UTC()
	// Create 3 campaigns with staggered times so sort order is deterministic.
	for i, id := range []string{"C", "A", "B"} {
		s.Put(Campaign{
			ID:        id,
			Name:      id,
			CreatedAt: now.Add(-time.Duration(i) * time.Hour).Format(time.RFC3339),
		})
	}

	list := s.List()
	if len(list) != 3 {
		t.Fatalf("expected 3 campaigns, got %d", len(list))
	}

	// List is sorted by CreatedAt descending — most recent first.
	// "C" at now, "A" at now-1h, "B" at now-2h.
	expected := []string{"C", "A", "B"}
	for i, c := range list {
		if c.ID != expected[i] {
			t.Errorf("list[%d].ID = %q, want %q", i, c.ID, expected[i])
		}
	}
}

func TestStoreGetMissing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "store.json")
	s := NewStore(path)
	_, ok := s.Get("nonexistent")
	if ok {
		t.Error("expected ok=false for missing campaign")
	}
}

func TestStorePersistence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "store.json")
	s1 := NewStore(path)
	c := Campaign{
		ID:        "persist-1",
		Name:      "Persistent Campaign",
		Status:    StatusDraft,
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}
	s1.Put(c)

	// Create a new store from the same file — campaign should survive.
	s2 := NewStore(path)
	got, ok := s2.Get("persist-1")
	if !ok {
		t.Fatal("expected campaign to persist across store instances")
	}
	if got.Name != c.Name {
		t.Errorf("Name = %q, want %q", got.Name, c.Name)
	}
}

func TestStoreConcurrent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "store.json")
	s := NewStore(path)

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			s.Put(Campaign{
				ID:        fmt.Sprintf("concurrent-%d", n),
				Name:      fmt.Sprintf("Campaign %d", n),
				CreatedAt: time.Now().UTC().Format(time.RFC3339),
			})
		}(i)
	}
	wg.Wait()

	list := s.List()
	if len(list) != 10 {
		t.Errorf("expected 10 campaigns after concurrent puts, got %d", len(list))
	}
}
