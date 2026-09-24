package core

import (
	"fmt"
	"strings"
)

// Verificar es la política de re-verificación de una nota, en un solo lugar
// porque la comparten las tres caras (CLI, MCP y visor) y una política que vive
// en tres copias deja de ser una política.
//
// Dos cosas que antes hacía mal, y son la razón de que esto exista:
//
//  1. Marcaba el check como pasado sin registrar QUIÉN lo dijo ni CÓMO. Ahora
//     queda asentado como declaración, con su autor. Un verde declarado y un
//     verde ejecutado dejan de ser indistinguibles.
//  2. Re-estampaba el hash de TODA la evidencia, y con eso borraba la deriva:
//     una nota cuyo archivo citado había cambiado volvía a verde y perdía el
//     rastro de que había cambiado. Ahora, si algo derivó, hay que decirlo.
type Verificacion struct {
	// Por identifica a quien verifica ("token:claude-code", "user:diego"). Queda
	// en la nota: en un vault compartido dice de quién es la palabra.
	Por string
	// Reanclar confirma explícitamente que se comprobó la afirmación contra el
	// contenido ACTUAL de la evidencia que derivó. Sin esto, una nota con deriva
	// no se puede re-verificar: es el único modo de que re-verificar no sea una
	// forma de hacer desaparecer el aviso.
	Reanclar bool
	// Ejecutado lo pone el runner interno cuando corrió el comando y vio su
	// código de salida. Nadie más debería ponerlo en true.
	Ejecutado bool
}

// ErrDeriva se devuelve cuando la evidencia cambió y no se confirmó contra qué
// se está verificando. Trae las citas para poder mostrarlas.
type ErrDeriva struct{ Refs []string }

func (e *ErrDeriva) Error() string {
	return fmt.Sprintf("la evidencia cambió desde la última verificación (%s): "+
		"si comprobaste la afirmación contra el contenido actual, re-verificá con reanchor; "+
		"si no, revisá la nota antes de verificarla", strings.Join(e.Refs, ", "))
}

// Verificar aplica la política y deja la nota lista para recomputar su color.
// No escribe el archivo ni calcula el veredicto: eso queda en el llamador.
func Verificar(n *Note, roots EvidenceRoots, today Date, v Verificacion) error {
	if derivadas := DriftedRefs(n); len(derivadas) > 0 && !v.Reanclar {
		return &ErrDeriva{Refs: derivadas}
	}

	n.Check.Status = "passed"
	n.Check.Attested = AttestDeclared
	if v.Ejecutado {
		n.Check.Attested = AttestExecuted
	}
	// Nunca dejar el registro en blanco: una declaración sin autor es
	// exactamente lo que este cambio vino a evitar. En una instancia local sin
	// autenticación no hay identidad que registrar, y decirlo así es más honesto
	// que un campo vacío que después nadie sabe interpretar.
	n.Check.AttestedBy = v.Por
	if strings.TrimSpace(n.Check.AttestedBy) == "" {
		n.Check.AttestedBy = "sin identificar"
	}
	n.LastVerified = today

	if v.Reanclar {
		// Se confirmó contra el contenido actual: recién ahí corresponde mover
		// la línea base de comparación.
		StampEvidenceHashes(n, roots)
		return nil
	}
	StampNewEvidenceHashes(n, roots)
	return nil
}
