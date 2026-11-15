package gancho

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/diegoparras/cogo/internal/accion"
)

func TestTraducirHerramientas(t *testing.T) {
	casos := []struct {
		tool   string
		input  map[string]any
		quiero string
		ok     bool
	}{
		{"Bash", map[string]any{"command": "rm -rf build"}, "rm -rf build", true},
		{"Edit", map[string]any{"file_path": "/x/a.go", "old_string": "a", "new_string": "b"}, "editar /x/a.go", true},
		{"MultiEdit", map[string]any{"file_path": "/x/a.go"}, "editar /x/a.go", true},
		{"Write", map[string]any{"file_path": "/x/b.go", "content": "..."}, "escribir /x/b.go", true},
		{"NotebookEdit", map[string]any{"notebook_path": "/x/n.ipynb"}, "editar /x/n.ipynb", true},
		{"Read", map[string]any{"file_path": "/x/a.go"}, "", false},
		{"Bash", map[string]any{}, "", false},
	}
	for _, c := range casos {
		got, ok := Traducir(c.tool, c.input)
		if got != c.quiero || ok != c.ok {
			t.Errorf("%s %v: esperaba (%q,%v), dio (%q,%v)", c.tool, c.input, c.quiero, c.ok, got, ok)
		}
	}
}

// Codex no tiene Edit ni Write: todas las ediciones son un parche. Lo que COGO
// juzga son los archivos que el parche toca y qué les hace.
func TestTraducirParcheDeCodex(t *testing.T) {
	parche := "*** Begin Patch\n*** Update File: src/a.go\n@@\n-x\n+y\n" +
		"*** Delete File: viejo.sql\n*** Add File: nuevo.go\n+package x\n*** End Patch\n"
	got, ok := Traducir("apply_patch", map[string]any{"command": parche})
	if want := "editar src/a.go; borrar viejo.sql; escribir nuevo.go"; !ok || got != want {
		t.Fatalf("parche: esperaba (%q,true), dio (%q,%v)", want, got, ok)
	}
	if _, ok := Traducir("apply_patch", map[string]any{"command": "cualquier cosa"}); ok {
		t.Error("un parche sin archivos reconocibles no tiene nada que juzgar")
	}
}

// El hook es una puerta sobre lo peligroso conocido: `ls` no consulta a nadie,
// y lo que no se reconoce tampoco. Un hook que pregunta por todo se apaga a la
// semana.
func TestHayQuePreguntarSoloDesdeLaMinima(t *testing.T) {
	casos := []struct {
		accion    string
		minima    accion.Clase
		pregunta  bool
		claseSiOk accion.Clase
	}{
		{"ls -la", accion.Costosa, false, ""},
		{"go test ./...", accion.Costosa, false, ""},
		{"rm -rf build", accion.Costosa, true, accion.Irreversible},
		{"psql -c 'drop table usuarios'", accion.Costosa, true, accion.Irreversible},
		{"terraform apply", accion.Costosa, true, accion.Costosa},
		{"editar /x/a.go", accion.Costosa, false, accion.Reversible},
		{"editar /x/a.go", accion.Reversible, true, accion.Reversible},
		{"git push --force origin main", accion.Irreversible, true, accion.Irreversible},
		{"terraform apply", accion.Irreversible, false, accion.Costosa},
	}
	for _, c := range casos {
		clase, pregunta := HayQuePreguntar(c.accion, c.minima)
		if pregunta != c.pregunta {
			t.Errorf("%q con mínima %s: esperaba pregunta=%v, dio %v (%s)", c.accion, c.minima, c.pregunta, pregunta, clase)
		}
		if pregunta && clase != c.claseSiOk {
			t.Errorf("%q: clase esperada %s, dio %s", c.accion, c.claseSiOk, clase)
		}
	}
}

func TestLeerStdinVacioNoEsError(t *testing.T) {
	e, err := Leer(strings.NewReader("  \n"))
	if err != nil || e.Tool != "" {
		t.Fatalf("stdin vacío: %+v %v", e, err)
	}
	e, err = Leer(strings.NewReader(`{"hook_event_name":"PreToolUse","tool_name":"Bash","tool_input":{"command":"ls"},"cwd":"/p"}`))
	if err != nil || e.Tool != "Bash" || e.ToolInput["command"] != "ls" || e.Cwd != "/p" {
		t.Fatalf("parseo: %+v %v", e, err)
	}
}

