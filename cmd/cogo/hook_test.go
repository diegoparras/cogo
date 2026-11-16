package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/diegoparras/cogo/internal/core"
	"github.com/diegoparras/cogo/internal/decision"
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

// stdinHookID es como Claude Code manda una llamada: con su tool_use_id.
func stdinHookID(evento, tool, comando, id string) *bytes.Buffer {
	return bytes.NewBufferString(`{"hook_event_name":"` + evento + `","tool_name":"` + tool +
		`","tool_input":{"command":` + jsonStr(comando) + `},"tool_use_id":"` + id + `","cwd":"/p"}`)
}

// decisionDe parsea el JSON de PreToolUse que el hook escribe en stdout.
func decisionDe(t *testing.T, out string) (string, string) {
	t.Helper()
	var r struct {
		H struct {
			D string `json:"permissionDecision"`
			M string `json:"permissionDecisionReason"`
		} `json:"hookSpecificOutput"`
	}
	if err := json.Unmarshal([]byte(out), &r); err != nil {
		t.Fatalf("el hook tenía que escribir JSON, escribió %q", out)
	}
	return r.H.D, r.H.M
}

func jsonStr(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
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

// Lo que COGO no autoriza lo decide el humano: Claude Code le muestra el
// diálogo de permiso con el motivo. Si después llega el PostToolUse con el
// mismo id, la herramienta corrió, y la decisión queda aprobada.
func TestHookPreguntaYAsientaLaAprobacion(t *testing.T) {
	soltarRegistro()
	dir := t.TempDir()
	var out, errb bytes.Buffer
	code := ejecutarHook([]string{"pre-tool", "-vault", dir}, stdinHookID("PreToolUse", "Bash", "rm -rf build", "toolu_9"), &out, &errb)
	if code != 0 {
		t.Fatalf("preguntar no es bloquear: code=%d stderr=%s", code, errb.String())
	}
	if d, m := decisionDe(t, out.String()); d != "ask" || !strings.Contains(m, "COGO no lo autoriza") {
		t.Fatalf("el humano tiene que ver el diálogo con el motivo: %q %q", d, m)
	}
	out.Reset()
	code = ejecutarHook([]string{"post-tool", "-vault", dir}, stdinHookID("PostToolUse", "Bash", "rm -rf build", "toolu_9"), &out, &errb)
	if code != 0 || out.Len() != 0 {
		t.Fatalf("el post-tool no opina: code=%d stdout=%q", code, out.String())
	}
	for _, dec := range decision.Abrir(dir).Estados(time.Now().UTC()) {
		if dec.Accion == "rm -rf build" && dec.Estado == decision.Aprobada {
			return
		}
	}
	t.Fatalf("la aprobación no quedó asentada: %+v", decision.Abrir(dir).Estados(time.Now().UTC()))
}

// Con -decide cogo, lo irreversible sin ninguna nota que lo respalde se
// frena, y el motivo va a stderr, que es lo que el modelo lee.
func TestHookBloqueaLoIrreversibleSinRespaldo(t *testing.T) {
	soltarRegistro()
	dir := t.TempDir()
	var out, errb bytes.Buffer
	code := ejecutarHook([]string{"pre-tool", "-vault", dir, "-decide", "cogo"}, stdinHook("Bash", "rm -rf build"), &out, &errb)
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

// Codex manda los borrados como un parche: el mismo hook lo frena igual, con
// el mismo contrato de salida que Claude Code (código 2 y el motivo).
func TestHookCodexFrenaUnParcheQueBorra(t *testing.T) {
	soltarRegistro()
	dir := t.TempDir()
	parche := "*** Begin Patch\n*** Delete File: migraciones/001.sql\n*** End Patch\n"
	var out, errb bytes.Buffer
	code := ejecutarHook([]string{"pre-tool", "-vault", dir, "-formato", "codex"}, stdinHook("apply_patch", parche), &out, &errb)
	if code != 2 || !strings.HasPrefix(errb.String(), "NOT AUTHORIZED") {
		t.Fatalf("Codex no sabe preguntar: esperaba 2 con el veredicto, dio %d:\n%s", code, errb.String())
	}
	// Un hooks.json de Codex instalado sin -formato también frena: Codex manda
	// turn_id, y eso alcanza para reconocerlo.
	errb.Reset()
	conTurno := bytes.NewBufferString(`{"hook_event_name":"PreToolUse","turn_id":"t1","tool_name":"Bash","tool_input":{"command":"rm -rf build"}}`)
	if code := ejecutarHook([]string{"pre-tool", "-vault", dir}, conTurno, &out, &errb); code != 2 {
		t.Fatalf("Codex reconocido por turn_id tiene que frenar: %d %s", code, errb.String())
	}
}

// hookCursor corre el hook en el formato de Cursor y exige un JSON válido: si
// Cursor recibe otra cosa de un hook de permiso, bloquea la acción.
func hookCursor(t *testing.T, args []string, stdin string) (map[string]any, int) {
	t.Helper()
	var out, errb bytes.Buffer
	code := ejecutarHook(append(args, "-formato", "cursor"), bytes.NewBufferString(stdin), &out, &errb)
	var r map[string]any
	if err := json.Unmarshal(out.Bytes(), &r); err != nil {
		t.Fatalf("Cursor necesita JSON en stdout, dio %q (stderr %q)", out.String(), errb.String())
	}
	return r, code
}

func TestHookCursorContestaSiempreJSON(t *testing.T) {
	soltarRegistro()
	dir := t.TempDir()
	r, code := hookCursor(t, []string{"pre-tool", "-vault", dir},
		`{"hook_event_name":"beforeShellExecution","command":"rm -rf build","cwd":"/p"}`)
	if m, _ := r["user_message"].(string); code != 0 || r["permission"] != "ask" || !strings.Contains(m, "COGO no lo autoriza") {
		t.Fatalf("en shell, Cursor le pregunta al humano: %d %v", code, r)
	}
	r, code = hookCursor(t, []string{"pre-tool", "-vault", dir, "-decide", "cogo"},
		`{"hook_event_name":"beforeShellExecution","command":"rm -rf build","cwd":"/p"}`)
	if code != 0 || r["permission"] != "deny" {
		t.Fatalf("con -decide cogo, rm -rf sin respaldo se niega en JSON, con código 0: %d %v", code, r)
	}
	if m, _ := r["agent_message"].(string); !strings.HasPrefix(m, "NOT AUTHORIZED") {
		t.Errorf("el agente tiene que leer el veredicto: %v", r)
	}
	if m, _ := r["user_message"].(string); m == "" {
		t.Errorf("el usuario tiene que ver por qué: %v", r)
	}
	if r, _ := hookCursor(t, []string{"pre-tool", "-vault", dir}, `{"hook_event_name":"beforeShellExecution","command":"ls -la"}`); r["permission"] != "allow" {
		t.Errorf("ls pasa: %v", r)
	}
	if r, _ := hookCursor(t, []string{"pre-tool", "-vault", dir}, `{roto`); r["permission"] != "allow" {
		t.Errorf("con un stdin roto falla abierto, pero en JSON: %v", r)
	}
}

func TestHookCursorSessionStartDaContexto(t *testing.T) {
	soltarRegistro()
	r, _ := hookCursor(t, []string{"session-start", "-vault", t.TempDir()}, `{"hook_event_name":"sessionStart","session_id":"s"}`)
	if c, _ := r["additional_context"].(string); !strings.Contains(c, "Context pack") {
		t.Fatalf("el pack va en additional_context: %v", r)
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
	if code := ejecutarHook([]string{"pre-tool", "-vault", dir, "-minima", "reversible", "-decide", "cogo"}, edit, &out, &errb); code != 2 {
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
		stdinHookID("PreToolUse", "Bash", "psql -c 'drop table usuarios'", "toolu_r"), &out, &errb)
	if d, _ := decisionDe(t, out.String()); code != 0 || d != "ask" {
		t.Fatalf("remoto sin respaldo: el humano decide, dio code=%d %q\n%s", code, d, errb.String())
	}
	out.Reset()
	if code := ejecutarHook([]string{"post-tool", "-url", url, "-token", "t0k"},
		stdinHookID("PostToolUse", "Bash", "psql -c 'drop table usuarios'", "toolu_r"), &out, &errb); code != 0 {
		t.Fatalf("post-tool remoto: %d %s", code, errb.String())
	}
	aprobada := false
	for _, dec := range decision.Abrir(dir).Estados(time.Now().UTC()) {
		aprobada = aprobada || (dec.Estado == decision.Aprobada && strings.Contains(dec.Accion, "drop table"))
	}
	if !aprobada {
		t.Fatalf("la aprobación remota tiene que quedar en el vault del servidor")
	}
	out.Reset()
	errb.Reset()
	code = ejecutarHook([]string{"pre-tool", "-url", url, "-token", "t0k", "-project", "p", "-decide", "cogo"},
		stdinHook("Bash", "psql -c 'drop table usuarios'"), &out, &errb)
	if code != 2 || !strings.HasPrefix(errb.String(), "NOT AUTHORIZED") {
		t.Fatalf("remoto con -decide cogo: esperaba 2 con el veredicto, dio %d\n%s", code, errb.String())
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
