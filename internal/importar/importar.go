// Package importar convierte lo que un proyecto ya tiene escrito —CLAUDE.md,
// AGENTS.md, docs/, ADRs— en notas de COGO.
//
// # POR QUÉ
//
// Un vault vacío no sirve para nada, y llenarlo a mano lleva meses. Pero
// ningún proyecto arranca de cero: tiene un README que dice cómo se despliega,
// un CLAUDE.md que dice qué no tocar, ADRs que dicen qué se decidió. Eso ya es
// memoria; lo que no tiene es color.
//
// # CON LA HONESTIDAD QUE CORRESPONDE
//
// Todo lo importado nace amarillo: `asserted`, sin criterio de verificación,
// con `origin: instrument` —nadie lo decidió acá, se leyó— y con la evidencia
// que de verdad tiene: el archivo y la línea de donde salió. Eso último es lo
// que lo vuelve útil desde el primer día: la cita queda ANCLADA por la
// materialidad, así que si alguien edita justo esa sección del documento, la
// nota se entera y se pone amarilla con motivo. Un documento que nadie
// verificó, pero que ya no puede envejecer en silencio.
package importar

import (
	"bufio"
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/diegoparras/cogo/internal/core"
)

// Seccion es un pedazo de documento con título propio: un H2 o un H3 y lo que
// sigue hasta el próximo del mismo nivel o superior.
type Seccion struct {
	Archivo string // ruta relativa a la raíz importada, con /
	Linea   int    // 1-based, la del encabezado
	Hasta   int    // última línea con texto del cuerpo (= Linea si no hay cuerpo)
	Nivel   int    // 2 o 3
	Titulo  string
	Cuerpo  string
}

// Cita es la referencia de evidencia de la sección: archivo y tramo de líneas.
func (s Seccion) Cita() string {
	if s.Hasta > s.Linea {
		return fmt.Sprintf("%s:%d-%d — importado de %s", s.Archivo, s.Linea, s.Hasta, filepath.Base(s.Archivo))
	}
	return fmt.Sprintf("%s:%d — importado de %s", s.Archivo, s.Linea, filepath.Base(s.Archivo))
}

var encabezado = regexp.MustCompile(`^(#{1,6})\s+(.+?)\s*#*\s*$`)

// Seccionar parte un markdown por H2/H3. Un archivo sin encabezados es una
// sección sola, con el nombre del archivo como título. Lo que hay antes del
// primer H2 (el título del documento, una intro) no es una afirmación de nada
// y se descarta.
func Seccionar(archivo string, contenido []byte) []Seccion {
	var out []Seccion
	var actual *Seccion
	var cuerpo []string
	ultima := 0
	cerrar := func() {
		if actual != nil {
			actual.Cuerpo = strings.TrimSpace(strings.Join(cuerpo, "\n"))
			// La cita cubre la sección ENTERA, no solo el encabezado: es lo
			// que hace que editar el cuerpo se note, y editar otra sección no.
			actual.Hasta = actual.Linea
			if ultima > actual.Linea {
				actual.Hasta = ultima
			}
			out = append(out, *actual)
		}
		actual, cuerpo = nil, nil
	}
	sc := bufio.NewScanner(bytes.NewReader(contenido))
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	linea := 0
	enCodigo := false
	for sc.Scan() {
		linea++
		l := sc.Text()
		if strings.HasPrefix(strings.TrimSpace(l), "```") {
			enCodigo = !enCodigo
		}
		if !enCodigo {
			if m := encabezado.FindStringSubmatch(l); m != nil {
				nivel := len(m[1])
				switch {
				case nivel == 2 || nivel == 3:
					cerrar()
					actual = &Seccion{Archivo: archivo, Linea: linea, Nivel: nivel, Titulo: strings.TrimSpace(m[2])}
					continue
				case nivel == 1:
					cerrar()
					continue
				}
			}
		}
		if actual != nil {
			cuerpo = append(cuerpo, l)
			if strings.TrimSpace(l) != "" {
				ultima = linea
			}
		}
	}
	cerrar()
	if len(out) == 0 && strings.TrimSpace(string(contenido)) != "" {
		base := strings.TrimSuffix(filepath.Base(archivo), filepath.Ext(archivo))
		return []Seccion{{Archivo: archivo, Linea: 1, Hasta: linea, Nivel: 2, Titulo: base, Cuerpo: strings.TrimSpace(string(contenido))}}
	}
	return out
}

