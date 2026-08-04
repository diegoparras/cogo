// Package parametros es el único lugar donde viven los números de COGO.
//
// # POR QUÉ EXISTE
//
// Todo motor de reglas tiene constantes: cuántos días dura fresca una decisión,
// cuánta evidencia hace falta para autorizar un borrado, cuántas observaciones
// se necesitan antes de creerle a una estadística. Repartidas por el código son
// invisibles: nadie sabe que están, nadie sabe qué pasa si se mueven, y quien
// las quiere cambiar tiene que recompilar.
//
// Juntas y auto-descriptas son otra cosa: son la superficie de control del
// sistema. Cada parámetro trae su etiqueta, qué hace, en qué unidad, entre qué
// valores es válido y qué se afloja si se mueve. El panel del visor se GENERA
// de acá — no hay una lista de controles escrita a mano que pueda desincronizarse
// de lo que el motor realmente lee.
//
// # LOS DOS MODOS
//
// El default es que esto no exista para el usuario. COGO decide, y decide bien:
// ningún flujo normal pide tocar un número. El vault de alguien que nunca abrió
// el panel no tiene siquiera archivo de parámetros.
//
// Y cuando hace falta, está TODO. No un subconjunto "seguro": todo, con su
// efecto escrito y con los que aflojan el sistema marcados como tales. La
// diferencia entre esconder los controles y no necesitarlos es la que separa una
// herramienta condescendiente de una que confía en quien la usa.
//
// # QUÉ SE GUARDA
//
// Solo lo que difiere del default. Un vault sin tocar no tiene archivo, y
// actualizar COGO mueve los defaults hacia adelante sin pisar lo que alguien
// decidió a mano. Es la misma razón por la que un .gitconfig no lista las 400
// opciones de git.
package parametros

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
)

// Tipo es cómo se edita un parámetro; determina el control que dibuja el panel.
type Tipo string

const (
	TEntero   Tipo = "entero"
	TBooleano Tipo = "booleano"
	TOpcion   Tipo = "opcion"
)

// Def describe un parámetro: lo suficiente como para poder dibujarlo, validarlo
// y explicarlo sin que nadie escriba nada dos veces.
type Def struct {
	Clave  string `json:"clave"`
	Grupo  string `json:"grupo"`
	Rotulo string `json:"rotulo"`
	// Explica qué es el número. Efecto dice qué cambia si lo movés — que es la
	// pregunta que de verdad tiene quien está por moverlo.
	Explica string `json:"explica"`
	Efecto  string `json:"efecto"`
	Unidad  string `json:"unidad,omitempty"`
	Tipo    Tipo   `json:"tipo"`

	Default  any      `json:"default"`
	Min      int      `json:"min,omitempty"`
	Max      int      `json:"max,omitempty"`
	Opciones []string `json:"opciones,omitempty"`

	// Afloja marca los parámetros que pueden hacer que COGO afirme más de lo que
	// puede sostener. No los bloquea: los señala. Quien los mueve tiene derecho a
	// hacerlo y derecho a saberlo.
	Afloja bool `json:"afloja,omitempty"`
}

// Estado es un parámetro con su valor actual, para el panel.
type Estado struct {
	Def
	Valor     any  `json:"valor"`
	EsDefault bool `json:"es_default"`
}

