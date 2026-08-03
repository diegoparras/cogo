// Comando gen: lee transitions.yaml, lo valida, y escribe states_gen.go.
//
// Las validaciones son la razón de que este generador exista. Una tabla de
// estados escrita a mano en Go compila igual con dos transiciones que se pisan,
// con un estado al que no se llega nunca, o con un retículo mal ordenado. Acá
// eso rompe el build, que es cuando conviene enterarse.
//
//	go run ./internal/confidence/gen -in transitions.yaml -out states_gen.go
package main

import (
	"bytes"
	"flag"
	"fmt"
	"go/format"
	"os"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

type spec struct {
	Version     int          `yaml:"version"`
	States      []state      `yaml:"states"`
	Decisions   []decision   `yaml:"decisions"`
	Events      []event      `yaml:"events"`
	Transitions []transition `yaml:"transitions"`
	Initial     string       `yaml:"initial"`
}

type state struct {
	ID        string `yaml:"id"`
	Color     string `yaml:"color"`
	Rank      *int   `yaml:"rank"`
	Transient bool   `yaml:"transient"`
	Doc       string `yaml:"doc"`
}

type decision struct {
	ID     string  `yaml:"id"`
	Doc    string  `yaml:"doc"`
	Guards []guard `yaml:"guards"`
}

type guard struct {
	ID  string `yaml:"id"`
	Doc string `yaml:"doc"`
}

type event struct {
	ID      string `yaml:"id"`
	Doc     string `yaml:"doc"`
	Emitter string `yaml:"emitter"`
}

type transition struct {
	From  string `yaml:"from"`
	To    string `yaml:"to"`
	On    string `yaml:"on"`
	Guard string `yaml:"guard"`
}

func main() {
	in := flag.String("in", "transitions.yaml", "tabla de transiciones")
	out := flag.String("out", "states_gen.go", "archivo Go a escribir")
	flag.Parse()

	raw, err := os.ReadFile(*in)
	check(err)
	var s spec
	check(yaml.Unmarshal(raw, &s))

	if errs := validar(&s); len(errs) > 0 {
		fmt.Fprintf(os.Stderr, "\n%s no es una máquina de estados válida:\n\n", *in)
		for _, e := range errs {
			fmt.Fprintf(os.Stderr, "  · %s\n", e)
		}
		fmt.Fprintln(os.Stderr)
		os.Exit(1)
	}

	code, err := generar(&s)
	check(err)
	check(os.WriteFile(*out, code, 0o644))
	fmt.Printf("%s: %d estados, %d eventos, %d transiciones\n", *out, len(s.States), len(s.Events), len(s.Transitions))
}

// validar junta TODOS los problemas antes de fallar, en vez de morir en el
// primero: si la tabla tiene tres errores, conviene verlos de una.
func validar(s *spec) []string {
	var errs []string
	estados := map[string]state{}
	rangos := map[int]string{}
	for _, st := range s.States {
		if _, dup := estados[st.ID]; dup {
			errs = append(errs, fmt.Sprintf("estado duplicado: %q", st.ID))
		}
		estados[st.ID] = st
		switch st.Color {
		case "green", "yellow", "red":
		default:
			errs = append(errs, fmt.Sprintf("estado %q: color %q desconocido (green|yellow|red)", st.ID, st.Color))
		}
		if st.Transient && st.Rank != nil {
			errs = append(errs, fmt.Sprintf("estado %q: es transitorio, no debe tener rango en el retículo", st.ID))
		}
		if !st.Transient {
			if st.Rank == nil {
				errs = append(errs, fmt.Sprintf("estado %q: falta rank — sin él no está en el retículo y meet no lo puede comparar", st.ID))
				continue
			}
			if otro, dup := rangos[*st.Rank]; dup {
				errs = append(errs, fmt.Sprintf("rango %d repetido entre %q y %q: el orden debe ser total", *st.Rank, otro, st.ID))
			}
			rangos[*st.Rank] = st.ID
		}
	}
	// El retículo tiene que ser una cadena sin huecos: si los rangos son
	// 0,1,3 hay un nivel fantasma y meet devuelve resultados sorprendentes.
	if n := len(rangos); n > 0 {
		for i := 0; i < n; i++ {
			if _, ok := rangos[i]; !ok {
				errs = append(errs, fmt.Sprintf("el retículo tiene un hueco en el rango %d: los rangos deben ser 0..%d sin saltos", i, n-1))
			}
		}
	}

	eventos := map[string]event{}
	for _, e := range s.Events {
		if _, dup := eventos[e.ID]; dup {
			errs = append(errs, fmt.Sprintf("evento duplicado: %q", e.ID))
		}
		eventos[e.ID] = e
	}

	// guarda -> decisión a la que pertenece
	deDecision := map[string]string{}
	guardasDe := map[string][]string{}
	for _, d := range s.Decisions {
		if len(d.Guards) < 2 {
			errs = append(errs, fmt.Sprintf("decisión %q: necesita al menos dos guardas, si no no decide nada", d.ID))
		}
		for _, g := range d.Guards {
			if otra, dup := deDecision[g.ID]; dup {
				errs = append(errs, fmt.Sprintf("guarda %q declarada en dos decisiones (%s y %s)", g.ID, otra, d.ID))
			}
			deDecision[g.ID] = d.ID
			guardasDe[d.ID] = append(guardasDe[d.ID], g.ID)
		}
	}

	if _, ok := estados[s.Initial]; !ok {
		errs = append(errs, fmt.Sprintf("el estado inicial %q no existe", s.Initial))
	}

	// Transiciones: referencias válidas, y guardas del mismo (from,on) que
	// pertenezcan a una única decisión y la cubran entera.
	type clave struct{ from, on string }
	porClave := map[clave][]transition{}
	alcanzables := map[string]bool{s.Initial: true}
	for _, t := range s.Transitions {
		if t.From != "*" {
			if _, ok := estados[t.From]; !ok {
				errs = append(errs, fmt.Sprintf("transición hacia %q: el estado de origen %q no existe", t.To, t.From))
			}
		}
		if _, ok := estados[t.To]; !ok {
			errs = append(errs, fmt.Sprintf("transición desde %q: el estado de destino %q no existe", t.From, t.To))
		}
		if _, ok := eventos[t.On]; !ok {
			errs = append(errs, fmt.Sprintf("transición %s->%s: el evento %q no existe", t.From, t.To, t.On))
		}
		if t.Guard != "" {
			if _, ok := deDecision[t.Guard]; !ok {
				errs = append(errs, fmt.Sprintf("transición %s->%s: la guarda %q no pertenece a ninguna decisión", t.From, t.To, t.Guard))
			}
		}
		alcanzables[t.To] = true
		porClave[clave{t.From, t.On}] = append(porClave[clave{t.From, t.On}], t)
	}

	for k, ts := range porClave {
		if len(ts) == 1 {
			continue
		}
		// Varias transiciones para el mismo estado y evento: solo es
		// determinista si todas discriminan por la misma decisión.
		decs := map[string]bool{}
		usadas := map[string]bool{}
		for _, t := range ts {
			if t.Guard == "" {
				errs = append(errs, fmt.Sprintf("(%s, %s) tiene %d transiciones y una no tiene guarda: el resultado dependería del orden",
					k.from, k.on, len(ts)))
				continue
			}
			// Dos transiciones con la misma guarda desde el mismo par: ambas se
			// disparan a la vez y el destino queda a merced del orden de la
			// tabla, que es exactamente el no-determinismo que hay que evitar.
			if usadas[t.Guard] {
				errs = append(errs, fmt.Sprintf("(%s, %s): dos transiciones con la misma guarda %q — el destino sería ambiguo",
					k.from, k.on, t.Guard))
			}
			decs[deDecision[t.Guard]] = true
			usadas[t.Guard] = true
		}
		if len(decs) > 1 {
			errs = append(errs, fmt.Sprintf("(%s, %s): sus guardas vienen de decisiones distintas (%s), así que pueden dispararse juntas",
				k.from, k.on, strings.Join(claves(decs), ", ")))
			continue
		}
		for d := range decs {
			for _, g := range guardasDe[d] {
				if !usadas[g] {
					errs = append(errs, fmt.Sprintf("(%s, %s): la decisión %q no está cubierta — falta el caso %q",
						k.from, k.on, d, g))
				}
			}
		}
	}

	// Alcanzabilidad: un estado al que no se llega nunca es código muerto que
	// alguien va a intentar usar.
	for _, st := range s.States {
		if !alcanzables[st.ID] {
			errs = append(errs, fmt.Sprintf("el estado %q es inalcanzable: ninguna transición lleva a él", st.ID))
		}
	}

	sort.Strings(errs)
	return errs
}

func claves(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func generar(s *spec) ([]byte, error) {
	var b bytes.Buffer
	fmt.Fprintf(&b, `// Code generated by internal/confidence/gen. NO EDITAR A MANO.
//
// Se genera desde transitions.yaml, que es la fuente única de la máquina de
// estados. Para cambiar algo, editá ese archivo y corré:
//
//	go generate ./internal/confidence

package confidence

// Estado de evidencia de una nota.
type Estado uint8

const (
`)
	// Constantes en orden de rango, con los transitorios AL FINAL. El orden
	// importa: hace que el cero value de Estado sea el de rango 0, es decir el
	// menos confiable. Una nota cuyo estado no se pobló debe leerse como lo
	// menos confiable posible, nunca como algo en lo que se puede apoyar.
	orden := append([]state(nil), s.States...)
	sort.SliceStable(orden, func(i, j int) bool {
		ri, rj := 1<<30, 1<<30 // los transitorios, al fondo
		if orden[i].Rank != nil {
			ri = *orden[i].Rank
		}
		if orden[j].Rank != nil {
			rj = *orden[j].Rank
		}
		return ri < rj
	})
	for _, st := range orden {
		fmt.Fprintf(&b, "\t// %s\n\t%s Estado = iota_placeholder_%s\n", limpiar(st.Doc), ident(st.ID), st.ID)
	}
	fmt.Fprint(&b, ")\n\n")

	// nombres
	fmt.Fprint(&b, "func (e Estado) String() string {\n\tswitch e {\n")
	for _, st := range orden {
		fmt.Fprintf(&b, "\tcase %s:\n\t\treturn %q\n", ident(st.ID), st.ID)
	}
	fmt.Fprint(&b, "\t}\n\treturn \"desconocido\"\n}\n\n")

	// color proyectado
	fmt.Fprint(&b, `// Color es la PROYECCIÓN del estado: lo que ve una persona. Dos estados
// distintos pueden proyectar el mismo color a propósito — la máquina distingue
// situaciones que el semáforo no necesita distinguir.
func (e Estado) Color() string {
	switch e {
`)
	for _, st := range orden {
		fmt.Fprintf(&b, "\tcase %s:\n\t\treturn %q\n", ident(st.ID), st.Color)
	}
	fmt.Fprint(&b, "\t}\n\treturn \"ungraded\"\n}\n\n")

	// rango y transitoriedad
	fmt.Fprint(&b, `// Rango es la posición en el retículo de confianza: mayor es más confiable.
// Los estados transitorios devuelven -1 y no participan de la propagación.
func (e Estado) Rango() int {
	switch e {
`)
	for _, st := range orden {
		r := -1
		if st.Rank != nil {
			r = *st.Rank
		}
		fmt.Fprintf(&b, "\tcase %s:\n\t\treturn %d\n", ident(st.ID), r)
	}
	fmt.Fprint(&b, "\t}\n\treturn -1\n}\n\n")

	fmt.Fprint(&b, `// Transitorio dice si el estado es de paso. Un estado transitorio no está en
// el retículo: una nota que se está verificando no debe arrastrar a sus
// dependientes ni hacer parpadear sus colores mientras corre.
func (e Estado) Transitorio() bool { return e.Rango() < 0 }

// Meet es la combinación del retículo. Como el orden es total, es el mínimo: de
// dos respaldos, vale el más débil. Es lo que hace que la duda se propague.
func Meet(a, b Estado) Estado {
	if a.Rango() <= b.Rango() {
		return a
	}
	return b
}

`)
	// eventos
	fmt.Fprint(&b, "// Evento es lo que puede cambiar el estado de una nota.\ntype Evento string\n\nconst (\n")
	for _, e := range s.Events {
		fmt.Fprintf(&b, "\t// %s\n\tEv%s Evento = %q\n", limpiar(e.Doc), ident(e.ID), e.ID)
	}
	fmt.Fprint(&b, ")\n\n")

	// guardas
	fmt.Fprint(&b, "// Guarda discrimina entre transiciones que salen del mismo estado con el\n// mismo evento. Las de una misma decisión son excluyentes entre sí.\ntype Guarda string\n\nconst (\n")
	for _, d := range s.Decisions {
		for _, g := range d.Guards {
			fmt.Fprintf(&b, "\t// %s\n\tG%s Guarda = %q\n", limpiar(g.Doc), ident(g.ID), g.ID)
		}
	}
	fmt.Fprint(&b, ")\n\n")

	// tabla
	fmt.Fprint(&b, `// Transicion es una arista de la máquina. Desde vacío significa "desde
// cualquier estado no transitorio".
type Transicion struct {
	Desde  Estado
	Hasta  Estado
	Evento Evento
	Guarda Guarda
	Any    bool // aplica desde cualquier estado
}

// Tabla es la máquina completa, en el orden en que se declaró.
var Tabla = []Transicion{
`)
	for _, t := range s.Transitions {
		g := ""
		if t.Guard != "" {
			g = ", Guarda: G" + ident(t.Guard)
		}
		if t.From == "*" {
			// Sin Desde: lo que manda es Any, y poner un estado ahí sería
			// sugerir un origen que la transición no tiene.
			fmt.Fprintf(&b, "\t{Any: true, Hasta: %s, Evento: Ev%s%s},\n", ident(t.To), ident(t.On), g)
			continue
		}
		fmt.Fprintf(&b, "\t{Desde: %s, Hasta: %s, Evento: Ev%s%s},\n",
			ident(t.From), ident(t.To), ident(t.On), g)
	}
	fmt.Fprint(&b, "}\n\n")

	fmt.Fprintf(&b, "// Inicial es el estado de una nota recién capturada.\nconst Inicial = %s\n", ident(s.Initial))

	// iota: se emite como una lista de constantes numeradas
	code := b.String()
	for i, st := range orden {
		code = strings.Replace(code, "iota_placeholder_"+st.ID, fmt.Sprint(i), 1)
	}
	return format.Source([]byte(code))
}

func ident(s string) string {
	parts := strings.FieldsFunc(s, func(r rune) bool { return r == '_' || r == '-' })
	for i, p := range parts {
		if p == "" {
			continue
		}
		parts[i] = strings.ToUpper(p[:1]) + p[1:]
	}
	return strings.Join(parts, "")
}

func limpiar(s string) string {
	return strings.TrimSpace(strings.ReplaceAll(strings.TrimSpace(s), "\n", " "))
}

func check(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
