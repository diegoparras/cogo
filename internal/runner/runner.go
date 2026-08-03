package runner

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// Resultado es lo que quedó de una ejecución. Se guarda con la nota y va al
// journal: es la prueba de que el check corrió de verdad.
type Resultado struct {
	CheckID   string        `json:"check_id"`
	Comando   []string      `json:"comando"`
	Workdir   string        `json:"workdir"`
	ExitCode  int           `json:"exit_code"`
	Duracion  time.Duration `json:"duracion"`
	Stdout    string        `json:"stdout"`
	Stderr    string        `json:"stderr"`
	Truncado  bool          `json:"truncado,omitempty"`
	PorTimeut bool          `json:"por_timeout,omitempty"`
}

// OK dice si el check pasó. Es la única lectura que importa para el color: el
// resto del resultado es para que una persona entienda por qué.
func (r Resultado) OK() bool { return r.ExitCode == 0 && !r.PorTimeut }

// Cuánta salida se guarda. Suficiente para entender un fallo, poco para que un
// test hablador llene el vault.
const maxSalida = 8 * 1024

var (
	// ErrDeshabilitado: el vault no activó el runner. No es un fallo del check.
	ErrDeshabilitado = errors.New("runner: está deshabilitado en este vault (poné enabled: true en .cogo/runner.yaml)")
	// ErrNoAutorizado: la nota pide un check que nadie declaró.
	ErrNoAutorizado = errors.New("runner: la nota pide un check que este vault no autoriza")
)

// Ejecutar corre un check autorizado. No recibe comandos: recibe el ID de uno
// que la configuración ya declaró, y esa es toda la diferencia de seguridad.
func Ejecutar(ctx context.Context, c *Config, checkID string) (Resultado, error) {
	if c == nil || !c.Habilitado {
		return Resultado{}, ErrDeshabilitado
	}
	ch, ok := c.Buscar(checkID)
	if !ok {
		return Resultado{}, fmt.Errorf("%w: %q. Autorizados: %s",
			ErrNoAutorizado, checkID, strings.Join(c.IDs(), ", "))
	}
	if fi, err := os.Stat(ch.Workdir); err != nil || !fi.IsDir() {
		return Resultado{}, fmt.Errorf("runner: el workdir del check %q no existe (%s)", ch.ID, ch.Workdir)
	}

	ctx, cancel := context.WithTimeout(ctx, ch.Timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, ch.Comando[0], ch.Comando[1:]...)
	cmd.Dir = ch.Workdir
	// Entorno mínimo declarado: heredar el del proceso filtraría tokens y claves
	// del propio COGO al comando que se ejecuta.
	cmd.Env = entorno(c)
	// Sin esto, el timeout no corta de verdad. CommandContext mata al proceso
	// que lanzó, pero no a sus hijos: un check que corre `sh -c "sleep 5"` deja
	// un nieto con los pipes abiertos, y Run() se queda esperando a que se
	// cierren — o sea que un check mal escrito puede colgar a COGO mucho después
	// de su timeout. WaitDelay fija cuánto se espera a los rezagados antes de
	// cerrar los pipes y volver.
	//
	// Limitación honesta: el nieto puede sobrevivir como huérfano. Matarlo
	// pediría manejar grupos de procesos, que es distinto en cada sistema
	// operativo. Lo que esto garantiza es que COGO no se queda esperándolo.
	cmd.WaitDelay = 2 * time.Second

	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb

	inicio := time.Now()
	err := cmd.Run()
	dur := time.Since(inicio)

	res := Resultado{
		CheckID: ch.ID, Comando: ch.Comando, Workdir: ch.Workdir,
		Duracion: dur,
	}
	res.Stdout, res.Truncado = recortar(out.String())
	s, t := recortar(errb.String())
	res.Stderr = s
	res.Truncado = res.Truncado || t

	switch {
	case ctx.Err() == context.DeadlineExceeded:
		res.PorTimeut = true
		res.ExitCode = -1
		// Un timeout NO es un check fallado: es un check que no se pudo
		// observar. Confundirlos haría que una máquina lenta refute notas
		// buenas.
		return res, fmt.Errorf("runner: el check %q no terminó en %s", ch.ID, ch.Timeout)
	case err == nil:
		res.ExitCode = 0
	default:
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			res.ExitCode = ee.ExitCode()
		} else {
			// No se pudo ni lanzar el proceso: tampoco es un check fallado.
			return res, fmt.Errorf("runner: no se pudo ejecutar el check %q: %w", ch.ID, err)
		}
	}
	return res, nil
}

// entorno arma el ambiente del comando: solo las variables que el vault declaró.
// PATH va siempre, porque sin él no se encuentra ni el binario.
func entorno(c *Config) []string {
	env := []string{"PATH=" + os.Getenv("PATH")}
	if h := os.Getenv("HOME"); h != "" {
		env = append(env, "HOME="+h)
	}
	for _, k := range c.Env {
		if v, ok := os.LookupEnv(k); ok {
			env = append(env, k+"="+v)
		}
	}
	return env
}

func recortar(s string) (string, bool) {
	if len(s) <= maxSalida {
		return s, false
	}
	// Se conserva el final: cuando algo falla, lo que explica el fallo suele
	// estar en las últimas líneas.
	return "…(recortado)\n" + s[len(s)-maxSalida:], true
}