// Registro es el catálogo completo, en el orden en que se muestra.
var Registro = []Def{
	// ── Frescura ────────────────────────────────────────────────────────────
	// Una nota no envejece igual según lo que afirme. Un invariante del negocio
	// dura años; el comando exacto para levantar el entorno, semanas. Estos son
	// los días que una nota de cada tipo se considera fresca: pasado eso baja a
	// amarillo, y al doble expira a rojo.
	dias("frescura.constraint", "constraint", 365,
		"Restricciones e invariantes: lo que no puede dejar de ser cierto.",
		"Más días = las restricciones tardan más en pedir revisión."),
	dias("frescura.decision", "decision", 180,
		"Decisiones tomadas y por qué.",
		"Más días = las decisiones viejas siguen contando como frescas más tiempo."),
	dias("frescura.architecture", "architecture", 180,
		"Cómo está armado el sistema.",
		"Más días = la arquitectura registrada envejece más lento."),
	dias("frescura.runbook", "runbook", 90,
		"Procedimientos: cómo se hace tal cosa.",
		"Más días = los procedimientos piden revisión más tarde."),
	dias("frescura.bug", "bug", 60,
		"Errores encontrados y su estado.",
		"Más días = los bugs registrados se dan por vigentes más tiempo."),
	dias("frescura.command", "command", 30,
		"Comandos exactos. Es lo que más rápido queda viejo.",
		"Más días = un comando que ya no funciona sigue apareciendo verde más tiempo."),
	dias("frescura.otros", "cualquier otro tipo", 90,
		"El default conservador para un tipo que COGO no conoce.",
		"Más días = los tipos desconocidos envejecen más lento."),

	// ── Umbrales por clase de acción ────────────────────────────────────────
	// Cuánto respaldo hace falta para autorizar cada clase de acción. No es lo
	// mismo explicar algo que borrar una base: pedir la misma evidencia para las
	// dos es pedir de menos en un lado o de más en el otro.
	estado("accion.informativa", "responder, explicar, resumir", Asserted,
		"Acciones que solo producen texto. Nada se rompe si la nota estaba mal; sí se dice algo falso.",
		"Bajarlo deja responder apoyado en notas sin ningún respaldo."),
	estado("accion.reversible", "editar código, crear un archivo, commitear", CheckDeclared,
		"Acciones que se deshacen con git o con un botón.",
		"Bajarlo deja escribir código apoyado en afirmaciones sin criterio de verificación."),
	estado("accion.costosa", "deploy, migración, provisionar, gastar", ClaimedPassed,
		"Acciones que cuestan plata o tiempo revertir, aunque se pueda.",
		"Bajarlo deja desplegar apoyado en algo que nadie declaró que funcione."),
	estado("accion.irreversible", "borrar datos, publicar, enviar, forzar push", Verified,
		"Acciones sin vuelta atrás. Es la única clase que por default exige un check EJECUTADO, no declarado.",
		"Bajarlo deja borrar o publicar apoyado en la palabra de un agente. Es el parámetro más peligroso del sistema."),
	booleano("accion.exigir_respaldo", "exigir que se declare en qué se apoya", true,
		"Una acción que no cita ninguna nota no tiene respaldo que evaluar.",
		"Apagarlo autoriza acciones que no declaran en qué se apoyan.", true),

	// ── Materialidad de las citas ───────────────────────────────────────────
	entero("ancla.caracteres_minimos", "texto mínimo para relocalizar una cita", 12, 1, 200, "caracteres",
		"Cuando un archivo cambia, COGO busca el texto citado donde haya quedado. Una cita de una línea que dice \"}\" coincide en cualquier parte: por debajo de este mínimo, COGO prefiere avisar antes que adivinar.",
		"Bajarlo hace que COGO relocalice citas poco distintivas — y una relocalización equivocada absuelve un cambio real.", true),

	// ── Calibración por emisor ──────────────────────────────────────────────
	booleano("calibracion.activa", "usar el historial de cada emisor", false,
		"Cuando un emisor declara \"el check pasa\" y después el check ejecutado falla, eso queda registrado. Con suficientes casos se puede dejar de creerle igual a todos.",
		"Encenderlo hace que las declaraciones de un emisor con mal historial valgan menos.", false),
	entero("calibracion.minimo_declaraciones", "mínimo de declaraciones para juzgar a un emisor", 20, 3, 1000, "declaraciones",
		"Con tres casos no se puede decir nada de nadie. Por debajo de este número, el emisor no se penaliza.",
		"Bajarlo hace que COGO saque conclusiones de muestras chicas.", true),
	entero("calibracion.desmentidas_toleradas", "desmentidas toleradas", 10, 0, 100, "%",
		"Qué porcentaje de declaraciones desmentidas se tolera antes de dejar de tomar las declaraciones de ese emisor como suficientes.",
		"Subirlo tolera emisores que declaran mal más seguido.", true),

	// ── Ventanas por supervivencia ──────────────────────────────────────────
	booleano("supervivencia.activa", "derivar las ventanas de los datos", false,
		"En vez de la tabla de días de arriba, calcular cuánto duran realmente las notas de cada tipo antes de ser desmentidas o corregidas.",
		"Encenderlo reemplaza los días fijos por los que salen del historial del vault, tipo por tipo, solo donde haya datos suficientes.", false),
	entero("supervivencia.minimo_observaciones", "mínimo de notas para estimar un tipo", 30, 5, 1000, "notas",
		"Cuántas notas de un tipo tienen que haber vivido y muerto para poder estimar su ventana. Por debajo, se usa el día fijo.",
		"Bajarlo produce ventanas estimadas sobre muy pocos casos.", true),
	entero("supervivencia.cuantil", "punto de corte de la curva", 20, 1, 90, "%",
		"La ventana se pone donde ya falló este porcentaje de las notas del tipo. 20% significa: fresca mientras 4 de cada 5 notas parecidas seguían siendo ciertas.",
		"Subirlo alarga las ventanas: se acepta que más notas ya estén mal antes de pedir revisión.", true),

	// ── Ejecución ───────────────────────────────────────────────────────────
	entero("runner.timeout_maximo", "techo del timeout de un check", 15, 1, 240, "minutos",
		"Ningún check declarado en runner.yaml puede pedir más que esto.",
		"Subirlo permite checks que bloquean la verificación por más tiempo.", false),
}

