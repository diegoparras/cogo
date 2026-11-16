package journal

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/diegoparras/cogo/internal/confidence"
)

func ejecucionLocal(t *testing.T) (*Journal, []Event) {
	t.Helper()
	local := abrir(t)
	if _, err := local.Append(Event{NoteID: "pool", Kind: "CheckDeclared", Emitter: "diego"}); err != nil {
		t.Fatal(err)
	}
	if _, err := local.AppendEjecucion(Event{NoteID: "pool", Kind: "VerificationStarted",
		Payload: json.RawMessage(`{"check":"go-test"}`)}); err != nil {
		t.Fatal(err)
	}
	if _, err := local.AppendEjecucion(Event{NoteID: "pool", Kind: "CheckExecuted", Guard: "ejecucion_ok",
		Payload: json.RawMessage(`{"check":"go-test","exit_code":0,"stdout":"ok"}`)}); err != nil {
		t.Fatal(err)
	}
	evs, err := local.Ejecuciones(0)
	if err != nil {
		t.Fatal(err)
	}
	if len(evs) != 2 {
		t.Fatalf("esperaba las dos ejecuciones, hay %d", len(evs))
	}
	return local, evs
}

// El circuito entero: el runner corre en el local, el hosteado importa, y la
// nota llega a verified allá con la cadena íntegra. Dos veces el mismo lote no
// duplica nada.
func TestImportarLlevaAVerifiedYEsIdempotente(t *testing.T) {
	_, evs := ejecucionLocal(t)
	remoto := abrir(t)
	if _, err := remoto.Append(Event{NoteID: "pool", Kind: "CheckDeclared", Emitter: "sembrado"}); err != nil {
		t.Fatal(err)
	}
	n, err := Importar(remoto, evs)
	if err != nil || n != 2 {
		t.Fatalf("importados=%d err=%v", n, err)
	}
	if err := remoto.Verificar(); err != nil {
		t.Fatalf("la cadena tiene que quedar íntegra: %v", err)
	}
	todos, _ := remoto.All()
	if got := Fold(todos, time.Time{}, time.Time{})["pool"]; got != confidence.Verified {
		t.Fatalf("esperaba verified, es %s", got)
	}
	// Y los eventos importados dicen de dónde vinieron, con su hora original.
	ult := todos[len(todos)-1]
	if !strings.Contains(string(ult.Payload), `"importado_de":"`) || !ult.ValidTime.Equal(evs[1].ValidTime) {
		t.Fatalf("el importado tiene que llevar origen y hora original: %s %v vs %v", ult.Payload, ult.ValidTime, evs[1].ValidTime)
	}
	if n, err := Importar(remoto, evs); err != nil || n != 0 {
		t.Fatalf("el segundo import no agrega nada: %d %v", n, err)
	}
	if todos2, _ := remoto.All(); len(todos2) != len(todos) {
		t.Fatal("se duplicaron eventos")
	}
}

// Un evento que no es del runner rechaza el lote ENTERO: un import a medias
// deja una nota en verifying para siempre.
func TestImportarRechazaLoQueNoEsDelRunner(t *testing.T) {
	_, evs := ejecucionLocal(t)
	remoto := abrir(t)
	falso := evs[1]
	falso.Emitter = "agent"
	if n, err := Importar(remoto, []Event{evs[0], falso}); err == nil || n != 0 {
		t.Fatalf("tiene que rechazar todo: n=%d err=%v", n, err)
	}
	if todos, _ := remoto.All(); len(todos) != 0 {
		t.Fatal("no puede haber quedado nada escrito")
	}
	otro := evs[1]
	otro.Kind = "VerifyDeclared"
	if _, err := Importar(remoto, []Event{otro}); err == nil {
		t.Fatal("un evento que no es de ejecución no entra por acá")
	}
}
