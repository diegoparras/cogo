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

func TestSettingsModoLocal(t *testing.T) {
	b, err := Settings(nil, Comando{Binario: "/usr/local/bin/cogo", Vault: "/home/d/vault"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "/usr/local/bin/cogo hook session-start -vault /home/d/vault") {
		t.Fatalf("modo local:\n%s", b)
	}
}