// helpers del catálogo, para que las 19 definiciones se lean como una tabla y no
// como diecinueve structs.
func dias(clave, rotulo string, def int, explica, efecto string) Def {
	return entero(clave, rotulo, def, 1, 3650, "días", explica, efecto, false)
}

func entero(clave, rotulo string, def, min, max int, unidad, explica, efecto string, afloja ...bool) Def {
	return Def{Clave: clave, Grupo: grupoDe(clave), Rotulo: rotulo, Explica: explica, Efecto: efecto,
		Unidad: unidad, Tipo: TEntero, Default: def, Min: min, Max: max, Afloja: len(afloja) > 0 && afloja[0]}
}

func booleano(clave, rotulo string, def bool, explica, efecto string, afloja bool) Def {
	return Def{Clave: clave, Grupo: grupoDe(clave), Rotulo: rotulo, Explica: explica, Efecto: efecto,
		Tipo: TBooleano, Default: def, Afloja: afloja}
}

func estado(clave, rotulo, def string, explica, efecto string) Def {
	return Def{Clave: clave, Grupo: grupoDe(clave), Rotulo: rotulo, Explica: explica, Efecto: efecto,
		Tipo: TOpcion, Default: def, Opciones: EstadosOrdenados, Afloja: true}
}

// Los estados del retículo, del más débil al más fuerte. Se repiten acá y no se
// importan de internal/confidence para que parametros no dependa de nada: es la
// hoja del árbol, la importa todo el mundo.
const (
	Quarantined   = "quarantined"
	Refuted       = "refuted"
	Contradicted  = "contradicted"
	Stale         = "stale"
	Asserted      = "asserted"
	CheckDeclared = "check_declared"
	ClaimedPassed = "claimed_passed"
	Verified      = "verified"
)

// EstadosOrdenados va de menos a más exigente. El orden ES el retículo, y el
// panel lo muestra así.
var EstadosOrdenados = []string{Quarantined, Refuted, Contradicted, Stale, Asserted, CheckDeclared, ClaimedPassed, Verified}

func grupoDe(clave string) string {
	if i := strings.Index(clave, "."); i > 0 {
		return clave[:i]
	}
	return "general"
}

// GruposOrdenados es el orden en que el panel muestra las secciones: de lo que
// más se toca a lo que casi nunca.
var GruposOrdenados = []string{"frescura", "accion", "ancla", "calibracion", "supervivencia", "runner"}

