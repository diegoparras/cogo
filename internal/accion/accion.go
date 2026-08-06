// Package accion decide si el respaldo que tiene un agente alcanza para lo que
// está por hacer.
//
// # EL PROBLEMA QUE RESUELVE
//
// Hasta acá COGO dice cuánto vale cada cosa que sabe. Eso es la mitad: la otra
// mitad es que cuánto tiene que valer DEPENDE de para qué. Explicar algo apoyado
// en una nota amarilla es aceptable —se dice que es probable y listo—. Borrar una
// base de datos apoyado en la misma nota no lo es. Un solo umbral para las dos
// cosas está pidiendo de más en un lado o de menos en el otro, y en la práctica
// termina siendo de menos: los umbrales bajan hasta donde no molestan.
//
// # LAS CUATRO CLASES
//
//	informativa   solo produce texto. Si la nota estaba mal, se dijo algo falso.
//	reversible    se deshace con git o con un botón.
//	costosa       se puede revertir, pero cuesta plata o tiempo.
//	irreversible  no hay vuelta atrás.
//
// La línea que importa es la última. Es la única clase que por default exige un
// check EJECUTADO y no declarado — o sea, la única donde la palabra de un agente
// no alcanza. Todo el aparato de las fases anteriores (el runner, la procedencia,
// el retículo) existe para poder trazar esa línea y que signifique algo.
//
// # POR QUÉ NO ALCANZA CON QUE EL AGENTE DECLARE LA CLASE
//
// Porque el agente que quiere hacer algo es exactamente quien tiene el incentivo
// de clasificarlo bajo. "Voy a limpiar unos archivos temporales" puede ser un
// `rm -rf`. Así que la clase se decide DOS veces —lo que el agente declara y lo
// que se infiere del texto de la acción— y gana la más estricta. Es la misma
// regla de meet que gobierna todo el resto del sistema: manda el más débil, o
// acá, el más exigente.
//
// Un agente puede subir la exigencia sobre sí mismo. No puede bajarla.
package accion

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// Clase es cuánto se puede romper.
type Clase string

const (
	Informativa  Clase = "informative"
	Reversible   Clase = "reversible"
	Costosa      Clase = "costly"
	Irreversible Clase = "irreversible"
)

// Orden va de menos a más grave; el índice ES la severidad.
var Orden = []Clase{Informativa, Reversible, Costosa, Irreversible}

func severidad(c Clase) int {
	for i, x := range Orden {
		if x == c {
			return i
		}
	}
	return 0
}

// Valida normaliza una clase declarada. Una clase que no existe no se rechaza
// con un error: se trata como la más grave. Un typo en la declaración no puede
// ser una forma de esquivar el control.
func Valida(s string) (Clase, bool) {
	c := Clase(strings.ToLower(strings.TrimSpace(s)))
	for _, x := range Orden {
		if x == c {
			return c, true
		}
	}
	return Irreversible, false
}

// LaMasGrave es el meet de este retículo chiquito.
func LaMasGrave(a, b Clase) Clase {
	if severidad(a) >= severidad(b) {
		return a
	}
	return b
}

// Rotulo es cómo se le dice a un humano.
var Rotulo = map[Clase]string{
	Informativa:  "informativa",
	Reversible:   "reversible",
	Costosa:      "costosa",
	Irreversible: "irreversible",
}

// ClaveParametro es el umbral que le corresponde a cada clase.
var ClaveParametro = map[Clase]string{
	Informativa:  "accion.informativa",
	Reversible:   "accion.reversible",
	Costosa:      "accion.costosa",
	Irreversible: "accion.irreversible",
}

// señal es un patrón que delata una clase de acción, con el nombre de lo que
// disparó — porque un veredicto que no dice qué lo disparó no se puede discutir.
type señal struct {
	re    *regexp.Regexp
	clase Clase
	que   string
	// ejemplo es una frase que este patrón TIENE que reconocer. No es
	// documentación: hay un test que los corre todos.
	//
	// Está acá porque un patrón se rompe en silencio. `rm\s+-[rf]` con un  al
	// final dejó de reconocer "rm -rf" —la clase de char toma UNA letra, y la "f"
	// que sigue impide el límite de palabra— y el resultado no fue un error: fue
	// un borrado autorizado como si fuera una respuesta.
	ejemplo string
}

