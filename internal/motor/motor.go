// Package motor arma el color de cada nota con la cadena nueva: los eventos del
// journal se pliegan sobre la máquina de estados, se combinan con los ejes que
// no viven en los eventos (evidencia, frescura, contradicciones), y la duda se
// propaga por el grafo resolviendo un punto fijo.
//
// Devuelve los mismos core.Verdict que el motor anterior. Eso no es casualidad:
// mantener el tipo de salida es lo que permite cambiar el motor sin tocar el
// visor, la CLI ni el servidor MCP — y, sobre todo, poder volver atrás con una
// variable de entorno si algo sale mal.
package motor

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/diegoparras/cogo/internal/confidence"
	"github.com/diegoparras/cogo/internal/core"
	"github.com/diegoparras/cogo/internal/journal"
)

// Legacy dice si hay que usar el motor anterior. Es la marcha atrás: si el
// corte sale mal en una instancia que ya está andando, se arregla con una
// variable de entorno y un reinicio, sin esperar un release.
func Legacy() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("COGO_MOTOR"))) {
	case "legacy", "viejo", "old":
		return true
	}
	return false
}

// NoGraduable dice si una nota queda fuera del semáforo.
//
// Son dos casos, y por la misma razón: no afirman nada que haya que creer.
//
//	mistake — registra algo que pasó. Es informativo.
//	gap     — es una PREGUNTA ABIERTA. Declara que algo no se sabe.
//
// Que una brecha no tenga color es la parte importante y la más fácil de hacer
// mal. Sería tentador pintarla de rojo —no hay evidencia, después de todo— pero
// eso confundiría dos cosas distintas: una nota roja AFIRMA algo sin respaldo,
// una brecha no afirma nada. Ponerle color la volvería una mala afirmación en
// vez de una buena pregunta.
//
// Quedan fuera del retículo Y de la propagación: ni arrastran a nadie ni son
// arrastradas.
func NoGraduable(n *core.Note) bool {
	return n.Type == "mistake" || core.EsBrecha(n)
}

// Evaluar calcula el veredicto de cada nota del vault.
// Opciones son los ejes que no salen del vault ni del registro, sino de cómo
// está configurada esta instancia. Van explícitos y no en una variable global
// para que el motor siga siendo una función de sus entradas: dos llamadas con
// los mismos argumentos dan el mismo resultado, que es lo que hace que el banco
// de golden tests signifique algo.
type Opciones struct {
	// Penalizados son los emisores cuya palabra dejó de alcanzar para dar por
	// pasado un check (ver internal/calibracion). Vacío = se le cree a todos, que
	// es el default hasta que un vault tenga datos.
	Penalizados map[string]bool
}

func Evaluar(vault map[string]*core.Note, contradicciones map[string]bool, hoy core.Date, evs []journal.Event) map[string]core.Verdict {
	return EvaluarCon(vault, contradicciones, hoy, evs, Opciones{})
}

// EvaluarCon es Evaluar con la configuración de la instancia.
func EvaluarCon(vault map[string]*core.Note, contradicciones map[string]bool, hoy core.Date, evs []journal.Event, op Opciones) map[string]core.Verdict {
	final, local := EstadosCon(vault, contradicciones, hoy, evs, op)

	out := make(map[string]core.Verdict, len(vault))
	for id, n := range vault {
		if NoGraduable(n) {
			out[id] = core.Verdict{Color: core.Ungraded, Reason: razonNoGraduable(n)}
			continue
		}
		est := final[id]
		out[id] = core.Verdict{
			Color:   color(est),
			Reason:  razon(est, n, local[id]),
			StaleAt: n.StaleAt,
		}
	}
	return out
}

// Estados devuelve el estado del retículo de cada nota: el final (después de
// propagar por el grafo) y el local (lo que vale por sí misma).
//
// Existe separado de Evaluar porque el color es una PROYECCIÓN del estado —tres
// valores donde hay ocho— y hay decisiones que necesitan el estado entero. La
// más clara: autorizar una acción irreversible pide un check ejecutado, y eso no
// se puede distinguir de un check declarado mirando un semáforo verde.
func Estados(vault map[string]*core.Note, contradicciones map[string]bool, hoy core.Date, evs []journal.Event) (final, local map[string]confidence.Estado) {
	return EstadosCon(vault, contradicciones, hoy, evs, Opciones{})
}

