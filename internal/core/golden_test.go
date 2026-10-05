package core

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"testing"
)

// Banco de casos dorados: congela el veredicto del motor para cada rama de
// decisión y para las notas de demostración del repositorio.
//
// No está acá para probar que el motor es correcto — para eso están los tests de
// color_test.go. Está para que NINGÚN cambio de color pase inadvertido: si un
// refactor mueve una nota de verde a amarillo, este test falla y obliga a
// decidirlo en vez de descubrirlo en producción tres semanas después.
//
// Cuando un cambio de color sea deliberado, se regenera con:
//
//	go test ./internal/core -run Golden -update
//
// y el diff del archivo .json queda en el commit, que es exactamente el registro
// que se quiere.
var actualizar = os.Getenv("COGO_GOLDEN_UPDATE") != ""

type casoDorado struct {
	Nombre string `json:"caso"`
	Color  string `json:"color"`
	Razon  string `json:"razon"`
}

// banco arma un vault sintético que toca cada rama de compute(), en el orden en
// que las evalúa. Los nombres dicen qué rama cubre cada uno.
func banco() (map[string]*Note, Date, map[string]bool) {
	hoy := MustDate("2026-08-02")
	obs := []Evidence{{Kind: "command_output", Ref: "cmd.log:1"}}
	rep := []Evidence{{Kind: "doc", Ref: "docs/adr.md"}}
	raz := []Evidence{{Kind: "inference", Ref: "se deduce del diseño"}}
	pas := Check{Test: "go test ./...", Status: "passed"}

	n := func(id, tipo string, ev []Evidence, ch Check, verificada string, deps ...string) *Note {
		return &Note{
			ID: id, Type: tipo, Project: "banco",
			LastVerified: MustDate(verificada), Evidence: ev, Check: ch,
			DependsOn: deps, Body: "## Claim\n" + id,
		}
	}

	v := map[string]*Note{
		// --- verde: nada la empuja para abajo
		"verde-limpia": n("verde-limpia", "architecture", obs, pas, "2026-08-01"),

		// --- rojo, en el orden en que el motor decide
		"roja-contradicha":   n("roja-contradicha", "architecture", obs, pas, "2026-08-01"),
		"roja-dep-roja":      n("roja-dep-roja", "architecture", obs, pas, "2026-08-01", "roja-sin-evidencia"),
		"roja-sin-evidencia": n("roja-sin-evidencia", "architecture", nil, pas, "2026-08-01"),
		"roja-expirada":      n("roja-expirada", "command", obs, pas, "2025-01-01"), // command: ventana 30d
		"roja-dep-ausente":   n("roja-dep-ausente", "architecture", obs, pas, "2026-08-01", "no-existe"),

		// --- amarillo
		"amarilla-reportada": n("amarilla-reportada", "architecture", rep, pas, "2026-08-01"),
		"amarilla-razonada":  n("amarilla-razonada", "architecture", raz, pas, "2026-08-01"),
		"amarilla-sin-check": n("amarilla-sin-check", "architecture", obs, Check{Test: "go test"}, "2026-08-01"),
		"amarilla-vencida":   n("amarilla-vencida", "command", obs, pas, "2026-06-20"), // pasó 30d, no 60
		"amarilla-dep-amar":  n("amarilla-dep-amar", "architecture", obs, pas, "2026-08-01", "amarilla-reportada"),

		// --- sin grado
		"sin-grado-mistake": n("sin-grado-mistake", "mistake", obs, pas, "2026-08-01"),

		// --- ciclo: hoy todo el ciclo cae a rojo. Esto es lo que el retículo
		//     con punto fijo tiene que cambiar, y el diff va a quedar acá.
		"ciclo-a": n("ciclo-a", "architecture", obs, pas, "2026-08-01", "ciclo-b"),
		"ciclo-b": n("ciclo-b", "architecture", obs, pas, "2026-08-01", "ciclo-a"),

		// --- ventanas de frescura por tipo
		"tipo-constraint": n("tipo-constraint", "constraint", obs, pas, "2025-10-01"), // 365d
		"tipo-runbook":    n("tipo-runbook", "runbook", obs, pas, "2026-06-01"),       // 90d
		"tipo-bug":        n("tipo-bug", "bug", obs, pas, "2026-06-25"),               // 60d
		"tipo-command":    n("tipo-command", "command", obs, pas, "2026-07-25"),       // 30d
	}
	return v, hoy, map[string]bool{"roja-contradicha": true}
}