// Clasificador es un juez externo (Jev, III.6): devuelve el tipo de nota y si
// la sección afirma algo comprobable. Se abstiene con ok=false, y entonces
// vale la heurística.
type Clasificador func(titulo, cuerpo string) (tipo string, afirma bool, ok bool)

var clasificador Clasificador

// SetClasificador instala el juez. nil lo quita.
func SetClasificador(c Clasificador) { clasificador = c }

// tipos válidos para importar. Ni `mistake` ni `gap`: un documento no
// registra errores ni preguntas abiertas de esa forma.
var tiposValidos = map[string]bool{"decision": true, "constraint": true, "runbook": true, "architecture": true, "bug": true, "command": true}

var (
	reDecision     = regexp.MustCompile(`(?i)\b(decidi[mó]|decisi[oó]n|adoptamos|elegimos|se eligi[oó]|we decided|decision|adr)\b`)
	reRestriccion  = regexp.MustCompile(`(?i)\b(nunca|jam[aá]s|prohibido|no se puede|no debe|siempre|obligatorio|must not|must always|never|invariant|restricci[oó]n|constraint)\b`)
	reRunbook      = regexp.MustCompile(`(?i)\b(pasos?|c[oó]mo (se )?(hace|despliega|instala|corre)|procedimiento|how to|runbook|deploy|instalaci[oó]n|setup)\b`)
	reBug          = regexp.MustCompile(`(?i)\b(bug|error conocido|known issue|falla|workaround|gotcha|ojo:)\b`)
	reArquitectura = regexp.MustCompile(`(?i)\b(arquitectura|architecture|componentes?|m[oó]dulos?|estructura|capas?|servicios?)\b`)
	reComando      = regexp.MustCompile("(?s)^\\s*```[a-z]*\\n[^\\n]{3,200}\\n```\\s*$")
	reSoloEnlaces  = regexp.MustCompile(`(?m)^\s*[-*]\s*\[[^\]]*\]\([^)]*\)\s*$`)
)

// Tipo decide el tipo de nota de una sección. Es una heurística por palabras,
// y por eso es conservadora: lo que no reconoce es `architecture`, que es una
// descripción de cómo son las cosas, con ventana larga y sin exigir nada.
func Tipo(titulo, cuerpo string) string {
	if clasificador != nil {
		if t, _, ok := clasificador(titulo, cuerpo); ok && tiposValidos[t] {
			return t
		}
	}
	texto := titulo + "\n" + cuerpo
	switch {
	case reComando.MatchString(cuerpo):
		return "command"
	case reDecision.MatchString(titulo) || reDecision.MatchString(cuerpo):
		return "decision"
	case reRestriccion.MatchString(titulo) || reRestriccion.MatchString(cuerpo):
		return "constraint"
	case reBug.MatchString(texto):
		return "bug"
	case reRunbook.MatchString(titulo):
		return "runbook"
	case reArquitectura.MatchString(titulo):
		return "architecture"
	}
	return "architecture"
}

// Afirma dice si una sección afirma algo que pueda ser una nota. Una lista de
// enlaces, un índice o dos palabras no lo son.
func Afirma(titulo, cuerpo string) bool {
	if clasificador != nil {
		if _, afirma, ok := clasificador(titulo, cuerpo); ok {
			return afirma
		}
	}
	c := strings.TrimSpace(cuerpo)
	if len([]rune(c)) < 40 {
		return false
	}
	sinEnlaces := strings.TrimSpace(reSoloEnlaces.ReplaceAllString(c, ""))
	return len([]rune(sinEnlaces)) >= 40
}

