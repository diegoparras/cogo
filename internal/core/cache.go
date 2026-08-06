package core

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"
	"sync"
)

// VaultCache is an mtime-keyed cache over a vault directory. A long-running
// server calls Load() on every request; without a cache that re-reads and
// re-parses every .md each time — O(n) disk + parse per request, which sinks at
// a few thousand notes. The cache turns each Load into an O(n) stat walk (cheap)
// plus a re-parse of only the files whose mtime or size changed.
//
// Load returns a fresh map of CLONED notes, so a caller (ResolveEvidence sets
// per-item evidence status; handlers set color/status) can mutate freely without
// corrupting the cached templates. Concurrency-safe.
//
// CLI one-shots keep calling LoadVault directly — a process that exits gains
// nothing from a cache.
type VaultCache struct {
	dir string
	mu  sync.Mutex
	fil map[string]*cachedNote // absolute path -> last parse
}

type cachedNote struct {
	mod  int64 // ModTime UnixNano
	size int64
	note *Note // pristine template; never handed out directly, only cloned
}

// NewVaultCache returns an empty cache for dir. The first Load populates it.
func NewVaultCache(dir string) *VaultCache {
	return &VaultCache{dir: dir, fil: map[string]*cachedNote{}}
}

// Load walks the vault, re-parsing only changed files, and returns the notes
// keyed by ID. Same skip rules and duplicate-ID error as LoadVault.
func (c *VaultCache) Load() (map[string]*Note, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	seen := map[string]bool{}
	vault := map[string]*Note{}
	err := filepath.WalkDir(c.dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == ".cogo" {
				return fs.SkipDir
			}
			return nil
		}
		name := strings.ToLower(d.Name())
		if !strings.HasSuffix(name, ".md") || name == "index.md" || name == "log.md" || name == "readme.md" {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		seen[path] = true
		mod, size := info.ModTime().UnixNano(), info.Size()
		ce := c.fil[path]
		if ce == nil || ce.mod != mod || ce.size != size {
			n, err := ReadNoteFile(path)
			if err != nil {
				return err
			}
			ce = &cachedNote{mod: mod, size: size, note: n}
			c.fil[path] = ce
		}
		if _, dup := vault[ce.note.ID]; dup {
			return fmt.Errorf("duplicate note id %q at %s", ce.note.ID, path)
		}
		vault[ce.note.ID] = ce.note.clone()
		return nil
	})
	if err != nil {
		return nil, err
	}
	for path := range c.fil {
		if !seen[path] {
			delete(c.fil, path) // note was deleted or trashed since last Load
		}
	}
	return vault, nil
}

// clone returns a deep-enough copy: the value fields copy by assignment, and the
// two mutated slices (Evidence — ResolveEvidence sets Status; DependsOn) get
// their own backing arrays so mutations never reach the cached template.
func (n *Note) clone() *Note {
	c := *n
	if n.Evidence != nil {
		c.Evidence = make([]Evidence, len(n.Evidence))
		copy(c.Evidence, n.Evidence)
	}
	if n.DependsOn != nil {
		c.DependsOn = append([]string(nil), n.DependsOn...)
	}
	return &c
}
