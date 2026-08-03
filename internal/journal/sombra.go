package journal

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/diegoparras/cogo/internal/confidence"
	"github.com/diegoparras/cogo/internal/core"
)

// El modo sombra corre la máquina de estados AL LADO del motor vigente, sin que
// nada dependa de ella, y anota cada vez que discrepan.
//
// UNA ACLARACIÓN QUE SALIÓ DE CORRER ESTO
//
// La máquina de estados NO reemplaza al motor de color: modela UN EJE de él, el
// del check — si hay criterio declarado, si se ejecutó, si pasó. El color final
// depende además de la fuerza de la evidencia, de la frescura y de las
// dependencias, que la máquina no ve porque no viven en los eventos.
//
// Se descubrió acá: una nota SIN evidencia pero con el check declarado como
// pasado sube a `claimed_passed`, que proyecta verde, mientras el motor vigente
// la deja roja con razón. El motor tiene razón, y la máquina no está equivocada:
// están midiendo cosas distintas.
//
// La conclusión es que el color es el MEET de varios ejes, y el estado de la
// máquina es uno de ellos. Eso encaja con el `min()` que el motor ya hace, y es
// lo que la Fase 3 tiene que unificar. Hasta entonces, comparar el estado contra
// el color directamente produce divergencias que no significan nada.
//
// Es la única forma honesta de decidir el reemplazo. Un motor nuevo que pasa sus
// tests no prueba nada sobre un vault real: lo que hay que saber es en qué notas
// concretas cambiaría el color y por qué, y si cada diferencia es una corrección
// buscada o un error. Esa lista es la evidencia para decidir el corte — y si no
// aparece ninguna divergencia, también dice algo.

// Divergencia es una nota donde los dos motores no coinciden.
type Divergencia struct {
	Cuando      time.Time `json:"cuando"`
	NoteID      string    `json:"note_id"`
	ColorViejo  string    `json:"color_viejo"`
	RazonVieja  string    `json:"razon_vieja"`
	EstadoNuevo string    `json:"estado_nuevo"`
	ColorNuevo  string    `json:"color_nuevo"`
	// Explicada la marca una persona al revisarla, para separar las correcciones
	// buscadas de los errores. Mientras haya divergencias sin explicar, no se
	// corta.
	Explicada bool   `json:"explicada"`
	Nota      string `json:"nota,omitempty"`
}

// TechoPorEvidencia es el otro eje: la fuerza de la evidencia pone un tope al
// estado que una nota puede alcanzar, sin importar cuánto se haya verificado el
// check. Sin evidencia observada no hay verificación que valga.
//
// Vive acá y no en la máquina porque no se deriva de eventos: se lee de la nota.
func TechoPorEvidencia(n *core.Note) confidence.Estado {
	switch core.TierDeEvidencia(n.Evidence) {
	case core.TierObserved:
		return confidence.Verified // sin techo: puede llegar arriba de todo
	case core.TierReported, core.TierReasoned:
		return confidence.CheckDeclared // tope amarillo
	default:
		return confidence.Contradicted // sin evidencia: no pasa de rojo
	}
}

// EstadoEfectivo combina el eje del check (lo que dicen los eventos) con el
// techo que impone la evidencia. Es el meet de los dos, o sea: manda el más
// débil, que es la regla de todo el sistema.
func EstadoEfectivo(est confidence.Estado, n *core.Note) confidence.Estado {
	return confidence.Meet(est, TechoPorEvidencia(n))
}

// Sombra compara y acumula. Es deliberadamente pasiva: no escribe en las notas
// ni cambia ningún color.
type Sombra struct {
	dir string
	mu  sync.Mutex
}

func NuevaSombra(vault string) *Sombra { return &Sombra{dir: filepath.Join(vault, ".cogo")} }

