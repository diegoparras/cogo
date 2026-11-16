package journal

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Un caché que se queda viejo es peor que no tener caché: haría que COGO decida
// con un registro que ya no es el que está en disco. Estos cinco tests son la
// razón por la que se puede confiar en el de All.

func abrirCon(t *testing.T) (*Journal, string) {
	t.Helper()
	vault := t.TempDir()
	j, err := Open(vault)
	if err != nil {
		t.Fatal(err)
	}
	return j, vault
}

func poner(t *testing.T, j *Journal, id string) {
	t.Helper()
	if _, err := j.Append(Event{NoteID: id, Kind: "CheckDeclared", Emitter: "test"}); err != nil {
		t.Fatal(err)
	}
}

// Lo que se escribe se ve en la lectura siguiente, sin excepciones.
func TestLoEscritoSeVeEnseguida(t *testing.T) {
	j, _ := abrirCon(t)
	for i, id := range []string{"a", "b", "c"} {
		poner(t, j, id)
		evs, err := j.All()
		if err != nil {
			t.Fatal(err)
		}
		if len(evs) != i+1 {
			t.Fatalf("después de escribir %q el registro devolvió %d eventos, se esperaban %d", id, len(evs), i+1)
		}
		if evs[i].NoteID != id {
			t.Errorf("el último evento es %q y se escribió %q", evs[i].NoteID, id)
		}
	}
}

// Un cambio hecho POR AFUERA —otro proceso de COGO sobre el mismo vault, alguien
// editando el archivo— tiene que invalidar el caché. Es el caso que justifica
// que la huella se calcule del disco y no de un contador en memoria.
func TestUnCambioExternoInvalidaElCache(t *testing.T) {
	j, vault := abrirCon(t)
	poner(t, j, "a")
	if evs, _ := j.All(); len(evs) != 1 {
		t.Fatalf("arranque: %d eventos", len(evs))
	}

	// Otro proceso escribe en el mismo journal.
	otro, err := Open(vault)
	if err != nil {
		t.Fatal(err)
	}
	poner(t, otro, "b")

	evs, err := j.All()
	if err != nil {
		t.Fatal(err)
	}
	if len(evs) != 2 {
		t.Fatalf("el primer journal siguió viendo %d eventos: no notó la escritura externa", len(evs))
	}
}

// Y un archivo que aparece de la nada también: la huella incluye el nombre, no
// solo el tamaño total.
func TestUnArchivoNuevoInvalidaElCache(t *testing.T) {
	j, vault := abrirCon(t)
	poner(t, j, "a")
	_, _ = j.All()

	// Un mes anterior que aparece después (una restauración, un rsync).
	linea := `{"seq":0,"valid_time":"2026-07-01T00:00:00Z","tx_time":"2026-07-01T00:00:00Z","note_id":"vieja","kind":"CheckDeclared","emitter":"test","prev":""}` + "\n"
	ruta := filepath.Join(vault, ".cogo", "journal", "2026-07.jsonl")
	if err := os.WriteFile(ruta, []byte(linea), 0o644); err != nil {
		t.Fatal(err)
	}
	evs, err := j.All()
	if err != nil {
		t.Fatal(err)
	}
	if len(evs) != 2 {
		t.Fatalf("no vio el archivo nuevo: %d eventos", len(evs))
	}
}

// El slice memorizado se entrega a varios llamadores a la vez. Si uno le hace
// append, no puede pisar lo que están leyendo los demás.
func TestElResultadoNoSePisaEntreLlamadores(t *testing.T) {
	j, _ := abrirCon(t)
	poner(t, j, "a")
	poner(t, j, "b")

	uno, _ := j.All()
	// Un llamador desprevenido extiende el resultado.
	_ = append(uno, Event{NoteID: "intruso", Kind: "CheckDeclared"})

	dos, _ := j.All()
	if len(dos) != 2 {
		t.Fatalf("el append de un llamador cambió el largo del registro: %d", len(dos))
	}
	for _, e := range dos {
		if e.NoteID == "intruso" {
			t.Fatal("el append de un llamador se coló en el registro compartido")
		}
	}
}

