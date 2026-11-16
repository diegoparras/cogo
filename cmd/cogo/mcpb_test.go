package main

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

// testdata/mcpb-manifest-0.3.schema.json es el esquema oficial del manifiesto
// de MCP Bundles: schemas/mcpb-manifest-latest.schema.json de
// github.com/modelcontextprotocol/mcpb, con licencia MIT o Apache-2.0. El
// manifiesto que genera COGO se valida contra él.
func esquemaMCPB(t *testing.T) map[string]any {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "mcpb-manifest-0.3.schema.json"))
	if err != nil {
		t.Fatal(err)
	}
	var e map[string]any
	if err := json.Unmarshal(b, &e); err != nil {
		t.Fatal(err)
	}
	return e
}

// validarEsquema recorre un esquema draft-07 en lo que importa acá: tipos,
// claves obligatorias, claves de más donde additionalProperties es false,
// enum, const, items, $ref internos y anyOf. Devuelve un error por violación.
func validarEsquema(raiz, nodo map[string]any, valor any, ruta string) []string {
	if ref, ok := nodo["$ref"].(string); ok {
		destino := any(raiz)
		for _, parte := range strings.Split(strings.TrimPrefix(ref, "#/"), "/") {
			m, _ := destino.(map[string]any)
			destino = m[parte]
		}
		sub, _ := destino.(map[string]any)
		return validarEsquema(raiz, sub, valor, ruta)
	}
	var errs []string
	if alts, ok := nodo["anyOf"].([]any); ok {
		for _, a := range alts {
			if sub, _ := a.(map[string]any); len(validarEsquema(raiz, sub, valor, ruta)) == 0 {
				return nil
			}
		}
		return []string{ruta + ": no cumple ninguna alternativa de anyOf"}
	}
	if c, ok := nodo["const"]; ok && !reflect.DeepEqual(c, valor) {
		errs = append(errs, fmt.Sprintf("%s: tiene que ser %v, es %v", ruta, c, valor))
	}
	if en, ok := nodo["enum"].([]any); ok {
		esta := false
		for _, v := range en {
			esta = esta || reflect.DeepEqual(v, valor)
		}
		if !esta {
			errs = append(errs, fmt.Sprintf("%s: %v no está en %v", ruta, valor, en))
		}
	}
	switch nodo["type"] {
	case "object":
		m, ok := valor.(map[string]any)
		if !ok {
			return append(errs, ruta+": tiene que ser un objeto")
		}
		props, _ := nodo["properties"].(map[string]any)
		for _, r := range asStrings(nodo["required"]) {
			if _, hay := m[r]; !hay {
				errs = append(errs, ruta+": falta "+r)
			}
		}
		for k, v := range m {
			if p, ok := props[k].(map[string]any); ok {
				errs = append(errs, validarEsquema(raiz, p, v, ruta+"."+k)...)
				continue
			}
			switch extra := nodo["additionalProperties"].(type) {
			case bool:
				if !extra {
					errs = append(errs, ruta+": clave no permitida "+k)
				}
			case map[string]any:
				errs = append(errs, validarEsquema(raiz, extra, v, ruta+"."+k)...)
			}
		}
	case "array":
		l, ok := valor.([]any)
		if !ok {
			return append(errs, ruta+": tiene que ser una lista")
		}
		if it, ok := nodo["items"].(map[string]any); ok {
			for i, v := range l {
				errs = append(errs, validarEsquema(raiz, it, v, fmt.Sprintf("%s[%d]", ruta, i))...)
			}
		}
	case "string":
		if _, ok := valor.(string); !ok {
			errs = append(errs, ruta+": tiene que ser texto")
		}
	case "boolean":
		if _, ok := valor.(bool); !ok {
			errs = append(errs, ruta+": tiene que ser booleano")
		}
	}
	return errs
}

func asStrings(v any) []string {
	l, _ := v.([]any)
	var out []string
	for _, x := range l {
		if s, ok := x.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

// El manifiesto de cada plataforma cumple el esquema oficial, trae todas las
// tools del servidor, y arranca el binario de adentro sobre la carpeta que la
// persona elige como vault.
func TestManifestMCPBCumpleElEsquemaOficial(t *testing.T) {
	esquema := esquemaMCPB(t)
	tools, err := herramientasCOGO()
	if err != nil {
		t.Fatal(err)
	}
	if len(tools) != len(registradores) {
		t.Fatalf("el manifiesto lista todas las tools: %d de %d", len(tools), len(registradores))
	}
	casos := []struct{ goos, plataforma, binario string }{
		{"linux", "linux", "server/cogo"},
		{"darwin", "darwin", "server/cogo"},
		{"windows", "win32", "server/cogo.exe"},
	}
	for _, c := range casos {
		b, err := manifestMCPB("0.2.0", c.goos, tools)
		if err != nil {
			t.Fatal(err)
		}
		var m map[string]any
		if err := json.Unmarshal(b, &m); err != nil {
			t.Fatal(err)
		}
		if errs := validarEsquema(esquema, esquema, m, "$"); len(errs) > 0 {
			sort.Strings(errs)
			t.Fatalf("%s no cumple el esquema:\n%s", c.goos, strings.Join(errs, "\n"))
		}
		srv := m["server"].(map[string]any)
		cfg := srv["mcp_config"].(map[string]any)
		args, _ := json.Marshal(cfg["args"])
		plats, _ := json.Marshal(m["compatibility"].(map[string]any)["platforms"])
		if srv["entry_point"] != c.binario || cfg["command"] != "${__dirname}/"+c.binario ||
			string(args) != `["serve","-vault","${user_config.vault}"]` || string(plats) != `["`+c.plataforma+`"]` {
			t.Errorf("%s: server=%v platforms=%s", c.goos, srv, plats)
		}
	}
}

// `cogo mcpb` arma el .mcpb: un zip con el manifiesto y el binario adentro.
func TestEmpaquetarMCPB(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "cogo-windows-amd64.exe")
	if err := os.WriteFile(bin, []byte("binario de prueba"), 0o755); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "cogo-windows-amd64.mcpb")
	if err := cmdMCPB([]string{"-binario", bin, "-os", "windows", "-version", "v0.2.0", "-o", out}); err != nil {
		t.Fatal(err)
	}
	z, err := zip.OpenReader(out)
	if err != nil {
		t.Fatal(err)
	}
	defer z.Close()
	contenido := map[string]string{}
	for _, f := range z.File {
		r, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(r)
		r.Close()
		contenido[f.Name] = string(b)
	}
	if contenido["server/cogo.exe"] != "binario de prueba" || len(contenido) != 2 {
		t.Fatalf("el .mcpb trae el manifiesto y el binario, nada más: %v", keys(contenido))
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(contenido["manifest.json"]), &m); err != nil || m["version"] != "0.2.0" {
		t.Fatalf("la versión del tag va sin la v: %v %v", m["version"], err)
	}
	if err := cmdMCPB([]string{"-binario", bin, "-os", "windows", "-version", "dos", "-o", out}); err == nil {
		t.Fatal("una versión que no es semver no sirve para un .mcpb")
	}
}

func keys(m map[string]string) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
