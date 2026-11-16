package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadVault(t *testing.T) {
	dir := t.TempDir()
	a := &Note{ID: "a", Type: "bug", LastVerified: MustDate("2026-06-20"), Evidence: obs("x.go:1"), Check: Check{Status: "passed"}, Body: "## Claim\nA."}
	b := &Note{ID: "b", Type: "command", LastVerified: MustDate("2026-06-20"), Evidence: []Evidence{{Kind: "doc", Ref: "d"}}, Body: "## Claim\nB."}
	if err := WriteNoteFile(filepath.Join(dir, "a.md"), a); err != nil {
		t.Fatal(err)
	}
	if err := WriteNoteFile(filepath.Join(dir, "b.md"), b); err != nil {
		t.Fatal(err)
	}
	// A catalog file must be ignored, not parsed as a note.
	if err := os.WriteFile(filepath.Join(dir, "index.md"), []byte("just a catalog\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	vault, err := LoadVault(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(vault) != 2 {
		t.Fatalf("want 2 notes, got %d", len(vault))
	}
	if _, ok := vault["a"]; !ok {
		t.Error("note a not loaded")
	}
}

// Un id repetido ya no tira abajo el vault entero: se queda la primera, la
// otra se saltea, y el problema se reporta para que alguien lo arregle.
func TestLoadVaultReportaElIDDuplicado(t *testing.T) {
	dir := t.TempDir()
	n := &Note{ID: "dup", Type: "bug", LastVerified: MustDate("2026-06-20"), Evidence: obs("x"), Check: Check{Status: "passed"}, Body: "x"}
	_ = WriteNoteFile(filepath.Join(dir, "one.md"), n)
	_ = WriteNoteFile(filepath.Join(dir, "two.md"), n)
	v, problemas, err := LoadVaultConProblemas(dir)
	if err != nil {
		t.Fatalf("no puede ser fatal: %v", err)
	}
	if len(v) != 1 || len(problemas) != 1 || !strings.Contains(problemas[0].Motivo, "repite el id") {
		t.Fatalf("una nota, un problema que lo nombre: %d notas, %v", len(v), problemas)
	}
}
