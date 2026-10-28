package translator

import (
	"crypto/sha256"
	"encoding/json"
	"sync"
	"time"
)

// A bounded, process-local memo for successful translations only. Keys are
// hashes: screenshots, source text and credentials are never retained here.
// Replies and other generative actions deliberately bypass it.
var recentTranslations translationMemo

type memoEntry struct {
	key       [32]byte
	text      string
	direction Direction
	expires   time.Time
}

type translationMemo struct {
	sync.Mutex
	entries [16]memoEntry
	next    int
}

func translationMemoKey(options Options, mode, input string, direction Direction) [32]byte {
	raw, _ := json.Marshal(struct {
		Options     Options
		Mode, Input string
		Direction   Direction
	}{options, mode, input, direction})
	return sha256.Sum256(raw)
}

func (m *translationMemo) get(key [32]byte, now time.Time) (memoEntry, bool) {
	m.Lock()
	defer m.Unlock()
	for i, entry := range m.entries {
		if entry.text != "" && !now.Before(entry.expires) {
			m.entries[i] = memoEntry{}
			continue
		}
		if entry.text != "" && entry.key == key {
			return entry, true
		}
	}
	return memoEntry{}, false
}

func (m *translationMemo) put(key [32]byte, text string, direction Direction, now time.Time) {
	if text == "" || len(text) > 64<<10 {
		return
	}
	m.Lock()
	defer m.Unlock()
	m.entries[m.next] = memoEntry{key: key, text: text, direction: direction, expires: now.Add(10 * time.Minute)}
	m.next = (m.next + 1) % len(m.entries)
}
