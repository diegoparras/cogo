// Package calibracion mide cuánto vale la palabra de cada emisor.
//
// # LA PREGUNTA
//
// COGO distingue un check DECLARADO de uno EJECUTADO desde la fase 4. Un agente
// que dice "el check pasa" produce `claimed_passed`; solo el runner produce
// `verified`. Eso ya es honesto, pero trata a todos los emisores igual: la
// palabra de alguien que nunca se equivocó vale lo mismo que la de alguien que
// declara cualquier cosa.
//
// El journal tiene con qué responderlo. Cada declaración quedó registrada con su
// emisor, y cada ejecución posterior del mismo check dice si esa declaración era
// cierta. Contar es todo lo que hace falta.
//
// # EL DENOMINADOR ES LO IMPORTANTE
//
// La tentación es dividir las desmentidas por el total de declaraciones. Estaría
// mal: la mayoría de las declaraciones nunca se ejecutan, y contarlas como
// aciertos regalaría una tasa de error diminuta a cualquiera que declare mucho y
// verifique poco — exactamente el comportamiento que habría que castigar.
//
// El denominador son las declaraciones que DESPUÉS FUERON PUESTAS A PRUEBA. Es
// un número mucho más chico, y por eso este módulo viene apagado: hasta que un
// vault acumule ejecuciones reales, cualquier conclusión sería ruido con forma de
// estadística.
//
// # POR QUÉ HAY UN INTERVALO Y NO UN PROMEDIO
//
// Dos desmentidas sobre tres pruebas da 67%, que suena catastrófico y no
// significa nada. La cota inferior de Wilson responde otra pregunta: cuál es el
// piso de la tasa de error que los datos SOSTIENEN. Con tres pruebas ese piso es
// bajísimo aunque las tres hayan fallado, y con doscientas se acerca a la tasa
// observada. Penalizar por la cota y no por el promedio es lo que hace que una
// muestra chica no acuse a nadie.
package calibracion

import (
	"encoding/json"
	"math"
	"sort"
	"strings"

	"github.com/diegoparras/cogo/internal/journal"
)

// Emisor es la hoja de un emisor.
type Emisor struct {
	Nombre string `json:"nombre"`
	// Declaraciones es todo lo que dijo. Comprobadas son las que una ejecución
	// posterior puso a prueba: el denominador honesto.
	Declaraciones int `json:"declaraciones"`
	Comprobadas   int `json:"comprobadas"`
	Confirmadas   int `json:"confirmadas"`
	Desmentidas   int `json:"desmentidas"`
	// Tasa es la proporción observada de desmentidas; CotaInferior es el piso que
	// los datos sostienen con 95% de confianza. Se penaliza por la cota.
	Tasa         float64 `json:"tasa"`
	CotaInferior float64 `json:"cota_inferior"`
	// Suficiente dice si hay bastantes comprobaciones como para concluir algo.
	Suficiente bool `json:"suficiente"`
	// Penalizado es el veredicto final, ya contra el umbral del vault.
	Penalizado bool `json:"penalizado"`
}

// Informe es lo que se le muestra a alguien, con el contexto que hace falta para
// leerlo sin sacar conclusiones de más.
type Informe struct {
	Emisores []Emisor `json:"emisores"`
	// Activa dice si esto está afectando los colores o solo mirando.
	Activa bool `json:"activa"`
	Minimo int  `json:"minimo"`
	// Tolerado es el porcentaje de desmentidas que el vault acepta.
	Tolerado int `json:"tolerado"`
	// SinDatos es la lectura honesta cuando todavía no hay nada que decir.
	SinDatos bool `json:"sin_datos"`
}

// zeta es 1.96: el 95% de confianza de toda la vida. Es una elección, no una
// constante de la naturaleza, y está acá arriba para que se vea que lo es.
const zeta = 1.96

