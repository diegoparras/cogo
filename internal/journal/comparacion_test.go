package journal

import (
	"sort"
	"testing"
	"time"

	"github.com/diegoparras/cogo/internal/confidence"
	"github.com/diegoparras/cogo/internal/core"
)

// La comparación completa de los dos motores sobre los mismos casos: el vigente
// contra la cadena entera de la máquina nueva (siembra → fold → ejes → punto
// fijo). Es la evidencia con la que se decide el corte de la Fase 4.
//
// No falla por haber diferencias: falla por diferencias que nadie declaró. Las
// esperadas están listadas con su motivo, y si una deja de aparecer también
// falla — porque significaría que el motor cambió y nadie revisó por qué.
func TestComparacionCompletaDeLosDosMotores(t *testing.T) {
	hoy := core.MustDate("2026-08-02")
	obs := []core.Evidence{{Kind: "command_output", Ref: "x.log:1"}}
	rep := []core.Evidence{{Kind: "doc", Ref: "d.md"}}

	mk := func(id, tipo string, ev []core.Evidence, st, att, verificada string, deps ...string) *core.Note {
		return &core.Note{
			ID: id, Type: tipo, LastVerified: core.MustDate(verificada), Evidence: ev,
			Check:     core.Check{Test: "go test ./...", Status: st, Attested: att, AttestedBy: "token:x"},
			DependsOn: deps, Body: "## Claim\n" + id,
		}
	}
	vault := map[string]*core.Note{
		"verde-declarada":  mk("verde-declarada", "architecture", obs, "passed", core.AttestDeclared, "2026-08-01"),
		"verde-ejecutada":  mk("verde-ejecutada", "architecture", obs, "passed", core.AttestExecuted, "2026-08-01"),
		"check-sin-correr": mk("check-sin-correr", "architecture", obs, "not_run", "", "2026-08-01"),
		"check-fallado":    mk("check-fallado", "architecture", obs, "failed", "", "2026-08-01"),
		"solo-reportada":   mk("solo-reportada", "architecture", rep, "passed", core.AttestDeclared, "2026-08-01"),
		"sin-evidencia":    mk("sin-evidencia", "architecture", nil, "passed", core.AttestDeclared, "2026-08-01"),
		"vencida":          mk("vencida", "command", obs, "passed", core.AttestDeclared, "2026-06-20"),
		"dep-de-podrida":   mk("dep-de-podrida", "architecture", obs, "passed", core.AttestDeclared, "2026-08-01", "sin-evidencia"),
		"ciclo-a":          mk("ciclo-a", "architecture", obs, "passed", core.AttestDeclared, "2026-08-01", "ciclo-b"),
		"ciclo-b":          mk("ciclo-b", "architecture", obs, "passed", core.AttestDeclared, "2026-08-01", "ciclo-a"),
	}

	// --- motor vigente
	verdicts := core.EvaluateVault(vault, nil, hoy)
	for id, n := range vault { // el StaleAt lo calcula el motor; se copia para el eje de frescura
		n.StaleAt = verdicts[id].StaleAt
	}

	// --- máquina nueva, la cadena completa
	j, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Sembrar(j, vault, verdicts); err != nil {
		t.Fatal(err)
	}
	evs, _ := j.All()
	crudos := Fold(evs, time.Time{}, time.Time{})

	local := map[string]confidence.Estado{}
	for id, n := range vault {
		local[id] = EstadoLocal(crudos[id], n, hoy, false)
	}
	g := confidence.Grafo{}
	for id, n := range vault {
		g[id] = n.DependsOn
	}
	final := confidence.PuntoFijo(g, local)

	// --- tabla comparativa
	ids := make([]string, 0, len(vault))
	for id := range vault {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	t.Log("")
	t.Logf("  %-17s %-9s %-9s   %s", "NOTA", "VIGENTE", "NUEVO", "ESTADO")
	for _, id := range ids {
		t.Logf("  %-17s %-9s %-9s   %s", id, verdicts[id].Color, final[id].Color(), final[id])
	}
	t.Log("")

	esperadas := map[string]string{
		"check-fallado": "un check EJECUTADO que falló es evidencia en contra, no ausencia de evidencia: " +
			"el motor vigente lo deja amarillo, la máquina lo pone en refuted (rojo)",
		"ciclo-a": "dos notas verificadas que se referencian mutuamente hoy caen las dos a rojo. " +
			"El mayor punto fijo las deja como están: nada fuera del ciclo las empuja hacia abajo",
		"ciclo-b": "ídem ciclo-a",
	}

	for _, id := range ids {
		if verdicts[id].Color.String() == final[id].Color() {
			if motivo, prevista := esperadas[id]; prevista {
				t.Errorf("se esperaba que %q divergiera y coincidió — revisá si el motor cambió.\n  motivo declarado: %s", id, motivo)
				delete(esperadas, id)
			}
			continue
		}
		motivo, prevista := esperadas[id]
		if !prevista {
			t.Errorf("divergencia NO prevista en %q: vigente=%s (%s) · nuevo=%s (%s)",
				id, verdicts[id].Color, verdicts[id].Reason, final[id].Color(), final[id])
			continue
		}
		t.Logf("  ✓ divergencia esperada · %s\n      %s", id, motivo)
		delete(esperadas, id)
	}
	for id, motivo := range esperadas {
		t.Errorf("la divergencia declarada para %q no apareció: %s", id, motivo)
	}
}
