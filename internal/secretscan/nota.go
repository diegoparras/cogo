package secretscan

import "github.com/diegoparras/cogo/internal/core"

// Nota pasa una nota ENTERA por el escáner: cuerpo, evidencia, check, todo lo
// que termina en el archivo. Devuelve el resumen de lo encontrado y si hay que
// negarse a guardarla.
//
// El manual prometía que COGO se niega a guardar una nota con un secreto, y el
// escáner solo corría sobre los artefactos de `stash`. Las notas —que es donde
// un agente pega el `.env` que acaba de leer— pasaban de largo.
func Nota(n *core.Note) (resumen string, bloqueada bool) {
	data, err := core.MarshalNote(n)
	if err != nil {
		return "", false // si no se puede serializar, fallará al escribir
	}
	_, findings, blocked := Guard(data, false)
	if !blocked {
		return "", false
	}
	return Summary(findings), true
}
