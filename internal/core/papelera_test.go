package core

import (
	"os"
	"path/filepath"
	"testing"
)

// La historia se va a la papelera con la nota y vuelve con ella. Si se
// quedara, `recall` seguiría devolviendo el claim de algo borrado — y si se
// borró por un secreto, el secreto seguiría ahí.
func TestLaPapeleraSeLlevaLaHistoria(t *testing.T) {
	dir := t.TempDir()
	n := &Note{ID: "filtrada", Type: "bug", Body: "## Claim\nla clave es hunter2"}
	if err := WriteNoteFile(filepath.Join(dir, "filtrada.md"), n); err != nil {
		t.Fatal(err)
	}
	hist := filepath.Join(dir, ".cogo", "history", "filtrada.jsonl")
	_ = os.MkdirAll(filepath.Dir(hist), 0o755)
	if err := os.WriteFile(hist, []byte(`{"time":"2026-09-08T00:00:00Z","claim":"la clave es hunter2"}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := TrashNote(dir, n); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(hist); !os.IsNotExist(err) {
		t.Fatal("la historia tiene que irse con la nota")
	}
	if err := RestoreTrash(dir, "filtrada"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(hist); err != nil {
		t.Fatal("restaurar tiene que traer la historia de vuelta")
	}

	if _, err := TrashNote(dir, n); err != nil {
		t.Fatal(err)
	}
	if err := PurgeTrash(dir, "filtrada"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".cogo", "trash", "filtrada.history.jsonl")); !os.IsNotExist(err) {
		t.Fatal("purgar tiene que borrar la historia también")
	}
}