// Las señales están en castellano y en inglés porque los agentes escriben en los
// dos, a veces en la misma frase. Se buscan como palabras completas para que
// "borrador" no dispare "borrar".
//
// La lista no pretende ser exhaustiva: es imposible, y no hace falta. Lo que no
// reconoce cae en la clase que el agente declaró, y si el agente no declaró
// nada, en la más grave. El default protege; la lista sube la precisión.
var señales = []señal{
	// Irreversible
	pat(Irreversible, `borrar|eliminar|elimina|borra|delete|remove|destroy|destruir|purgar|purge|wipe`, "borrado", "borrar los registros viejos"),
	pat(Irreversible, `drop\s+(table|database|schema|index)|truncate|drop_all`, "DDL destructivo", "drop table sesiones"),
	pat(Irreversible, `rm\s+-[rf]+|rmdir|del\s+/[sqf]+|remove-item\s+-recurse`, "borrado de archivos", "hacer rm -rf en la carpeta de build"),
	pat(Irreversible, `force[-\s]?push|push\s+--force|reset\s+--hard|git\s+clean|filter-branch`, "reescritura de historia", "hacer un force push a main"),
	pat(Irreversible, `publicar|publish|postear|post\b|tuitear|tweet|deploy\w*\s+a\s+prod\w*|release`, "publicación", "publicar el post en el blog"),
	pat(Irreversible, `enviar|mandar|send\b|email|correo|notificar|notify|sms|mensaje\s+a`, "envío a terceros", "enviar el mail a los suscriptos"),
	pat(Irreversible, `pagar|cobrar|transferir|transfer|charge|refund|reembolsar|facturar`, "movimiento de dinero", "transferir el saldo a la cuenta"),
	pat(Irreversible, `revocar|revoke|rotar\s+(clave|token|secret)|invalidar\s+sesion`, "revocación de credenciales", "revocar el token de acceso"),

	// Costosa
	pat(Costosa, `deploy|desplegar|rollout|release\s+candidate|promover\s+a`, "despliegue", "desplegar la versión nueva"),
	pat(Costosa, `migrar|migration|migracion|migración|alter\s+table|backfill|reindex`, "migración", "migrar la base de datos"),
	pat(Costosa, `provisionar|provision|terraform\s+apply|escalar|scale\b|autoscal`, "infraestructura", "escalar el cluster a 8 nodos"),
	pat(Costosa, `comprar|contratar|suscribir|subscribe|purchase|alquilar`, "gasto", "comprar el dominio"),
	pat(Costosa, `restaurar\s+backup|restore\b|failover|switchover`, "recuperación", "restaurar backup del martes"),

	// Reversible
	pat(Reversible, `editar|modificar|cambiar|refactor\w*|reescribir|renombrar|rename`, "edición", "refactorizar el módulo de cobro"),
	pat(Reversible, `crear|agregar|añadir|add\b|write|escribir|generar|implementar`, "creación", "crear el archivo de configuración"),
	pat(Reversible, `commit|branch|rama|stash|merge|rebase|pull\s+request|abrir\s+pr`, "operación de git", "hacer un commit con el cambio"),
	pat(Reversible, `instalar|install|npm\s+i|go\s+get|pip\s+install`, "instalación", "instalar la dependencia nueva"),

	// Informativa
	pat(Informativa, `explicar|responder|contestar|resumir|describir|listar|mostrar`, "respuesta", "explicar cómo funciona el pool"),
	pat(Informativa, `leer|revisar|analizar|inspeccionar|buscar|consultar|read\b|search`, "lectura", "leer el archivo de configuración"),
}

// pat compila una señal. Los dos \b son la diferencia entre reconocer una
// palabra y reconocer un pedazo de otra: sin el de la derecha, "escribir un
// borrador" disparaba la clase de un borrado.
func pat(c Clase, cuerpo, que, ejemplo string) señal {
	return señal{re: regexp.MustCompile(`(?i)\b(?:` + cuerpo + `)\b`), clase: c, que: que, ejemplo: ejemplo}
}

// Inferida es lo que el texto de la acción delata por sí solo.
type Inferida struct {
	Clase   Clase
	Porques []string // qué patrones dispararon, del más grave al menos
	Ninguna bool     // el texto no dijo nada reconocible
}

// Clasificar mira el texto de la acción y se queda con la señal MÁS GRAVE que
// encuentre. No promedia ni vota: una acción que menciona un borrado es un
// borrado aunque mencione diez lecturas.
func Clasificar(texto string) Inferida {
	var vistos []señal
	peor := Informativa
	hubo := false
	for _, s := range señales {
		if s.re.MatchString(texto) {
			vistos = append(vistos, s)
			if !hubo || severidad(s.clase) > severidad(peor) {
				peor = s.clase
			}
			hubo = true
		}
	}
	if !hubo {
		return Inferida{Ninguna: true}
	}
	sort.SliceStable(vistos, func(i, j int) bool { return severidad(vistos[i].clase) > severidad(vistos[j].clase) })
	var porques []string
	for _, s := range vistos {
		if s.clase == peor {
			porques = append(porques, s.que)
		}
	}
	return Inferida{Clase: peor, Porques: porques}
}

// Decidir combina lo declarado con lo inferido. Es una función aparte y con su
// propio test porque es la regla que hace que declarar la clase no sea una forma
// de esquivar el control.
func Decidir(declarada string, texto string) (final Clase, explicacion string) {
	inf := Clasificar(texto)
	dec, valida := Valida(declarada)

	switch {
	case strings.TrimSpace(declarada) == "" && inf.Ninguna:
		// Ni el agente dijo qué clase es, ni el texto lo delata. No se adivina
		// hacia abajo: si no se sabe qué se está por hacer, se pide lo máximo.
		return Irreversible, "no se declaró la clase de acción y el texto no permite inferirla: se pide el respaldo máximo"
	case strings.TrimSpace(declarada) == "":
		return inf.Clase, fmt.Sprintf("inferida del texto (%s)", strings.Join(inf.Porques, ", "))
	case !valida:
		return Irreversible, fmt.Sprintf("%q no es una clase válida: se pide el respaldo máximo", declarada)
	case inf.Ninguna:
		return dec, "declarada por quien pide"
	}

	final = LaMasGrave(dec, inf.Clase)
	switch {
	case final == dec && dec == inf.Clase:
		return final, fmt.Sprintf("declarada y confirmada por el texto (%s)", strings.Join(inf.Porques, ", "))
	case final == dec:
		return final, "declarada por quien pide (más estricta que lo que sugiere el texto)"
	default:
		return final, fmt.Sprintf("declarada %q, pero el texto dice %s (%s): manda la más estricta",
			Rotulo[dec], Rotulo[inf.Clase], strings.Join(inf.Porques, ", "))
	}
}
