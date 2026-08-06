package accion

import (
	"fmt"
	"strings"

	"github.com/diegoparras/cogo/internal/parametros"
)

// Peticion es lo que un agente pregunta antes de actuar.
type Peticion struct {
	// Accion en palabras: qué está por hacer. Es lo que se clasifica.
	Accion string
	// Clase declarada por quien pide. Vacía se permite: se infiere.
	Clase string
	// Notas son los ids en los que se apoya. La clave está en el plural: una
	// acción se apoya en varias cosas y basta que UNA sea floja para que el
	// conjunto lo sea. Es la misma regla del punto fijo, aplicada al pedido.
	Notas []string
}

// Faltante es una nota que no llega, con qué le falta y qué hacer.
type Faltante struct {
	NoteID   string `json:"note_id"`
	Estado   string `json:"estado"`
	Necesita string `json:"necesita"`
	Como     string `json:"como"`
}

// Veredicto es la respuesta. Autoriza es la única parte que un agente necesita
// leer para obedecer; el resto es para que un humano pueda discutirla.
type Veredicto struct {
	Autoriza bool   `json:"autoriza"`
	Clase    string `json:"clase"`
	// PorQueClase explica cómo se determinó la clase. Importa cuando el texto
	// contradijo lo declarado: ahí es donde alguien va a querer discutir.
	PorQueClase string `json:"por_que_clase"`
	// Necesita es el estado mínimo para esta clase; Apoyo el más débil de las
	// notas citadas. Autoriza es, exactamente, Apoyo >= Necesita.
	Necesita string     `json:"necesita"`
	Apoyo    string     `json:"apoyo,omitempty"`
	Porque   string     `json:"porque"`
	Falta    []Faltante `json:"falta,omitempty"`
	// Brechas son preguntas abiertas citadas como si fueran respaldo. Merecen su
	// propio campo porque no son notas flojas: son notas que dicen "no se sabe".
	Brechas []string `json:"brechas,omitempty"`
	// Bloqueo es un rechazo que NO viene de la evidencia: otro agente ya tomó
	// esto. Va en su propio campo porque la salida es distinta — no se arregla
	// verificando nada, se arregla hablando con el otro o esperando.
	Bloqueo string `json:"bloqueo,omitempty"`
}

// Fuente es lo que Autorizar necesita saber del vault. Es una interfaz chica a
// propósito: mantiene este paquete sin depender del motor, y hace que la regla
// se pueda testear con un mapa.
type Fuente interface {
	// Estado devuelve el estado del retículo de una nota, y si existe.
	Estado(id string) (string, bool)
	// EsBrecha dice si esa nota es una pregunta abierta.
	EsBrecha(id string) bool
}

func rango(estado string) int {
	for i, e := range parametros.EstadosOrdenados {
		if e == estado {
			return i
		}
	}
	return -1
}

// comoSubir es el paso siguiente concreto para cada estado. Es la parte del
// veredicto que lo vuelve accionable: "no alcanza" sin un "hacé esto" es un
// obstáculo, no un control.
var comoSubir = map[string]string{
	parametros.Asserted:      "declarale un check: cuál sería el test que la probaría",
	parametros.CheckDeclared: "corré el check con el runner, o declará que pasa si lo comprobaste",
	parametros.ClaimedPassed: "el check está declarado como pasado pero nadie lo ejecutó: corrélo con el runner",
	parametros.Stale:         "venció su ventana de frescura: re-verificala",
	parametros.Contradicted:  "resolvé la contradicción abierta, o sumale evidencia observada",
	parametros.Refuted:       "el check se ejecutó y falló: arreglá lo que afirma, o corregí la nota",
	parametros.Quarantined:   "está en cuarentena a propósito: no se puede usar como respaldo",
}

// Autorizar aplica la política: clase de la acción → umbral → estado de las
// notas en las que se apoya.
func Autorizar(p Peticion, f Fuente, pars *parametros.Set) Veredicto {
	clase, porQue := Decidir(p.Clase, p.Accion)
	necesita := pars.Texto(ClaveParametro[clase])
	v := Veredicto{Clase: string(clase), PorQueClase: porQue, Necesita: necesita}

	notas := limpiar(p.Notas)
	if len(notas) == 0 {
		if !pars.Bool("accion.exigir_respaldo") {
			v.Autoriza = true
			v.Porque = "no se declaró respaldo, y este vault no lo exige"
			return v
		}
		v.Porque = "no declaraste en qué te apoyás. Una acción " + Rotulo[clase] +
			" necesita al menos una nota que la sostenga: pedí contexto con pack y citá los ids."
		return v
	}

	peor := ""
	for _, id := range notas {
		if f.EsBrecha(id) {
			v.Brechas = append(v.Brechas, id)
			continue
		}
		est, existe := f.Estado(id)
		if !existe {
			v.Falta = append(v.Falta, Faltante{NoteID: id, Estado: "no existe",
				Necesita: necesita, Como: "esa nota no está en el vault: revisá el id"})
			continue
		}
		if rango(est) < 0 {
			// Un estado que no está en el retículo no se puede comparar. Bloquear
			// es lo correcto —no se puede afirmar que alcance— pero hay que decir
			// que el problema es del sistema y no de la nota, o alguien va a
			// perder una tarde tratando de "subir" una nota que ya estaba bien.
			v.Falta = append(v.Falta, Faltante{NoteID: id, Estado: est, Necesita: necesita,
				Como: "COGO no reconoce ese estado: es un problema del motor, no de la nota"})
			continue
		}
		if rango(est) < rango(necesita) {
			v.Falta = append(v.Falta, Faltante{NoteID: id, Estado: est, Necesita: necesita,
				Como: comoSubir[est]})
		}
		if peor == "" || rango(est) < rango(peor) {
			peor = est
		}
	}
	v.Apoyo = peor

	switch {
	case len(v.Brechas) > 0:
		v.Porque = fmt.Sprintf("citaste una pregunta abierta como si fuera respaldo (%s). "+
			"Una brecha declara que algo NO se sabe: no sostiene nada. Resolvela primero.",
			strings.Join(v.Brechas, ", "))
	case len(v.Falta) > 0:
		v.Porque = fmt.Sprintf("una acción %s necesita respaldo %s, y %s no llega%s",
			Rotulo[clase], necesita, plural(len(v.Falta), "la nota", "las notas"), listar(v.Falta))
	default:
		v.Autoriza = true
		v.Porque = fmt.Sprintf("acción %s: todo lo que citás llega a %s o más", Rotulo[clase], necesita)
	}
	return v
}

func listar(f []Faltante) string {
	var b strings.Builder
	for _, x := range f {
		fmt.Fprintf(&b, "\n  · %s está en %s — %s", x.NoteID, x.Estado, x.Como)
	}
	return b.String()
}

func plural(n int, uno, muchos string) string {
	if n == 1 {
		return uno
	}
	return muchos
}

func limpiar(ids []string) []string {
	var out []string
	visto := map[string]bool{}
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" || visto[id] {
			continue
		}
		visto[id] = true
		out = append(out, id)
	}
	return out
}
