package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/diegoparras/cogo/internal/core"
)

// La memoria que Claude Code ya tiene entra con color: amarilla, anclada a su
// archivo, con la vigencia desde que se escribió. Una memoria de proyecto de
// hace casi dos años vence sola; una que trae un token no entra; y si Claude
// reescribe una memoria después, la nota se entera.
func TestImportarMemoriaDeClaudeCode(t *testing.T) {
	soltarRegistro()
	t.Cleanup(soltarRegistro)
	repo, mem, vault := t.TempDir(), t.TempDir(), t.TempDir()
	escribirMem := func(nombre, contenido string) {
		if err := os.WriteFile(filepath.Join(mem, nombre), []byte(contenido), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	escribirMem("MEMORY.md", "- [Sin multiagente](sin-multiagente.md)\n")
	escribirMem("sin-multiagente.md", "---\nname: sin-multiagente\ndescription: Nunca lanzar agentes en paralelo sin aprobación explícita con costo estimado.\nmetadata:\n  type: feedback\nmodified: "+today().String()+"\n---\n\nUn error de escala más y se pierde el cliente.\n")
	escribirMem("migracion-vieja.md", "---\nname: migracion-vieja\ndescription: La migración a Postgres 15 se termina este mes y no hay que tocar el esquema.\ntype: project\nmodified: 2025-01-10\n---\n\nPlan de la migración.\n")
	escribirMem("con-token.md", "---\nname: con-token\ndescription: Para el deploy usar este token de GitHub con permisos de paquetes.\ntype: reference\n---\n\nghp_A1b2C3d4E5f6G7h8I9j0K1l2M3n4O5p6Q7r8\n")

	if err := cmdImportar([]string{"-vault", vault, "-project", "suite", "-memoria", "claude", "-claude-dir", mem, repo}); err != nil {
		t.Fatalf("importar: %v", err)
	}
	v, err := core.LoadVault(vault)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := v[core.DeriveID("suite", "con-token")]; ok || len(v) != 2 {
		t.Fatalf("la memoria con un token no entra, las otras dos sí: %d notas", len(v))
	}
	roots := core.LoadEvidenceRoots(vault)
	core.ResolveEvidence(v, roots)
	fb := v[core.DeriveID("suite", "sin-multiagente")]
	if fb == nil || fb.Evidence[0].Status != core.EvResolved || fb.Evidence[0].Anchor == "" {
		t.Fatalf("la memoria queda anclada a su archivo, aunque viva fuera del repo: %+v", fb)
	}
	if ev := core.Evaluate(fb, v, nil, today()); ev.Color != core.Yellow {
		t.Fatalf("una memoria nace amarilla, dio %s: %s", ev.Color, ev.Reason)
	}
	vieja := v[core.DeriveID("suite", "migracion-vieja")]
	if ev := core.Evaluate(vieja, v, nil, today()); ev.Color != core.Red {
		t.Fatalf("una memoria de proyecto de hace casi dos años vence, dio %s: %s", ev.Color, ev.Reason)
	}
	// Claude reescribe la memoria: la nota deriva.
	escribirMem("sin-multiagente.md", "---\nname: sin-multiagente\ndescription: Ya se puede lanzar agentes sin preguntar.\nmetadata:\n  type: feedback\n---\n\nCambió.\n")
	v, _ = core.LoadVault(vault)
	core.ResolveEvidence(v, roots)
	if st := v[core.DeriveID("suite", "sin-multiagente")].Evidence[0].Status; st != core.EvDrifted {
		t.Fatalf("una memoria reescrita tiene que derivar: %s", st)
	}
	if err := cmdImportar([]string{"-vault", vault, "-project", "suite", "-memoria", "otra", repo}); err == nil {
		t.Fatal("-memoria solo conoce claude y codex")
	}
}

// La de Codex entra igual, desde su home.
func TestImportarMemoriaDeCodex(t *testing.T) {
	soltarRegistro()
	t.Cleanup(soltarRegistro)
	repo, home, vault := t.TempDir(), t.TempDir(), t.TempDir()
	_ = os.MkdirAll(filepath.Join(home, "memories"), 0o755)
	if err := os.WriteFile(filepath.Join(home, "memories", "MEMORY.md"), []byte("## Tests en Docker\n\nLos tests de Go corren en un contenedor golang porque el antivirus pone en cuarentena los binarios.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := cmdImportar([]string{"-vault", vault, "-project", "suite", "-memoria", "codex", "-codex-home", home, repo}); err != nil {
		t.Fatalf("importar: %v", err)
	}
	if v, _ := core.LoadVault(vault); len(v) != 1 {
		t.Fatalf("una sección, una nota: %d", len(v))
	}
}

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
