package journal

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/diegoparras/cogo/internal/confidence"
)

func abrir(t *testing.T) *Journal {
	t.Helper()
	j, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return j
}

func TestAppendNumeraYEncadena(t *testing.T) {
	j := abrir(t)
	for i := 0; i < 5; i++ {
		if _, err := j.Append(Event{NoteID: "n1", Kind: "CheckDeclared", Emitter: "agent"}); err != nil {
			t.Fatal(err)
		}
	}
	evs, err := j.All()
	if err != nil {
		t.Fatal(err)
	}
	if len(evs) != 5 {
		t.Fatalf("se escribieron 5 eventos y se leyeron %d", len(evs))
	}
	for i, e := range evs {
		if e.Seq != uint64(i+1) {
			t.Errorf("evento %d tiene seq %d", i, e.Seq)
		}
		if e.TxTime.IsZero() || e.ValidTime.IsZero() {
			t.Errorf("evento %d sin poblar los dos ejes de tiempo", e.Seq)
		}
	}
	if err := j.Verificar(); err != nil {
		t.Errorf("la cadena debería estar íntegra: %v", err)
	}
}

// La cadena existe para esto: si alguien edita un evento viejo, se nota.
func TestLaCadenaDetectaUnaEdicionPosterior(t *testing.T) {
	dir := t.TempDir()
	j, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 4; i++ {
		if _, err := j.Append(Event{NoteID: "n1", Kind: "CheckDeclared", Emitter: "agent"}); err != nil {
			t.Fatal(err)
		}
	}
	// alguien reescribe el emisor de un evento intermedio
	files, _ := filepath.Glob(filepath.Join(dir, ".cogo", "journal", "*.jsonl"))
	if len(files) != 1 {
		t.Fatalf("se esperaba un archivo, hay %d", len(files))
	}
	b, _ := os.ReadFile(files[0])
	manipulado := strings.Replace(string(b), `"emitter":"agent"`, `"emitter":"internal_runner"`, 1)
	if manipulado == string(b) {
		t.Fatal("no se pudo manipular el archivo: el test no probaría nada")
	}
	if err := os.WriteFile(files[0], []byte(manipulado), 0o644); err != nil {
		t.Fatal(err)
	}

	j2, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := j2.Verificar(); err == nil {
		t.Error("la manipulación de un evento pasado NO fue detectada")
	}
}

func TestSeContinuaTrasReabrir(t *testing.T) {
	dir := t.TempDir()
	j, _ := Open(dir)
	for i := 0; i < 3; i++ {
		j.Append(Event{NoteID: "n1", Kind: "CheckDeclared", Emitter: "agent"})
	}
	j2, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if j2.Seq() != 3 {
		t.Errorf("al reabrir, seq = %d; debería continuar en 3", j2.Seq())
	}
	if _, err := j2.Append(Event{NoteID: "n1", Kind: "CheckDeclared", Emitter: "agent"}); err != nil {
		t.Fatal(err)
	}
	if err := j2.Verificar(); err != nil {
		t.Errorf("la cadena se rompió al reabrir: %v", err)
	}
}

func TestNoSeAceptaElFuturo(t *testing.T) {
	j := abrir(t)
	_, err := j.Append(Event{
		NoteID: "n1", Kind: "CheckDeclared", Emitter: "agent",
		ValidTime: time.Now().Add(2 * time.Hour),
	})
	if err == nil {
		t.Error("se aceptó un evento cuyo valid_time está en el futuro")
	}
}

// ---------------------------------------------------------------------------
// Fold
// ---------------------------------------------------------------------------

func TestFoldLlevaLaNotaPorLaMaquina(t *testing.T) {
	j := abrir(t)
	j.Append(Event{NoteID: "n", Kind: "CheckDeclared", Emitter: "agent"})
	j.Append(Event{NoteID: "n", Kind: "VerifyDeclared", Emitter: "agent", Guard: "declara_un_tercero"})
	evs, _ := j.All()

	if got := EstadoDe(evs, "n"); got != confidence.ClaimedPassed {
		t.Errorf("tras declarar, la nota quedó en %s; se esperaba claimed_passed", got)
	}

	// y el runner la lleva a verified
	j.AppendEjecucion(Event{NoteID: "n", Kind: "VerificationStarted"})
	j.AppendEjecucion(Event{NoteID: "n", Kind: "CheckExecuted", Guard: "ejecucion_ok"})
	evs, _ = j.All()
	if got := EstadoDe(evs, "n"); got != confidence.Verified {
		t.Errorf("tras ejecutar el check, la nota quedó en %s; se esperaba verified", got)
	}
}

