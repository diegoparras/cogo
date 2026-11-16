package purga

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/diegoparras/cogo/internal/core"
	"github.com/diegoparras/cogo/internal/journal"
	"github.com/diegoparras/cogo/internal/runner"
)

const nota = "---\nid: pool\ntype: architecture\nproject: p\nlast_verified: 2026-09-01\nevidence:\n  - kind: file_read\n    ref: compose.yml:1\ncheck:\n  test: psql -c select 1\n  status: passed\n---\n## Claim\nEl pool tiene 20 conexiones.\n"

func TestPurgarBorraPapeleraYRegistroPeroNoUnaNotaViva(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "pool.md"), []byte(nota), 0o644); err != nil {
		t.Fatal(err)
	}
	j, err := journal.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	// La salida la escribe el runner de verdad: fuera del runner nadie produce
	// ejecuciones (internal/runner/i1_test.go). `go version` existe donde
	// corren estos tests.
	_ = os.MkdirAll(filepath.Join(dir, ".cogo"), 0o755)
	cfg := "enabled: true\nchecks:\n  - id: go-version\n    command: [\"go\", \"version\"]\n    workdir: " + filepath.ToSlash(dir) + "\n    timeout: 1m\n"
	if err := os.WriteFile(filepath.Join(dir, ".cogo", "runner.yaml"), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	c, err := runner.Cargar(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runner.Verificar(context.Background(), j, c, "pool", "go-version"); err != nil {
		t.Fatal(err)
	}

	// Viva: no.
	if _, err := Purgar(context.Background(), dir, j, "pool", "diego", ""); err == nil || !strings.Contains(err.Error(), "viva") {
		t.Fatalf("una nota viva no se purga: %v", err)
	}
	// A la papelera, y ahora sí.
	viva, err := core.ReadNoteFile(filepath.Join(dir, "pool.md"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := core.TrashNote(dir, viva); err != nil {
		t.Fatal(err)
	}
	res, err := Purgar(context.Background(), dir, j, "pool", "diego", "clave en stdout")
	if err != nil {
		t.Fatal(err)
	}
	// El runner deja dos eventos: VerificationStarted, sin salida, y
	// CheckExecuted, con la salida. Solo el segundo se reescribe.
	if !res.ArchivoBorrado || len(res.EventosReescritos) != 1 || res.EventosReescritos[0] != 2 || !res.CadenaRota || res.EventoPurged != 3 || res.CabezaAnterior == "" {
		t.Fatalf("resultado: %+v", res)
	}
	if len(core.ListTrash(dir)) != 0 {
		t.Fatal("la papelera tiene que quedar vacía")
	}
	evs, _ := j.All()
	for _, e := range evs {
		if strings.Contains(string(e.Payload), "go version go") {
			t.Fatalf("la salida del runner sigue en el registro: %s", e.Payload)
		}
	}
	if err := j.Verificar(); err == nil || !strings.Contains(err.Error(), "purgó") {
		t.Fatalf("la cadena queda rota y explicada: %v", err)
	}
	// Un id que no es un id, tampoco.
	if _, err := Purgar(context.Background(), dir, j, "../x", "diego", ""); err == nil {
		t.Fatal("id inválido")
	}
}
