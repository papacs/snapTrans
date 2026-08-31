package history

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// Keep up to max recent translations, plus explicitly saved items.
func (s *Store) trim(entries []Entry) []Entry {
	result := make([]Entry, 0, len(entries))
	recent := 0
	for _, e := range entries {
		if e.Favorite || e.Kind == "learning" {
			result = append(result, e)
			continue
		}
		if recent < s.max {
			result = append(result, e)
			recent++
		}
	}
	return result
}
func (s *Store) SetFavorite(id string, favorite bool) error {
	if s == nil || s.path == "" {
		return errors.New("history is unavailable")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	entries, err := s.loadLocked()
	if err != nil {
		return err
	}
	for i := range entries {
		if entries[i].ID == id {
			entries[i].Favorite = favorite
			return s.saveLocked(s.trim(entries))
		}
	}
	return errors.New("history entry no longer exists")
}
func (s *Store) AddLearning(source, meaning, example string) error {
	if s == nil || s.path == "" {
		return errors.New("history is unavailable")
	}
	source = strings.TrimSpace(source)
	meaning = strings.TrimSpace(meaning)
	if source == "" || meaning == "" || len(source)+len(meaning)+len(example) > 100000 {
		return errors.New("card requires source and meaning, within 100 KB")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	entries, err := s.loadLocked()
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.Kind == "learning" && e.Source == source && e.Translation == meaning && e.Example == example {
			return nil
		}
	}
	entry := Entry{ID: fmt.Sprintf("card-%d", time.Now().UnixNano()), Timestamp: time.Now(), Source: source, Translation: meaning, Example: example, Kind: "learning", Favorite: true}
	return s.saveLocked(append([]Entry{entry}, entries...))
}
func (s *Store) DeleteSaved(id string) error {
	if s == nil || s.path == "" {
		return errors.New("history is unavailable")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	entries, err := s.loadLocked()
	if err != nil {
		return err
	}
	for i, e := range entries {
		if e.ID == id {
			return s.saveLocked(append(entries[:i], entries[i+1:]...))
		}
	}
	return errors.New("history entry no longer exists")
}

// Clearing recent activity must never silently erase explicit saves.
func (s *Store) ClearRecent() error {
	if s == nil || s.path == "" {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	entries, err := s.loadLocked()
	if err != nil {
		return err
	}
	saved := make([]Entry, 0)
	for _, e := range entries {
		if e.Favorite || e.Kind == "learning" {
			saved = append(saved, e)
		}
	}
	return s.saveLocked(saved)
}
