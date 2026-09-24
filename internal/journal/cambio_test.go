package journal

import (
	"testing"
	"time"

	"github.com/diegoparras/cogo/internal/confidence"
	"github.com/diegoparras/cogo/internal/core"
)

var zero time.Time

func notaCon(test, status, attested, por string, dia core.Date) *core.Note {
	return &core.Note{ID: "n", Author: "diego", LastVerified: dia,
		Evidence: []core.Evidence{{Kind: "command_output", Ref: "x.log:1"}},
		Check:    core.Check{Test: test, Status: status, Attested: attested, AttestedBy: por}}
}

func tipos(evs []Event) []string {
	out := []string{}
	for _, e := range evs {
		out = append(out, e.Kind)
	}
	return out
}

func igual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// El agujero que esto cierra: una nota verificada después de la siembra no
// avanzaba nunca, porque nadie traducía la escritura a eventos.
func TestVerificarDespuesDeSembradaAvanza(t *testing.T) {
	hoy := core.NewDate(2026, 9, 8)
	antes := notaCon("go test ./...", "not_run", "", "", core.Date{})
	sembrada := eventosDeSiembra(antes, core.Verdict{})
	if got := Fold(sembrada, zero, zero)["n"]; got != confidence.CheckDeclared {
		t.Fatalf("sembrada debería estar en check_declared, está en %s", got)
	}

	despues := notaCon("go test ./...", "passed", core.AttestDeclared, "token:claude", hoy)
	evs := append(sembrada, EventosDeCambio(antes, despues)...)
	if got := Fold(evs, zero, zero)["n"]; got != confidence.ClaimedPassed {
		t.Fatalf("verificada por un tercero debería ser claimed_passed, es %s (eventos %v)", got, tipos(evs))
	}
	// Y la palabra queda a nombre de quien la dio, que es lo que mide la calibración.
	if e := evs[len(evs)-1]; e.Kind != "VerifyDeclared" || e.Emitter != "token:claude" {
		t.Errorf("la declaración tiene que ir a nombre del declarante: %+v", e)
	}
}

func TestNotaNuevaSeSiembraAlEscribirla(t *testing.T) {
	n := notaCon("go test", "not_run", "", "", core.Date{})
	if got := tipos(EventosDeCambio(nil, n)); !igual(got, []string{"CheckDeclared"}) {
		t.Fatalf("una nota nueva con criterio declara el criterio, vino %v", got)
	}
	sin := notaCon("", "not_run", "", "", core.Date{})
	if got := EventosDeCambio(nil, sin); len(got) != 0 {
		t.Fatalf("sin criterio no hay nada que anotar, vino %v", tipos(got))
	}
}

func TestSinCambioNoHayEventos(t *testing.T) {
	hoy := core.NewDate(2026, 9, 8)
	a := notaCon("go test", "passed", core.AttestDeclared, "diego", hoy)
	b := notaCon("go test", "passed", core.AttestDeclared, "diego", hoy)
	if got := EventosDeCambio(a, b); len(got) != 0 {
		t.Fatalf("editar el cuerpo no toca el eje del check, vino %v", tipos(got))
	}
}

// Renovar la verificación es lo que saca a una nota de `stale`.
func TestReverificarRenueva(t *testing.T) {
	a := notaCon("go test", "passed", core.AttestDeclared, "diego", core.NewDate(2026, 1, 1))
	b := notaCon("go test", "passed", core.AttestDeclared, "diego", core.NewDate(2026, 9, 8))
	if got := tipos(EventosDeCambio(a, b)); !igual(got, []string{"VerifyDeclared"}) {
		t.Fatalf("una fecha posterior es una renovación, vino %v", got)
	}
	evs := append(eventosDeSiembra(a, core.Verdict{}), Event{NoteID: "n", Kind: "TTLExpired"})
	if got := Fold(evs, zero, zero)["n"]; got != confidence.Stale {
		t.Fatalf("precondición: vencida, está en %s", got)
	}
	evs = append(evs, EventosDeCambio(a, b)...)
	if got := Fold(evs, zero, zero)["n"]; got != confidence.ClaimedPassed {
		t.Fatalf("renovada debería salir de stale, quedó en %s", got)
	}
}