// EstadosCon es Estados con la configuración de la instancia.
func EstadosCon(vault map[string]*core.Note, contradicciones map[string]bool, hoy core.Date, evs []journal.Event, op Opciones) (final, local map[string]confidence.Estado) {
	crudos := journal.Fold(evs, time.Time{}, time.Time{})

	local = map[string]confidence.Estado{}
	g := confidence.Grafo{}
	for id, n := range vault {
		if NoGraduable(n) {
			continue // fuera del retículo: no entra al punto fijo
		}
		local[id] = journal.EstadoLocal(crudos[id], n, hoy, contradicciones[id])
		local[id] = confidence.Meet(local[id], techoPorEmisor(n, op.Penalizados))
		// Una dependencia no graduable tampoco arrastra: se la saca del grafo
		// en vez de tratarla como ausente, que hundiría a quien se apoye en ella.
		var deps []string
		for _, d := range n.DependsOn {
			if dn, existe := vault[d]; existe && NoGraduable(dn) {
				continue
			}
			deps = append(deps, d)
		}
		g[id] = deps
	}
	return confidence.PuntoFijo(g, local), local
}

// techoPorEmisor es el tercer techo, junto al de la evidencia y al de la
// frescura: cuánto vale la palabra de quien declaró que el check pasa.
//
// Solo toca las DECLARACIONES. Un check ejecutado por el runner no depende de la
// reputación de nadie —lo vio una máquina, y el código de salida es el código de
// salida— así que un emisor con mal historial no puede bajar un `verified`.
// Puede, sí, dejar de convertir su palabra en `claimed_passed`.
func techoPorEmisor(n *core.Note, penalizados map[string]bool) confidence.Estado {
	if len(penalizados) == 0 || n.Check.Attestation() != core.AttestDeclared {
		return confidence.Verified // sin techo
	}
	if penalizados[strings.TrimSpace(n.Check.AttestedBy)] {
		return confidence.CheckDeclared
	}
	return confidence.Verified
}

func razonNoGraduable(n *core.Note) string {
	if core.EsBrecha(n) {
		if k := len(n.Blocks); k > 0 {
			return fmt.Sprintf("pregunta abierta: no se sabe todavía, y hay %d decisión(es) esperándola", k)
		}
		return "pregunta abierta: es algo que el proyecto no sabe, no una afirmación"
	}
	return "error registrado: es informativo, no se gradúa por confianza"
}

func color(e confidence.Estado) core.Color {
	switch e.Color() {
	case "green":
		return core.Green
	case "yellow":
		return core.Yellow
	default:
		return core.Red
	}
}

// razon explica el veredicto en una frase. No se genera desde el estado a secas:
// cuando una nota cayó por una dependencia, lo que hay que decir es cuál — si no,
// el usuario ve "rojo" sin saber dónde mirar.
func razon(final confidence.Estado, n *core.Note, propio confidence.Estado) string {
	base := explicar(propio)
	// La deriva material entra al retículo como un techo, y el techo a secas
	// dice "todavía no se ejecutó ni se declaró" — que en este caso es falso: se
	// declaró, y después cambió el archivo. Decir cuál es lo que convierte el
	// amarillo en algo que se puede arreglar.
	if propio == confidence.CheckDeclared && core.HayDerivaMaterial(n) {
		base = fmt.Sprintf("cambió lo que la nota cita (%s): la evidencia ya no la respalda igual",
			strings.Join(core.DriftedRefs(n), ", "))
	}
	if final != propio {
		return base + " — y además depende de una nota más débil"
	}
	return base
}

func explicar(e confidence.Estado) string {
	switch e {
	case confidence.Verified:
		return "el check se ejecutó y pasó"
	case confidence.ClaimedPassed:
		return "hay evidencia observada y alguien declaró que el check pasa"
	case confidence.CheckDeclared:
		return "hay un criterio de verificación, pero todavía no se ejecutó ni se declaró"
	case confidence.Asserted:
		return "capturada, sin criterio de verificación declarado"
	case confidence.Stale:
		return "venció su ventana de frescura: hay que re-verificarla"
	case confidence.Contradicted:
		return "sin evidencia suficiente, o con una contradicción abierta"
	case confidence.Refuted:
		return "el check se ejecutó y falló"
	case confidence.Quarantined:
		return "en cuarentena: excluida a propósito"
	case confidence.Verifying:
		return "verificándose ahora mismo"
	}
	return "sin determinar"
}
