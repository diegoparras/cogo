package importar

// La memoria que el agente ya tiene.
//
// Claude Code y Codex guardan lo que aprenden en archivos locales. Esa memoria
// ya existe y ya se usa, pero no tiene color: el agente la lee como si todo
// fuera igual de cierto, lo de ayer y lo de hace un año. COGO no compite con
// ella: le pone el semáforo. Cada memoria entra amarilla, anclada a su
// archivo, y con la vigencia contada desde que se escribió, no desde que se
// importó. Así una memoria de proyecto de hace un año vence sola, y una que
// el agente reescribe después se nota por la deriva del archivo.

import (
	"bytes"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/diegoparras/cogo/internal/core"
	"gopkg.in/yaml.v3"
)

var noAlfanumerico = regexp.MustCompile(`[^A-Za-z0-9]`)

// CarpetaClaude es el nombre de la carpeta de un proyecto bajo
// ~/.claude/projects: la ruta del repo con todo lo que no es letra o número
// cambiado por "-".
func CarpetaClaude(ruta string) string { return noAlfanumerico.ReplaceAllString(ruta, "-") }

// DirMemoriaClaude es la carpeta de la memoria automática de Claude Code para
// un repo. Claude Code la deriva de la raíz git, así que se sube hasta ella.
func DirMemoriaClaude(home, raiz string) string {
	return filepath.Join(home, ".claude", "projects", CarpetaClaude(RaizGit(raiz)), "memory")
}

// RaizGit sube desde `raiz` hasta la carpeta que tiene .git. Si no hay
// ninguna, la raíz es la que se dio.
func RaizGit(raiz string) string {
	for d := raiz; ; {
		if _, err := os.Stat(filepath.Join(d, ".git")); err == nil {
			return d
		}
		arriba := filepath.Dir(d)
		if arriba == d {
			return raiz
		}
		d = arriba
	}
}

// DirMemoriaCodex es la carpeta de memorias de Codex en su home: la vieja,
// memories, si existe, porque Codex la sigue usando; si no, memories_v2.
func DirMemoriaCodex(home string) (string, error) {
	for _, n := range []string{"memories", "memories_v2"} {
		d := filepath.Join(home, n)
		if fi, err := os.Stat(d); err == nil && fi.IsDir() {
			return d, nil
		}
	}
	return "", fmt.Errorf("no hay memorias de Codex en %s: ¿están activadas?", home)
}

// tipoMemoria lleva el tipo de memoria de Claude Code al de nota. Una
// corrección o una preferencia es una regla de trabajo; lo que pasa en el
// proyecto es una decisión, con su ventana; una referencia describe dónde
// está algo.
var tipoMemoria = map[string]string{
	"feedback":  "constraint",
	"user":      "constraint",
	"project":   "decision",
	"reference": "architecture",
}

type frente struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
	Type        string `yaml:"type"`
	Modified    any    `yaml:"modified"`
	Metadata    struct {
		Type string `yaml:"type"`
	} `yaml:"metadata"`
}

// partirFrontmatter separa el YAML de arriba del cuerpo.
func partirFrontmatter(b []byte) (frente, string, bool) {
	var f frente
	s := strings.ReplaceAll(string(b), "\r\n", "\n")
	if !strings.HasPrefix(s, "---\n") {
		return f, s, false
	}
	fin := strings.Index(s[4:], "\n---")
	if fin < 0 {
		return f, s, false
	}
	if err := yaml.Unmarshal([]byte(s[4:4+fin]), &f); err != nil {
		return frente{}, s, false
	}
	resto := s[4+fin+4:]
	if i := strings.IndexByte(resto, '\n'); i >= 0 {
		resto = resto[i+1:]
	} else {
		resto = ""
	}
	return f, resto, true
}

// fechaDe es desde cuándo corre la vigencia: la fecha en que se escribió la
// memoria, o la del archivo; nunca después de hoy.
func fechaDe(modificado any, archivo string, hoy core.Date) core.Date {
	var t time.Time
	switch v := modificado.(type) {
	case time.Time:
		t = v
	case string:
		if p, err := time.Parse(time.RFC3339, strings.TrimSpace(v)); err == nil {
			t = p
		} else if d, err := core.ParseDate(strings.TrimSpace(v)); err == nil {
			t = d.Time()
		}
	}
	if t.IsZero() {
		if fi, err := os.Stat(archivo); err == nil {
			t = fi.ModTime()
		}
	}
	if t.IsZero() {
		return hoy
	}
	d := core.NewDate(t.Year(), t.Month(), t.Day())
	if d.After(hoy) {
		return hoy
	}
	return d
}

// rutaCita es como se escribe un archivo fuera del repo en una cita: la ruta
// absoluta, o una URI file:// si tiene espacios, que la cita no admite.
func rutaCita(abs string) string {
	if !strings.ContainsAny(abs, " \t") {
		return abs
	}
	return (&url.URL{Scheme: "file", Path: "/" + strings.TrimPrefix(filepath.ToSlash(abs), "/")}).String()
}

func lineas(b []byte) int {
	n := bytes.Count(b, []byte("\n"))
	if len(b) > 0 && b[len(b)-1] != '\n' {
		n++
	}
	return n
}

func recortar(s string, max int) string {
	if max <= 0 {
		max = 1500
	}
	if len([]rune(s)) > max {
		return string([]rune(s)[:max]) + "\n\n_(recortado: sigue en el archivo citado)_"
	}
	return s
}

