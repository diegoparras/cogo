package web

import (
	"encoding/json"
	"net/http"
	"reflect"
	"strings"
	"testing"
)

// La simetría entre lo que el editor ESCRIBE y lo que la API DEVUELVE.
//
// # EL BUG QUE ESTE TEST EXISTE PARA IMPEDIR
//
// El editor carga una nota desde /api/note, arma su borrador con eso y guarda el
// borrador entero. Si la respuesta no trae un campo que el borrador sí acepta, el
// campo llega vacío al editor y se guarda vacío: abrir una nota para corregirle
// una coma le borra lo que no se estaba mirando.
//
// Pasó de verdad. Las notas de brecha se podían escribir con su pregunta, sus
// bloqueos, su costo y lo ya intentado, y /api/note no devolvía ninguno de los
// cuatro. La pérdida era silenciosa: ni error, ni aviso, ni forma de notarlo
// salvo comparar el archivo antes y después.
//
// No es un caso especial de las brechas: es lo que pasa CADA VEZ que se agrega un
// campo de entrada y se toca un solo lado. Por eso el test no lista campos —
// los saca de la struct — y así cubre también los que todavía no existen.
func TestTodoLoQueSeEscribeSeLee(t *testing.T) {
	s := testServer(t)

	// Una nota con TODOS los campos de entrada puestos.
	completa := draft{
		ID: "completa", Type: "gap", Project: "demo",
		Body:       "## Claim\n¿se satura el pool?",
		Evidence:   nil, // una brecha no lleva evidencia
		CheckTest:  "",
		DependsOn:  []string{"redis"},
		Supersedes: "", CausedBy: "",
		Scope:     map[string]string{"os": "linux"},
		Origin:    "human",
		Pinned:    true,
		Question:  "¿se satura el pool?",
		Blocks:    []string{"migrar-db"},
		Cost:      "medio",
		Attempted: []string{"se miró el dashboard"},
	}
	if rec := call(s.handleCapture, http.MethodPost, "/api/capture", completa); rec.Code != http.StatusOK {
		t.Fatalf("no se pudo guardar: %d %s", rec.Code, rec.Body.String())
	}

	rec := call(s.handleNote, http.MethodGet, "/api/note?id=completa", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("no se pudo leer: %d", rec.Code)
	}
	var leido map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &leido); err != nil {
		t.Fatal(err)
	}

	// Cada campo que el borrador acepta tiene que volver en la respuesta.
	tipo := reflect.TypeOf(draft{})
	var faltan []string
	for i := 0; i < tipo.NumField(); i++ {
		clave := strings.Split(tipo.Field(i).Tag.Get("json"), ",")[0]
		if clave == "" || clave == "-" {
			continue
		}
		if _, hay := leido[clave]; !hay {
			faltan = append(faltan, clave)
		}
	}
	if len(faltan) > 0 {
		t.Errorf("el editor puede escribir estos campos y /api/note no los devuelve: %s\n"+
			"  abrir una nota para editarla los borraría en silencio", strings.Join(faltan, ", "))
	}
}

// Y la vuelta completa: leer, guardar sin cambios, y que nada se haya perdido.
// Es exactamente lo que hace alguien que abre una nota y aprieta guardar.
func TestGuardarSinCambiosNoPierdeNada(t *testing.T) {
	s := testServer(t)
	original := draft{
		ID: "brecha", Type: "gap", Project: "demo",
		Body: "## Claim\n¿cuánto aguanta el pool?", Scope: map[string]string{"os": "linux"},
		Pinned: true, Question: "¿cuánto aguanta el pool?",
		Blocks: []string{"migrar-db", "subir-replicas"}, Cost: "alto",
		Attempted: []string{"se miró el dashboard", "se probó en staging"},
	}
	if rec := call(s.handleCapture, http.MethodPost, "/api/capture", original); rec.Code != http.StatusOK {
		t.Fatalf("no se pudo guardar: %d", rec.Code)
	}

	antes := leerNota(t, s, "brecha")
	// El editor rearma el borrador desde la respuesta y lo manda igual.
	d := draft{
		ID: str(antes["id"]), Type: str(antes["type"]), Project: str(antes["project"]),
		Body: str(antes["body"]), CheckTest: str(antes["check_test"]),
		Scope: mapa(antes["scope"]), Origin: str(antes["origin"]),
		Pinned: antes["pinned"] == true, Question: str(antes["question"]),
		Blocks: lista(antes["blocks"]), Cost: str(antes["cost_to_resolve"]),
		Attempted: lista(antes["attempted"]),
	}
	if rec := call(s.handleCapture, http.MethodPost, "/api/capture", d); rec.Code != http.StatusOK {
		t.Fatalf("no se pudo re-guardar: %d", rec.Code)
	}

	despues := leerNota(t, s, "brecha")
	for _, k := range []string{"question", "blocks", "cost_to_resolve", "attempted", "scope", "pinned"} {
		a, _ := json.Marshal(antes[k])
		b, _ := json.Marshal(despues[k])
		if string(a) != string(b) {
			t.Errorf("guardar sin cambios perdió %q: %s → %s", k, a, b)
		}
	}
}

func leerNota(t *testing.T, s *Server, id string) map[string]any {
	t.Helper()
	rec := call(s.handleNote, http.MethodGet, "/api/note?id="+id, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("leyendo %s: %d", id, rec.Code)
	}
	var m map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
		t.Fatal(err)
	}
	return m
}

func str(v any) string {
	s, _ := v.(string)
	return s
}

func lista(v any) []string {
	xs, _ := v.([]any)
	out := make([]string, 0, len(xs))
	for _, x := range xs {
		out = append(out, str(x))
	}
	return out
}

func mapa(v any) map[string]string {
	m, _ := v.(map[string]any)
	if m == nil {
		return nil
	}
	out := map[string]string{}
	for k, x := range m {
		out[k] = str(x)
	}
	return out
}
