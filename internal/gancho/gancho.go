// Package gancho es lo que hace que el agente llame a COGO sin acordarse.
//
// # EL PROBLEMA
//
// El protocolo de COGO —pack antes de trabajar, authorize antes de tocar
// algo— era un pedido escrito en AGENTS.md. Un pedido a un modelo. Un agente
// que se lo saltea no falla ruidosamente: COGO simplemente no participa, y el
// vault envejece sin que nadie se entere.
//
// # LA SOLUCIÓN
//
// Claude Code tiene hooks: comandos que el harness corre solo, al empezar la
// sesión y antes de cada herramienta. Con eso el protocolo deja de ser un
// pedido y pasa a ser una regla del entorno. Este paquete traduce lo que el
// hook recibe (la llamada a una herramienta) a lo que COGO entiende (una
// acción con clase), y decide cuándo vale la pena preguntar.
//
// # CUÁNDO PREGUNTA
//
// No en cada `ls`. Un hook que consulta al servidor por cada comando le agrega
// latencia a todo y bloquea lo que no reconoce, y un hook así se apaga a la
// semana. Se pregunta solo cuando el texto de la acción delata una clase igual
// o peor que un mínimo (costosa, por defecto). Lo que no se reconoce pasa: el
// hook es una puerta sobre lo peligroso conocido, no un juez de lo desconocido.
package gancho

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/diegoparras/cogo/internal/accion"
)

// Entrada es lo que Claude Code manda por stdin a un hook. Solo los campos que
// se usan; el resto se ignora.
type Entrada struct {
	Evento    string         `json:"hook_event_name"`
	Tool      string         `json:"tool_name"`
	ToolInput map[string]any `json:"tool_input"`
	Cwd       string         `json:"cwd"`
}

// Leer parsea la entrada del hook. Un stdin vacío no es un error: es un hook
// invocado a mano, y se trata como "no hay nada que juzgar".
func Leer(r io.Reader) (Entrada, error) {
	b, err := io.ReadAll(io.LimitReader(r, 4<<20))
	if err != nil {
		return Entrada{}, err
	}
	if strings.TrimSpace(string(b)) == "" {
		return Entrada{}, nil
	}
	var e Entrada
	if err := json.Unmarshal(b, &e); err != nil {
		return Entrada{}, fmt.Errorf("entrada del hook: %w", err)
	}
	return e, nil
}

// Traducir convierte una llamada a herramienta en el texto de una acción, que
// es lo que COGO clasifica. Devuelve false si la herramienta no es de las que
// cambian algo afuera de la conversación.
func Traducir(tool string, input map[string]any) (string, bool) {
	campo := func(k string) string {
		v, _ := input[k].(string)
		return strings.TrimSpace(v)
	}
	switch tool {
	case "Bash":
		if c := campo("command"); c != "" {
			return c, true
		}
	case "Edit", "MultiEdit":
		if p := campo("file_path"); p != "" {
			return "editar " + p, true
		}
	case "Write":
		if p := campo("file_path"); p != "" {
			return "escribir " + p, true
		}
	case "NotebookEdit":
		if p := campo("notebook_path"); p != "" {
			return "editar " + p, true
		}
	}
	return "", false
}

// HayQuePreguntar decide si la acción amerita consultar a COGO: solo si el
// texto delata una clase igual o peor que `minima`. Devuelve la clase inferida
// para que la consulta la declare y el servidor no escale a "irreversible" por
// falta de señal.
func HayQuePreguntar(accionTexto string, minima accion.Clase) (accion.Clase, bool) {
	inf := accion.Clasificar(accionTexto)
	if inf.Ninguna {
		return "", false
	}
	if severidad(inf.Clase) < severidad(minima) {
		return inf.Clase, false
	}
	return inf.Clase, true
}

func severidad(c accion.Clase) int {
	for i, x := range accion.Orden {
		if x == c {
			return i
		}
	}
	return 0
}
