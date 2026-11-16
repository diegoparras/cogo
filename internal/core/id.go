package core

import (
	"fmt"
	"path/filepath"
	"strings"
)

// DeriveID builds a stable id from a project and the first claim line of a body.
// Shared by every capture path (web form, MCP) so ids are consistent.
func DeriveID(project, body string) string {
	claim := ""
	for _, ln := range strings.Split(body, "\n") {
		t := strings.TrimSpace(ln)
		if t == "" || strings.HasPrefix(t, "#") {
			continue
		}
		claim = t
		break
	}
	slug := Slugify(claim)
	if project != "" {
		return Slugify(project) + "-" + slug
	}
	return slug
}

// Slugify turns text into a lowercase a-z0-9 dash slug, capped at 48 runes.
func Slugify(s string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(s) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			dash = false
		case b.Len() > 0 && !dash:
			b.WriteByte('-')
			dash = true
		}
		if b.Len() >= 48 {
			break
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "note"
	}
	return out
}

// ValidarID acepta lo que puede ser el nombre de un archivo dentro del vault y
// nada más. Un id lo manda un agente, y `filepath.Join(dir, id+".md")`
// normaliza `..`: sin esto, `../../x` escribía fuera del vault.
//
// Solo se aplica al escribir. Lo que ya está en disco se lee como esté.
func ValidarID(id string) error {
	if id == "" {
		return fmt.Errorf("el id no puede estar vacío")
	}
	if len(id) > 120 {
		return fmt.Errorf("el id es demasiado largo (%d caracteres; el máximo es 120)", len(id))
	}
	if strings.Contains(id, "..") {
		return fmt.Errorf("el id %q no puede contener \"..\"", id)
	}
	for i, r := range id {
		ok := r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' ||
			(i > 0 && (r == '-' || r == '_' || r == '.'))
		if !ok {
			return fmt.Errorf("el id %q solo puede tener letras, números, guiones, guiones bajos y puntos, y empezar con letra o número", id)
		}
	}
	return nil
}

// RutaDeNota es el único lugar donde un id se convierte en un path. Valida y
// además comprueba que el resultado siga adentro del vault: cinturón y
// tiradores, porque el costo de equivocarse acá es escribir donde no se debe.
func RutaDeNota(dir, id string) (string, error) {
	if err := ValidarID(id); err != nil {
		return "", err
	}
	base := filepath.Clean(dir)
	path := filepath.Join(base, id+".md")
	if filepath.Dir(path) != base {
		return "", fmt.Errorf("el id %q sale del vault", id)
	}
	return path, nil
}
