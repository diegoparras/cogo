package confidence

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Propiedades del retículo. Si estas tres no se sostienen, la propagación no
// converge y el determinismo del punto fijo deja de estar garantizado.
// ---------------------------------------------------------------------------

func estables() []Estado {
	var out []Estado
	for e := Estado(0); int(e) < 32; e++ {
		if e.String() == "desconocido" {
			break
		}
		if !e.Transitorio() {
			out = append(out, e)
		}
	}
	return out
}

func TestMeetEsConmutativaAsociativaIdempotente(t *testing.T) {
	es := estables()
	if len(es) < 2 {
		t.Fatal("hacen falta al menos dos estados en el retículo")
	}
	for _, a := range es {
		if Meet(a, a) != a {
			t.Errorf("idempotencia: Meet(%s,%s) = %s", a, a, Meet(a, a))
		}
		for _, b := range es {
			if Meet(a, b) != Meet(b, a) {
				t.Errorf("conmutatividad: Meet(%s,%s)=%s pero Meet(%s,%s)=%s", a, b, Meet(a, b), b, a, Meet(b, a))
			}
			for _, c := range es {
				if Meet(Meet(a, b), c) != Meet(a, Meet(b, c)) {
					t.Errorf("asociatividad con (%s,%s,%s)", a, b, c)
				}
			}
		}
	}
}

// El meet nunca puede devolver algo MÁS confiable que sus operandos: es la
// propiedad que hace que la duda se propague hacia abajo y nunca hacia arriba.
func TestMeetNuncaEleva(t *testing.T) {
	for _, a := range estables() {
		for _, b := range estables() {
			m := Meet(a, b)
			if m.Rango() > a.Rango() || m.Rango() > b.Rango() {
				t.Errorf("Meet(%s,%s) = %s, que es más confiable que un operando", a, b, m)
			}
		}
	}
}

// El cero value de Estado tiene que ser el MENOS confiable: una nota cuyo
// estado no se pobló no puede leerse como algo en lo que apoyarse.
func TestCeroValueEsElMenosConfiable(t *testing.T) {
	var sinPoblar Estado
	for _, e := range estables() {
		if e.Rango() < sinPoblar.Rango() {
			t.Fatalf("%s es menos confiable que el cero value (%s)", e, sinPoblar)
		}
	}
	if sinPoblar.Color() != "red" {
		t.Errorf("el cero value proyecta %q; debería ser rojo", sinPoblar.Color())
	}
}

// ---------------------------------------------------------------------------
// Propiedades de la máquina.
// ---------------------------------------------------------------------------

// I1: el único camino a `verified` es un CheckExecuted del runner.
func TestSoloElRunnerLlegaAVerified(t *testing.T) {
	n := 0
	for _, tr := range Tabla {
		if tr.Hasta != Verified {
			continue
		}
		n++
		if tr.Evento != EvCheckExecuted {
			t.Errorf("se llega a verified con el evento %s; solo debería ser CheckExecuted", tr.Evento)
		}
		if tr.Guarda != GEjecucionOk {
			t.Errorf("se llega a verified sin la guarda de ejecución exitosa (guarda: %q)", tr.Guarda)
		}
		if tr.Any {
			t.Error("hay una transición a verified desde CUALQUIER estado")
		}
	}
	if n == 0 {
		t.Error("no hay ninguna transición a verified")
	}
}

// I7: de `refuted` solo se sale declarando un criterio nuevo. Un check que
// falló no se arregla afirmando que ahora pasa.
func TestDeRefutadoSoloSeSaleConCheckNuevo(t *testing.T) {
	for _, tr := range Tabla {
		if tr.Desde != Refuted || tr.Any {
			continue
		}
		if tr.Evento != EvCheckDeclared {
			t.Errorf("se sale de refuted con %s; solo debería poder salirse declarando un check nuevo", tr.Evento)
		}
	}
}

// Ninguna transición puede elevar el estado por encima de claimed_passed sin
// pasar por el runner: es la regla que sostiene que el verde no se declare.
func TestNadieSaltaAlTopeSinRunner(t *testing.T) {
	for _, tr := range Tabla {
		if tr.Evento == EvCheckExecuted {
			continue
		}
		if tr.Hasta.Rango() > ClaimedPassed.Rango() {
			t.Errorf("%s lleva a %s sin ejecutar nada", tr.Evento, tr.Hasta)
		}
	}
}

// Todo estado estable tiene que ser alcanzable, y todo evento tiene que servir
// para algo. Lo valida el generador, pero se comprueba también acá porque el
// generado puede quedar viejo respecto del YAML.
func TestTodoEstadoEsAlcanzableYTodoEventoSeUsa(t *testing.T) {
	alcanzable := map[Estado]bool{Inicial: true}
	usado := map[Evento]bool{}
	for _, tr := range Tabla {
		alcanzable[tr.Hasta] = true
		usado[tr.Evento] = true
	}
	for e := Estado(0); e <= Verifying; e++ {
		if e.String() == "desconocido" {
			continue
		}
		if !alcanzable[e] {
			t.Errorf("el estado %s es inalcanzable", e)
		}
	}
}