// Calcular arma la hoja de cada emisor a partir del journal.
//
// La regla, por nota: la última declaración manda, y la primera ejecución
// posterior la juzga. Si nadie ejecutó nada después, esa declaración no cuenta
// para ningún lado — no es un acierto, es una incógnita.
func Calcular(evs []journal.Event, minimo, tolerado int) Informe {
	type pendiente struct {
		emisor string
		vivo   bool
	}
	ultima := map[string]pendiente{} // por nota
	cuenta := map[string]*Emisor{}

	de := func(nombre string) *Emisor {
		if e, ok := cuenta[nombre]; ok {
			return e
		}
		e := &Emisor{Nombre: nombre}
		cuenta[nombre] = e
		return e
	}

	// El journal ya viene en orden de escritura, que es el orden en que pasaron
	// las cosas: es un registro append-only, no una tabla.
	for _, ev := range evs {
		switch ev.Kind {
		case "VerifyDeclared":
			quien := emisorDe(ev)
			if quien == "" || quien == journal.EmisorEjecucion {
				continue // el runner no "declara": ejecuta
			}
			de(quien).Declaraciones++
			ultima[ev.NoteID] = pendiente{emisor: quien, vivo: true}
		case "CheckExecuted":
			p, hay := ultima[ev.NoteID]
			if !hay || !p.vivo {
				continue // una ejecución sin declaración previa no juzga a nadie
			}
			e := de(p.emisor)
			e.Comprobadas++
			if ev.Guard == "ejecucion_falla" {
				e.Desmentidas++
			} else {
				e.Confirmadas++
			}
			ultima[ev.NoteID] = pendiente{emisor: p.emisor} // ya juzgada
		}
	}

	out := make([]Emisor, 0, len(cuenta))
	for _, e := range cuenta {
		if e.Comprobadas > 0 {
			e.Tasa = float64(e.Desmentidas) / float64(e.Comprobadas)
			e.CotaInferior = wilsonInferior(e.Desmentidas, e.Comprobadas)
		}
		e.Suficiente = e.Comprobadas >= minimo
		e.Penalizado = e.Suficiente && e.CotaInferior > float64(tolerado)/100
		out = append(out, *e)
	}
	// Primero los penalizados, después por tasa: quien mira esto quiere ver
	// arriba lo que requiere una decisión.
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Penalizado != out[j].Penalizado {
			return out[i].Penalizado
		}
		if out[i].Tasa != out[j].Tasa {
			return out[i].Tasa > out[j].Tasa
		}
		return out[i].Nombre < out[j].Nombre
	})

	inf := Informe{Emisores: out, Minimo: minimo, Tolerado: tolerado, SinDatos: true}
	for _, e := range out {
		if e.Comprobadas > 0 {
			inf.SinDatos = false
			break
		}
	}
	return inf
}

// Penalizados es el conjunto que el motor consulta: los emisores cuya palabra
// dejó de alcanzar para dar por pasado un check.
func Penalizados(inf Informe) map[string]bool {
	out := map[string]bool{}
	for _, e := range inf.Emisores {
		if e.Penalizado {
			out[e.Nombre] = true
		}
	}
	return out
}

// emisorDe saca quién declaró. Los eventos de siembra guardan el `attested_by`
// de la nota, que es el que corresponde: la declaración fue de esa identidad
// aunque el evento lo haya escrito la migración.
func emisorDe(ev journal.Event) string {
	if q := strings.TrimSpace(ev.Emitter); q != "" && q != "sembrado" && q != "sin identificar" {
		return q
	}
	// Algunas siembras viejas dejaron el emisor en el payload.
	var p struct {
		Por string `json:"por"`
	}
	if len(ev.Payload) > 0 && json.Unmarshal(ev.Payload, &p) == nil {
		return strings.TrimSpace(p.Por)
	}
	return ""
}

// wilsonInferior es la cota inferior del intervalo de Wilson al 95%. Con n=0
// devuelve 0: sin datos no se acusa a nadie.
func wilsonInferior(exitos, n int) float64 {
	if n == 0 {
		return 0
	}
	p := float64(exitos) / float64(n)
	fn := float64(n)
	den := 1 + zeta*zeta/fn
	centro := (p + zeta*zeta/(2*fn)) / den
	margen := zeta / den * math.Sqrt(p*(1-p)/fn+zeta*zeta/(4*fn*fn))
	if c := centro - margen; c > 0 {
		return c
	}
	return 0
}
