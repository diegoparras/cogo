package core

import (
	"strings"
	"testing"
)

// Un solo .md inválido dejaba TODO el vault fuera de servicio: visor y los 16
// tools. Ahora se saltea, y queda anotado para que alguien lo arregle.
func TestUnArchivoRotoNoTiraElVault(t *testing.T) {
	dir := t.TempDir()
	writeNote(t, dir, "a.md", noteA)
	writeNote(t, dir, "rota.md", "---\nid: rota\ntype: [esto no cierra\n---\n")
	writeNote(t, dir, "a-otra-vez.md", noteA) // repite el id "a"

	c := NewVaultCache(dir)
	v, err := c.Load()
	if err != nil {
		t.Fatalf("un archivo roto no puede ser fatal: %v", err)
	}
	if _, ok := v["a"]; !ok || len(v) != 1 {
		t.Fatalf("la nota sana tiene que estar, y solo ella: %v", v)
	}
	ps := c.Problemas()
	if len(ps) != 2 {
		t.Fatalf("esperaba 2 problemas (ilegible + duplicada), hay %d: %v", len(ps), ps)
	}
	var vio, dup bool
	for _, p := range ps {
		if strings.HasSuffix(p.Path, "rota.md") {
			vio = true
		}
		if strings.Contains(p.Motivo, "repite el id") {
			dup = true
		}
	}
	if !vio || !dup {
		t.Fatalf("faltan problemas: %v", ps)
	}

	// Arreglada, deja de ser un problema — y no queda un registro viejo colgado.
	writeNote(t, dir, "rota.md", "---\nid: rota\ntype: bug\n---\nya está\n")
	if _, err := c.Load(); err != nil {
		t.Fatal(err)
	}
	if ps := c.Problemas(); len(ps) != 1 {
		t.Fatalf("arreglada la ilegible tendría que quedar solo la duplicada: %v", ps)
	}

	// Y el camino sin caché tolera igual.
	v2, ps2, err := LoadVaultConProblemas(dir)
	if err != nil || len(v2) != 2 || len(ps2) != 1 {
		t.Fatalf("LoadVaultConProblemas: %v %d notas %d problemas", err, len(v2), len(ps2))
	}
}