// Una falla declarada vale tanto como un pase declarado, y no se disfraza de
// ejecución: la calibración no la cuenta como verdad de máquina.
func TestFallaDeclaradaRefutaSinFingirEjecucion(t *testing.T) {
	hoy := core.NewDate(2026, 9, 8)
	a := notaCon("go test", "passed", core.AttestDeclared, "diego", hoy)
	b := notaCon("go test", "failed", core.AttestDeclared, "diego", hoy)
	got := EventosDeCambio(a, b)
	if !igual(tipos(got), []string{"FailDeclared"}) {
		t.Fatalf("vino %v", tipos(got))
	}
	evs := append(eventosDeSiembra(a, core.Verdict{}), got...)
	if st := Fold(evs, zero, zero)["n"]; st != confidence.Refuted {
		t.Fatalf("refutada, es %s", st)
	}
	// Y de ahí solo se sale con un criterio nuevo (I7).
	c := notaCon("go test -run Nuevo", "not_run", "", "", core.Date{})
	evs = append(evs, EventosDeCambio(b, c)...)
	if st := Fold(evs, zero, zero)["n"]; st != confidence.CheckDeclared {
		t.Fatalf("un criterio nuevo saca de refuted, quedó en %s", st)
	}
}

// Cambiar el criterio de una nota verde la baja: lo declarado cubría otro check.
func TestCriterioNuevoInvalidaLoDeclarado(t *testing.T) {
	hoy := core.NewDate(2026, 9, 8)
	a := notaCon("go test", "passed", core.AttestDeclared, "diego", hoy)
	b := notaCon("go test -race", "not_run", "", "", hoy)
	evs := append(eventosDeSiembra(a, core.Verdict{}), EventosDeCambio(a, b)...)
	if st := Fold(evs, zero, zero)["n"]; st != confidence.CheckDeclared {
		t.Fatalf("con criterio nuevo tiene que volver a check_declared, quedó en %s", st)
	}
	// También desde verified: la ejecución verificó el check viejo.
	v := notaCon("go test", "passed", core.AttestExecuted, EmisorEjecucion, hoy)
	evs = append(eventosDeSiembra(v, core.Verdict{}), EventosDeCambio(v, b)...)
	if st := Fold(evs, zero, zero)["n"]; st != confidence.CheckDeclared {
		t.Fatalf("desde verified también, quedó en %s", st)
	}
}

// Una ejecución la escribe el runner por su puerta: la escritura del archivo
// no puede inventar una segunda.
func TestLaEjecucionNoSeTraduce(t *testing.T) {
	hoy := core.NewDate(2026, 9, 8)
	a := notaCon("go test", "not_run", "", "", core.Date{})
	b := notaCon("go test", "passed", core.AttestExecuted, EmisorEjecucion, hoy)
	if got := EventosDeCambio(a, b); len(got) != 0 {
		t.Fatalf("el runner ya escribió sus eventos; vino %v", tipos(got))
	}
	f := notaCon("go test", "failed", core.AttestExecuted, EmisorEjecucion, hoy)
	if got := EventosDeCambio(a, f); len(got) != 0 {
		t.Fatalf("tampoco la falla ejecutada; vino %v", tipos(got))
	}
	// Y ninguna escritura común puede firmar como el runner.
	c := notaCon("go test -v", "not_run", "", EmisorEjecucion, core.Date{})
	c.Author = ""
	for _, e := range EventosDeCambio(b, c) {
		if e.Emitter == EmisorEjecucion {
			t.Fatalf("una escritura firmó como el runner: %+v", e)
		}
	}
}

func TestRegistrarCambioEscribeEnElRegistro(t *testing.T) {
	j := abrir(t)
	hoy := core.NewDate(2026, 9, 8)
	a := notaCon("go test", "not_run", "", "", core.Date{})
	if err := RegistrarCambio(j, nil, a); err != nil {
		t.Fatal(err)
	}
	b := notaCon("go test", "passed", core.AttestDeclared, "diego", hoy)
	if err := RegistrarCambio(j, a, b); err != nil {
		t.Fatal(err)
	}
	evs, err := j.All()
	if err != nil {
		t.Fatal(err)
	}
	if got := Fold(evs, zero, zero)["n"]; got != confidence.ClaimedPassed {
		t.Fatalf("el registro tiene que llevar la nota a claimed_passed, está en %s (%v)", got, tipos(evs))
	}
}
