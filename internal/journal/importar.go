package journal

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Importar trae al registro los eventos de ejecución que produjo OTRO COGO —el
// local, en la máquina donde vive el código— y los encadena acá con su propio
// número y su propio hash.
//
// # POR QUÉ EXISTE
//
// En el despliegue real nada llega a `verified`: la imagen es scratch, sin go
// ni node ni shell, y el runner no puede ejecutar ningún check. El check corre
// donde está el repo; el hosteado solo recibe el resultado. Este es el camino
// por el que el resultado entra, y es el único: la puerta reservada del emisor
// `internal_runner` con un remitente que además tiene que ser administrador.
//
// # QUÉ ACEPTA
//
// Solo VerificationStarted y CheckExecuted, y solo con el emisor reservado. Un
// lote con un evento de otra clase se rechaza entero: un import a medias es
// peor que ninguno, porque deja un VerificationStarted sin su CheckExecuted y la
// nota en `verifying` para siempre.
//
// Cada evento importado lleva en su payload `importado_de`, el digest que tenía
// en el registro de origen. Es lo que hace idempotente el import: el mismo
// evento dos veces se ignora, y el segundo COGO puede reconstruir de dónde vino
// cada verificación.
func Importar(j *Journal, evs []Event) (importados int, err error) {
	for i, e := range evs {
		if e.Emitter != EmisorEjecucion {
			return 0, fmt.Errorf("evento %d (nota %q): solo se importan ejecuciones del runner; el emisor es %q", i, e.NoteID, e.Emitter)
		}
		switch e.Kind {
		case "VerificationStarted", "CheckExecuted":
		default:
			return 0, fmt.Errorf("evento %d (nota %q): %s no es un evento de ejecución", i, e.NoteID, e.Kind)
		}
		if strings.TrimSpace(e.NoteID) == "" || e.PrevDigest == "" && e.Seq == 0 {
			return 0, fmt.Errorf("evento %d: sin nota o sin identidad de origen", i)
		}
	}
	yaTengo, err := j.importados()
	if err != nil {
		return 0, err
	}
	for _, e := range evs {
		origen := e.digest()
		if yaTengo[origen] {
			continue
		}
		nuevo := Event{
			NoteID: e.NoteID, Kind: e.Kind, Guard: e.Guard, Fence: e.Fence,
			ValidTime: e.ValidTime, // cuándo pasó de verdad: eso no cambia
			Payload:   conOrigen(e.Payload, origen),
		}
		if _, err := j.AppendEjecucion(nuevo); err != nil {
			return importados, err
		}
		yaTengo[origen] = true
		importados++
	}
	return importados, nil
}

// importados devuelve los digests de origen de todo lo ya importado.
func (j *Journal) importados() (map[string]bool, error) {
	evs, err := j.All()
	if err != nil {
		return nil, err
	}
	out := map[string]bool{}
	for _, e := range evs {
		if len(e.Payload) == 0 {
			continue
		}
		var p struct {
			ImportadoDe string `json:"importado_de"`
		}
		if json.Unmarshal(e.Payload, &p) == nil && p.ImportadoDe != "" {
			out[p.ImportadoDe] = true
		}
	}
	return out, nil
}

func conOrigen(payload json.RawMessage, origen string) json.RawMessage {
	m := map[string]any{}
	if len(payload) > 0 {
		_ = json.Unmarshal(payload, &m)
	}
	m["importado_de"] = origen
	b, err := json.Marshal(m)
	if err != nil {
		return payload
	}
	return b
}

// Ejecuciones devuelve los eventos del runner con número de secuencia mayor
// que `desde`: lo que un `cogo sync` tiene que empujar.
func (j *Journal) Ejecuciones(desde uint64) ([]Event, error) {
	evs, err := j.All()
	if err != nil {
		return nil, err
	}
	var out []Event
	for _, e := range evs {
		if e.Seq > desde && e.Emitter == EmisorEjecucion {
			out = append(out, e)
		}
	}
	return out, nil
}
