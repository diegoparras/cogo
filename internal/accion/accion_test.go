package accion

import (
	"strings"
	"testing"

	"github.com/diegoparras/cogo/internal/parametros"
)

// La propiedad que sostiene todo el módulo: declarar una clase más baja no puede
// bajar la exigencia. Si pudiera, el control sería voluntario.
func TestDeclararMasBajoNoBajaLaExigencia(t *testing.T) {
	casos := []struct {
		declarada string
		texto     string
		quiero    Clase
	}{
		{"informative", "borrar los registros viejos de la tabla de sesiones", Irreversible},
		{"reversible", "hacer un force push a main", Irreversible},
		{"informative", "desplegar la versión nueva a producción", Costosa},
		{"reversible", "explicar cómo funciona el pool", Reversible}, // declarar MÁS alto sí sube
		{"irreversible", "leer el archivo de configuración", Irreversible},
	}
	for _, c := range casos {
		got, porQue := Decidir(c.declarada, c.texto)
		if got != c.quiero {
			t.Errorf("Decidir(%q, %q) = %q, se esperaba %q\n  %s", c.declarada, c.texto, got, c.quiero, porQue)
		}
	}
}

// Sin nada que clasificar, se pide lo máximo. Es la única política segura: no
// saber qué se está por hacer no puede ser más barato que saberlo.
func TestLoDesconocidoPideElMaximo(t *testing.T) {
	if got, _ := Decidir("", "hacer lo que hablamos"); got != Irreversible {
		t.Errorf("una acción sin clase ni señales quedó en %q", got)
	}
	if got, _ := Decidir("supercalifragilistico", "explicar algo"); got != Irreversible {
		t.Errorf("una clase inventada quedó en %q — un typo no puede ser una forma de esquivar el control", got)
	}
}

// La señal más grave manda, aunque esté rodeada de señales suaves.
func TestGanaLaSeñalMasGrave(t *testing.T) {
	texto := "leer el config, revisar los logs, y después borrar la carpeta de temporales"
	inf := Clasificar(texto)
	if inf.Clase != Irreversible {
		t.Fatalf("clase %q: una acción que menciona un borrado es un borrado", inf.Clase)
	}
	if len(inf.Porques) == 0 {
		t.Error("no dijo qué la disparó: un veredicto que no se puede discutir no sirve")
	}
}

// "borrador" no puede disparar "borrar".
func TestNoDisparaPorPedazosDePalabra(t *testing.T) {
	if inf := Clasificar("escribir un borrador del informe"); inf.Clase == Irreversible {
		t.Errorf("\"borrador\" disparó la clase irreversible (%v)", inf.Porques)
	}
}

// ── el autorizador ──────────────────────────────────────────────────────────

type fuenteFalsa struct {
	estados map[string]string
	brechas map[string]bool
}

func (f fuenteFalsa) Estado(id string) (string, bool) { e, ok := f.estados[id]; return e, ok }
func (f fuenteFalsa) EsBrecha(id string) bool         { return f.brechas[id] }

func TestUnaAccionIrreversibleExigeUnCheckEjecutado(t *testing.T) {
	pars := parametros.Defaults()
	f := fuenteFalsa{estados: map[string]string{
		"declarada": parametros.ClaimedPassed,
		"ejecutada": parametros.Verified,
	}}

	// Declarado no alcanza: es toda la razón de existir del runner.
	v := Autorizar(Peticion{Accion: "borrar la tabla de sesiones", Notas: []string{"declarada"}}, f, pars)
	if v.Autoriza {
		t.Errorf("un borrado se autorizó con un check solo declarado: %s", v.Porque)
	}
	if len(v.Falta) != 1 || v.Falta[0].Como == "" {
		t.Errorf("no dijo qué hacer para destrabarlo: %+v", v.Falta)
	}

	v = Autorizar(Peticion{Accion: "borrar la tabla de sesiones", Notas: []string{"ejecutada"}}, f, pars)
	if !v.Autoriza {
		t.Errorf("un borrado con un check ejecutado no se autorizó: %s", v.Porque)
	}
}

