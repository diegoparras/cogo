package uso

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func enMomento(t *testing.T, vault string, ahora time.Time) *Store {
	t.Helper()
	s := Abrir(vault)
	s.SetReloj(func() time.Time { return ahora })
	return s
}

// La propiedad que hace que desplegar esto no borre nada de golpe: una nota sin
// registro no es una que nadie consultó, es una que nadie consultó DESDE QUE SE
// EMPEZÓ A MIRAR.
//
// Sin esto, el día que esta versión sale, cada nota vencida del vault se
// volvería latente a la vez — que es exactamente el estreno que nadie quiere.
func TestUnaNotaSinRegistroCuentaDesdeQueSeEmpezoAMirar(t *testing.T) {
	vault := t.TempDir()
	s := enMomento(t, vault, time.Now().UTC())

	// Diez días después de que se EMPEZÓ A MIRAR (que lo fija Abrir, no el
	// reloj inyectado), una nota que nunca se consultó lleva diez días sin
	// consultar. No dos años, aunque la nota tenga dos años.
	diezDias := s.Desde().Add(10 * 24 * time.Hour)
	if d := s.SinConsultar("jamas-vista", diezDias); d != 10*24*time.Hour {
		t.Errorf("una nota sin registro lleva %v sin consultar; se esperaban 10 días", d)
	}
}

func TestConsultarReiniciaElReloj(t *testing.T) {
	vault := t.TempDir()
	s := enMomento(t, vault, time.Now().UTC())

	// Se cuenta desde s.Desde(), que lo fija Abrir con el reloj real. Tomar una
	// marca propia antes de abrir hace que la diferencia sean 100 días MENOS unos
	// microsegundos: en Windows el reloj es lo bastante grueso como para que
	// coincidan y el test pase igual; en Linux no, y ahí se cae.
	tarde := s.Desde().Add(100 * 24 * time.Hour)
	if d := s.SinConsultar("n", tarde); d != 100*24*time.Hour {
		t.Fatalf("antes de consultarla: %v", d)
	}
	s.SetReloj(func() time.Time { return tarde })
	s.Consultada("n")
	if d := s.SinConsultar("n", tarde); d != 0 {
		t.Errorf("después de consultarla quedó en %v; tendría que ser cero", d)
	}
	if s.Veces("n") != 1 {
		t.Errorf("veces = %d", s.Veces("n"))
	}
}

// Un pack entrega decenas de notas juntas: se anotan en una sola pasada.
func TestSeAnotanVariasDeUnaVez(t *testing.T) {
	s := enMomento(t, t.TempDir(), time.Now().UTC())
	s.Consultada("a", "b", "", "c") // el vacío se ignora
	for _, id := range []string{"a", "b", "c"} {
		if _, hay := s.Ultima(id); !hay {
			t.Errorf("%s no quedó registrada", id)
		}
	}
	if _, hay := s.Ultima(""); hay {
		t.Error("se registró un id vacío")
	}
}

// El registro sobrevive el reinicio, incluida la fecha de inicio: si se
// reiniciara en cada arranque, nada llegaría nunca al umbral.
func TestElRegistroSobreviveElReinicio(t *testing.T) {
	vault := t.TempDir()
	if err := os.MkdirAll(filepath.Join(vault, ".cogo"), 0o755); err != nil {
		t.Fatal(err)
	}
	t0 := time.Now().UTC().Add(-365 * 24 * time.Hour)
	s := enMomento(t, vault, t0)
	s.Consultada("n")
	if err := s.Guardar(); err != nil {
		t.Fatal(err)
	}

	otro := Abrir(vault)
	if !otro.Desde().Equal(s.Desde()) {
		t.Errorf("la fecha de inicio se reinició: %v vs %v", otro.Desde(), s.Desde())
	}
	if otro.Veces("n") != 1 {
		t.Errorf("se perdió el registro: veces = %d", otro.Veces("n"))
	}
	if u, hay := otro.Ultima("n"); !hay || !u.Equal(t0) {
		t.Errorf("la última consulta volvió mal: %v (hay=%v)", u, hay)
	}
}

// Un archivo ilegible no rompe nada: se arranca de cero, que es la respuesta
// correcta — el registro de uso es una optimización, no la verdad del vault.
func TestUnArchivoRotoNoRompeNada(t *testing.T) {
	vault := t.TempDir()
	if err := os.MkdirAll(filepath.Join(vault, ".cogo"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(vault, ".cogo", "uso.json"), []byte("{roto"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := Abrir(vault)
	if s.Desde().IsZero() {
		t.Error("se quedó sin fecha de inicio: todo contaría como consultado hace un instante o hace una eternidad")
	}
	if s.Veces("x") != 0 {
		t.Error("inventó registros")
	}
}

// Olvidar saca los ids de notas que ya no existen, o el archivo crecería para
// siempre con la basura de cada nota borrada.
func TestSeLimpianLasNotasBorradas(t *testing.T) {
	vault := t.TempDir()
	s := enMomento(t, vault, time.Now().UTC())
	s.Consultada("viva", "borrada")

	s.Olvidar(map[string]bool{"viva": true})
	if _, hay := s.Ultima("borrada"); hay {
		t.Error("quedó el registro de una nota que ya no existe")
	}
	if _, hay := s.Ultima("viva"); !hay {
		t.Error("se limpió una nota que sí existe")
	}
}

// Guardar no escribe si no hay nada nuevo: un pack por segundo no puede ser una
// escritura por segundo.
func TestNoSeEscribeSiNoCambioNada(t *testing.T) {
	vault := t.TempDir()
	if err := os.MkdirAll(filepath.Join(vault, ".cogo"), 0o755); err != nil {
		t.Fatal(err)
	}
	s := enMomento(t, vault, time.Now().UTC())
	s.Consultada("n")
	if err := s.Guardar(); err != nil {
		t.Fatal(err)
	}
	ruta := filepath.Join(vault, ".cogo", "uso.json")
	antes, err := os.Stat(ruta)
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(5 * time.Millisecond)
	if err := s.Guardar(); err != nil { // nada cambió
		t.Fatal(err)
	}
	despues, err := os.Stat(ruta)
	if err != nil {
		t.Fatal(err)
	}
	if !antes.ModTime().Equal(despues.ModTime()) {
		t.Error("volvió a escribir el archivo sin que hubiera nada nuevo")
	}
}
