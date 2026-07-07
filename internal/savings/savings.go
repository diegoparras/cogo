// Package savings tracks a running tally of the tokens COGO saved an agent: each
// pack hands back a budgeted, colored digest instead of the notes read in full,
// and the difference accumulates here. It's the mirror of the model token meter —
// that one counts what the optional model spent; this one counts what the pack
// avoided. Persisted next to the vault in .cogo/savings.json (gitignored).
package savings

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

// Stat is the cumulative saving.
type Stat struct {
	Total int    `json:"total"` // tokens saved across every pack
	Packs int    `json:"packs"` // packs that saved something
	Since string `json:"since"` // date of the first saved pack
}

var mu sync.Mutex

func path(dir string) string { return filepath.Join(dir, ".cogo", "savings.json") }

// Read returns the running tally (zero value if none yet).
func Read(dir string) Stat {
	var s Stat
	if b, err := os.ReadFile(path(dir)); err == nil {
		_ = json.Unmarshal(b, &s)
	}
	return s
}

// Add folds one pack's saving in. Non-positive savings are ignored (a pack over
// short notes can be bigger than the raw — that adds structure, not a saving).
func Add(dir string, tokens int, today string) {
	if tokens <= 0 {
		return
	}
	mu.Lock()
	defer mu.Unlock()
	s := Read(dir)
	s.Total += tokens
	s.Packs++
	if s.Since == "" {
		s.Since = today
	}
	if b, err := json.Marshal(s); err == nil {
		_ = os.MkdirAll(filepath.Dir(path(dir)), 0o755)
		_ = os.WriteFile(path(dir), b, 0o644)
	}
}
