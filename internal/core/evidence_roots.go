package core

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// EvidenceRoots maps a project to the base directory its repo-relative evidence
// refs resolve against. Projects live in different repos, so one global root
// can't resolve them all: a ref "cmd/main.go" in project A and in project B
// point at different files on disk. An empty or unmapped project falls back to
// the global default (the COGO_EVIDENCE_ROOT env var, or the file's default).
//
// Zero value is valid: no roots, everything relative is "unchecked".
type EvidenceRoots struct {
	def   string
	byPrj map[string]string
}

// Root returns the base dir for a project's relative refs, or the default.
func (r EvidenceRoots) Root(project string) string {
	if r.byPrj != nil {
		if p, ok := r.byPrj[project]; ok && p != "" {
			return p
		}
	}
	return r.def
}

// Default is the global fallback root.
func (r EvidenceRoots) Default() string { return r.def }

// Projects returns a copy of the per-project map (safe to hand to a caller).
func (r EvidenceRoots) Projects() map[string]string {
	out := map[string]string{}
	for k, v := range r.byPrj {
		out[k] = v
	}
	return out
}

// Configured reports whether any resolving root is set at all.
func (r EvidenceRoots) Configured() bool { return r.def != "" || len(r.byPrj) > 0 }

type evidenceRootsFile struct {
	Default  string            `json:"default,omitempty"`
	Projects map[string]string `json:"projects,omitempty"`
}

func evidenceRootsPath(dir string) string {
	return filepath.Join(dir, ".cogo", "evidence-roots.json")
}

// LoadEvidenceRoots reads .cogo/evidence-roots.json (per-project roots configured
// through the UI) and merges it with the COGO_EVIDENCE_ROOT env default. The file's
// own default, if set, overrides the env — the UI is the more specific signal.
func LoadEvidenceRoots(dir string) EvidenceRoots {
	r := EvidenceRoots{def: os.Getenv("COGO_EVIDENCE_ROOT")}
	if b, err := os.ReadFile(evidenceRootsPath(dir)); err == nil {
		var f evidenceRootsFile
		if json.Unmarshal(b, &f) == nil {
			if f.Default != "" {
				r.def = f.Default
			}
			r.byPrj = f.Projects
		}
	}
	return r
}

// SaveEvidenceRoots persists the per-project roots and default to disk. Empty
// entries are dropped so the file stays clean.
func SaveEvidenceRoots(dir, def string, projects map[string]string) error {
	clean := map[string]string{}
	for k, v := range projects {
		if k != "" && v != "" {
			clean[k] = v
		}
	}
	f := evidenceRootsFile{Default: def, Projects: clean}
	b, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(dir, ".cogo"), 0o755); err != nil {
		return err
	}
	return os.WriteFile(evidenceRootsPath(dir), b, 0o644)
}
