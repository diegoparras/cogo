package supervivencia

import (
	"testing"
)

func viva(tipo string, dias int) Observacion { return Observacion{Tipo: tipo, Dias: dias} }
func murio(tipo string, dias int) Observacion {
	return Observacion{Tipo: tipo, Dias: dias, Fallo: true}
}
func repetir(o Observacion, n int) []Observacion {
	out := make([]Observacion, n)
	for i := range out {
		out[i] = o
	}
	return out
}

// El error que este paquete existe para no cometer: promediar solo las notas que
// fallaron. Es preguntar cuánto vive la gente encuestando velorios.
//
// Acá 10 notas fallan a los 10 días y 90 siguen vivas a los 300. El promedio de
// las muertas daría 10; la respuesta correcta está mucho más arriba, porque el
// 90% de la población nunca falló.
func TestNoEsElPromedioDeLasQueFallaron(t *testing.T) {
	obs := append(repetir(murio("command", 10), 10), repetir(viva("command", 300), 90)...)
	e := Estimar(obs, 30, 20)["command"]

	if e.Suficiente {
		// Con 10% de fallos, la curva nunca baja al 80%: no hay ventana al 20%.
		t.Fatalf("estimó una ventana donde la curva no llega al corte: %d días", e.Ventana)
	}
	if e.Motivo == "" {
		t.Error("no explicó por qué no hay estimación")
	}

	// Con un corte del 5%, sí hay respuesta — y NO son 10 días de promedio.
	e = Estimar(obs, 30, 5)["command"]
	if !e.Suficiente {
		t.Fatalf("con corte al 5%% tendría que haber ventana: %s", e.Motivo)
	}
	if e.Ventana != 10 {
		t.Errorf("ventana = %d, se esperaba 10 (el escalón donde cae al 90%%)", e.Ventana)
	}
}

// La censura es información, no un dato faltante — y CUÁNDO se censuró cambia
// la respuesta. Los mismos cinco fallos, a los mismos 100 días, dan una
// estimación distinta según cuánto se alcanzó a observar al resto.
//
// Es la prueba de que el estimador usa las observaciones censuradas y no las
// descarta: si las tirara, los dos casos serían idénticos.
func TestCuandoSeCensuraCambiaLaRespuesta(t *testing.T) {
	// Caso A: 95 notas siguen vivas a los 200 días. Cinco fallos sobre una
	// población entera de 100 es un 5%: la curva no llega al corte del 20%.
	a := Estimar(append(repetir(murio("bug", 100), 5), repetir(viva("bug", 200), 95)...), 20, 20)["bug"]
	if a.Suficiente {
		t.Errorf("5 fallos sobre 100 observadas cruzaron el corte del 20%%: %d días", a.Ventana)
	}

	// Caso B: las mismas 95 se dejaron de observar a los 10 días — son notas
	// nuevas, no notas que sobrevivieron. Para el día 100 quedaban 5 en riesgo y
	// fallaron las 5: la curva se desploma, y la ventana existe.
	b := Estimar(append(repetir(murio("bug", 100), 5), repetir(viva("bug", 10), 95)...), 20, 20)["bug"]
	if !b.Suficiente {
		t.Fatalf("con las 95 censuradas temprano tendría que haber ventana: %s", b.Motivo)
	}
	if b.Ventana != 100 {
		t.Errorf("ventana = %d, se esperaba 100", b.Ventana)
	}
}

// La ventana se pone donde la curva cruza el corte, no en el primer fallo.
func TestLaVentanaVaDondeLaCurvaCruzaElCorte(t *testing.T) {
	var obs []Observacion
	// 100 notas: van muriendo de a 10 cada 10 días.
	for i := 1; i <= 10; i++ {
		obs = append(obs, repetir(murio("decision", i*10), 10)...)
	}
	e := Estimar(obs, 30, 20)["decision"]
	if !e.Suficiente {
		t.Fatal(e.Motivo)
	}
	if e.Ventana != 20 {
		t.Errorf("ventana = %d, se esperaba 20: a los 20 días ya murió el 20%%", e.Ventana)
	}
}

// Sin datos suficientes se dice, no se inventa.
func TestConPocasNotasNoSeEstima(t *testing.T) {
	e := Estimar(repetir(murio("runbook", 5), 4), 30, 20)["runbook"]
	if e.Suficiente {
		t.Error("estimó una ventana con 4 observaciones")
	}
	if e.Ventana != 0 {
		t.Errorf("devolvió una ventana igual: %d", e.Ventana)
	}
}

// Un tipo donde nunca falló nada no tiene curva. Decir "dura para siempre" sería
// una conclusión, y no hay ninguna.
func TestSinFallosNoHayCurva(t *testing.T) {
	e := Estimar(repetir(viva("constraint", 500), 50), 30, 20)["constraint"]
	if e.Suficiente {
		t.Error("estimó una ventana sin un solo fallo observado")
	}
	if e.Fallos != 0 || e.Vivas != 50 {
		t.Errorf("mal contadas: fallos=%d vivas=%d", e.Fallos, e.Vivas)
	}
}

// Kaplan-Meier: la proporción viva solo puede bajar, y nunca por debajo de cero.
func TestLaCurvaSoloBaja(t *testing.T) {
	obs := []Observacion{
		murio("x", 5), viva("x", 6), murio("x", 10), viva("x", 12),
		murio("x", 20), viva("x", 25), murio("x", 30), viva("x", 40),
	}
	c := kaplanMeier(obs)
	if len(c) == 0 {
		t.Fatal("curva vacía")
	}
	prev := 1.0
	for _, p := range c {
		if p.Vivas > prev {
			t.Errorf("la curva subió: %v después de %v", p.Vivas, prev)
		}
		if p.Vivas < 0 {
			t.Errorf("proporción negativa: %v", p.Vivas)
		}
		prev = p.Vivas
	}
}
