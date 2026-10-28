// Package history persists recent translation results to a local JSON file
// so users can review, copy, or re-run them without recapturing the screen.
package history

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"snaptrans/internal/jsonfile"
)

type Entry struct {
	Favorite    bool      `json:"favorite"`
	Kind        string    `json:"kind,omitempty"`
	Example     string    `json:"example,omitempty"`
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
	entries = s.trim(entries)

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

	if err := s.removeQuarantinesLocked(); err != nil {
		return err
	}
	// Remove the recovery copy first; a failed cleanup leaves the live data
	// intact and reports failure instead of silently resurrecting deleted items.
	for _, path := range []string{s.path + ".bak", s.path} {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return nil
}

func (s *Store) loadLocked() ([]Entry, error) {
	var entries []Entry
	_, err := jsonfile.ReadValidated(s.path, func(raw []byte) error {
		var decoded []Entry
		if err := json.Unmarshal(raw, &decoded); err != nil {
			return err
		}
		entries = decoded
		return nil
	})
	if err != nil {
		if errors.Is(err, jsonfile.ErrCorrupt) {
			// Preserve the damaged file for manual recovery, then allow new activity.
			for _, path := range []string{s.path, s.path + ".bak"} {
				if err := os.Rename(path, fmt.Sprintf("%s.corrupt-%d", path, time.Now().UnixNano())); err != nil && !errors.Is(err, os.ErrNotExist) {
					return nil, err
				}
			}
			return []Entry{}, nil
		}
		if errors.Is(err, os.ErrNotExist) {
			return []Entry{}, nil
		}
		return nil, err
	}

	if entries == nil {
		entries = []Entry{}
	}
	return entries, nil
}

func (s *Store) saveLocked(entries []Entry) error {
	raw, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return err
	}
	return jsonfile.Write(s.path, raw)
}

func (s *Store) saveDeletedLocked(entries []Entry) error {
	raw, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return err
	}
	if err := jsonfile.Reset(s.path, raw); err != nil {
		return err
	}
	return s.removeQuarantinesLocked()
}

func (s *Store) removeQuarantinesLocked() error {
	files, err := os.ReadDir(filepath.Dir(s.path))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	name := filepath.Base(s.path)
	for _, file := range files {
		if strings.HasPrefix(file.Name(), name+".corrupt-") || strings.HasPrefix(file.Name(), name+".bak.corrupt-") {
			if err := os.Remove(filepath.Join(filepath.Dir(s.path), file.Name())); err != nil {
				return err
			}
		}
	}
	return nil
}
