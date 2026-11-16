// Package presencia responde quién más está trabajando en este vault ahora
// mismo, y sobre qué.
//
// # POR QUÉ HACE FALTA
//
// COGO ya era memoria compartida: dos agentes en dos máquinas leen y escriben el
// mismo vault. Pero compartir memoria no es coordinarse. Si uno está migrando la
// base en este momento, el otro pide contexto, recibe un pack impecable, y NO SE
// ENTERA — porque cada respuesta contestaba exactamente lo que le preguntaron.
//
// Los permisos existían y eran una isla: un agente solo se enteraba de que otro
// había tomado uno si preguntaba explícitamente. Nadie pregunta por algo que no
// sabe que existe.
//
// # POR QUÉ NO SE EMPUJA
//
// MCP es pregunta y respuesta: el servidor no puede hablar si no le hablan. No
// hay forma de interrumpir a un agente en la mitad de un turno.
//
// Y no hace falta, porque el protocolo de COGO ya obliga a llamar ANTES de
// actuar: `pack` primero, `authorize` antes de tocar nada. Esos llamados son el
// canal. COGO no tiene que interrumpir — tiene que contestar más de lo que le
// preguntaron, y el aviso llega exactamente cuando es accionable.
//
// # DE DÓNDE SALE
//
// De la auditoría (.cogo/audit.jsonl), que es la única fuente que sabe QUIÉN en
// el sentido vivo: un token autenticado, con hora, herramienta y sobre qué nota
// o proyecto llamó.
//
// Deliberadamente NO sale del registro de eventos. Ahí el emisor es un rol —
// "sembrado", el runner, quien atestiguó un check— y no la sesión que está
// conectada ahora. Usarlo daría una lista de agentes llamados "sembrado", que
// es peor que no dar ninguna: un aviso equivocado enseña a ignorar los avisos.
//
// # LO QUE NO VE
//
// Solo ve lo que pasa por HTTP. Un COGO local por stdio no deja auditoría, así
// que dos procesos sobre el mismo vault en la misma máquina no se ven acá — a
// esos los cubre el cerrojo del registro, que es otra capa y otro problema.
package presencia

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Agente es alguien que estuvo activo en la ventana mirada.
type Agente struct {
	Token  string    `json:"token"`
	Ultima time.Time `json:"ultima"`
	// Notas son las que tocó o consultó, de más reciente a menos. Es lo que dice
	// en qué anda, mejor que cualquier resumen.
	Notas []string `json:"notas,omitempty"`
	// Proyectos permite avisar solo a quien está en el mismo lugar, en vez de a
	// todos por cualquier cosa.
	Proyectos []string `json:"proyectos,omitempty"`
	// Herramientas son las que usó, para que el aviso pueda decir "está
	// escribiendo" en vez de solo "está".
	Herramientas []string `json:"herramientas,omitempty"`
	Escrituras   int      `json:"escrituras"`
	Llamadas     int      `json:"llamadas"`
}

// Escribiendo dice si el agente hizo algo más que leer. Un lector concurrente no
// es un problema; un escritor concurrente sí.
func (a Agente) Escribiendo() bool { return a.Escrituras > 0 }

// escriben son las herramientas que cambian el vault. `authorize` entra aunque
// no escriba: quien lo llama está por hacer algo, y eso es exactamente lo que
// el otro necesita saber a tiempo.
var escriben = map[string]bool{
	"capture": true, "verify": true, "archive": true, "restore": true,
	"remove": true, "gap": true, "stash": true, "lease": true, "authorize": true,
}

// Ver arma la foto de quién estuvo activo desde `desde`.
func Ver(vaultDir string, desde time.Time) []Agente {
	porToken := map[string]*Agente{}
	var orden []string

	for _, l := range leerAuditoria(vaultDir, desde) {
		a, ok := porToken[l.quien]
		if !ok {
			a = &Agente{Token: l.quien}
			porToken[l.quien] = a
			orden = append(orden, l.quien)
		}
		a.Llamadas++
		if escriben[l.tool] {
			a.Escrituras++
		}
		if l.cuando.After(a.Ultima) {
			a.Ultima = l.cuando
		}
		agregar(&a.Herramientas, l.tool, 6)
		agregar(&a.Notas, l.nota, 8)
		agregar(&a.Proyectos, l.proyecto, 4)
	}

	out := make([]Agente, 0, len(porToken))
	for _, t := range orden {
		out = append(out, *porToken[t])
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Ultima.After(out[j].Ultima) })
	return out
}

// Otros saca de la lista al que pregunta. Avisarle a alguien de sí mismo es
// ruido, y ruido en un aviso lo vuelve ignorable.
func Otros(agentes []Agente, yo string) []Agente {
	yo = strings.TrimSpace(yo)
	out := make([]Agente, 0, len(agentes))
	for _, a := range agentes {
		if a.Token != yo {
			out = append(out, a)
		}
	}
	return out
}

// EnProyecto filtra a los que están en el mismo proyecto. Un agente que trabaja
// en otro repo no es una colisión: avisarlo entrena a ignorar los avisos.
//
// Un agente sin proyecto conocido queda igual: no se puede afirmar que NO esté
// en el mismo lugar, y ante la duda vale más avisar de más que de menos.
func EnProyecto(agentes []Agente, proyecto string) []Agente {
	if strings.TrimSpace(proyecto) == "" {
		return agentes
	}
	out := make([]Agente, 0, len(agentes))
	for _, a := range agentes {
		if len(a.Proyectos) == 0 || contiene(a.Proyectos, proyecto) {
			out = append(out, a)
		}
	}
	return out
}

type lineaAudit struct {
	quien    string
	tool     string
	nota     string
	proyecto string
	cuando   time.Time
}

// maxLineas acota cuánto se lee del final del log. La auditoría ya se recorta
// sola, pero esto sostiene el costo aunque alguien la haya dejado sin tope: el
// aviso se arma en cada `pack`, y una lectura que crece sin límite convierte una
// ayuda en un impuesto.
const maxLineas = 3000

func leerAuditoria(vaultDir string, desde time.Time) []lineaAudit {
	b, err := os.ReadFile(filepath.Join(vaultDir, ".cogo", "audit.jsonl"))
	if err != nil {
		return nil
	}
	lineas := strings.Split(string(b), "\n")
	if len(lineas) > maxLineas {
		lineas = lineas[len(lineas)-maxLineas:]
	}
	var out []lineaAudit
	for _, ln := range lineas {
		if strings.TrimSpace(ln) == "" {
			continue
		}
		var e struct {
			Time     string `json:"time"`
			Caller   string `json:"caller"`
			Tool     string `json:"tool"`
			Nota     string `json:"nota"`
			Proyecto string `json:"proyecto"`
		}
		if json.Unmarshal([]byte(ln), &e) != nil {
			continue
		}
		t, err := time.Parse(time.RFC3339, e.Time)
		if err != nil || t.Before(desde) {
			continue
		}
		q := strings.TrimSpace(e.Caller)
		// "anon" es un llamado sin token identificado: no se le puede atribuir a
		// nadie, y presentarlo como un agente sería inventar una sesión.
		if q == "" || q == "anon" {
			continue
		}
		out = append(out, lineaAudit{quien: q, tool: e.Tool, nota: e.Nota,
			proyecto: strings.TrimSpace(e.Proyecto), cuando: t})
	}
	return out
}

func agregar(xs *[]string, x string, tope int) {
	x = strings.TrimSpace(x)
	if x == "" || len(*xs) >= tope || contiene(*xs, x) {
		return
	}
	*xs = append(*xs, x)
}

func contiene(xs []string, x string) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}