// Fuentes son los archivos que se leen de una raíz: lo que un proyecto escribe
// para que un agente o una persona lo lea.
func Fuentes(raiz string) ([]string, error) {
	var out []string
	err := filepath.WalkDir(raiz, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		nombre := d.Name()
		if d.IsDir() {
			switch strings.ToLower(nombre) {
			case ".git", "node_modules", "vendor", ".cogo", "dist", "build", "target", ".next":
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(strings.ToLower(nombre), ".md") {
			return nil
		}
		rel, _ := filepath.Rel(raiz, p)
		rel = filepath.ToSlash(rel)
		top := strings.ToUpper(nombre)
		enDocs := strings.HasPrefix(rel, "docs/") || strings.Contains(rel, "/adr/") || strings.HasPrefix(rel, "adr/") || strings.Contains(rel, "/decisions/")
		if top == "CLAUDE.MD" || top == "AGENTS.MD" || top == "README.MD" || enDocs {
			out = append(out, rel)
		}
		return nil
	})
	sort.Strings(out)
	return out, err
}

// Resumen es lo que se le cuenta a quien importó.
type Resumen struct {
	Archivos  int
	Secciones int
	Creadas   []string
	Saltadas  map[string]int // motivo → cuántas
}

// Opciones del import.
type Opciones struct {
	Proyecto string
	Hoy      core.Date
	// Existe dice si un id ya está en el vault: nunca se pisa una nota.
	Existe func(id string) bool
	// MaxCuerpo recorta el cuerpo de la nota (0 = 1500 caracteres).
	MaxCuerpo int
}

// Notas arma las notas de una raíz sin escribir nada: la escritura es del
// llamador, que sabe dónde va el vault y cómo se ancla la evidencia.
func Notas(raiz string, o Opciones) ([]*core.Note, Resumen, error) {
	if strings.TrimSpace(o.Proyecto) == "" {
		return nil, Resumen{}, fmt.Errorf("importar: hace falta el proyecto")
	}
	max := o.MaxCuerpo
	if max <= 0 {
		max = 1500
	}
	r := Resumen{Saltadas: map[string]int{}}
	fuentes, err := Fuentes(raiz)
	if err != nil {
		return nil, r, err
	}
	var notas []*core.Note
	vistos := map[string]bool{}
	for _, rel := range fuentes {
		b, err := os.ReadFile(filepath.Join(raiz, filepath.FromSlash(rel)))
		if err != nil {
			r.Saltadas["ilegible"]++
			continue
		}
		r.Archivos++
		for _, s := range Seccionar(rel, b) {
			r.Secciones++
			if !Afirma(s.Titulo, s.Cuerpo) {
				r.Saltadas["no afirma nada"]++
				continue
			}
			id := core.DeriveID(o.Proyecto, s.Titulo)
			if vistos[id] {
				r.Saltadas["título repetido"]++
				continue
			}
			if o.Existe != nil && o.Existe(id) {
				r.Saltadas["ya existía"]++
				continue
			}
			vistos[id] = true
			cuerpo := s.Cuerpo
			if len([]rune(cuerpo)) > max {
				cuerpo = string([]rune(cuerpo)[:max]) + "\n\n_(recortado: sigue en el archivo citado)_"
			}
			notas = append(notas, &core.Note{
				ID: id, Type: Tipo(s.Titulo, s.Cuerpo), Project: o.Proyecto,
				Body:   "## Claim\n" + s.Titulo + "\n\n" + cuerpo,
				Origin: string(core.OrigenInstrumento),
				// La fecha en que el instrumento leyó el documento: ahí arranca
				// el reloj de frescura. Sin fecha, el motor la da por expirada.
				LastVerified: o.Hoy,
				Evidence:     []core.Evidence{{Kind: "file_read", Ref: s.Cita()}},
			})
			r.Creadas = append(r.Creadas, id)
		}
	}
	return notas, r, nil
}
