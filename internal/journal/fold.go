package journal

import (
	"time"

	"github.com/diegoparras/cogo/internal/confidence"
)

// Fold reconstruye el estado de cada nota aplicando los eventos sobre la máquina
// de estados. Es la operación central del registro: el estado deja de ser algo
// que se guarda y pasa a ser algo que se deriva, y por eso se puede preguntar
// por cualquier momento del pasado.
//
// Los dos cortes temporales responden preguntas distintas:
//
//	asOfValid — "¿qué era cierto en el mundo en ese momento?"
//	asOfTx    — "¿qué creía COGO en ese momento?"
//
// La segunda es la que sirve para auditar a un agente: no importa qué era
// verdad, importa qué información tenía cuando decidió. Un cero en cualquiera de
// las dos significa "sin corte".
func Fold(events []Event, asOfValid, asOfTx time.Time) map[string]confidence.Estado {
	out := map[string]confidence.Estado{}
	for _, e := range events {
		if !asOfValid.IsZero() && e.ValidTime.After(asOfValid) {
			continue
		}
		if !asOfTx.IsZero() && e.TxTime.After(asOfTx) {
			continue
		}
		actual, visto := out[e.NoteID]
		if !visto {
			actual = confidence.Inicial
		}
		out[e.NoteID] = aplicar(actual, e)
	}
	return out
}

// aplicar busca la transición que corresponde. Si no hay ninguna, el estado no
// cambia: un evento que no aplica es información, no un error. Que la máquina
// sea total —que nunca se quede sin respuesta— es lo que garantiza que el fold
// termine para cualquier secuencia de eventos.
func aplicar(desde confidence.Estado, e Event) confidence.Estado {
	ev := confidence.Evento(e.Kind)
	// Primero las transiciones específicas: una regla que nombra el estado de
	// origen gana sobre un comodín, porque es la más informada.
	for _, t := range confidence.Tabla {
		if t.Any || t.Desde != desde || t.Evento != ev {
			continue
		}
		if t.Guarda != "" && string(t.Guarda) != e.Guard {
			continue
		}
		return destino(desde, ev, t.Hasta)
	}
	for _, t := range confidence.Tabla {
		if !t.Any || t.Evento != ev {
			continue
		}
		if t.Guarda != "" && string(t.Guarda) != e.Guard {
			continue
		}
		// Un comodín no alcanza a los estados transitorios: una nota que se
		// está verificando no debe vencer ni contradecirse a mitad de camino.
		if desde.Transitorio() {
			return desde
		}
		return destino(desde, ev, t.Hasta)
	}
	return desde
}

// EstadoDe devuelve el estado actual de una nota, o el inicial si nunca se
// registró nada sobre ella.
func EstadoDe(events []Event, noteID string) confidence.Estado {
	st := confidence.Inicial
	for _, e := range events {
		if e.NoteID != noteID {
			continue
		}
		st = aplicar(st, e)
	}
	return st
}

// destino aplica la transición, salvo que el evento sea de los que solo bajan:
// esos son un TECHO, no un salto.
//
// La diferencia la encontró una invariante corriendo sobre vaults al azar: abrir
// una contradicción sobre una nota ya refutada la movía de `refuted` a
// `contradicted` —hacia ARRIBA— o sea que registrar un problema mejoraba la
// nota. Vencer la frescura de una nota refutada hacía lo mismo.
//
// Ninguno de los dos casos cambiaba un color (los dos estados son rojos), y por
// eso ningún test de color lo habría visto nunca. Rompía el retículo igual, y
// con él la propiedad de la que dependen los umbrales de las acciones.
func destino(desde confidence.Estado, ev confidence.Evento, hasta confidence.Estado) confidence.Estado {
	if confidence.Degrada(ev) {
		return confidence.Meet(desde, hasta)
	}
	return hasta
}