// TituloGrupo es cómo se llama cada sección para un humano.
var TituloGrupo = map[string]string{
	"frescura":      "Cuánto dura fresca cada cosa",
	"accion":        "Cuánto respaldo pide cada tipo de acción",
	"ancla":         "Cuándo un archivo que cambió invalida una nota",
	"calibracion":   "Cuánto vale la palabra de cada emisor",
	"supervivencia": "Ventanas derivadas de los datos, no de la tabla",
	"runner":        "Ejecución de checks",
}

var porClave = func() map[string]Def {
	m := make(map[string]Def, len(Registro))
	for _, d := range Registro {
		m[d.Clave] = d
	}
	return m
}()

// Set son los parámetros vigentes de un vault.
type Set struct {
	mu    sync.RWMutex
	ruta  string
	valor map[string]any // SOLO lo que difiere del default
}

// registro, si está puesto, recibe cada cambio de parámetro. Es el punto por el
// que el servidor lo manda a la auditoría, sin que este paquete sepa nada de
// HTTP ni de quién llamó.
var registro func(clave string, antes, despues any, quien string)

// SetRegistro instala el testigo de los cambios.
func SetRegistro(f func(clave string, antes, despues any, quien string)) { registro = f }

// Cargar lee los parámetros de un vault. Un vault sin archivo devuelve los
// defaults y ningún error: no tener parámetros propios es lo normal.
func Cargar(vault string) *Set {
	s := &Set{ruta: filepath.Join(vault, ".cogo", "parametros.json"), valor: map[string]any{}}
	b, err := os.ReadFile(s.ruta)
	if err != nil {
		return s
	}
	var crudo map[string]any
	if json.Unmarshal(b, &crudo) != nil {
		return s
	}
	for k, v := range crudo {
		if _, ok := porClave[k]; !ok {
			continue // parámetro de otra versión: se ignora, no se rompe
		}
		if norm, err := normalizar(porClave[k], v); err == nil {
			s.valor[k] = norm
		}
	}
	return s
}

// Defaults devuelve un Set sin nada editado. Para tests y para el modo sin vault.
func Defaults() *Set { return &Set{valor: map[string]any{}} }

func (s *Set) crudo(clave string) any {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if v, ok := s.valor[clave]; ok {
		return v
	}
	return porClave[clave].Default
}

// Entero devuelve un parámetro numérico. Una clave que no existe devuelve 0: es
// un error de programación, y el generador de tests lo detecta antes.
func (s *Set) Entero(clave string) int {
	if v, ok := s.crudo(clave).(int); ok {
		return v
	}
	if f, ok := s.crudo(clave).(float64); ok {
		return int(f)
	}
	return 0
}

func (s *Set) Bool(clave string) bool {
	v, _ := s.crudo(clave).(bool)
	return v
}

func (s *Set) Texto(clave string) string {
	v, _ := s.crudo(clave).(string)
	return v
}

// Poner cambia un parámetro, validando contra su definición. quien queda en el
// registro de cambios.
func (s *Set) Poner(clave string, v any, quien string) error {
	d, ok := porClave[clave]
	if !ok {
		return fmt.Errorf("no existe el parámetro %q", clave)
	}
	norm, err := normalizar(d, v)
	if err != nil {
		return err
	}
	antes := s.crudo(clave)

	s.mu.Lock()
	if igual(norm, d.Default) {
		delete(s.valor, clave) // volver al default es no guardar nada
	} else {
		s.valor[clave] = norm
	}
	s.mu.Unlock()

	if registro != nil && !igual(antes, norm) {
		registro(clave, antes, norm, quien)
	}
	return nil
}

// Restaurar vuelve un parámetro a su default.
func (s *Set) Restaurar(clave string, quien string) error {
	d, ok := porClave[clave]
	if !ok {
		return fmt.Errorf("no existe el parámetro %q", clave)
	}
	return s.Poner(clave, d.Default, quien)
}

