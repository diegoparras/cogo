package motor

import (
	"testing"

	"github.com/diegoparras/cogo/internal/confidence"
	"github.com/diegoparras/cogo/internal/core"
	"github.com/diegoparras/cogo/internal/journal"
)

// El motor sobre el registro estaba desenchufado: se sembraba al arrancar y
// nadie le escribía más, así que `verify` cambiaba el archivo y el estado no
// se movía. Este test es el circuito completo, de la siembra a la
// re-verificación, pasando por el registro y no por el archivo.
func TestVerificarDespuesDeLaSiembraAvanzaElEstado(t *testing.T) {
	hoy := core.NewDate(2026, 9, 8)
	n := &core.Note{ID: "pool", Type: "architecture", Author: "diego", LastVerified: hoy,
		Evidence: []core.Evidence{{Kind: "command_output", Ref: "k6.log:1"}},
		Check:    core.Check{Test: "k6 p95 < 400ms", Status: "not_run"}}
	vault := map[string]*core.Note{"pool": n}

	j, err := journal.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := journal.Sembrar(j, vault, core.EvaluateVaultCore(vault, nil, hoy)); err != nil {
		t.Fatal(err)
	}
	evs, _ := j.All()
	if final, _ := Estados(vault, nil, hoy, evs); final["pool"] != confidence.CheckDeclared {
		t.Fatalf("sembrada: esperaba check_declared, es %s", final["pool"])
	}

	// Alguien verifica. El archivo cambia; el registro TIENE que enterarse.
	antes := *n
	n.Check.Status, n.Check.Attested, n.Check.AttestedBy = "passed", core.AttestDeclared, "token:claude"
	n.LastVerified = hoy
	if err := journal.RegistrarCambio(j, &antes, n); err != nil {
		t.Fatal(err)
	}
	evs, _ = j.All()
	final, _ := Estados(vault, nil, hoy, evs)
	if final["pool"] != confidence.ClaimedPassed {
		t.Fatalf("verificada por un tercero: esperaba claimed_passed, es %s", final["pool"])
	}
	// Y no puede pasar de ahí: nadie llega a verified sin ejecutar (I1).
	if final["pool"] == confidence.Verified {
		t.Fatal("una declaración llegó a verified")
	}
}
