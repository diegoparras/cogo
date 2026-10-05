package core

import "strings"

// El eje de origen: quién ORIGINÓ la afirmación.
//
// # EL BUCLE QUE CIERRA
//
// Un agente propone Fastify. El humano dice "dale". El agente captura "se
// decidió usar Fastify", con su autor y su evidencia. Mañana lo lee de vuelta
// como un hecho establecido del proyecto y construye encima.
//
// En cada vuelta, una opinión se lava en hecho. Y los ejes que COGO ya tenía no
// lo ven: la evidencia puede ser impecable —un `file_read` del package.json que
// el propio agente escribió— y la procedencia dice quién corrió el check, no
// quién tuvo la idea.
//
// # POR QUÉ NO ALCANZABA CON LA EVIDENCIA
//
// Porque las afirmaciones NORMATIVAS no se prueban observando. "El checkout no
// puede pasar de 400 ms" y "decidimos usar Postgres" no dicen cómo es el mundo:
// dicen qué se eligió. Ninguna salida de comando puede demostrar que alguien
// eligió algo. La única evidencia posible de una decisión es que quien podía
// tomarla la haya tomado.
//
// De ahí sale lo que este eje hace: una decisión o una restricción que originó
// el agente se MUESTRA como lo que es, una propuesta.
//
// # POR QUÉ NO BAJA EL COLOR
//
// Se evaluó y se descartó. Un techo amarillo obligaría a ratificar a mano cada
// decisión que tome un agente, y COGO se juega justamente en no agregar tareas:
// una herramienta que pide trabajo para seguir siendo confiable termina no
// usándose, y una memoria que no se usa no protege de nada.
//
// La etiqueta da casi todo el valor. El que lee sabe que eso se puede revisar,
// que es lo único que hacía falta. Y si con el tiempo resulta que ver la
// etiqueta siempre lleva a actuar, ponerle techo es un cambio de una línea —
// mientras que sacárselo, después de acostumbrar a un equipo a ratificar, no.
//
// # LO QUE NO TOCA
//
// Las afirmaciones descriptivas. Que un agente descubra y registre que el pool
// se satura a las 200 conexiones es exactamente su trabajo, y ahí el respaldo lo
// da la evidencia. El origen importa donde la evidencia no llega.

// Origen es de dónde salió la afirmación.
type Origen string

const (
	// OrigenHumano — lo dijo la persona. Para una decisión, es la única forma de
	// que sea una decisión.
	OrigenHumano Origen = "human"
	// OrigenAgente — lo propuso, lo eligió o lo dedujo el modelo.
	OrigenAgente Origen = "agent"
	// OrigenInstrumento — salió de una herramienta: la salida de un comando, un
	// test, la lectura de un archivo. Nadie lo decidió; se observó.
	OrigenInstrumento Origen = "instrument"
	// OrigenSinDeclarar es lo que tienen las notas escritas antes de que este eje
	// existiera. NO se asume que sean del agente: asumirlo cambiaría de color, de
	// forma retroactiva, notas capturadas cuando el campo no se podía llenar.
	// Se marcan y se muestran, para que alguien pueda revisarlas.
	OrigenSinDeclarar Origen = ""
)

// OrigenesValidos son los que se pueden declarar al capturar.
var OrigenesValidos = []Origen{OrigenHumano, OrigenAgente, OrigenInstrumento}

// NormalizarOrigen valida lo que llegó. Un valor que no existe se trata como del
// agente: quien captura ES un agente, así que es la suposición conservadora, y
// un typo no puede ser una forma de esquivar el eje.
func NormalizarOrigen(s string) Origen {
	o := Origen(strings.ToLower(strings.TrimSpace(s)))
	for _, v := range OrigenesValidos {
		if v == o {
			return v
		}
	}
	if strings.TrimSpace(s) == "" {
		return OrigenSinDeclarar
	}
	return OrigenAgente
}

// EsNormativa dice si la nota afirma qué se ELIGIÓ, en vez de cómo es el mundo.
//
// Son los dos tipos donde la evidencia no puede responder la pregunta:
//
//	decision    lo que se resolvió hacer, y por qué
//	constraint  lo que no puede dejar de cumplirse
//
// Todo lo demás —bug, runbook, architecture, command— describe. Una salida de
// comando puede respaldarlo; ninguna puede respaldar una elección.
func EsNormativa(n *Note) bool {
	return n != nil && (n.Type == "decision" || n.Type == "constraint")
}

// OrigenDe devuelve el origen declarado de una nota.
func OrigenDe(n *Note) Origen { return NormalizarOrigen(n.Origin) }

// EsPropuesta es la etiqueta que importa: una decisión o una restricción que
// originó el agente. Nadie con autoridad para elegir eligió: es una propuesta.
//
// NO baja el color, y es una decisión pensada. Bajarlo obligaría a ratificar a
// mano cada decisión que tome un agente, y COGO se juega en no agregar tareas.
// La etiqueta da casi todo el valor: el que lee sabe que eso se puede revisar.
// Si con el tiempo resulta que ver la etiqueta siempre lleva a actuar, ponerle
// techo es un cambio de una línea; al revés no se deshace tan barato.
//
// El instrumento queda afuera a propósito. Una restricción que sale de un
// instrumento —"el disco no da más de 500 IOPS"— no la eligió nadie: es del
// mundo, y ahí la evidencia sí alcanza.
func EsPropuesta(n *Note) bool {
	return EsNormativa(n) && OrigenDe(n) == OrigenAgente
}

// SinDeclarar marca las notas normativas de antes de este eje: no consta quién
// las decidió. Se señalan, que es lo que permite que alguien las mire.
func SinDeclarar(n *Note) bool {
	return EsNormativa(n) && OrigenDe(n) == OrigenSinDeclarar
}

// RotuloOrigen es cómo se le dice a un humano.
func RotuloOrigen(o Origen) string {
	switch o {
	case OrigenHumano:
		return "lo decidió una persona"
	case OrigenAgente:
		return "lo propuso el agente"
	case OrigenInstrumento:
		return "salió de un instrumento"
	}
	return "no consta quién lo decidió"
}