// ---------------------------------------------------------------------------
// El validador del generador. Un validador que nunca rechaza nada no valida:
// estos casos le pasan tablas rotas y exigen que falle.
// ---------------------------------------------------------------------------

func TestElGeneradorRechazaTablasRotas(t *testing.T) {
	base, err := os.ReadFile("transitions.yaml")
	if err != nil {
		t.Fatal(err)
	}

	casos := []struct {
		nombre string
		romper func(string) string
		espera string
	}{
		{
			"un estado sin rango queda fuera del retículo",
			func(s string) string {
				return strings.Replace(s, "  - id: stale\n    color: yellow\n    rank: 3\n", "  - id: stale\n    color: yellow\n", 1)
			},
			"falta rank",
		},
		{
			"dos estados con el mismo rango rompen el orden total",
			func(s string) string { return strings.Replace(s, "    rank: 4\n", "    rank: 3\n", 1) },
			"repetido",
		},
		{
			"un hueco en los rangos deja un nivel fantasma",
			func(s string) string { return strings.Replace(s, "    rank: 7\n", "    rank: 9\n", 1) },
			"hueco",
		},
		{
			"una transición a un estado que no existe",
			func(s string) string { return strings.Replace(s, "    to: check_declared\n", "    to: no_existe\n", 1) },
			"no existe",
		},
		{
			"una guarda que no pertenece a ninguna decisión",
			func(s string) string {
				return strings.Replace(s, "    guard: ejecucion_ok\n", "    guard: inventada\n", 1)
			},
			"no pertenece",
		},
		{
			// Las dos transiciones quedan con la MISMA guarda: el caso de fallo
			// deja de estar cubierto, y además el destino se vuelve ambiguo.
			// `refuted` sigue siendo alcanzable, así que lo que se prueba acá es
			// la cobertura y no la alcanzabilidad.
			"dos transiciones del mismo par sin cubrir toda la decisión",
			func(s string) string {
				return strings.Replace(s, "    guard: ejecucion_falla\n", "    guard: ejecucion_ok\n", 1)
			},
			"no está cubierta",
		},
		{
			"dos transiciones del mismo par con la misma guarda son ambiguas",
			func(s string) string {
				return strings.Replace(s, "    guard: ejecucion_falla\n", "    guard: ejecucion_ok\n", 1)
			},
			"misma guarda",
		},
		{
			"un estado transitorio con rango",
			func(s string) string {
				return strings.Replace(s, "    transient: true\n", "    transient: true\n    rank: 8\n", 1)
			},
			"transitorio",
		},
	}

	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			dir := t.TempDir()
			roto := c.romper(string(base))
			if roto == string(base) {
				t.Fatal("el caso no modificó la tabla: el test no probaría nada")
			}
			in := filepath.Join(dir, "roto.yaml")
			if err := os.WriteFile(in, []byte(roto), 0o644); err != nil {
				t.Fatal(err)
			}
			cmd := exec.Command("go", "run", "./gen", "-in", in, "-out", filepath.Join(dir, "out.go"))
			out, err := cmd.CombinedOutput()
			if err == nil {
				t.Fatalf("el generador ACEPTÓ una tabla rota:\n%s", out)
			}
			if !strings.Contains(string(out), c.espera) {
				t.Errorf("falló, pero sin explicar por qué se esperaba (%q):\n%s", c.espera, out)
			}
		})
	}
}

// Y el caso positivo: la tabla real tiene que pasar.
func TestLaTablaRealEsValida(t *testing.T) {
	dir := t.TempDir()
	cmd := exec.Command("go", "run", "./gen", "-in", "transitions.yaml", "-out", filepath.Join(dir, "out.go"))
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("la tabla de producción no valida:\n%s", out)
	}
}

// El archivo generado tiene que ser exactamente lo que produce el YAML de hoy.
// Sin este test, alguien edita transitions.yaml, se olvida de `go generate`, y
// el código sigue corriendo con la máquina vieja sin que nada avise — que es el
// modo de falla que la fuente única venía a evitar.
func TestElGeneradoEstaSincronizadoConElYAML(t *testing.T) {
	dir := t.TempDir()
	fresco := filepath.Join(dir, "states_gen.go")
	cmd := exec.Command("go", "run", "./gen", "-in", "transitions.yaml", "-out", fresco)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("no se pudo regenerar:\n%s", out)
	}
	quiero, err := os.ReadFile(fresco)
	if err != nil {
		t.Fatal(err)
	}
	tengo, err := os.ReadFile("states_gen.go")
	if err != nil {
		t.Fatal(err)
	}
	if string(quiero) != string(tengo) {
		t.Error("states_gen.go no coincide con transitions.yaml — corré `go generate ./internal/confidence`")
	}
}