// La escritura extiende el caché en vez de invalidarlo, y el resultado tiene que
// ser idéntico a releer todo del disco. Si divergieran, el caché sería una
// segunda implementación del registro — y la que se leería.
func TestExtenderElCacheDaLoMismoQueReleer(t *testing.T) {
	j, vault := abrirCon(t)
	for _, id := range []string{"a", "b", "c", "d"} {
		poner(t, j, id)
	}
	cacheado, err := j.All()
	if err != nil {
		t.Fatal(err)
	}

	// Un journal nuevo sobre el mismo disco: sin caché, lee todo.
	fresco, err := Open(vault)
	if err != nil {
		t.Fatal(err)
	}
	deDisco, err := fresco.All()
	if err != nil {
		t.Fatal(err)
	}

	if len(cacheado) != len(deDisco) {
		t.Fatalf("largos distintos: caché %d, disco %d", len(cacheado), len(deDisco))
	}
	for i := range cacheado {
		a, b := cacheado[i], deDisco[i]
		if a.Seq != b.Seq || a.NoteID != b.NoteID || a.Kind != b.Kind || a.PrevDigest != b.PrevDigest ||
			!a.TxTime.Equal(b.TxTime) || !a.ValidTime.Equal(b.ValidTime) {
			t.Errorf("evento %d difiere:\n  caché %+v\n  disco %+v", i, a, b)
		}
	}
	// Y la cadena sigue siendo verificable, que es la prueba de que no se
	// perdió ni se reordenó nada.
	if err := j.Verificar(); err != nil {
		t.Errorf("la cadena no verifica después de usar el caché: %v", err)
	}
}

// La huella distingue dos estados del directorio con el mismo tamaño total.
func TestLaHuellaCambiaConElContenido(t *testing.T) {
	j, _ := abrirCon(t)
	poner(t, j, "a")
	antes, err := j.huella()
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(2 * time.Millisecond)
	poner(t, j, "b")
	despues, err := j.huella()
	if err != nil {
		t.Fatal(err)
	}
	if antes == despues {
		t.Errorf("la huella no cambió al escribir un evento: %q", antes)
	}
}

// Escribir y leer al mismo tiempo. Es lo que pasa en un servidor: un agente
// captura mientras otro pide contexto.
//
// La carrera que este test cierra: entre que el evento se escribe al archivo y
// que se agrega al caché, otra goroutine puede releer el disco y ya tenerlo.
// Agregarlo ahí lo duplicaba, y un registro con un evento repetido no verifica.
func TestEscribirYLeerALaVez(t *testing.T) {
	j, _ := abrirCon(t)
	const n = 60

	listo := make(chan struct{})
	go func() {
		defer close(listo)
		for i := 0; i < n; i++ {
			poner(t, j, "nota")
		}
	}()
	for i := 0; i < 200; i++ {
		evs, err := j.All()
		if err != nil {
			t.Error(err)
			break
		}
		// En cualquier instante, lo leído tiene que ser un prefijo válido del
		// registro: secuencias correlativas, sin huecos ni repeticiones.
		for k, e := range evs {
			if e.Seq != uint64(k+1) {
				t.Errorf("el evento %d tiene seq %d: el caché quedó inconsistente", k, e.Seq)
				return
			}
		}
	}
	<-listo

	evs, err := j.All()
	if err != nil {
		t.Fatal(err)
	}
	if len(evs) != n {
		t.Errorf("quedaron %d eventos de %d escritos", len(evs), n)
	}
	if err := j.Verificar(); err != nil {
		t.Errorf("la cadena no verifica después de escribir y leer en paralelo: %v", err)
	}
}
