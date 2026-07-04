// Package contra persists the contradictions the lint pass finds, so a "red"
// verdict survives a restart instead of vanishing until the next scan — and so a
// human can resolve or dismiss one. It lives next to the vault at
// .cogo/contradictions.json (gitignored). The color engine only cares about the
// set of note ids currently under an OPEN contradiction (OpenNoteSet).
package contra

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"sync"
)

const (
	StatusOpen      = "open"      // paints both notes red
	StatusDismissed = "dismissed" // a human said "not a real contradiction" — never re-flag
)

// Item is one persisted contradiction between two notes.
type Item struct {
	ID       string `json:"id"`
	A        string `json:"a"`
	B        string `json:"b"`
	Reason   string `json:"reason"`
	Detected string `json:"detected"`
	Status   string `json:"status"`
}

// Found is a fresh detection handed in by the lint pass.
type Found struct{ A, B, Reason string }

type Store struct {
	dir   string
	mu    sync.Mutex
	items []Item
}

func Open(dir string) *Store {
	s := &Store{dir: dir}
	if b, err := os.ReadFile(s.path()); err == nil {
		var w struct {
			Contradictions []Item `json:"contradictions"`
		}
		if json.Unmarshal(b, &w) == nil {
			s.items = w.Contradictions
		}
	}
	return s
}

func (s *Store) path() string { return filepath.Join(s.dir, ".cogo", "contradictions.json") }

// pairID is a stable id for an unordered pair of note ids.
func pairID(a, b string) string {
	if a > b {
		a, b = b, a
	}
	sum := sha256.Sum256([]byte(a + "\x00" + b))
	return "cx_" + hex.EncodeToString(sum[:6])
}

func (s *Store) saveLocked() error {
	if err := os.MkdirAll(filepath.Join(s.dir, ".cogo"), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(struct {
		Contradictions []Item `json:"contradictions"`
	}{s.items}, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path(), b, 0o644)
}

// List returns every stored contradiction (open and dismissed).
func (s *Store) List() []Item {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Item, len(s.items))
	copy(out, s.items)
	return out
}

// Merge folds fresh lint findings in. A new pair becomes OPEN unless it was
// previously dismissed (then it stays dismissed — the human's call wins and it
// is never re-flagged). A lint pass NEVER clears an open contradiction on its
// own (the model is noisy — only a human resolves), except that pairs whose
// notes no longer exist are pruned. exists reports if a note id is still around.
func (s *Store) Merge(found []Found, today string, exists func(string) bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	known := map[string]int{}
	for i, c := range s.items {
		known[c.ID] = i
	}
	for _, f := range found {
		id := pairID(f.A, f.B)
		if idx, ok := known[id]; ok {
			if s.items[idx].Status == StatusOpen && f.Reason != "" {
				s.items[idx].Reason = f.Reason
			}
			continue
		}
		s.items = append(s.items, Item{ID: id, A: f.A, B: f.B, Reason: f.Reason, Detected: today, Status: StatusOpen})
	}
	kept := s.items[:0]
	for _, c := range s.items {
		if exists(c.A) && exists(c.B) {
			kept = append(kept, c)
		}
	}
	s.items = kept
	sort.Slice(s.items, func(i, j int) bool { return s.items[i].ID < s.items[j].ID })
	_ = s.saveLocked()
}

func (s *Store) mutate(id string, del bool, status string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.items {
		if s.items[i].ID == id {
			if del {
				s.items = append(s.items[:i], s.items[i+1:]...)
			} else {
				s.items[i].Status = status
			}
			_ = s.saveLocked()
			return true
		}
	}
	return false
}

// Resolve forgets a contradiction (the human fixed the underlying notes). If it
// still holds, the next lint pass will surface it again.
func (s *Store) Resolve(id string) bool { return s.mutate(id, true, "") }

// Dismiss marks a contradiction as a false positive: it stays on record but
// never paints red and is never re-flagged by lint.
func (s *Store) Dismiss(id string) bool { return s.mutate(id, false, StatusDismissed) }

// Conflict is one open contradiction seen from a single note's side: which OTHER
// note it clashes with, and why. It turns the bare red color into a trace — the
// agent (or the visor) can see what to resolve instead of just "this is red".
type Conflict struct {
	ID     string `json:"id"`     // the contradiction's stable id
	Other  string `json:"other"`  // the note on the other side of the clash
	Reason string `json:"reason"` // why they contradict (from the lint pass)
}

// ForNote returns the open contradictions that touch note id, each pointing at
// the note on the other side. Empty if the note is in no open contradiction.
func (s *Store) ForNote(id string) []Conflict {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []Conflict
	for _, c := range s.items {
		if c.Status != StatusOpen {
			continue
		}
		switch id {
		case c.A:
			out = append(out, Conflict{ID: c.ID, Other: c.B, Reason: c.Reason})
		case c.B:
			out = append(out, Conflict{ID: c.ID, Other: c.A, Reason: c.Reason})
		}
	}
	return out
}

// OpenNoteSet is the set of note ids under an open contradiction — fed to
// core.Evaluate so those notes go red.
func (s *Store) OpenNoteSet() map[string]bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	m := map[string]bool{}
	for _, c := range s.items {
		if c.Status == StatusOpen {
			m[c.A] = true
			m[c.B] = true
		}
	}
	return m
}