// Instalar dos veces no duplica, y lo que ya había queda como estaba.
func TestSettingsMezclaYEsIdempotente(t *testing.T) {
	previo := []byte(`{
	  "permissions": {"allow": ["Bash(go test *)"]},
	  "hooks": {
	    "PreToolUse": [{"matcher": "Bash", "hooks": [{"type": "command", "command": "./mio.sh"}]}],
	    "Stop": [{"hooks": [{"type": "command", "command": "echo fin"}]}]
	  }
	}`)
	c := Comando{Binario: `C:\Program Files\cogo\cogo.exe`, URL: "https://cogo.example/mcp", Token: "cogo_abc", Proyecto: "custodia", Minima: "costly"}
	una, err := Settings(previo, c)
	if err != nil {
		t.Fatal(err)
	}
	dos, err := Settings(una, c)
	if err != nil {
		t.Fatal(err)
	}
	if string(una) != string(dos) {
		t.Fatalf("no es idempotente:\n%s\n---\n%s", una, dos)
	}
	var root map[string]any
	if err := json.Unmarshal(dos, &root); err != nil {
		t.Fatal(err)
	}
	if _, ok := root["permissions"]; !ok {
		t.Error("se perdieron los permisos que ya estaban")
	}
	hooks := root["hooks"].(map[string]any)
	pre := hooks["PreToolUse"].([]any)
	if len(pre) != 2 {
		t.Fatalf("PreToolUse tendría que tener el ajeno y el nuestro: %d", len(pre))
	}
	if _, ok := hooks["Stop"]; !ok {
		t.Error("se perdió el hook Stop ajeno")
	}
	if len(hooks["SessionStart"].([]any)) != 1 {
		t.Error("SessionStart tendría que tener exactamente el nuestro")
	}
	s := string(dos)
	for _, quiero := range []string{`\"C:\\Program Files\\cogo\\cogo.exe\" hook pre-tool`, "-url https://cogo.example/mcp", "-token cogo_abc", "-project custodia", "-minima costly", `"matcher": "Bash|Edit|Write|MultiEdit|NotebookEdit"`} {
		if !strings.Contains(s, quiero) {
			t.Errorf("falta %s en:\n%s", quiero, s)
		}
	}
	if strings.Contains(s, "-vault") {
		t.Error("en modo remoto no va el vault")
	}
}

// El hooks.json de Codex vive en el repo y no tiene variante local: el token no
// se escribe nunca, aunque venga en el Comando. El hook lo lee de COGO_TOKEN.
func TestHooksCodexSinTokenEIdempotente(t *testing.T) {
	previo := []byte(`{"hooks":{"PreToolUse":[{"matcher":"Bash","hooks":[{"type":"command","command":"./mio.sh"}]}]}}`)
	c := Comando{Binario: "/usr/local/bin/cogo", URL: "https://cogo.example/mcp", Token: "cogo_secreto", Proyecto: "p", Minima: "costly"}
	una, err := HooksCodex(previo, c)
	if err != nil {
		t.Fatal(err)
	}
	dos, err := HooksCodex(una, c)
	if err != nil {
		t.Fatal(err)
	}
	if string(una) != string(dos) {
		t.Fatalf("no es idempotente:\n%s\n---\n%s", una, dos)
	}
	s := string(dos)
	if strings.Contains(s, "cogo_secreto") || strings.Contains(s, "-token") {
		t.Fatalf("el token no puede ir al hooks.json de Codex:\n%s", s)
	}
	for _, quiero := range []string{`"matcher": "Bash|apply_patch"`, "/usr/local/bin/cogo hook session-start -url https://cogo.example/mcp -project p"} {
		if !strings.Contains(s, quiero) {
			t.Errorf("falta %s en:\n%s", quiero, s)
		}
	}
	var root map[string]any
	if err := json.Unmarshal(dos, &root); err != nil {
		t.Fatal(err)
	}
	if pre := root["hooks"].(map[string]any)["PreToolUse"].([]any); len(pre) != 2 {
		t.Fatalf("PreToolUse tendría que tener el ajeno y el nuestro: %d", len(pre))
	}
}

