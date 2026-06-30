package core

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"
)

// LoadVault reads every .md note under dir (recursively), parses it, and keys
// it by ID. index.md, log.md and readme.md are catalog/log files, not notes,
// and are skipped. A duplicate ID is an error: the ID is the stable identity,
// so two notes can't share one.
func LoadVault(dir string) (map[string]*Note, error) {
	vault := map[string]*Note{}
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		name := strings.ToLower(d.Name())
		if !strings.HasSuffix(name, ".md") || name == "index.md" || name == "log.md" || name == "readme.md" {
			return nil
		}
		n, err := ReadNoteFile(path)
		if err != nil {
			return err
		}
		if _, dup := vault[n.ID]; dup {
			return fmt.Errorf("duplicate note id %q at %s", n.ID, path)
		}
		vault[n.ID] = n
		return nil
	})
	if err != nil {
		return nil, err
	}
	return vault, nil
}