// MemoriaClaude convierte la memoria automática de Claude Code de un proyecto
// en notas: un archivo, una nota. El índice MEMORY.md no es una memoria, solo
// apunta a las otras.
func MemoriaClaude(dir string, o Opciones) ([]*core.Note, Resumen, error) {
	r := Resumen{Saltadas: map[string]int{}}
	if strings.TrimSpace(o.Proyecto) == "" {
		return nil, r, fmt.Errorf("importar: hace falta el proyecto")
	}
	entradas, err := os.ReadDir(dir)
	if err != nil {
		return nil, r, fmt.Errorf("no hay memoria de Claude Code en %s: %w", dir, err)
	}
	var notas []*core.Note
	vistos := map[string]bool{}
	for _, e := range entradas {
		nombre := e.Name()
		if e.IsDir() || !strings.HasSuffix(strings.ToLower(nombre), ".md") || strings.EqualFold(nombre, "MEMORY.md") {
			continue
		}
		ruta := filepath.Join(dir, nombre)
		b, err := os.ReadFile(ruta)
		if err != nil {
			r.Saltadas["ilegible"]++
			continue
		}
		r.Archivos++
		r.Secciones++
		f, cuerpo, _ := partirFrontmatter(b)
		cuerpo = strings.TrimSpace(cuerpo)
		titulo := strings.TrimSpace(f.Name)
		if titulo == "" {
			titulo = strings.TrimSuffix(nombre, filepath.Ext(nombre))
		}
		claim := strings.TrimSpace(f.Description)
		if claim == "" {
			claim = primeraLinea(cuerpo)
		}
		if len([]rune(claim+cuerpo)) < 40 {
			r.Saltadas["no afirma nada"]++
			continue
		}
		id := core.DeriveID(o.Proyecto, titulo)
		if vistos[id] {
			r.Saltadas["título repetido"]++
			continue
		}
		if o.Existe != nil && o.Existe(id) {
			r.Saltadas["ya existía"]++
			continue
		}
		vistos[id] = true
		clase := f.Type
		if clase == "" {
			clase = f.Metadata.Type
		}
		tipo, ok := tipoMemoria[strings.ToLower(clase)]
		if !ok {
			tipo = Tipo(titulo, cuerpo)
		}
		etiqueta := "memoria de Claude Code"
		if clase != "" {
			etiqueta += " (" + clase + ")"
		}
		notas = append(notas, &core.Note{
			ID: id, Type: tipo, Project: o.Proyecto,
			Body:         "## Claim\n" + claim + "\n\n" + recortar(cuerpo, o.MaxCuerpo),
			Origin:       string(core.OrigenInstrumento),
			LastVerified: fechaDe(f.Modified, ruta, o.Hoy),
			Evidence:     []core.Evidence{{Kind: "file_read", Ref: fmt.Sprintf("%s:1-%d — %s", rutaCita(ruta), lineas(b), etiqueta)}},
		})
		r.Creadas = append(r.Creadas, id)
	}
	return notas, r, nil
}

// archivosCodex son los que se leen de la memoria de Codex: el resumen que
// lee al arrancar y el consolidado. raw_memories.md y rollout_summaries/ son
// pasos intermedios de la consolidación, y meterlos duplicaría todo.
var archivosCodex = []string{"memory_summary.md", "MEMORY.md"}

// MemoriaCodex convierte la memoria de Codex en notas. Es Markdown sin una
// estructura documentada, así que se secciona como un documento. Ojo: Codex
// guarda una memoria por máquina, no por proyecto.
func MemoriaCodex(home string, o Opciones) ([]*core.Note, Resumen, error) {
	r := Resumen{Saltadas: map[string]int{}}
	if strings.TrimSpace(o.Proyecto) == "" {
		return nil, r, fmt.Errorf("importar: hace falta el proyecto")
	}
	dir, err := DirMemoriaCodex(home)
	if err != nil {
		return nil, r, err
	}
	var notas []*core.Note
	vistos := map[string]bool{}
	for _, nombre := range archivosCodex {
		ruta := filepath.Join(dir, nombre)
		b, err := os.ReadFile(ruta)
		if err != nil {
			continue
		}
		r.Archivos++
		fecha := fechaDe(nil, ruta, o.Hoy)
		for _, s := range Seccionar(nombre, b) {
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
			tramo := fmt.Sprintf("%d", s.Linea)
			if s.Hasta > s.Linea {
				tramo = fmt.Sprintf("%d-%d", s.Linea, s.Hasta)
			}
			notas = append(notas, &core.Note{
				ID: id, Type: Tipo(s.Titulo, s.Cuerpo), Project: o.Proyecto,
				Body:         "## Claim\n" + s.Titulo + "\n\n" + recortar(s.Cuerpo, o.MaxCuerpo),
				Origin:       string(core.OrigenInstrumento),
				LastVerified: fecha,
				Evidence:     []core.Evidence{{Kind: "file_read", Ref: fmt.Sprintf("%s:%s — memoria de Codex (%s)", rutaCita(ruta), tramo, nombre)}},
			})
			r.Creadas = append(r.Creadas, id)
		}
	}
	if r.Archivos == 0 {
		return nil, r, fmt.Errorf("la memoria de Codex en %s no tiene %s", dir, strings.Join(archivosCodex, " ni "))
	}
	return notas, r, nil
}

func primeraLinea(s string) string {
	for _, l := range strings.Split(s, "\n") {
		if l = strings.TrimSpace(strings.TrimLeft(l, "#*- ")); l != "" {
			if len([]rune(l)) > 200 {
				l = string([]rune(l)[:200])
			}
			return l
		}
	}
	return ""
}
