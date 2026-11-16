// Package runner ejecuta los checks de las notas. Es el único componente que
// puede producir el estado `verified`: sin él, todo check pasado es una
// declaración.
//
// # EL MODELO DE AMENAZA, PRIMERO
//
// Ejecutar comandos que salen de notas escritas por un LLM es la superficie de
// ataque más grande que COGO puede tener, y una lista de comandos permitidos no
// la acota: si el agente escribe la nota Y el comando, y `go test` está
// permitido, entonces el agente hace que `go test` corra código que él mismo
// escribió — porque `go test` ejecuta `TestMain` e `init()`. Lo mismo con
// `npm test`, que corre lo que diga el package.json.
//
// Por eso acá el comando NO sale de la nota. Los comandos los declara una
// persona en la configuración del vault, con su directorio y su timeout; la nota
// solo ELIGE cuál de esos checks le aplica, por nombre. Un agente puede pedir
// que se corra el check "go-test", no puede inventar qué se ejecuta.
//
// Y viene apagado. Un COGO recién instalado no ejecuta nada.
package runner

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Config es lo que una persona declara en .cogo/runner.yaml.
type Config struct {
	// Habilitado tiene que ponerse a mano. El default es no ejecutar nada.
	Habilitado bool     `yaml:"enabled"`
	Checks     []Check  `yaml:"checks"`
	Env        []string `yaml:"env,omitempty"` // variables que SÍ se pasan; el resto no

	ruta string
}

// Check es un comando que el vault autoriza a ejecutar.
type Check struct {
	ID string `yaml:"id"`
	// Comando es argv: el programa y sus argumentos, ya separados. No es una
	// línea de shell a propósito — sin shell no hay expansión, ni tuberías, ni
	// `;` para encadenar otra cosa.
	Comando []string      `yaml:"command"`
	Workdir string        `yaml:"workdir"`
	Timeout time.Duration `yaml:"timeout"`
	Doc     string        `yaml:"doc,omitempty"`
}

const timeoutPorDefecto = 2 * time.Minute
const timeoutMaximo = 15 * time.Minute

// Cargar lee la configuración del vault. Un vault sin archivo devuelve una
// configuración deshabilitada y sin error: no tener runner es lo normal.
func Cargar(vault string) (*Config, error) {
	ruta := filepath.Join(vault, ".cogo", "runner.yaml")
	b, err := os.ReadFile(ruta)
	if os.IsNotExist(err) {
		return &Config{ruta: ruta}, nil
	}
	if err != nil {
		return nil, err
	}
	var c Config
	if err := yaml.Unmarshal(b, &c); err != nil {
		return nil, fmt.Errorf("runner: %s no se puede leer: %w", ruta, err)
	}
	c.ruta = ruta
	if err := c.validar(); err != nil {
		return nil, err
	}
	return &c, nil
}

func (c *Config) validar() error {
	vistos := map[string]bool{}
	for i := range c.Checks {
		ch := &c.Checks[i]
		id := strings.TrimSpace(ch.ID)
		if id == "" {
			return fmt.Errorf("runner: hay un check sin id en %s", c.ruta)
		}
		if vistos[id] {
			return fmt.Errorf("runner: el check %q está declarado dos veces", id)
		}
		vistos[id] = true
		if len(ch.Comando) == 0 {
			return fmt.Errorf("runner: el check %q no declara comando", id)
		}
		// El comando es argv, no una línea de shell. Si alguien escribió una
		// línea entera en el primer elemento, conviene decírselo ahora y no
		// dejar que falle de forma rara al ejecutar.
		if strings.ContainsAny(ch.Comando[0], " \t|;&><$`") {
			return fmt.Errorf("runner: el check %q parece una línea de shell (%q). "+
				"El comando es una lista: [\"go\", \"test\", \"./...\"] — sin shell no hay expansión ni encadenado", id, ch.Comando[0])
		}
		if ch.Workdir == "" {
			return fmt.Errorf("runner: el check %q no declara workdir; se necesita saber dónde corre", id)
		}
		if !filepath.IsAbs(ch.Workdir) {
			return fmt.Errorf("runner: el workdir del check %q debe ser absoluto (%q)", id, ch.Workdir)
		}
		if ch.Timeout <= 0 {
			ch.Timeout = timeoutPorDefecto
		}
		if ch.Timeout > timeoutMaximo {
			return fmt.Errorf("runner: el timeout del check %q (%s) supera el máximo de %s", id, ch.Timeout, timeoutMaximo)
		}
	}
	return nil
}

// Buscar devuelve el check autorizado con ese id.
func (c *Config) Buscar(id string) (Check, bool) {
	for _, ch := range c.Checks {
		if ch.ID == strings.TrimSpace(id) {
			return ch, true
		}
	}
	return Check{}, false
}

// IDs lista los checks autorizados, para poder mostrárselos a quien escribe una
// nota: son las únicas opciones válidas.
func (c *Config) IDs() []string {
	out := make([]string, 0, len(c.Checks))
	for _, ch := range c.Checks {
		out = append(out, ch.ID)
	}
	return out
}

// Ejemplo es la plantilla que se le muestra a alguien que quiere activar el
// runner. Está acá y no en la documentación para que no se desincronicen.
const Ejemplo = `# Checks que este vault autoriza a ejecutar.
#
# El comando NO sale de la nota: lo declarás vos acá, y la nota elige cuál le
# aplica por su id. Un agente puede pedir que se corra "go-test"; no puede
# inventar qué se ejecuta.
#
# El comando es una lista, no una línea de shell: sin shell no hay expansión de
# variables, ni tuberías, ni ";" para encadenar otra cosa.

enabled: false

checks:
  - id: go-test
    command: ["go", "test", "./..."]
    workdir: /ruta/absoluta/a/tu/repo
    timeout: 2m
    doc: la suite completa

  - id: build
    command: ["go", "build", "./..."]
    workdir: /ruta/absoluta/a/tu/repo
    timeout: 1m
`
