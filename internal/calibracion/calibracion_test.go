package calibracion

import (
	"testing"

	"github.com/diegoparras/cogo/internal/journal"
)

func decl(nota, quien string) journal.Event {
	return journal.Event{NoteID: nota, Kind: "VerifyDeclared", Emitter: quien}
}

func ejec(nota string, ok bool) journal.Event {
	g := "ejecucion_ok"
	if !ok {
		g = "ejecucion_falla"
	}
	return journal.Event{NoteID: nota, Kind: "CheckExecuted", Emitter: journal.EmisorEjecucion, Guard: g}
}

// El denominador es lo que hace honesta a toda la métrica: solo cuentan las
// declaraciones que después fueron puestas a prueba. Contar como acierto lo que
// nadie verificó le regalaría una tasa perfecta a quien declare mucho y
// verifique poco — el comportamiento que habría que castigar.
func TestSoloCuentanLasDeclaracionesQueSePusieronAPrueba(t *testing.T) {
	evs := []journal.Event{
		decl("a", "agente-x"), ejec("a", true),
		decl("b", "agente-x"), ejec("b", false),
		decl("c", "agente-x"), // nadie la ejecutó: no cuenta para ningún lado
		decl("d", "agente-x"),
		decl("e", "agente-x"),
	}
	inf := Calcular(evs, 2, 10)
	e := inf.Emisores[0]
	if e.Declaraciones != 5 {
		t.Errorf("declaraciones = %d, se esperaban 5", e.Declaraciones)
	}
	if e.Comprobadas != 2 || e.Confirmadas != 1 || e.Desmentidas != 1 {
		t.Errorf("comprobadas=%d confirmadas=%d desmentidas=%d — solo dos se pusieron a prueba",
			e.Comprobadas, e.Confirmadas, e.Desmentidas)
	}
	if e.Tasa != 0.5 {
		t.Errorf("tasa = %v, se esperaba 0.5 (1 de 2), no 1 de 5", e.Tasa)
	}
}

// Con una muestra chica no se acusa a nadie, aunque el promedio sea horrible.
// Es toda la razón por la que se penaliza por la cota y no por la tasa.
func TestUnaMuestraChicaNoAcusaANadie(t *testing.T) {
	evs := []journal.Event{
		decl("a", "novato"), ejec("a", false),
		decl("b", "novato"), ejec("b", false),
		decl("c", "novato"), ejec("c", false),
	}
	inf := Calcular(evs, 20, 10)
	e := inf.Emisores[0]
	if e.Tasa != 1 {
		t.Fatalf("la tasa observada tendría que ser 1: %v", e.Tasa)
	}
	if e.Penalizado {
		t.Error("penalizó a un emisor con tres casos: tres fallos de tres no sostienen ninguna conclusión")
	}
	if e.Suficiente {
		t.Error("dio por suficiente una muestra de 3 con mínimo 20")
	}
}

// Y con muestra grande sí.
func TestConDatosSuficientesSePenaliza(t *testing.T) {
	var evs []journal.Event
	for i := 0; i < 40; i++ {
		id := string(rune('a' + i%26))
		id = id + string(rune('0'+i/26))
		evs = append(evs, decl(id, "descuidado"), ejec(id, i%2 == 0)) // 50% de desmentidas
	}
	inf := Calcular(evs, 20, 10)
	e := inf.Emisores[0]
	if !e.Suficiente {
		t.Fatalf("40 comprobaciones no alcanzaron el mínimo de 20: %+v", e)
	}
	if !e.Penalizado {
		t.Errorf("50%% de desmentidas sobre 40 casos no penalizó (cota inferior %.3f)", e.CotaInferior)
	}
	if e.CotaInferior >= e.Tasa {
		t.Errorf("la cota inferior (%.3f) no puede ser mayor que la tasa observada (%.3f)", e.CotaInferior, e.Tasa)
	}
}

// El runner no declara: ejecuta. Su código de salida no es la palabra de nadie.
func TestElRunnerNoEsUnEmisorQueSeJuzgue(t *testing.T) {
	evs := []journal.Event{
		{NoteID: "a", Kind: "VerifyDeclared", Emitter: journal.EmisorEjecucion},
		ejec("a", false),
	}
	if inf := Calcular(evs, 1, 10); len(inf.Emisores) != 0 {
		t.Errorf("se juzgó al runner: %+v", inf.Emisores)
	}
}

// Una ejecución sin declaración previa no juzga a nadie: no había palabra que
// contradecir.
func TestUnaEjecucionSolaNoJuzga(t *testing.T) {
	if inf := Calcular([]journal.Event{ejec("a", false)}, 1, 10); len(inf.Emisores) != 0 {
		t.Errorf("una ejecución sin declaración previa acusó a alguien: %+v", inf.Emisores)
	}
}

// Sin datos, el informe lo dice en vez de mostrar ceros que parecen resultados.
func TestSinDatosSeDice(t *testing.T) {
	inf := Calcular([]journal.Event{decl("a", "x")}, 20, 10)
	if !inf.SinDatos {
		t.Error("con declaraciones pero ninguna comprobación, el informe tendría que decir que no hay datos")
	}
}