// RestaurarTodo deja el vault como recién instalado.
func (s *Set) RestaurarTodo(quien string) {
	s.mu.RLock()
	claves := make([]string, 0, len(s.valor))
	for k := range s.valor {
		claves = append(claves, k)
	}
	s.mu.RUnlock()
	for _, k := range claves {
		_ = s.Restaurar(k, quien)
	}
}

// Guardar persiste solo lo editado. Sin nada editado, borra el archivo: un vault
// en defaults no tiene por qué tener uno.
func (s *Set) Guardar() error {
	if s.ruta == "" {
		return nil
	}
	s.mu.RLock()
	n := len(s.valor)
	b, err := json.MarshalIndent(s.valor, "", "  ")
	s.mu.RUnlock()
	if err != nil {
		return err
	}
	if n == 0 {
		if err := os.Remove(s.ruta); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(s.ruta), 0o755); err != nil {
		return err
	}
	return os.WriteFile(s.ruta, append(b, '\n'), 0o644)
}

// Listar devuelve el catálogo con los valores vigentes, agrupado y en orden.
func (s *Set) Listar() []Estado {
	out := make([]Estado, 0, len(Registro))
	for _, d := range Registro {
		v := s.crudo(d.Clave)
		out = append(out, Estado{Def: d, Valor: v, EsDefault: igual(v, d.Default)})
	}
	sort.SliceStable(out, func(i, j int) bool {
		return orden(out[i].Grupo) < orden(out[j].Grupo)
	})
	return out
}

// Editados cuenta cuántos parámetros difieren del default. El panel lo usa para
// decir "12 de 19 en su valor original" sin que nadie los cuente a mano.
func (s *Set) Editados() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.valor)
}

func orden(grupo string) int {
	for i, g := range GruposOrdenados {
		if g == grupo {
			return i
		}
	}
	return len(GruposOrdenados)
}

// normalizar convierte lo que llegó (de JSON, de un formulario) al tipo del
// parámetro y verifica que sea un valor válido. Es el único lugar donde se
// valida: si un valor pasó por acá, el resto del código puede usarlo sin mirar.
func normalizar(d Def, v any) (any, error) {
	switch d.Tipo {
	case TEntero:
		n, err := aEntero(v)
		if err != nil {
			return nil, fmt.Errorf("%s: se esperaba un número entero", d.Clave)
		}
		if n < d.Min || n > d.Max {
			return nil, fmt.Errorf("%s: %d está fuera del rango permitido (%d a %d)", d.Clave, n, d.Min, d.Max)
		}
		return n, nil
	case TBooleano:
		switch t := v.(type) {
		case bool:
			return t, nil
		case string:
			b, err := strconv.ParseBool(t)
			if err != nil {
				return nil, fmt.Errorf("%s: se esperaba verdadero o falso", d.Clave)
			}
			return b, nil
		}
		return nil, fmt.Errorf("%s: se esperaba verdadero o falso", d.Clave)
	case TOpcion:
		str, ok := v.(string)
		if !ok {
			return nil, fmt.Errorf("%s: se esperaba una de las opciones", d.Clave)
		}
		for _, o := range d.Opciones {
			if o == str {
				return str, nil
			}
		}
		return nil, fmt.Errorf("%s: %q no es una opción válida (%s)", d.Clave, str, strings.Join(d.Opciones, ", "))
	}
	return nil, fmt.Errorf("%s: tipo desconocido", d.Clave)
}

func aEntero(v any) (int, error) {
	switch t := v.(type) {
	case int:
		return t, nil
	case float64:
		if t != float64(int(t)) {
			return 0, fmt.Errorf("no entero")
		}
		return int(t), nil
	case string:
		return strconv.Atoi(strings.TrimSpace(t))
	}
	return 0, fmt.Errorf("no numérico")
}

// igual compara dos valores de parámetro sin importar si vinieron como int o
// como float64 (JSON no distingue).
func igual(a, b any) bool {
	if na, err := aEntero(a); err == nil {
		if nb, err := aEntero(b); err == nil {
			return na == nb
		}
	}
	return fmt.Sprint(a) == fmt.Sprint(b)
}