// La misma nota alcanza o no según para qué. Es la tesis entera de la fase.
func TestLaMismaNotaAlcanzaONoSegunLaAccion(t *testing.T) {
	pars := parametros.Defaults()
	f := fuenteFalsa{estados: map[string]string{"n": parametros.CheckDeclared}}

	if v := Autorizar(Peticion{Accion: "explicar cómo funciona el cobro", Notas: []string{"n"}}, f, pars); !v.Autoriza {
		t.Errorf("no dejó explicar algo apoyado en una nota con check declarado: %s", v.Porque)
	}
	if v := Autorizar(Peticion{Accion: "desplegar a producción", Notas: []string{"n"}}, f, pars); v.Autoriza {
		t.Error("dejó desplegar con la misma nota que solo sirve para explicar")
	}
}

// Basta que UNA sea floja. Uno se apoya en el conjunto, y el conjunto vale lo
// que su parte más débil.
func TestLaNotaMasDebilManda(t *testing.T) {
	pars := parametros.Defaults()
	f := fuenteFalsa{estados: map[string]string{
		"fuerte": parametros.Verified,
		"floja":  parametros.Asserted,
	}}
	v := Autorizar(Peticion{Accion: "migrar la base", Notas: []string{"fuerte", "floja"}}, f, pars)
	if v.Autoriza {
		t.Fatal("autorizó apoyado en un conjunto con una nota sin respaldo")
	}
	if !strings.Contains(v.Porque, "floja") {
		t.Errorf("no nombró cuál es la floja: %s", v.Porque)
	}
}

// Una brecha no es respaldo flojo: no es respaldo. Merece su propio mensaje
// porque el error que comete quien la cita es distinto.
func TestUnaBrechaNoSostieneNada(t *testing.T) {
	pars := parametros.Defaults()
	f := fuenteFalsa{brechas: map[string]bool{"pool-bajo-carga": true}}
	v := Autorizar(Peticion{Accion: "desplegar", Notas: []string{"pool-bajo-carga"}}, f, pars)
	if v.Autoriza {
		t.Fatal("autorizó apoyado en una pregunta abierta")
	}
	if len(v.Brechas) != 1 || !strings.Contains(v.Porque, "pregunta abierta") {
		t.Errorf("no explicó que era una brecha: %s", v.Porque)
	}
}

// Sin respaldo declarado no hay nada que evaluar.
func TestSinRespaldoNoSeAutoriza(t *testing.T) {
	pars := parametros.Defaults()
	f := fuenteFalsa{}
	if v := Autorizar(Peticion{Accion: "borrar todo"}, f, pars); v.Autoriza {
		t.Error("autorizó una acción que no declaró en qué se apoya")
	}
	// Salvo que el vault lo haya decidido a propósito.
	if err := pars.Poner("accion.exigir_respaldo", false, "test"); err != nil {
		t.Fatal(err)
	}
	if v := Autorizar(Peticion{Accion: "borrar todo"}, f, pars); !v.Autoriza {
		t.Error("el vault apagó la exigencia y siguió bloqueando")
	}
}

// Los umbrales son del vault, no del código.
func TestLosUmbralesSonDelVault(t *testing.T) {
	pars := parametros.Defaults()
	f := fuenteFalsa{estados: map[string]string{"n": parametros.Asserted}}
	if v := Autorizar(Peticion{Accion: "borrar la base", Notas: []string{"n"}}, f, pars); v.Autoriza {
		t.Fatal("autorizó un borrado con una nota apenas capturada")
	}
	if err := pars.Poner("accion.irreversible", parametros.Asserted, "test"); err != nil {
		t.Fatal(err)
	}
	if v := Autorizar(Peticion{Accion: "borrar la base", Notas: []string{"n"}}, f, pars); !v.Autoriza {
		t.Error("se bajó el umbral a mano y siguió bloqueando")
	}
}

// Una nota que no existe no puede sostener nada, y hay que decirlo distinto de
// "está floja": el problema es otro y la solución también.
func TestUnaNotaInexistenteNoSostiene(t *testing.T) {
	v := Autorizar(Peticion{Accion: "desplegar", Notas: []string{"fantasma"}}, fuenteFalsa{}, parametros.Defaults())
	if v.Autoriza {
		t.Fatal("autorizó apoyado en una nota que no existe")
	}
	if v.Falta[0].Estado != "no existe" {
		t.Errorf("no distinguió una nota ausente de una floja: %+v", v.Falta[0])
	}
}
