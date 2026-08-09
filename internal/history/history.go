// Package history persists recent translation results to a local JSON file
// so users can review, copy, or re-run them without recapturing the screen.
package history

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type Entry struct {
	ID          string    `json:"id"`
	Timestamp   time.Time `json:"timestamp"`
	Source      string    `json:"source"`
	Translation string    `json:"translation"`
	Direction   string    `json:"direction"`
}

type Store struct {
	path string
	max  int
	mu   sync.Mutex
}

func NewStore(path string, maxEntries int) *Store {
	if maxEntries <= 0 {
		maxEntries = 50
	}
	return &Store{path: path, max: maxEntries}
}

// Add prepends a new entry and trims the persisted list to the maximum
// size. Errors are returned but callers may treat them as non-fatal.
func (s *Store) Add(source string, translation string, direction string) error {
	if s == nil || s.path == "" {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	entries, err := s.loadLocked()
	if err != nil {
		return err
	}

	entry := Entry{
		ID:          fmt.Sprintf("%d", time.Now().UnixNano()),
		Timestamp:   time.Now(),
		Source:      source,
		Translation: translation,
		Direction:   direction,
	}
	entries = append([]Entry{entry}, entries...)
	if len(entries) > s.max {
		entries = entries[:s.max]
	}

	return s.saveLocked(entries)
}

// List returns the stored entries, most recent first.
func (s *Store) List() ([]Entry, error) {
	if s == nil || s.path == "" {
		return []Entry{}, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.loadLocked()
}

// Clear removes all stored entries.
func (s *Store) Clear() error {
	if s == nil || s.path == "" {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := os.Remove(s.path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func (s *Store) loadLocked() ([]Entry, error) {
	raw, err := os.ReadFile(s.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []Entry{}, nil
		}
		return nil, err
	}

	var entries []Entry
	if err := json.Unmarshal(raw, &entries); err != nil {
		return nil, fmt.Errorf("history: parse %s: %w", s.path, err)
	}
	if entries == nil {
		entries = []Entry{}
	}
	return entries, nil
}

func (s *Store) saveLocked(entries []Entry) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, raw, 0o600)
}
