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
	v, _, err := LoadVaultConProblemas(dir)
	return v, err
}

// LoadVaultConProblemas es LoadVault diciendo además qué archivos quedaron
// afuera. Un .md ilegible o un id repetido se saltea: el resto del vault sigue
// sirviendo.
func LoadVaultConProblemas(dir string) (map[string]*Note, []Problema, error) {
	vault := map[string]*Note{}
	var problemas []Problema
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == ".cogo" {
				return fs.SkipDir // config, settings and the delete trash live here — never notes
			}
			return nil
		}
		name := strings.ToLower(d.Name())
		if !strings.HasSuffix(name, ".md") || name == "index.md" || name == "log.md" || name == "readme.md" {
			return nil
		}
		n, err := ReadNoteFile(path)
		if err != nil {
			problemas = append(problemas, Problema{Path: path, Motivo: motivoSinRuta(err, path)})
			return nil
		}
		if _, dup := vault[n.ID]; dup {
			problemas = append(problemas, Problema{Path: path,
				Motivo: fmt.Sprintf("repite el id %q de otra nota; se ignora esta", n.ID)})
			return nil
		}
		vault[n.ID] = n
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	return vault, problemas, nil
}
