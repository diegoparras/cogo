package journal

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// Dos Journal distintos sobre el mismo directorio son, para todo lo que importa
// acá, dos procesos: cada uno lleva su propio número de secuencia y su propio
// encadenado en memoria, y los cerrojos de flock y LockFileEx se disputan entre
// descriptores aunque salgan del mismo binario.

// El caso que motiva todo: dos escritores a la vez, sin coordinación previa.
//
// Sin cerrojo, los dos creen que el último evento es el N y los dos escriben el
// N+1: quedan dos eventos con el mismo número y dos ramas de la cadena.
func TestDosEscritoresNoPisanElNumeroDeSecuencia(t *testing.T) {
	vault := t.TempDir()
	uno, err := Open(vault)
	if err != nil {
		t.Fatal(err)
	}
	otro, err := Open(vault)
	if err != nil {
		t.Fatal(err)
	}

	const cada = 40
	var wg sync.WaitGroup
	for _, j := range []*Journal{uno, otro} {
		wg.Add(1)
		go func(j *Journal) {
			defer wg.Done()
			for i := 0; i < cada; i++ {
				if _, err := j.Append(Event{
					NoteID: fmt.Sprintf("n%02d", i), Kind: "CheckDeclared", Emitter: "test",
				}); err != nil {
					t.Error(err)
					return
				}
			}
		}(j)
	}
	wg.Wait()

	// Se lee con un tercero, sin caché de ninguno de los dos escritores.
	lector, err := Open(vault)
	if err != nil {
		t.Fatal(err)
	}
	evs, err := lector.All()
	if err != nil {
		t.Fatal(err)
	}
	if len(evs) != 2*cada {
		t.Errorf("se escribieron %d eventos y quedaron %d", 2*cada, len(evs))
	}
	for i, e := range evs {
		if e.Seq != uint64(i+1) {
			t.Fatalf("el evento %d tiene seq %d: los números se pisaron", i, e.Seq)
		}
	}
	if err := lector.Verificar(); err != nil {
		t.Errorf("la cadena no verifica después de dos escritores: %v", err)
	}
}

// Un escritor y un lector: leer nunca toma el cerrojo, así que no se estorban.
func TestLeerNoEsperaAlQueEscribe(t *testing.T) {
	vault := t.TempDir()
	escritor, err := Open(vault)
	if err != nil {
		t.Fatal(err)
	}
	lector, err := Open(vault)
	if err != nil {
		t.Fatal(err)
	}

	listo := make(chan struct{})
	go func() {
		defer close(listo)
		for i := 0; i < 50; i++ {
			if _, err := escritor.Append(Event{NoteID: "n", Kind: "CheckDeclared", Emitter: "test"}); err != nil {
				t.Error(err)
				return
			}
		}
	}()
	for i := 0; i < 200; i++ {
		evs, err := lector.All()
		if err != nil {
			t.Fatal(err)
		}
		for k, e := range evs {
			if e.Seq != uint64(k+1) {
				t.Fatalf("lectura inconsistente: el evento %d tiene seq %d", k, e.Seq)
			}
		}
	}
	<-listo
	if err := lector.Verificar(); err != nil {
		t.Errorf("la cadena no verifica: %v", err)
	}
}

// El cerrojo es exclusivo de verdad, y se libera.
func TestElCerrojoEsExclusivoYSeLibera(t *testing.T) {
	dir := t.TempDir()
	a, err := bloquear(dir, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := bloquear(dir, 50*time.Millisecond); err == nil {
		t.Fatal("dos cerrojos exclusivos a la vez sobre el mismo registro")
	}
	a.liberar()

	b, err := bloquear(dir, time.Second)
	if err != nil {
		t.Fatalf("no se pudo volver a tomar el cerrojo después de liberarlo: %v", err)
	}
	b.liberar()
}

// Un registro trabado no cuelga la petición para siempre: falla con un mensaje
// que dice qué mirar.
func TestUnRegistroTrabadoFallaConMensaje(t *testing.T) {
	dir := t.TempDir()
	c, err := bloquear(dir, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer c.liberar()

	t0 := time.Now()
	_, err = bloquear(dir, 150*time.Millisecond)
	if err == nil {
		t.Fatal("tomó un cerrojo que estaba en uso")
	}
	if d := time.Since(t0); d > time.Second {
		t.Errorf("esperó %s con un límite de 150ms", d)
	}
	if !strings.Contains(err.Error(), "otro proceso") {
		t.Errorf("el error no dice qué pasó: %v", err)
	}
}

// Verificar tiene que nombrar la colisión de secuencias, no solo decir que la
// cadena se rompió: el que la lee necesita saber que hay dos COGO escribiendo.
//
// Se arma sobre el archivo y no sobre el caché a propósito: Verificar revalida
// contra el disco, así que inyectar en memoria no probaría nada. Esto es lo que
// una colisión dejaría escrito.
func TestVerificarNombraLaColision(t *testing.T) {
	j, vault := abrirCon(t)
	poner(t, j, "a")
	poner(t, j, "b")

	ruta := filepath.Join(vault, ".cogo", "journal", time.Now().UTC().Format("2006-01")+".jsonl")
	crudo, err := os.ReadFile(ruta)
	if err != nil {
		t.Fatal(err)
	}
	lineas := strings.Split(strings.TrimSpace(string(crudo)), "\n")
	if len(lineas) != 2 {
		t.Fatalf("se esperaban 2 líneas, hay %d", len(lineas))
	}
	var segundo Event
	if err := json.Unmarshal([]byte(lineas[1]), &segundo); err != nil {
		t.Fatal(err)
	}
	segundo.Seq = 1 // lo que dejaría el otro proceso: el mismo número otra vez
	b, err := json.Marshal(segundo)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ruta, []byte(lineas[0]+"\n"+string(b)+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	lector, err := Open(vault)
	if err != nil {
		t.Fatal(err)
	}
	err = lector.Verificar()
	if err == nil {
		t.Fatal("no detectó dos eventos con el mismo número de secuencia")
	}
	if !strings.Contains(err.Error(), "dos procesos") {
		t.Errorf("el mensaje no nombra la causa: %v", err)
	}
}