// Comparar contrasta el veredicto del motor vigente con el estado que produce el
// fold del journal, y registra las diferencias. Devuelve cuántas encontró.
func (s *Sombra) Comparar(verdicts map[string]core.Verdict, estados map[string]confidence.Estado, ahora time.Time) int {
	var nuevas []Divergencia
	ids := make([]string, 0, len(verdicts))
	for id := range verdicts {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	for _, id := range ids {
		v := verdicts[id]
		est, hay := estados[id]
		if !hay {
			// La nota no tiene eventos: el journal todavía no sabe nada de ella.
			// No es una divergencia, es una nota sin sembrar.
			continue
		}
		if est.Color() == v.Color.String() {
			continue
		}
		nuevas = append(nuevas, Divergencia{
			Cuando: ahora.UTC(), NoteID: id,
			ColorViejo: v.Color.String(), RazonVieja: v.Reason,
			EstadoNuevo: est.String(), ColorNuevo: est.Color(),
		})
	}
	if len(nuevas) > 0 {
		s.anotar(nuevas)
	}
	return len(nuevas)
}

func (s *Sombra) anotar(ds []Divergencia) {
	s.mu.Lock()
	defer s.mu.Unlock()
	_ = os.MkdirAll(s.dir, 0o755)
	f, err := os.OpenFile(filepath.Join(s.dir, "divergencias.jsonl"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return // la sombra nunca debe romper el camino principal
	}
	defer f.Close()
	for _, d := range ds {
		if b, err := json.Marshal(d); err == nil {
			_, _ = f.Write(append(b, '\n'))
		}
	}
}

// Divergencias devuelve lo acumulado, para revisarlo.
func (s *Sombra) Divergencias() ([]Divergencia, error) {
	b, err := os.ReadFile(filepath.Join(s.dir, "divergencias.jsonl"))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []Divergencia
	for _, ln := range strings.Split(string(b), "\n") {
		if strings.TrimSpace(ln) == "" {
			continue
		}
		var d Divergencia
		if json.Unmarshal([]byte(ln), &d) == nil {
			out = append(out, d)
		}
	}
	return out, nil
}

// Resumen agrupa las divergencias por el par de colores, que es como conviene
// mirarlas: veinte notas que pasan de verde a amarillo por la misma razón son
// un solo caso a decidir, no veinte.
func (s *Sombra) Resumen() (map[string]int, error) {
	ds, err := s.Divergencias()
	if err != nil {
		return nil, err
	}
	out := map[string]int{}
	for _, d := range ds {
		out[fmt.Sprintf("%s -> %s (%s)", d.ColorViejo, d.ColorNuevo, d.EstadoNuevo)]++
	}
	return out, nil
}

// Sembrar registra el estado en que el motor vigente ve hoy cada nota, para que
// el journal tenga de dónde partir. Sin esto el fold arrancaría desde cero y
// toda nota existente divergiría, lo que no diría nada útil.
//
// Es honesto sobre lo que hace: no inventa la historia que no se registró, anota
// "COGO observó esta nota así, en este momento".
func Sembrar(j *Journal, vault map[string]*core.Note, verdicts map[string]core.Verdict) (int, error) {
	evs, err := j.All()
	if err != nil {
		return 0, err
	}
	yaTiene := map[string]bool{}
	for _, e := range evs {
		yaTiene[e.NoteID] = true
	}

	ids := make([]string, 0, len(vault))
	for id := range vault {
		ids = append(ids, id)
	}
	sort.Strings(ids) // sembrar en orden estable: el journal es un registro

	n := 0
	for _, id := range ids {
		if yaTiene[id] {
			continue
		}
		nota := vault[id]
		for _, e := range eventosDeSiembra(nota, verdicts[id]) {
			if _, err := j.Append(e); err != nil {
				return n, err
			}
		}
		n++
	}
	return n, nil
}

// eventosDeSiembra traduce el estado observable de una nota a la secuencia de
// eventos que la habría llevado ahí. No es su historia real —esa no se
// registró— sino la mínima que explica lo que hoy se ve.
func eventosDeSiembra(n *core.Note, v core.Verdict) []Event {
	quien := n.Check.AttestedBy
	if strings.TrimSpace(quien) == "" {
		quien = "sembrado"
	}
	var out []Event
	add := func(kind, guard string) {
		out = append(out, Event{NoteID: n.ID, Kind: kind, Emitter: quien, Guard: guard,
			Payload: json.RawMessage(`{"origen":"siembra"}`)})
	}

	if strings.TrimSpace(n.Check.Test) != "" {
		add("CheckDeclared", "")
	}
	switch n.Check.Status {
	case "passed":
		if n.Check.Attestation() == core.AttestExecuted {
			add("VerificationStarted", "")
			add("CheckExecuted", "ejecucion_ok")
		} else {
			add("VerifyDeclared", "declara_un_tercero")
		}
	case "failed":
		add("VerificationStarted", "")
		add("CheckExecuted", "ejecucion_falla")
	}
	// El motor vigente ya decidió que está vencida o contradicha: se registra,
	// porque es información que el fold no puede derivar por su cuenta.
	if v.Color == core.Red && strings.Contains(v.Reason, "contradic") {
		add("ContradictionOpened", "")
	} else if strings.Contains(v.Reason, "venc") || strings.Contains(v.Reason, "expirada") {
		add("TTLExpired", "")
	}
	return out
}