// Lo que un tercero declara no puede llegar a verified por ningún camino: es el
// invariante I1 comprobado sobre el fold, no solo sobre la tabla.
func TestNingunaSecuenciaDeclaradaLlegaAVerified(t *testing.T) {
	j := abrir(t)
	// se prueban todos los eventos que puede emitir alguien que no es el runner
	for _, k := range []string{"CheckDeclared", "VerifyDeclared", "ContradictionResolved", "Unquarantined", "VerificationStarted"} {
		for i := 0; i < 3; i++ {
			j.Append(Event{NoteID: "n", Kind: k, Emitter: "agent", Guard: "declara_un_tercero"})
		}
	}
	evs, _ := j.All()
	if got := EstadoDe(evs, "n"); got == confidence.Verified {
		t.Error("una secuencia sin el runner alcanzó verified")
	}
}

// El corte por tiempo de registro es lo que permite auditar a un agente: qué
// creía COGO cuando el agente actuó, no qué se supo después.
func TestElCorteTemporalReconstruyeElPasado(t *testing.T) {
	dir := t.TempDir()
	j, _ := Open(dir)
	reloj := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	j.SetClock(func() time.Time { return reloj })

	j.Append(Event{NoteID: "n", Kind: "CheckDeclared", Emitter: "agent"})
	reloj = reloj.Add(24 * time.Hour)
	j.Append(Event{NoteID: "n", Kind: "VerifyDeclared", Emitter: "agent", Guard: "declara_un_tercero"})
	reloj = reloj.Add(24 * time.Hour)
	j.Append(Event{NoteID: "n", Kind: "TTLExpired", Emitter: "agent"})

	evs, _ := j.All()
	corte := time.Date(2026, 1, 2, 12, 0, 0, 0, time.UTC) // después del segundo evento

	if got := Fold(evs, time.Time{}, corte)["n"]; got != confidence.ClaimedPassed {
		t.Errorf("al 2 de enero la nota estaba en %s; se esperaba claimed_passed", got)
	}
	if got := Fold(evs, time.Time{}, time.Time{})["n"]; got != confidence.Stale {
		t.Errorf("hoy la nota está en %s; se esperaba stale", got)
	}
}

// Un evento que no aplica al estado actual no rompe nada ni cambia el estado:
// la máquina es total, y eso es lo que hace que el fold termine siempre.
func TestUnEventoQueNoAplicaNoCambiaNada(t *testing.T) {
	j := abrir(t)
	j.AppendEjecucion(Event{NoteID: "n", Kind: "CheckExecuted", Guard: "ejecucion_ok"})
	evs, _ := j.All()
	if got := EstadoDe(evs, "n"); got != confidence.Inicial {
		t.Errorf("un CheckExecuted sin haber empezado a verificar movió la nota a %s", got)
	}
}

// Plegar dos veces la misma secuencia da lo mismo: sin esto, el estado
// materializado y el reconstruido podrían diferir.
func TestElFoldEsDeterminista(t *testing.T) {
	j := abrir(t)
	for i := 0; i < 20; i++ {
		j.Append(Event{NoteID: "a", Kind: "CheckDeclared", Emitter: "agent"})
		j.Append(Event{NoteID: "b", Kind: "TTLExpired", Emitter: "agent"})
		j.Append(Event{NoteID: "a", Kind: "VerifyDeclared", Emitter: "agent", Guard: "declara_un_tercero"})
	}
	evs, _ := j.All()
	primero := Fold(evs, time.Time{}, time.Time{})
	for i := 0; i < 10; i++ {
		otro := Fold(evs, time.Time{}, time.Time{})
		for k, v := range primero {
			if otro[k] != v {
				t.Fatalf("el fold no es determinista: %q dio %s y después %s", k, v, otro[k])
			}
		}
	}
}
