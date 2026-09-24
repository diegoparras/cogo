package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/diegoparras/cogo/internal/core"
	"github.com/diegoparras/cogo/internal/journal"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// El registro del proceso es uno solo (ver journalDe): entre tests con vaults
// distintos hay que soltarlo, o el segundo lee el journal del primero.
func soltarRegistro() {
	muRegistro.Lock()
	registro = nil
	muRegistro.Unlock()
	// Y el motor: instalarMotor lo deja global, apuntando a un journal de un
	// TempDir que ya no existe cuando corre el test siguiente.
	core.SetMotor(nil)
}

func stdinHook(tool, comando string) *bytes.Buffer {
	return bytes.NewBufferString(`{"hook_event_name":"PreToolUse","tool_name":"` + tool +
		`","tool_input":{"command":` + jsonStr(comando) + `},"cwd":"/p"}`)
}

func jsonStr(s string) string {
	return `"` + strings.ReplaceAll(strings.ReplaceAll(s, `\`, `\\`), `"`, `\"`) + `"`
}

// notaVerificada es una nota que llegó a `verified`: check ejecutado por el
// runner, evidencia observada, fresca.
func notaVerificada(t *testing.T, dir, id, claim string) {
	t.Helper()
	n := &core.Note{
		ID: id, Type: "runbook", Project: "p", LastVerified: today(),
		Evidence: []core.Evidence{{Kind: "command_output", Ref: "go build ./... → ok (hoy)"}},
		Check:    core.Check{Test: "go build ./... después de borrar", Status: "passed", Attested: core.AttestExecuted, AttestedBy: journal.EmisorEjecucion},
		Body:     "## Claim\n" + claim,
	}
	if err := core.WriteNoteFile(filepath.Join(dir, id+".md"), n); err != nil {
		t.Fatal(err)
	}
}

// Un `ls` no consulta a nadie: ni siquiera toca el vault. Un hook que pregunta
// por todo se apaga a la semana.
func TestHookIgnoraLoInofensivo(t *testing.T) {
	var out, errb bytes.Buffer
	code := ejecutarHook([]string{"pre-tool", "-vault", filepath.Join(t.TempDir(), "no-existe")},
		stdinHook("Bash", "ls -la"), &out, &errb)
	if code != 0 || errb.Len() != 0 {
		t.Fatalf("ls tiene que pasar en silencio: code=%d stderr=%q", code, errb.String())
	}
	code = ejecutarHook([]string{"pre-tool", "-vault", filepath.Join(t.TempDir(), "no-existe")},
		bytes.NewBufferString(`{"tool_name":"Read","tool_input":{"file_path":"/x"}}`), &out, &errb)
	if code != 0 {
		t.Fatalf("Read no cambia nada afuera: code=%d", code)
	}
}

// Lo irreversible sin ninguna nota que lo respalde se frena, y el motivo va a
// stderr, que es lo que el modelo lee.
func TestHookBloqueaLoIrreversibleSinRespaldo(t *testing.T) {
	soltarRegistro()
	dir := t.TempDir()
	var out, errb bytes.Buffer
	code := ejecutarHook([]string{"pre-tool", "-vault", dir}, stdinHook("Bash", "rm -rf build"), &out, &errb)
	if code != 2 {
		t.Fatalf("esperaba 2, dio %d (stderr: %s)", code, errb.String())
	}
	if !strings.HasPrefix(errb.String(), "NOT AUTHORIZED") || !strings.Contains(errb.String(), "support found by COGO: none") {
		t.Fatalf("el motivo tiene que ser el veredicto de COGO:\n%s", errb.String())
	}
}

// Con una nota verificada que habla justo de eso, pasa — y sin que el agente
// haya citado nada: COGO la encontró sola.
func TestHookDejaPasarConRespaldoVerificado(t *testing.T) {
	soltarRegistro()
	dir := t.TempDir()
	notaVerificada(t, dir, "build-se-regenera", "La carpeta build se regenera con go build: borrarla con rm -rf build es seguro.")
	var out, errb bytes.Buffer
	code := ejecutarHook([]string{"pre-tool", "-vault", dir}, stdinHook("Bash", "rm -rf build"), &out, &errb)
	if code != 0 {
		t.Fatalf("esperaba 0, dio %d:\n%s", code, errb.String())
	}
}

// La clase mínima es la perilla: con `reversible`, editar un archivo también
// pregunta; con el default, no.
func TestHookRespetaLaClaseMinima(t *testing.T) {
	soltarRegistro()
	dir := t.TempDir()
	edit := bytes.NewBufferString(`{"tool_name":"Edit","tool_input":{"file_path":"/p/main.go","old_string":"a","new_string":"b"}}`)
	var out, errb bytes.Buffer
	if code := ejecutarHook([]string{"pre-tool", "-vault", dir}, edit, &out, &errb); code != 0 {
		t.Fatalf("con la mínima por defecto, editar no pregunta: %d", code)
	}
	edit = bytes.NewBufferString(`{"tool_name":"Edit","tool_input":{"file_path":"/p/main.go","old_string":"a","new_string":"b"}}`)
	errb.Reset()
	if code := ejecutarHook([]string{"pre-tool", "-vault", dir, "-minima", "reversible"}, edit, &out, &errb); code != 2 {
		t.Fatalf("con mínima reversible y vault vacío, editar se frena: %d %s", code, errb.String())
	}
}

// El camino real: el hook en la máquina del agente, el vault en el servidor.
// Mismo Bearer, mismos tools, sin protocolo nuevo.
func TestHookRemoto(t *testing.T) {
	soltarRegistro()
	dir := t.TempDir()
	notaVerificada(t, dir, "build-se-regenera", "La carpeta build se regenera con go build: borrarla con rm -rf build es seguro.")
	srv := newMCPServer(dir)
	h := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return srv }, nil)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer t0k" {
			http.Error(w, "sin token", http.StatusUnauthorized)
			return
		}
		h.ServeHTTP(w, r)
	}))
	defer ts.Close()
	url := ts.URL + "/mcp"

	var out, errb bytes.Buffer
	code := ejecutarHook([]string{"session-start", "-url", url, "-token", "t0k", "-project", "p"},
		bytes.NewBufferString(`{"hook_event_name":"SessionStart","cwd":"/p"}`), &out, &errb)
	if code != 0 || !strings.Contains(out.String(), "Context pack") || !strings.Contains(out.String(), "build-se-regenera") {
		t.Fatalf("el pack de arranque tiene que salir por stdout: code=%d\nstdout=%s\nstderr=%s", code, out.String(), errb.String())
	}

	out.Reset()
	errb.Reset()
	code = ejecutarHook([]string{"pre-tool", "-url", url, "-token", "t0k", "-project", "p"},
		stdinHook("Bash", "rm -rf build"), &out, &errb)
	if code != 0 {
		t.Fatalf("remoto con respaldo verificado: esperaba 0, dio %d\n%s", code, errb.String())
	}
	out.Reset()
	errb.Reset()
	code = ejecutarHook([]string{"pre-tool", "-url", url, "-token", "t0k", "-project", "p"},
		stdinHook("Bash", "psql -c 'drop table usuarios'"), &out, &errb)
	if code != 2 || !strings.HasPrefix(errb.String(), "NOT AUTHORIZED") {
		t.Fatalf("remoto sin respaldo: esperaba 2 con el veredicto, dio %d\n%s", code, errb.String())
	}

	// Sin token, el servidor rechaza y el hook falla ABIERTO: avisa y deja
	// seguir. Un hook que bloquea todo cuando no puede consultar se apaga.
	errb.Reset()
	code = ejecutarHook([]string{"pre-tool", "-url", url, "-token", "malo"},
		stdinHook("Bash", "rm -rf build"), &out, &errb)
	if code != 0 || !strings.Contains(errb.String(), "no se pudo consultar") {
		t.Fatalf("sin poder consultar tiene que seguir y avisar: %d %s", code, errb.String())
	}
}
