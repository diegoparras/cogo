package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/diegoparras/cogo/internal/core"
)

// registroEntero junta todos los archivos del registro como texto, para buscar
// lo que quedó escrito en disco y no lo que el caché recuerda.
func registroEntero(t *testing.T, dir string) string {
	t.Helper()
	archivos, _ := filepath.Glob(filepath.Join(dir, ".cogo", "journal", "*.jsonl"))
	var b strings.Builder
	for _, f := range archivos {
		x, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		b.Write(x)
	}
	return b.String()
}

// `cogo purgar` borra de la papelera y limpia el registro; una nota viva no.
// La salida que se purga la escribe el runner de verdad: fuera del runner nadie
// produce ejecuciones (ver internal/runner/i1_test.go).
func TestPurgarDesdeLaCLI(t *testing.T) {
	soltarRegistro()
	t.Cleanup(soltarRegistro)
	dir := vaultConRunner(t)
	if err := cmdRun([]string{"-vault", dir, "go-anda", "go-version"}); err != nil {
		t.Fatalf("cogo run: %v", err)
	}
	if !strings.Contains(registroEntero(t, dir), "go version go") {
		t.Fatal("precondición: el runner tiene que haber dejado su salida en el registro")
	}
	if err := cmdPurgar([]string{"-vault", dir, "go-anda"}); err == nil || !strings.Contains(err.Error(), "viva") {
		t.Fatalf("viva no se purga: %v", err)
	}
	n, err := core.ReadNoteFile(filepath.Join(dir, "go-anda.md"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := core.TrashNote(dir, n); err != nil {
		t.Fatal(err)
	}
	if err := cmdPurgar([]string{"-vault", dir, "go-anda", "-motivo", "salida sensible"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".cogo", "trash", "go-anda.md")); !os.IsNotExist(err) {
		t.Fatal("tiene que desaparecer de la papelera")
	}
	reg := registroEntero(t, dir)
	if strings.Contains(reg, "go version go") || !strings.Contains(reg, `"kind":"Purged"`) || !strings.Contains(reg, `"purgado":true`) {
		t.Fatalf("el registro tiene que quedar sin la salida del runner y con el Purged:\n%s", reg)
	}
	if err := cmdPurgar([]string{"-vault", dir}); err == nil {
		t.Fatal("sin id no hay purga")
	}
}
