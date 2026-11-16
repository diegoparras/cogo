package parametros

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// El catálogo se dibuja solo, así que un parámetro mal declarado es un control
// roto en el panel. Este test es el que impide que eso llegue a producción.
func TestElCatalogoEstaBienDeclarado(t *testing.T) {
	vistas := map[string]bool{}
	for _, d := range Registro {
		if vistas[d.Clave] {
			t.Errorf("%s está declarado dos veces", d.Clave)
		}
		vistas[d.Clave] = true

		if d.Rotulo == "" || d.Explica == "" || d.Efecto == "" {
			t.Errorf("%s: le falta rótulo, explicación o efecto — el panel los muestra los tres", d.Clave)
		}
		if !strings.Contains(d.Clave, ".") {
			t.Errorf("%s: la clave tiene que llevar grupo", d.Clave)
		}
		if orden(d.Grupo) >= len(GruposOrdenados) {
			t.Errorf("%s: el grupo %q no está en GruposOrdenados, va a caer al final sin título", d.Clave, d.Grupo)
		}
		if TituloGrupo[d.Grupo] == "" {
			t.Errorf("%s: el grupo %q no tiene título", d.Clave, d.Grupo)
		}

		// El default tiene que ser un valor válido de su propio parámetro. Suena
		// obvio; es exactamente el error que un rango mal escrito produce.
		if _, err := normalizar(d, d.Default); err != nil {
			t.Errorf("%s: el default no pasa su propia validación: %v", d.Clave, err)
		}
		switch d.Tipo {
		case TEntero:
			if d.Min >= d.Max {
				t.Errorf("%s: rango vacío (%d a %d)", d.Clave, d.Min, d.Max)
			}
			if d.Unidad == "" {
				t.Errorf("%s: un número sin unidad no se puede leer", d.Clave)
			}
		case TOpcion:
			if len(d.Opciones) < 2 {
				t.Errorf("%s: una opción con menos de dos alternativas no es una opción", d.Clave)
			}
		}
	}
}

// Los umbrales por clase de acción tienen que existir para las cuatro clases, o
// autorizar una acción de la clase que falta leería un umbral vacío.
func TestHayUmbralParaCadaClaseDeAccion(t *testing.T) {
	for _, c := range []string{"informativa", "reversible", "costosa", "irreversible"} {
		clave := "accion." + c
		if _, ok := porClave[clave]; !ok {
			t.Errorf("falta el umbral %s", clave)
		}
	}
}

// Y una ventana de frescura para cada tipo que el motor conoce.
func TestHayVentanaParaCadaTipoDeNota(t *testing.T) {
	for _, tipo := range []string{"constraint", "decision", "architecture", "runbook", "bug", "command", "otros"} {
		if _, ok := porClave["frescura."+tipo]; !ok {
			t.Errorf("falta la ventana de frescura de %q", tipo)
		}
	}
}

// Solo se guarda lo editado: un vault en defaults no tiene archivo. Es lo que
// hace que actualizar COGO mueva los defaults sin pisar lo que alguien decidió.
func TestSoloSeGuardaLoEditado(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".cogo"), 0o755); err != nil {
		t.Fatal(err)
	}
	ruta := filepath.Join(dir, ".cogo", "parametros.json")

	s := Cargar(dir)
	if err := s.Guardar(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(ruta); !os.IsNotExist(err) {
		t.Error("un vault sin nada editado dejó archivo de parámetros")
	}

	if err := s.Poner("frescura.command", 7, "test"); err != nil {
		t.Fatal(err)
	}
	if err := s.Guardar(); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(ruta)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "frescura.constraint") {
		t.Errorf("guardó parámetros que nadie tocó:\n%s", b)
	}

	if v := Cargar(dir); v.Entero("frescura.command") != 7 {
		t.Errorf("no releyó el valor editado: %d", v.Entero("frescura.command"))
	}

	// Y volver al default lo saca del archivo.
	if err := s.Restaurar("frescura.command", "test"); err != nil {
		t.Fatal(err)
	}
	if s.Editados() != 0 {
		t.Errorf("restaurar dejó %d editados", s.Editados())
	}
	if err := s.Guardar(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(ruta); !os.IsNotExist(err) {
		t.Error("volver a los defaults dejó el archivo")
	}
}

// La validación es la única defensa: si un valor fuera de rango entra, el motor
// lo usa sin volver a mirar.
func TestNoEntraUnValorInvalido(t *testing.T) {
	s := Defaults()
	casos := []struct {
		clave string
		valor any
	}{
		{"frescura.command", 0},               // por debajo del mínimo
		{"frescura.command", 99999},           // por encima del máximo
		{"frescura.command", "muchos"},        // no numérico
		{"accion.irreversible", "muy_seguro"}, // opción inexistente
		{"calibracion.activa", "quizás"},      // booleano que no lo es
		{"inventado.total", 1},                // parámetro que no existe
	}
	for _, c := range casos {
		if err := s.Poner(c.clave, c.valor, "test"); err == nil {
			t.Errorf("aceptó %s = %v", c.clave, c.valor)
		}
	}
	if s.Editados() != 0 {
		t.Errorf("un valor rechazado quedó guardado igual (%d editados)", s.Editados())
	}
}

// Un archivo con un parámetro de otra versión no rompe nada: se ignora esa clave
// y el resto se carga. Un vault no puede quedar inutilizable por haber usado una
// versión más nueva.
func TestUnArchivoDeOtraVersionNoRompe(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".cogo"), 0o755); err != nil {
		t.Fatal(err)
	}
	crudo := `{"frescura.command": 7, "parametro.del.futuro": 42, "frescura.bug": 9999999}`
	if err := os.WriteFile(filepath.Join(dir, ".cogo", "parametros.json"), []byte(crudo), 0o644); err != nil {
		t.Fatal(err)
	}
	s := Cargar(dir)
	if s.Entero("frescura.command") != 7 {
		t.Error("no cargó el parámetro válido")
	}
	if s.Entero("frescura.bug") != 60 {
		t.Errorf("aceptó un valor fuera de rango del archivo: %d", s.Entero("frescura.bug"))
	}
}

// Cada cambio queda registrado con quién lo hizo. Aflojar un umbral tiene que
// dejar rastro, o el panel sería un agujero en la auditoría.
func TestCadaCambioQuedaRegistrado(t *testing.T) {
	var log []string
	SetRegistro(func(clave string, antes, despues any, quien string) {
		log = append(log, clave+" "+quien)
	})
	defer SetRegistro(nil)

	s := Defaults()
	_ = s.Poner("accion.irreversible", Asserted, "user:diego")
	_ = s.Poner("accion.irreversible", Asserted, "user:diego") // sin cambio: no se registra
	if len(log) != 1 || !strings.Contains(log[0], "user:diego") {
		t.Errorf("registro = %v; se esperaba un solo cambio, con autor", log)
	}
}

// Los que aflojan el sistema están marcados. No se bloquean: se señalan.
func TestLosPeligrososEstanMarcados(t *testing.T) {
	deben := []string{"accion.irreversible", "accion.exigir_respaldo", "ancla.caracteres_minimos"}
	for _, k := range deben {
		if !porClave[k].Afloja {
			t.Errorf("%s no está marcado como parámetro que afloja el sistema", k)
		}
	}
}
