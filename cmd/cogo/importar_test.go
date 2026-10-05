package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/diegoparras/cogo/internal/core"
)

// El arranque en frío de punta a punta: un repo con docs, un vault vacío, y
// después notas amarillas ancladas que detectan la edición del documento.
func TestImportarArranqueEnFrio(t *testing.T) {
	soltarRegistro()
	t.Cleanup(soltarRegistro)
	repo := t.TempDir()
	_ = os.MkdirAll(filepath.Join(repo, "docs"), 0o755)
	if err := os.WriteFile(filepath.Join(repo, "CLAUDE.md"), []byte("# Proyecto\n\n## Nunca se toca la tabla usuarios sin backup\n\nEs obligatorio correr el backup diario antes de tocar usuarios. Nunca un drop directo.\n\n## Cómo se despliega\n\nLos pasos: build, push y force rebuild en el panel. Tarda tres minutos.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	vault := t.TempDir()
	if err := cmdImportar([]string{"-vault", vault, "-project", "tienda", repo}); err != nil {
		t.Fatalf("importar: %v", err)
	}
	v, err := core.LoadVault(vault)
	if err != nil {
		t.Fatal(err)
	}
	if len(v) != 2 {
		t.Fatalf("esperaba 2 notas, hay %d", len(v))
	}
	roots := core.LoadEvidenceRoots(vault)
	if roots.Root("tienda") != repo {
		t.Fatalf("la raíz de evidencia del proyecto tiene que quedar fijada: %q", roots.Root("tienda"))
	}
	core.ResolveEvidence(v, roots)
	for id, n := range v {
		if n.Origin != "instrument" || n.Evidence[0].Anchor == "" || n.Evidence[0].Status != core.EvResolved {
			t.Fatalf("%s: %+v", id, n.Evidence[0])
		}
		if ev := core.Evaluate(n, v, nil, today()); ev.Color != core.Yellow {
			t.Fatalf("%s nace amarilla, dio %s: %s (last_verified=%v stale_at=%v)", id, ev.Color, ev.Reason, n.LastVerified, n.StaleAt)
		}
	}
	// Dos veces: nada nuevo.
	if err := cmdImportar([]string{"-vault", vault, "-project", "tienda", repo}); err != nil {
		t.Fatal(err)
	}
	if v2, _ := core.LoadVault(vault); len(v2) != 2 {
		t.Fatalf("importar dos veces no duplica: %d", len(v2))
	}
	// Editar la sección del despliegue: solo esa nota deriva.
	b, _ := os.ReadFile(filepath.Join(repo, "CLAUDE.md"))
	_ = os.WriteFile(filepath.Join(repo, "CLAUDE.md"), []byte(strings.Replace(string(b), "Tarda tres minutos.", "Tarda diez minutos.", 1)), 0o644)
	v, _ = core.LoadVault(vault)
	core.ResolveEvidence(v, roots)
	desp := core.DeriveID("tienda", "Cómo se despliega")
	back := core.DeriveID("tienda", "Nunca se toca la tabla usuarios sin backup")
	if v[desp].Evidence[0].Status != core.EvDrifted || v[back].Evidence[0].Status == core.EvDrifted {
		t.Fatalf("deriva: despliega=%s backup=%s", v[desp].Evidence[0].Status, v[back].Evidence[0].Status)
	}
}
