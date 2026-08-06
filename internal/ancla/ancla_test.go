package ancla

import (
	"strings"
	"testing"
	"time"
)

// La prueba que justifica todo el paquete: un registro reescrito se detecta.
//
// La cadena de hashes sola no puede hacerlo — quien rehace el registro entero
// recalcula todos los digests y queda internamente perfecto. Lo único que lo
// delata es un valor publicado ANTES, afuera.
func TestUnRegistroReescritoNoCoincide(t *testing.T) {
	s := Sello{Seq: 41, Cabeza: "a41f9c", Donde: "commit a1b2c3", Cuando: time.Now()}

	// El registro de hoy da lo mismo que se publicó: todo en orden.
	ok := Verificar([]Sello{s}, func(uint64) (string, bool) { return "a41f9c", true })
	if !ok[0].OK {
		t.Fatalf("un registro intacto dio por roto: %s", ok[0].Dice)
	}

	// Alguien rehizo el registro. Internamente cierra perfecto; contra el sello,
	// no.
	mal := Verificar([]Sello{s}, func(uint64) (string, bool) { return "9e2b77", true })
	if mal[0].OK {
		t.Fatal("un registro reescrito pasó la verificación")
	}
	if !strings.Contains(mal[0].Dice, "se reescribió") {
		t.Errorf("el mensaje no dice qué pasó: %s", mal[0].Dice)
	}
}

// Y un registro truncado tampoco pasa, pero se dice distinto: el problema es
// otro y la explicación también.
func TestUnRegistroTruncadoSeDistingue(t *testing.T) {
	s := Sello{Seq: 900, Cabeza: "a41f9c", Donde: "manual"}
	r := Verificar([]Sello{s}, func(uint64) (string, bool) { return "", false })
	if r[0].OK {
		t.Fatal("pasó con un registro que no llega al evento sellado")
	}
	if !strings.Contains(r[0].Dice, "no llega") {
		t.Errorf("no distinguió truncado de reescrito: %s", r[0].Dice)
	}
}

// Un sello sin destino no se acepta. Es la regla que impide que esto sea teatro:
// un hash guardado al lado del registro que resume no prueba nada contra quien
// tiene los dos archivos.
func TestUnSelloSinDestinoNoSeGuarda(t *testing.T) {
	st := Abrir(t.TempDir())
	err := st.Agregar(Sello{Seq: 1, Cabeza: "abc"})
	if err == nil {
		t.Fatal("aceptó un sello que no dice dónde se publicó")
	}
	if !strings.Contains(err.Error(), "dónde") {
		t.Errorf("el error no explica por qué: %v", err)
	}
	if err := st.Agregar(Sello{Seq: 1, Donde: "manual"}); err == nil {
		t.Error("aceptó un sello sin cabeza")
	}
}

func TestLosSellosSobrevivenYVienenDelMasNuevo(t *testing.T) {
	dir := t.TempDir()
	st := Abrir(dir)
	for _, s := range []Sello{
		{Seq: 10, Cabeza: "aaa", Donde: "manual", Nota: "primero"},
		{Seq: 40, Cabeza: "ccc", Donde: "manual", Nota: "tercero"},
		{Seq: 25, Cabeza: "bbb", Donde: "manual", Nota: "segundo"},
	} {
		if err := st.Agregar(s); err != nil {
			t.Fatal(err)
		}
	}

	otro := Abrir(dir) // otro proceso, sin estado en memoria
	xs, err := otro.Todos()
	if err != nil {
		t.Fatal(err)
	}
	if len(xs) != 3 {
		t.Fatalf("volvieron %d sellos de 3", len(xs))
	}
	if xs[0].Seq != 40 {
		t.Errorf("el primero es el evento %d; se esperaba el más nuevo (40)", xs[0].Seq)
	}
	if u, hay := otro.Ultimo(); !hay || u.Seq != 40 {
		t.Errorf("Ultimo dio %v/%v", u.Seq, hay)
	}
}

// El destino manual no publica nada, y eso es a propósito: si COGO publicara
// solo, el sello sería un archivo que COGO se manda a sí mismo.
func TestElDestinoManualNoPublicaNada(t *testing.T) {
	recibo, err := Publicar(DestinoManual, "", 41, "a41f9c")
	if err != nil || recibo != "" {
		t.Errorf("el destino manual publicó algo: %q %v", recibo, err)
	}
	texto := ComoPublicarloAMano(41, "a41f9c")
	for _, debe := range []string{"41", "a41f9c", "no puedas reescribir"} {
		if !strings.Contains(texto, debe) {
			t.Errorf("las instrucciones no mencionan %q:\n%s", debe, texto)
		}
	}
}

func TestUnDestinoHTTPSSinURLFalla(t *testing.T) {
	if _, err := Publicar(DestinoHTTPS, "", 1, "abc"); err == nil {
		t.Error("aceptó publicar por https sin URL")
	}
	if _, err := Publicar("inventado", "", 1, "abc"); err == nil {
		t.Error("aceptó un destino que no existe")
	}
}
