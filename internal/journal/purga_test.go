package journal

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// Purgar saca la salida del runner de los eventos de esa nota, deja la marca,
// y la cadena queda rota A PROPÓSITO: Verificar no dice "se rompió", dice
// quién purgó qué y cuándo.
func TestPurgarBorraLaSalidaYExplicaLaRotura(t *testing.T) {
	dir := t.TempDir()
	j, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	reloj := time.Date(2026, 9, 24, 10, 0, 0, 0, time.UTC)
	j.SetClock(func() time.Time { return reloj })

	carga := func(m map[string]any) json.RawMessage { b, _ := json.Marshal(m); return b }
	if _, err := j.Append(Event{NoteID: "pool", Kind: "CheckDeclared", Emitter: "diego"}); err != nil {
		t.Fatal(err)
	}
	if _, err := j.AppendEjecucion(Event{NoteID: "pool", Kind: "CheckExecuted", Emitter: EmisorEjecucion,
		Payload: carga(map[string]any{"check": "psql", "stdout": "password=SECRETO", "stderr": ""})}); err != nil {
		t.Fatal(err)
	}
	// Otra nota con salida: no se toca.
	if _, err := j.AppendEjecucion(Event{NoteID: "otra", Kind: "CheckExecuted", Emitter: EmisorEjecucion,
		Payload: carga(map[string]any{"check": "ls", "stdout": "queda"})}); err != nil {
		t.Fatal(err)
	}
	if _, err := j.Append(Event{NoteID: "pool", Kind: "VerifyDeclared", Guard: "declara_un_tercero", Emitter: "diego"}); err != nil {
		t.Fatal(err)
	}
	if err := j.Verificar(); err != nil {
		t.Fatalf("precondición: íntegro, %v", err)
	}
	_, cabezaAntes := j.Cabeza()

	reloj = reloj.Add(48 * time.Hour)
	p, ev, err := j.Purgar("pool", "diego", "salió una clave por stdout")
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Eventos) != 1 || p.Eventos[0] != 2 || p.CabezaAnterior != cabezaAntes || p.SeqAnterior != 4 || ev.Kind != KindPurged || ev.Seq != 5 {
		t.Fatalf("purga mal descrita: %+v %+v", p, ev)
	}

	// El secreto ya no está en ningún lado; la otra nota sigue entera.
	j2, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	evs, _ := j2.All()
	if len(evs) != 5 {
		t.Fatalf("cinco eventos: %d", len(evs))
	}
	for _, e := range evs {
		s := string(e.Payload)
		if strings.Contains(s, "SECRETO") {
			t.Fatalf("el secreto sigue en el evento %d: %s", e.Seq, s)
		}
		if e.Seq == 2 && (!strings.Contains(s, `"purgado":true`) || !strings.Contains(s, `"check":"psql"`)) {
			t.Fatalf("el evento purgado tiene que conservar lo demás y llevar la marca: %s", s)
		}
		if e.NoteID == "otra" && !strings.Contains(s, "queda") {
			t.Fatalf("la otra nota no se toca: %s", s)
		}
	}

	// La cadena está rota, y lo dice con nombre y fecha.
	err = j2.Verificar()
	if err == nil {
		t.Fatal("purgar tiene que dejar la cadena rota")
	}
	for _, frag := range []string{"evento 3", `"pool"`, "2026-09-26", "purgó", "a propósito"} {
		if !strings.Contains(err.Error(), frag) {
			t.Errorf("Verificar tiene que decir %q: %v", frag, err)
		}
	}
	// CadenaHasta también: un recibo anterior a la purga se explica, no se acusa.
	if _, err := j2.CadenaHasta(4); err == nil || !strings.Contains(err.Error(), "purgó") {
		t.Errorf("CadenaHasta tiene que explicar la purga: %v", err)
	}
	// Antes del evento purgado, la cadena sigue cerrando.
	if _, err := j2.CadenaHasta(1); err != nil {
		t.Errorf("hasta antes de la purga la cadena cierra: %v", err)
	}
	ps, pevs, _ := j2.Purgas()
	if len(ps) != 1 || ps[0].Motivo != "salió una clave por stdout" || pevs[0].Emitter != "diego" {
		t.Fatalf("purgas: %+v", ps)
	}

	// Purgar de nuevo la misma nota no reescribe nada, pero queda asentado.
	p2, _, err := j2.Purgar("pool", "diego", "de nuevo")
	if err != nil || len(p2.Eventos) != 0 {
		t.Fatalf("segunda purga: %+v %v", p2, err)
	}
	// Y el registro sigue aceptando escrituras después.
	if _, err := j2.Append(Event{NoteID: "pool", Kind: "CheckDeclared", Emitter: "diego"}); err != nil {
		t.Fatal(err)
	}
}

// Una rotura que NO viene de una purga sigue siendo lo que era: una acusación
// sin atenuantes.
func TestUnaRoturaSinPurgaNoSeExcusa(t *testing.T) {
	evs := []Event{{Seq: 1, NoteID: "a"}, {Seq: 2, NoteID: "a", PrevDigest: "x"}}
	if s := explicarRotura(evs, 1); s != "" {
		t.Fatalf("sin Purged no hay explicación: %q", s)
	}
}
