package runner

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func vaultCon(t *testing.T, yaml string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".cogo"), 0o755); err != nil {
		t.Fatal(err)
	}
	if yaml != "" {
		if err := os.WriteFile(filepath.Join(dir, ".cogo", "runner.yaml"), []byte(yaml), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// Lo primero: un COGO recién instalado no ejecuta nada.
func TestVieneApagado(t *testing.T) {
	c, err := Cargar(vaultCon(t, ""))
	if err != nil {
		t.Fatal(err)
	}
	if c.Habilitado {
		t.Error("un vault sin configuración tiene el runner ENCENDIDO")
	}
	if _, err := Ejecutar(context.Background(), c, "lo-que-sea"); !errors.Is(err, ErrDeshabilitado) {
		t.Errorf("ejecutó con el runner apagado: %v", err)
	}
}

// LA propiedad de seguridad: la nota no puede traer su propio comando. Solo
// puede nombrar uno que el vault ya autorizó.
func TestSoloSeEjecutaLoQueElVaultAutorizo(t *testing.T) {
	dir := vaultCon(t, `
enabled: true
checks:
  - id: autorizado
    command: ["echo", "ok"]
    workdir: `+t.TempDir()+`
`)
	c, err := Cargar(dir)
	if err != nil {
		t.Fatal(err)
	}
	_, err = Ejecutar(context.Background(), c, "rm-rf-todo")
	if !errors.Is(err, ErrNoAutorizado) {
		t.Errorf("se aceptó un check que nadie declaró: %v", err)
	}
	// y el error dice cuáles sí valen, que es lo que necesita quien lo lee
	if !strings.Contains(err.Error(), "autorizado") {
		t.Errorf("el error no dice qué checks están autorizados: %v", err)
	}
}

func TestEjecutaYCapturaElResultado(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("los comandos del test son de shell POSIX")
	}
	wd := t.TempDir()
	c, err := Cargar(vaultCon(t, `
enabled: true
checks:
  - id: pasa
    command: ["sh", "-c", "echo hola; exit 0"]
    workdir: `+wd+`
  - id: falla
    command: ["sh", "-c", "echo se rompio >&2; exit 3"]
    workdir: `+wd+`
`))
	if err != nil {
		t.Fatal(err)
	}

	res, err := Ejecutar(context.Background(), c, "pasa")
	if err != nil {
		t.Fatal(err)
	}
	if !res.OK() || res.ExitCode != 0 {
		t.Errorf("el check que pasa dio exit %d", res.ExitCode)
	}
	if !strings.Contains(res.Stdout, "hola") {
		t.Errorf("no se capturó la salida: %q", res.Stdout)
	}

	res, err = Ejecutar(context.Background(), c, "falla")
	if err != nil {
		t.Fatal(err)
	}
	if res.OK() {
		t.Error("un check con exit 3 se reportó como pasado")
	}
	if res.ExitCode != 3 {
		t.Errorf("exit code = %d, se esperaba 3", res.ExitCode)
	}
	if !strings.Contains(res.Stderr, "se rompio") {
		t.Errorf("no se capturó stderr: %q", res.Stderr)
	}
}

// Un timeout NO es un check fallado: es un check que no se pudo observar.
// Confundirlos haría que una máquina lenta refute notas buenas.
func TestUnTimeoutNoEsUnCheckFallado(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("los comandos del test son de shell POSIX")
	}
	c, err := Cargar(vaultCon(t, `
enabled: true
checks:
  - id: lento
    command: ["sh", "-c", "sleep 5"]
    workdir: `+t.TempDir()+`
    timeout: 300ms
`))
	if err != nil {
		t.Fatal(err)
	}
	res, err := Ejecutar(context.Background(), c, "lento")
	if err == nil {
		t.Fatal("un timeout debería devolver error")
	}
	if !res.PorTimeut {
		t.Error("el resultado no está marcado como timeout")
	}
	if res.OK() {
		t.Error("un check que no terminó se reportó como pasado")
	}
}

