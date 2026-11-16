package motor

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/diegoparras/cogo/internal/core"
	"github.com/diegoparras/cogo/internal/journal"
)

// evaluarConCadena arma la cadena completa sobre un vault: siembra el journal,
// pliega y evalúa. Es lo que hace el servidor al arrancar.
func evaluarConCadena(t *testing.T, vault map[string]*core.Note, hoy core.Date) map[string]core.Verdict {
	t.Helper()
	previos := core.EvaluateVault(vault, nil, hoy)
	for id, n := range vault {
		n.StaleAt = previos[id].StaleAt
	}
	j, err := journal.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := journal.Sembrar(j, vault, previos); err != nil {
		t.Fatal(err)
	}
	evs, _ := j.All()
	return Evaluar(vault, nil, hoy, evs)
}

// Las notas informativas quedan fuera del semáforo, igual que antes. Esto lo
// encontró la comparación contra un vault real: la máquina de estados no tiene
// el concepto de "no graduable" y las mandaba a rojo.
func TestLasNotasInformativasNoSeGraduan(t *testing.T) {
	hoy := core.MustDate("2026-08-03")
	vault := map[string]*core.Note{
		"error-aprendido": {
			ID: "error-aprendido", Type: "mistake", LastVerified: hoy,
			Body: "## Claim\nMe olvidé de mirar los logs antes de reiniciar.",
		},
		"apoyada": {
			ID: "apoyada", Type: "architecture", LastVerified: hoy,
			Evidence:  []core.Evidence{{Kind: "command_output", Ref: "x.log:1"}},
			Check:     core.Check{Test: "t", Status: "passed"},
			DependsOn: []string{"error-aprendido"},
			Body:      "## Claim\nalgo",
		},
	}
	got := evaluarConCadena(t, vault, hoy)

	if got["error-aprendido"].Color != core.Ungraded {
		t.Errorf("la nota informativa quedó en %s; no debería graduarse", got["error-aprendido"].Color)
	}
	// Y no arrastra: depender de algo que no se gradúa no puede degradarte.
	if got["apoyada"].Color != core.Green {
		t.Errorf("depender de una nota informativa degradó a %s: %s",
			got["apoyada"].Color, got["apoyada"].Reason)
	}
}

// El veredicto tiene que explicar POR QUÉ, no solo qué color. Cuando una nota
// cae por una dependencia, decirlo es la diferencia entre poder arreglarlo y
// mirar un rojo sin saber dónde.
func TestElVeredictoExplicaLaCaidaPorDependencia(t *testing.T) {
	hoy := core.MustDate("2026-08-03")
	obs := []core.Evidence{{Kind: "command_output", Ref: "x.log:1"}}
	vault := map[string]*core.Note{
		"buena": {ID: "buena", Type: "architecture", LastVerified: hoy, Evidence: obs,
			Check: core.Check{Test: "t", Status: "passed"}, DependsOn: []string{"floja"}, Body: "## Claim\na"},
		"floja": {ID: "floja", Type: "architecture", LastVerified: hoy,
			Check: core.Check{Test: "t", Status: "passed"}, Body: "## Claim\nb"},
	}
	got := evaluarConCadena(t, vault, hoy)
	if got["buena"].Color == core.Green {
		t.Fatal("una nota que depende de otra sin evidencia quedó verde")
	}
	if !contiene(got["buena"].Reason, "depende") {
		t.Errorf("la razón no menciona la dependencia: %q", got["buena"].Reason)
	}
}

func contiene(s, sub string) bool { return strings.Contains(s, sub) }

// El corte, medido sobre un vault real. Se saltea sin VAULT=; con él, imprime
// exactamente qué notas cambian de color y falla si el total supera lo que se
// declaró aceptable.
func TestCorteSobreVaultReal(t *testing.T) {
	dir := os.Getenv("VAULT")
	if dir == "" {
		t.Skip("sin VAULT= no hay corte que medir")
	}
	vault := map[string]*core.Note{}
	files, _ := filepath.Glob(filepath.Join(dir, "*.md"))
	for _, f := range files {
		if n, err := core.ReadNoteFile(f); err == nil {
			vault[n.ID] = n
		}
	}
	core.ResolveEvidence(vault, core.EvidenceRoots{})
	hoy := core.MustDate("2026-08-03")

	viejo := core.EvaluateVault(vault, nil, hoy)
	nuevo := evaluarConCadena(t, vault, hoy)

	ids := make([]string, 0, len(vault))
	for id := range vault {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	cambios := 0
	for _, id := range ids {
		if viejo[id].Color == nuevo[id].Color {
			continue
		}
		cambios++
		t.Logf("  %-46s %s -> %s", id, viejo[id].Color, nuevo[id].Color)
		t.Logf("      antes: %s", viejo[id].Reason)
		t.Logf("      ahora: %s", nuevo[id].Reason)
	}
	t.Logf("cambian %d de %d notas", cambios, len(ids))
	if cambios > 0 {
		t.Errorf("el corte cambiaría el color de %d notas — revisá cada una antes de cortar", cambios)
	}
}

// La deriva material tiene que bajar el color también en el motor nuevo. Antes
// de la Fase 6 no lo hacía: la máquina de estados mira el check y el techo mira
// el TIER de la evidencia, y ninguno de los dos preguntaba si el archivo citado
// seguía diciendo lo mismo. Una nota podía quedar verde apoyada en un archivo
// que ya no la respaldaba.
func TestLaDerivaMaterialBajaElColor(t *testing.T) {
	hoy := core.MustDate("2026-08-03")
	mk := func(id, estadoEv string) *core.Note {
		return &core.Note{
			ID: id, Type: "architecture", LastVerified: hoy,
			Evidence: []core.Evidence{{Kind: "command_output", Ref: "x.log:1", Status: estadoEv}},
			Check:    core.Check{Test: "t", Status: "passed"},
			Body:     "## Claim\n" + id,
		}
	}
	vault := map[string]*core.Note{
		"intacta":  mk("intacta", core.EvResolved),
		"movida":   mk("movida", core.EvMoved),
		"derivada": mk("derivada", core.EvDrifted),
	}
	got := evaluarConCadena(t, vault, hoy)

	if got["intacta"].Color != core.Green {
		t.Errorf("la nota intacta quedó en %s: %s", got["intacta"].Color, got["intacta"].Reason)
	}
	// Lo que la Fase 6 vino a arreglar del otro lado: que el archivo cambie lejos
	// de la cita no puede costar nada.
	if got["movida"].Color != core.Green {
		t.Errorf("un cambio ajeno a la cita bajó la nota a %s: %s", got["movida"].Color, got["movida"].Reason)
	}
	if got["derivada"].Color != core.Yellow {
		t.Errorf("la evidencia derivó y la nota quedó en %s: %s", got["derivada"].Color, got["derivada"].Reason)
	}
}
