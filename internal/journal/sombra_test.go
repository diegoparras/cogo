package journal

import (
	"testing"
	"time"

	"github.com/diegoparras/cogo/internal/confidence"
	"github.com/diegoparras/cogo/internal/core"
)

func nota(id, status, attested string, ev []core.Evidence) *core.Note {
	return &core.Note{
		ID: id, Type: "architecture", LastVerified: core.MustDate("2026-08-01"),
		Evidence: ev,
		Check:    core.Check{Test: "go test ./...", Status: status, Attested: attested, AttestedBy: "token:x"},
		Body:     "## Claim\n" + id,
	}
}

// La siembra tiene que dejar al journal contando la misma historia que el motor
// vigente: si no, toda nota existente divergiría y la comparación no diría nada.
func TestLaSiembraCoincideConElMotorVigente(t *testing.T) {
	obs := []core.Evidence{{Kind: "command_output", Ref: "x.log:1"}}
	vault := map[string]*core.Note{
		"declarada": nota("declarada", "passed", core.AttestDeclared, obs),
		"ejecutada": nota("ejecutada", "passed", core.AttestExecuted, obs),
		"sin-check": nota("sin-check", "not_run", "", obs),
		"fallada":   nota("fallada", "failed", "", obs),
		"sin-ev":    nota("sin-ev", "passed", core.AttestDeclared, nil),
	}
	hoy := core.MustDate("2026-08-02")
	verdicts := core.EvaluateVault(vault, nil, hoy)

	dir := t.TempDir()
	j, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	n, err := Sembrar(j, vault, verdicts)
	if err != nil {
		t.Fatal(err)
	}
	if n != len(vault) {
		t.Fatalf("se sembraron %d notas de %d", n, len(vault))
	}

	evs, _ := j.All()
	crudos := Fold(evs, time.Time{}, time.Time{})

	// El estado que hay que comparar es el EFECTIVO: el eje del check combinado
	// con el techo que impone la evidencia. Comparar el crudo contra el color
	// mide dos cosas distintas y siempre diverge.
	estados := map[string]struct{ crudo, efectivo string }{}
	efectivos := map[string]confidence.Estado{}
	for id, n := range vault {
		ef := EstadoEfectivo(crudos[id], n)
		efectivos[id] = ef
		estados[id] = struct{ crudo, efectivo string }{crudos[id].String(), ef.String()}
	}

	s := NuevaSombra(dir)
	divs := s.Comparar(verdicts, efectivos, time.Now())

	for id, v := range verdicts {
		t.Logf("  %-10s motor=%-8s  eje-check=%-15s  efectivo=%-15s (%s)",
			id, v.Color, estados[id].crudo, estados[id].efectivo, efectivos[id].Color())
	}
	// Las divergencias esperadas son CORRECCIONES buscadas, y están declaradas
	// una por una con su motivo. Que aparezca cualquier otra es lo que hace
	// fallar el test: el criterio para cortar es que no queden diferencias sin
	// explicar, no que no haya diferencias.
	esperadas := map[string]string{
		"fallada": "el motor vigente trata un check FALLADO igual que uno no corrido (ambos amarillo). " +
			"Un check que se ejecutó y falló es evidencia EN CONTRA, no ausencia de evidencia: la máquina lo pone en refuted.",
	}
	ds, _ := s.Divergencias()
	for _, d := range ds {
		motivo, prevista := esperadas[d.NoteID]
		if !prevista {
			t.Errorf("divergencia NO prevista en %q: el motor dice %s (%s) y la máquina %s (%s)",
				d.NoteID, d.ColorViejo, d.RazonVieja, d.ColorNuevo, d.EstadoNuevo)
			continue
		}
		t.Logf("  divergencia esperada en %q: %s", d.NoteID, motivo)
		delete(esperadas, d.NoteID)
	}
	for id, motivo := range esperadas {
		t.Errorf("se esperaba una divergencia en %q que no apareció — si se corrigió el motor, sacala de la lista.\n  motivo declarado: %s", id, motivo)
	}
	if divs != 1 {
		t.Logf("total de divergencias: %d", divs)
	}
}

// Sembrar dos veces no debe duplicar: el journal es append-only y una siembra
// repetida escribiría la historia dos veces.
func TestLaSiembraEsIdempotente(t *testing.T) {
	vault := map[string]*core.Note{
		"a": nota("a", "passed", core.AttestDeclared, []core.Evidence{{Kind: "doc", Ref: "d.md"}}),
	}
	verdicts := core.EvaluateVault(vault, nil, core.MustDate("2026-08-02"))
	j, _ := Open(t.TempDir())

	if n, _ := Sembrar(j, vault, verdicts); n != 1 {
		t.Fatalf("primera siembra: %d", n)
	}
	antes := j.Seq()
	if n, _ := Sembrar(j, vault, verdicts); n != 0 {
		t.Errorf("la segunda siembra volvió a escribir %d notas", n)
	}
	if j.Seq() != antes {
		t.Errorf("la segunda siembra agregó eventos: %d -> %d", antes, j.Seq())
	}
}
