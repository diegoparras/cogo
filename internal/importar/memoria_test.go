package importar

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/diegoparras/cogo/internal/core"
)

// Claude Code guarda la memoria de un proyecto en una carpeta cuyo nombre sale
// de la ruta del repo, con todo lo que no es letra o número cambiado por "-".
func TestCarpetaDeProyectoDeClaudeCode(t *testing.T) {
	casos := map[string]string{
		`E:\Claude\COGO`:            "E--Claude-COGO",
		`E:\Claude\Escriba-Suite\x`: "E--Claude-Escriba-Suite-x",
		"/home/d/mi_repo.v2":        "-home-d-mi-repo-v2",
	}
	for ruta, quiero := range casos {
		if got := CarpetaClaude(ruta); got != quiero {
			t.Errorf("%s: esperaba %s, dio %s", ruta, quiero, got)
		}
	}
}

func escribir(t *testing.T, ruta, contenido string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(ruta), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ruta, []byte(contenido), 0o644); err != nil {
		t.Fatal(err)
	}
}

// Cada archivo de la memoria de Claude Code es una nota: el description es la
// afirmación, el tipo de memoria decide el tipo de nota, y la fecha en que se
// escribió es desde cuándo corre su vigencia. El índice MEMORY.md no es una
// memoria: solo apunta a las otras.
func TestMemoriaDeClaudeCode(t *testing.T) {
	dir := t.TempDir()
	escribir(t, filepath.Join(dir, "MEMORY.md"), "# Memoria\n\n- [Sin multiagente](sin-multiagente.md) — no lanzar agentes sin aprobación\n")
	escribir(t, filepath.Join(dir, "sin-multiagente.md"), "---\nname: sin-multiagente\ndescription: \"Nunca lanzar agentes en paralelo sin aprobación explícita con costo estimado.\"\nmetadata:\n  type: feedback\nmodified: 2026-09-08T10:00:00Z\n---\n\nLe gastó el 75% del cupo con 220 agentes.\n\n**Why:** costo.\n")
	escribir(t, filepath.Join(dir, "entrega-tp.md"), "---\nname: entrega-tp\ndescription: El TP de Métodos Estocásticos se entrega el 16/11/2026.\ntype: project\n---\n\nDra. Vietri, entrega individual.\n")
	escribir(t, filepath.Join(dir, "suelta.md"), "Una nota sin frontmatter que dice algo útil sobre cómo se despliega el sistema en EasyPanel.\n")

	hoy, _ := core.ParseDate("2026-11-15")
	notas, r, err := MemoriaClaude(dir, Opciones{Proyecto: "suite", Hoy: hoy})
	if err != nil {
		t.Fatal(err)
	}
	if len(notas) != 3 || r.Archivos != 3 {
		t.Fatalf("tres memorias, sin el índice: %d notas de %d archivos", len(notas), r.Archivos)
	}
	porID := map[string]*core.Note{}
	for _, n := range notas {
		porID[n.ID] = n
	}
	fb := porID[core.DeriveID("suite", "sin-multiagente")]
	if fb == nil {
		t.Fatalf("falta la memoria de feedback: %v", r.Creadas)
	}
	if fb.Type != "constraint" || !strings.Contains(fb.Body, "Nunca lanzar agentes en paralelo") || fb.Origin != string(core.OrigenInstrumento) {
		t.Errorf("feedback: %+v", fb)
	}
	if fb.LastVerified.String() != "2026-09-08" {
		t.Errorf("la vigencia corre desde que se escribió, no desde hoy: %s", fb.LastVerified)
	}
	ref := fb.Evidence[0].Ref
	if fb.Evidence[0].Kind != "file_read" || !strings.Contains(ref, "sin-multiagente.md:1-") || !strings.Contains(ref, "memoria de Claude Code") {
		t.Errorf("la cita ancla al archivo de la memoria: %q", ref)
	}
	if p := porID[core.DeriveID("suite", "entrega-tp")]; p == nil || p.Type != "decision" {
		t.Errorf("una memoria de proyecto es una decisión: %+v", p)
	}
	if s := porID[core.DeriveID("suite", "suelta")]; s == nil {
		t.Errorf("sin frontmatter, el nombre del archivo es el título: %v", r.Creadas)
	}

	// Lo que ya está en el vault no se pisa.
	ya := func(id string) bool { return id == fb.ID }
	notas, r, _ = MemoriaClaude(dir, Opciones{Proyecto: "suite", Hoy: hoy, Existe: ya})
	if len(notas) != 2 || r.Saltadas["ya existía"] != 1 {
		t.Fatalf("reimportar no pisa: %d notas, saltadas %v", len(notas), r.Saltadas)
	}
}

// La memoria de Codex es Markdown sin estructura documentada: se secciona como
// un documento. Se leen el resumen y el archivo consolidado; lo crudo y los
// resúmenes por sesión son pasos intermedios y no entran.
func TestMemoriaDeCodex(t *testing.T) {
	home := t.TempDir()
	m := filepath.Join(home, "memories")
	escribir(t, filepath.Join(m, "memory_summary.md"), "# Resumen\n\n## Tests en Docker\n\nLos tests de Go corren en un contenedor golang porque el antivirus pone en cuarentena los binarios de test.\n")
	escribir(t, filepath.Join(m, "MEMORY.md"), "# Memoria\n\n## Deploy en EasyPanel\n\nCOGO se despliega en EasyPanel como App, con un volumen en /vault y el token por variable de entorno.\n")
	escribir(t, filepath.Join(m, "raw_memories.md"), "## Crudo\n\nEsto es extracción previa a consolidar y no tiene que entrar al vault nunca.\n")
	escribir(t, filepath.Join(m, "rollout_summaries", "s1.md"), "## Sesión\n\nResumen de una sesión puntual que alimenta la consolidación y tampoco entra.\n")

	hoy, _ := core.ParseDate("2026-11-15")
	notas, _, err := MemoriaCodex(home, Opciones{Proyecto: "suite", Hoy: hoy})
	if err != nil {
		t.Fatal(err)
	}
	var titulos []string
	for _, n := range notas {
		titulos = append(titulos, n.ID)
		if !strings.Contains(n.Evidence[0].Ref, "memoria de Codex") {
			t.Errorf("la cita dice de dónde salió: %q", n.Evidence[0].Ref)
		}
	}
	if len(notas) != 2 {
		t.Fatalf("solo el resumen y el consolidado: %v", titulos)
	}
	// Codex arranca en memories_v2 si memories no existe.
	home2 := t.TempDir()
	escribir(t, filepath.Join(home2, "memories_v2", "MEMORY.md"), "## Una\n\nUna memoria consolidada en la carpeta nueva de Codex, con texto suficiente.\n")
	if notas, _, _ := MemoriaCodex(home2, Opciones{Proyecto: "suite", Hoy: hoy}); len(notas) != 1 {
		t.Fatalf("memories_v2: %d", len(notas))
	}
	if _, _, err := MemoriaCodex(t.TempDir(), Opciones{Proyecto: "suite", Hoy: hoy}); err == nil {
		t.Fatal("sin carpeta de memorias tiene que decirlo")
	}
}
