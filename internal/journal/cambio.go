package journal

import (
	"encoding/json"
	"strings"

	"github.com/diegoparras/cogo/internal/core"
)

// Cada escritura de una nota se traduce a los eventos que la explican.
//
// # EL AGUJERO QUE CIERRA
//
// El registro se sembraba al arrancar y después nadie le escribía. `verify`
// cambiaba el archivo —status: passed— pero el pliegue seguía viendo los eventos
// de la siembra, así que una nota verificada después del arranque no avanzaba
// de estado nunca. El motor estaba bien diseñado y desenchufado.
//
// # QUÉ SE TRADUCE Y QUÉ NO
//
// Solo el eje del check, que es el único que vive en los eventos: apareció un
// criterio, alguien declaró que pasa, alguien declaró que falla. La frescura, la
// evidencia y las contradicciones se calculan en cada lectura desde la nota y
// el almacén, y no hace falta —ni conviene— duplicarlas como eventos.
//
// Y nunca se traduce una ejecución: cuando el archivo dice `attested: executed`
// es porque el runner ya escribió sus propios eventos por la puerta reservada.
// Volver a emitirlos acá sería inventar una segunda ejecución.

// EventosDeCambio devuelve los eventos que explican pasar de `antes` a
// `despues`. `antes` nil significa que la nota es nueva.
func EventosDeCambio(antes, despues *core.Note) []Event {
	if despues == nil {
		return nil
	}
	if antes == nil {
		return eventosDe(despues, core.Verdict{}, "escritura")
	}

	var out []Event
	add := func(kind, guard, quien string) {
		out = append(out, Event{NoteID: despues.ID, Kind: kind, Guard: guard,
			Emitter: sinReservado(quien), Payload: json.RawMessage(`{"origen":"escritura"}`)})
	}

	// Un criterio nuevo —o el primero— se declara. La máquina decide qué hace
	// con eso según el estado: desde una nota verde lo baja a check_declared,
	// porque lo declarado cubría otro check.
	testAntes := strings.TrimSpace(antes.Check.Test)
	testDespues := strings.TrimSpace(despues.Check.Test)
	if testDespues != "" && testDespues != testAntes {
		add("CheckDeclared", "", autorDe(despues))
	}

	ejecutado := despues.Check.Attestation() == core.AttestExecuted
	switch despues.Check.Status {
	case "passed":
		// Renovar una verificación también cuenta: es lo que saca a una nota de
		// `stale` cuando alguien la vuelve a mirar.
		renovada := antes.Check.Status != "passed" ||
			despues.LastVerified.After(antes.LastVerified)
		if renovada && !ejecutado {
			add("VerifyDeclared", "declara_un_tercero", declaranteDe(despues))
		}
	case "failed":
		if antes.Check.Status != "failed" && !ejecutado {
			add("FailDeclared", "", declaranteDe(despues))
		}
	}
	return out
}

// RegistrarCambio escribe en el registro lo que EventosDeCambio derive. Es lo
// que se engancha en la escritura de notas.
func RegistrarCambio(j *Journal, antes, despues *core.Note) error {
	for _, e := range EventosDeCambio(antes, despues) {
		if _, err := j.Append(e); err != nil {
			return err
		}
	}
	return nil
}

func declaranteDe(n *core.Note) string {
	if q := strings.TrimSpace(n.Check.AttestedBy); q != "" {
		return q
	}
	return autorDe(n)
}

func autorDe(n *core.Note) string {
	if q := strings.TrimSpace(n.Author); q != "" {
		return q
	}
	return "agente"
}

// sinReservado evita que una escritura común se atribuya al runner. El journal
// la rechazaría igual (ErrEmisorReservado); esto es para no perder el evento.
func sinReservado(quien string) string {
	if quien == EmisorEjecucion {
		return "agente"
	}
	return quien
}
