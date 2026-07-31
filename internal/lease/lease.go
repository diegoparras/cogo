// Package lease is COGO's coordination primitive for a shared vault: a named,
// time-bounded permit so two agents on two machines don't both run the same
// migration. It is the roadmap's "leases": an agent acquires "migrate-db" with a
// TTL; anyone else who asks is told it's held, by whom, and until when. Leases
// expire on their own (a crashed holder never wedges the vault forever) and are
// re-entrant for the same holder (renew by re-acquiring).
//
// Deliberately advisory, like git: COGO reports the conflict, it doesn't enforce
// it at the filesystem. The value is that the collision becomes visible.
package lease

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// Lease is one held permit.
type Lease struct {
	Name     string `json:"name"`
	Holder   string `json:"holder"`   // who holds it (agent id / token label)
	Acquired string `json:"acquired"` // RFC3339 UTC
	Expires  string `json:"expires"`  // RFC3339 UTC
	Note     string `json:"note,omitempty"`
}

// held reports whether the lease is still active at t.
func (l Lease) held(t time.Time) bool {
	exp, err := time.Parse(time.RFC3339, l.Expires)
	return err == nil && t.Before(exp)
}

// Store persists leases next to the vault in .cogo/leases.json.
type Store struct {
	path string
	mu   sync.Mutex
}

// Open returns the lease store for a vault dir.
func Open(dir string) *Store { return &Store{path: filepath.Join(dir, ".cogo", "leases.json")} }

func (s *Store) read() map[string]Lease {
	m := map[string]Lease{}
	if b, err := os.ReadFile(s.path); err == nil {
		_ = json.Unmarshal(b, &m)
	}
	return m
}

func (s *Store) write(m map[string]Lease) {
	if b, err := json.MarshalIndent(m, "", "  "); err == nil {
		_ = os.MkdirAll(filepath.Dir(s.path), 0o755)
		_ = os.WriteFile(s.path, b, 0o644)
	}
}

// Acquire grants (or renews) a lease. It succeeds when the name is free, expired,
// or already held by the same holder (renewal); it fails with the conflicting
// lease's details when another holder still holds it. ttl is clamped to at least
// one second. now is injected for testability.
func (s *Store) Acquire(name, holder, note string, ttl time.Duration, now time.Time) (Lease, error) {
	if name == "" || holder == "" {
		return Lease{}, fmt.Errorf("lease needs a name and a holder")
	}
	if ttl < time.Second {
		ttl = time.Second
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	m := s.read()
	if cur, ok := m[name]; ok && cur.held(now) && cur.Holder != holder {
		return cur, fmt.Errorf("lease %q is held by %q until %s", name, cur.Holder, cur.Expires)
	}
	acquired := now.UTC().Format(time.RFC3339)
	if cur, ok := m[name]; ok && cur.Holder == holder {
		acquired = cur.Acquired // renewal keeps the original acquisition time
	}
	l := Lease{Name: name, Holder: holder, Acquired: acquired, Expires: now.UTC().Add(ttl).Format(time.RFC3339), Note: note}
	m[name] = l
	s.write(m)
	return l, nil
}

// Release drops a lease the holder owns. Releasing one you don't hold, or one
// that doesn't exist, is a no-op (idempotent) but reported so a caller can tell.
func (s *Store) Release(name, holder string) (released bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	m := s.read()
	if cur, ok := m[name]; ok && cur.Holder == holder {
		delete(m, name)
		s.write(m)
		return true
	}
	return false
}

// List returns the currently-held (non-expired) leases, soonest-to-expire first.
// It also prunes expired entries from disk as a side effect.
func (s *Store) List(now time.Time) []Lease {
	s.mu.Lock()
	defer s.mu.Unlock()
	m := s.read()
	out := []Lease{}
	changed := false
	for name, l := range m {
		if l.held(now) {
			out = append(out, l)
		} else {
			delete(m, name)
			changed = true
		}
	}
	if changed {
		s.write(m)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Expires < out[j].Expires })
	return out
}