// Cursor manda el comando de shell sin tool_name, arriba de todo.
func TestLeerEntradaDeCursor(t *testing.T) {
	e, err := Leer(strings.NewReader(`{"hook_event_name":"beforeShellExecution","command":"rm -rf build","cwd":"/p","sandbox":false}`))
	if err != nil || e.Tool != "Bash" || e.ToolInput["command"] != "rm -rf build" {
		t.Fatalf("entrada de Cursor: %+v %v", e, err)
	}
}

// En el preToolUse de Cursor las ediciones son Write y los borrados Delete. Los
// campos de esos dos no están documentados: se aceptan los nombres probables.
func TestTraducirHerramientasDeCursor(t *testing.T) {
	casos := []struct {
		tool   string
		input  map[string]any
		quiero string
	}{
		{"Shell", map[string]any{"command": "rm -rf build"}, "rm -rf build"},
		{"Delete", map[string]any{"file_path": "a.sql"}, "borrar a.sql"},
		{"Delete", map[string]any{"path": "a.sql"}, "borrar a.sql"},
		{"Write", map[string]any{"path": "b.go"}, "escribir b.go"},
	}
	for _, c := range casos {
		if got, ok := Traducir(c.tool, c.input); !ok || got != c.quiero {
			t.Errorf("%s %v: esperaba %q, dio (%q,%v)", c.tool, c.input, c.quiero, got, ok)
		}
	}
}

// El hooks.json de Cursor tiene otra forma: version, y cada entrada es el
// comando directo. Tampoco lleva el token.
func TestHooksCursorSinTokenEIdempotente(t *testing.T) {
	previo := []byte(`{"version":1,"hooks":{"beforeShellExecution":[{"command":"./mio.sh"}],"afterFileEdit":[{"command":"./fmt.sh"}]}}`)
	c := Comando{Binario: "/usr/local/bin/cogo", URL: "https://cogo.example/mcp", Token: "cogo_secreto", Proyecto: "p", Minima: "costly"}
	una, err := HooksCursor(previo, c)
	if err != nil {
		t.Fatal(err)
	}
	dos, err := HooksCursor(una, c)
	if err != nil {
		t.Fatal(err)
	}
	if string(una) != string(dos) {
		t.Fatalf("no es idempotente:\n%s\n---\n%s", una, dos)
	}
	s := string(dos)
	if strings.Contains(s, "cogo_secreto") || strings.Contains(s, "-token") {
		t.Fatalf("el token no puede ir al hooks.json de Cursor:\n%s", s)
	}
	for _, quiero := range []string{`"version": 1`, "hook pre-tool -url https://cogo.example/mcp -project p -minima costly -formato cursor", `"matcher": "Write|Delete"`} {
		if !strings.Contains(s, quiero) {
			t.Errorf("falta %s en:\n%s", quiero, s)
		}
	}
	var root map[string]any
	if err := json.Unmarshal(dos, &root); err != nil {
		t.Fatal(err)
	}
	h := root["hooks"].(map[string]any)
	if n := len(h["beforeShellExecution"].([]any)); n != 2 {
		t.Errorf("beforeShellExecution tendría que tener el ajeno y el nuestro: %d", n)
	}
	if _, ok := h["afterFileEdit"]; !ok {
		t.Error("se perdió el hook ajeno afterFileEdit")
	}
	if n := len(h["sessionStart"].([]any)); n != 1 {
		t.Errorf("sessionStart tendría que tener exactamente el nuestro: %d", n)
	}
}

func TestSettingsModoLocal(t *testing.T) {
	b, err := Settings(nil, Comando{Binario: "/usr/local/bin/cogo", Vault: "/home/d/vault"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "/usr/local/bin/cogo hook session-start -vault /home/d/vault") {
		t.Fatalf("modo local:\n%s", b)
	}
}