func TestGoldenColores(t *testing.T) {
	vault, hoy, contras := banco()
	verdicts := EvaluateVault(vault, contras, hoy)

	got := make([]casoDorado, 0, len(verdicts))
	for id, v := range verdicts {
		got = append(got, casoDorado{Nombre: id, Color: v.Color.String(), Razon: v.Reason})
	}
	sort.Slice(got, func(i, j int) bool { return got[i].Nombre < got[j].Nombre })

	path := filepath.Join("testdata", "golden-colores.json")
	if actualizar {
		_ = os.MkdirAll("testdata", 0o755)
		b, _ := json.MarshalIndent(got, "", " ")
		if err := os.WriteFile(path, append(b, '\n'), 0o644); err != nil {
			t.Fatal(err)
		}
		t.Log("banco dorado regenerado:", path)
		return
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("falta el banco dorado (%v). Generalo con COGO_GOLDEN_UPDATE=1 go test ./internal/core -run Golden", err)
	}
	var want []casoDorado
	if err := json.Unmarshal(raw, &want); err != nil {
		t.Fatal(err)
	}
	if len(want) != len(got) {
		t.Fatalf("el banco tiene %d casos y el motor produjo %d — regeneralo si el cambio es deliberado", len(want), len(got))
	}
	for i := range want {
		if want[i] != got[i] {
			t.Errorf("CAMBIO DE VEREDICTO en %q:\n  antes: %s — %s\n  ahora: %s — %s",
				want[i].Nombre, want[i].Color, want[i].Razon, got[i].Color, got[i].Razon)
		}
	}
}

// TestGoldenDemo hace lo mismo con las notas de demostración que viajan en el
// repositorio: son las que ve cualquiera que clone, así que su color es parte
// del contrato observable.
func TestGoldenDemo(t *testing.T) {
	files, _ := filepath.Glob(filepath.Join("..", "..", "vault", "fisherboy-*.md"))
	if len(files) == 0 {
		t.Skip("sin vault de demostración")
	}
	vault := map[string]*Note{}
	for _, f := range files {
		n, err := ReadNoteFile(f)
		if err != nil {
			t.Fatalf("la nota de demostración %s no parsea: %v", filepath.Base(f), err)
		}
		vault[n.ID] = n
	}
	verdicts := EvaluateVault(vault, nil, MustDate("2026-08-02"))

	got := make([]casoDorado, 0, len(verdicts))
	for id, v := range verdicts {
		got = append(got, casoDorado{Nombre: id, Color: v.Color.String(), Razon: v.Reason})
	}
	sort.Slice(got, func(i, j int) bool { return got[i].Nombre < got[j].Nombre })

	path := filepath.Join("testdata", "golden-demo.json")
	if actualizar {
		_ = os.MkdirAll("testdata", 0o755)
		b, _ := json.MarshalIndent(got, "", " ")
		if err := os.WriteFile(path, append(b, '\n'), 0o644); err != nil {
			t.Fatal(err)
		}
		t.Log("banco dorado de demostración regenerado:", path)
		return
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("falta %s; generalo con COGO_GOLDEN_UPDATE=1", path)
	}
	var want []casoDorado
	_ = json.Unmarshal(raw, &want)
	if len(want) != len(got) {
		t.Fatalf("el banco tiene %d notas de demostración y hay %d", len(want), len(got))
	}
	for i := range want {
		if want[i] != got[i] {
			t.Errorf("CAMBIO DE VEREDICTO en la nota de demostración %q:\n  antes: %s — %s\n  ahora: %s — %s",
				want[i].Nombre, want[i].Color, want[i].Razon, got[i].Color, got[i].Razon)
		}
	}
}
