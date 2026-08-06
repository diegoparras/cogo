package main

import (
	"fmt"
	"strings"

	"github.com/diegoparras/cogo/internal/accion"
	"github.com/diegoparras/cogo/internal/confidence"
	"github.com/diegoparras/cogo/internal/core"
)

// authorizeIn es el input de `authorize`. Los tres campos son la pregunta
// entera: qué vas a hacer, de qué gravedad, y en qué te apoyás.
type authorizeIn struct {
	Action string   `json:"action" jsonschema:"what you are about to do, in words. It gets classified, so describe the actual operation ('delete the staging database', not 'clean up')"`
	Class  string   `json:"class,omitempty" jsonschema:"informative|reversible|costly|irreversible. Optional: COGO also infers it from the text and takes the STRICTER of the two, so declaring a lower class does not lower the bar"`
	Notes  []string `json:"notes,omitempty" jsonschema:"ids of the notes you are relying on. Get them from pack or search. One weak note is enough to sink the request: you are only as backed as the weakest thing you lean on"`
}

// fuenteVault adapta el vault a lo que necesita el autorizador. Es un mapa de
// estados ya calculados: mantiene a internal/accion sin conocer el motor.
type fuenteVault struct {
	estados map[string]confidence.Estado
	vault   map[string]*core.Note
}

func (f fuenteVault) Estado(id string) (string, bool) {
	e, ok := f.estados[id]
	if !ok {
		return "", false
	}
	// e.String(), NO string(e): confidence.Estado es un uint8, así que la
	// conversión directa produce el carácter con ese código en vez del nombre del
	// estado. Compila, pasa vet, y deja el autorizador comparando contra basura.
	return e.String(), true
}

func (f fuenteVault) EsBrecha(id string) bool {
	n, ok := f.vault[id]
	return ok && core.EsBrecha(n)
}

// textoAutorizacion arma la respuesta que lee el agente. El primer renglón es
// una decisión, no un informe: un agente que solo lee la primera línea tiene que
// obedecer bien igual.
func textoAutorizacion(v accion.Veredicto) string {
	var b strings.Builder
	if v.Autoriza {
		b.WriteString("AUTHORIZED — ")
	} else {
		b.WriteString("NOT AUTHORIZED — ")
	}
	b.WriteString(v.Porque)
	b.WriteString("\n\n")
	fmt.Fprintf(&b, "action class: %s (%s)\nrequires: %s", v.Clase, v.PorQueClase, v.Necesita)
	if v.Apoyo != "" {
		fmt.Fprintf(&b, "\nweakest support cited: %s", v.Apoyo)
	}
	// El bloqueo por otro agente va antes que el consejo de siempre: es de otra
	// naturaleza, y quien lo lee tiene que ver primero que el problema no es lo
	// que sabe.
	if v.Bloqueo != "" {
		fmt.Fprintf(&b, "\n\n%s", v.Bloqueo)
	}
	if !v.Autoriza {
		b.WriteString("\n\nDo not proceed. Either raise the support to the required level, " +
			"or tell the human what is missing and let them decide. " +
			"Reporting the block is a valid outcome; working around it is not.")
	}
	return b.String()
}