// La configuración es lo que separa a COGO de un ejecutor de comandos
// arbitrarios: si acepta cosas mal formadas, esa separación se diluye.
func TestLaConfiguracionRechazaLoMalFormado(t *testing.T) {
	casos := []struct {
		nombre string
		yaml   string
		espera string
	}{
		{"sin id", "enabled: true\nchecks:\n  - command: [\"echo\"]\n    workdir: /tmp\n", "sin id"},
		{"sin comando", "enabled: true\nchecks:\n  - id: x\n    workdir: /tmp\n", "no declara comando"},
		{"sin workdir", "enabled: true\nchecks:\n  - id: x\n    command: [\"echo\"]\n", "no declara workdir"},
		{"workdir relativo", "enabled: true\nchecks:\n  - id: x\n    command: [\"echo\"]\n    workdir: ./rel\n", "absoluto"},
		{"id duplicado", "enabled: true\nchecks:\n  - id: x\n    command: [\"echo\"]\n    workdir: /tmp\n  - id: x\n    command: [\"ls\"]\n    workdir: /tmp\n", "dos veces"},
		{"timeout desmedido", "enabled: true\nchecks:\n  - id: x\n    command: [\"echo\"]\n    workdir: /tmp\n    timeout: 3h\n", "supera el máximo"},
		{"una línea de shell en vez de argv", "enabled: true\nchecks:\n  - id: x\n    command: [\"go test ./... && rm -rf /\"]\n    workdir: /tmp\n", "línea de shell"},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			_, err := Cargar(vaultCon(t, c.yaml))
			if err == nil {
				t.Fatal("se aceptó una configuración mal formada")
			}
			if !strings.Contains(err.Error(), c.espera) {
				t.Errorf("el error no explica el problema (esperaba %q): %v", c.espera, err)
			}
		})
	}
}

// El comando no hereda el entorno de COGO: si lo heredara, el token del vault y
// la clave del modelo viajarían a cualquier check que alguien configure.
func TestElComandoNoHeredaElEntornoDeCogo(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("los comandos del test son de shell POSIX")
	}
	t.Setenv("COGO_MCP_TOKEN", "secreto-que-no-debe-viajar")
	c, err := Cargar(vaultCon(t, `
enabled: true
checks:
  - id: espia
    command: ["sh", "-c", "env"]
    workdir: `+t.TempDir()+`
`))
	if err != nil {
		t.Fatal(err)
	}
	res, err := Ejecutar(context.Background(), c, "espia")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(res.Stdout, "secreto-que-no-debe-viajar") {
		t.Error("GRAVE: el token de COGO llegó al entorno del comando ejecutado")
	}
}

// Lo que el vault SÍ declara, se pasa. Es la vía para un check que necesita una
// variable, sin abrir el entorno entero.
func TestSeleccionaLasVariablesDeclaradas(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("los comandos del test son de shell POSIX")
	}
	t.Setenv("MI_VAR", "valor-declarado")
	t.Setenv("OTRA", "no-declarada")
	c, err := Cargar(vaultCon(t, `
enabled: true
env: [MI_VAR]
checks:
  - id: e
    command: ["sh", "-c", "env"]
    workdir: `+t.TempDir()+`
`))
	if err != nil {
		t.Fatal(err)
	}
	res, _ := Ejecutar(context.Background(), c, "e")
	if !strings.Contains(res.Stdout, "valor-declarado") {
		t.Error("la variable declarada no llegó al comando")
	}
	if strings.Contains(res.Stdout, "no-declarada") {
		t.Error("llegó una variable que el vault no declaró")
	}
}

func TestLaSalidaSeRecorta(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("los comandos del test son de shell POSIX")
	}
	c, _ := Cargar(vaultCon(t, `
enabled: true
checks:
  - id: hablador
    command: ["sh", "-c", "yes abcdefghij | head -c 100000"]
    workdir: `+t.TempDir()+`
    timeout: 10s
`))
	res, err := Ejecutar(context.Background(), c, "hablador")
	if err != nil {
		t.Fatal(err)
	}
	if !res.Truncado {
		t.Error("una salida de 100 KB no se marcó como recortada")
	}
	if len(res.Stdout) > maxSalida+64 {
		t.Errorf("la salida guardada pesa %d bytes", len(res.Stdout))
	}
}

func TestElContextoDelLlamadorCorta(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("los comandos del test son de shell POSIX")
	}
	c, _ := Cargar(vaultCon(t, `
enabled: true
checks:
  - id: lento
    command: ["sh", "-c", "sleep 5"]
    workdir: `+t.TempDir()+`
    timeout: 30s
`))
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	inicio := time.Now()
	if _, err := Ejecutar(ctx, c, "lento"); err == nil {
		t.Error("debería haber cortado")
	}
	if d := time.Since(inicio); d > 3*time.Second {
		t.Errorf("no respetó el contexto del llamador: tardó %s", d)
	}
}
