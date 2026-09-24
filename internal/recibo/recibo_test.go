package recibo

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/diegoparras/cogo/internal/journal"
)

func registroCon(t *testing.T) (*journal.Journal, string) {
	t.Helper()
	dir := t.TempDir()
	j, err := journal.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"CheckDeclared", "VerifyDeclared"} {
		e := journal.Event{NoteID: "pool", Kind: k, Emitter: "diego"}
		if k == "VerifyDeclared" {
			e.Guard = "declara_un_tercero"
		}
		if _, err := j.Append(e); err != nil {
			t.Fatal(err)
		}
	}
	return j, dir
}

// El circuito: se pide con la nota en check_declared, después la nota sube, y
// el recibo sigue diciendo —y probando— lo que se sabía entonces.
func TestElReciboReconstruyeLoQueSeSabia(t *testing.T) {
	dir := t.TempDir()
	j, err := journal.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := j.Append(journal.Event{NoteID: "pool", Kind: "CheckDeclared", Emitter: "diego"}); err != nil {
		t.Fatal(err)
	}
	seq, cabeza := j.Cabeza()
	evs, _ := j.All()
	st := Abrir(dir)
	r, err := st.Agregar(Recibo{
		Quien: "token:claude", Accion: "drop table usuarios", Clase: "irreversible", Necesita: "verified",
		Autoriza: false, Notas: []Nota{{ID: "pool", Final: "check_declared", Eventos: EstadosDeEventos(evs, seq, []string{"pool"})["pool"]}},
		Seq: seq, Cabeza: cabeza,
	})
	if err != nil || !strings.HasPrefix(r.ID, "r-") {
		t.Fatalf("agregar: %v %q", err, r.ID)
	}
	// Después: la nota sube a claimed_passed. El recibo no cambia.
	if _, err := j.Append(journal.Event{NoteID: "pool", Kind: "VerifyDeclared", Guard: "declara_un_tercero", Emitter: "diego"}); err != nil {
		t.Fatal(err)
	}
	rec := Reconstruir(j, r)
	if !rec.Fiel || rec.Eventos["pool"] != "check_declared" {
		t.Fatalf("tiene que reconstruir el estado de entonces: %+v", rec)
	}
	if todos, _ := st.Todos(); len(todos) != 1 || todos[0].ID != r.ID {
		t.Fatalf("todos: %+v", todos)
	}
	if _, ok := st.Buscar(r.ID); !ok {
		t.Fatal("buscar por id")
	}
}

// Si alguien reescribe la historia después del veredicto, el recibo lo
// nombra: es la respuesta que ninguna otra pieza puede dar.
func TestElReciboDetectaLaHistoriaReescrita(t *testing.T) {
	j, dir := registroCon(t)
	seq, cabeza := j.Cabeza()
	evs, _ := j.All()
	r := Recibo{Accion: "x", Seq: seq, Cabeza: cabeza,
		Notas: []Nota{{ID: "pool", Final: "claimed_passed", Eventos: EstadosDeEventos(evs, seq, []string{"pool"})["pool"]}}}
	if rec := Reconstruir(j, r); !rec.Fiel {
		t.Fatalf("precondición: fiel, %+v", rec)
	}
	// Reescribir: cambiar el emisor del primer evento en el archivo.
	entradas, _ := os.ReadDir(filepath.Join(dir, ".cogo", "journal"))
	for _, e := range entradas {
		p := filepath.Join(dir, ".cogo", "journal", e.Name())
		b, _ := os.ReadFile(p)
		_ = os.WriteFile(p, []byte(strings.Replace(string(b), `"emitter":"diego"`, `"emitter":"otro"`, 1)), 0o644)
	}
	j2, err := journal.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	rec := Reconstruir(j2, r)
	if rec.Fiel || !strings.Contains(rec.Motivo, "reescribió") {
		t.Fatalf("tiene que detectar la reescritura: %+v", rec)
	}
	// Y un registro que no llega al evento del recibo también se nombra —
	// sobre un registro sano, para que el motivo sea ese y no otro.
	j3, _ := registroCon(t)
	r3 := r
	r3.Seq = 99
	if rec := Reconstruir(j3, r3); rec.Fiel || !strings.Contains(rec.Motivo, "no llega") {
		t.Fatalf("recortado: %+v", rec)
	}
}
