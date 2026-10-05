package jev

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

// servidor es un Jev de mentira: contesta según el id de la pregunta con lo
// que el test le programó, y cuenta cuántas veces lo llamaron.
type servidor struct {
	t        *testing.T
	llamadas atomic.Int32
	// responder recibe el cuerpo parseado y devuelve answers.
	responder func(state map[string]any, preguntas map[string]map[string]any) map[string]any
	status    int
}

func (s *servidor) arrancar() (*httptest.Server, *Cliente) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.llamadas.Add(1)
		if r.Header.Get("Authorization") != "Bearer k" {
			http.Error(w, "no", http.StatusUnauthorized)
			return
		}
		if s.status != 0 && s.status != 200 {
			w.Header().Set("Retry-After", "1")
			http.Error(w, "ocupado", s.status)
			return
		}
		var cuerpo struct {
			Model     string                    `json:"model"`
			State     map[string]any            `json:"state"`
			Questions map[string]map[string]any `json:"questions"`
		}
		if err := json.NewDecoder(r.Body).Decode(&cuerpo); err != nil {
			http.Error(w, err.Error(), 422)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"model": "jev-1.13.0", "answers": s.responder(cuerpo.State, cuerpo.Questions),
			"usage": map[string]int{"input_tokens": 10, "output_tokens": 0},
		})
	}))
	c := FromEnv()
	c.URL, c.Clave = ts.URL, "k"
	return ts, c
}

func noul(p float64) map[string]any { return map[string]any{"type": "noul", "noul": p} }
func choice(c string) map[string]any {
	return map[string]any{"type": "choice", "choice": c, "probabilities": map[string]float64{c: 0.9}, "confidence": 0.9}
}

func TestSinClaveSeAbstiene(t *testing.T) {
	c := &Cliente{}
	j := Nuevo(t.TempDir(), c)
	if j.Disponible() {
		t.Fatal("sin clave no está disponible")
	}
	if _, ok := j.Pertinencia(context.Background(), "x", "y"); ok {
		t.Fatal("sin clave tiene que abstenerse, no inventar")
	}
	if _, _, ok := j.Radiografia(context.Background(), "x"); ok {
		t.Fatal("sin clave tiene que abstenerse")
	}
}

func TestPertinenciaYCache(t *testing.T) {
	s := &servidor{t: t, responder: func(state map[string]any, q map[string]map[string]any) map[string]any {
		if state["note"] != "Redis vive en fisherboy-redis:6379" || state["action"] != "reiniciar redis" {
			t.Errorf("el estado tiene que llevar la nota y la acción: %v", state)
		}
		return map[string]any{"q": noul(0.91)}
	}}
	ts, c := s.arrancar()
	defer ts.Close()
	dir := t.TempDir()
	j := Nuevo(dir, c)

	p, ok := j.Pertinencia(context.Background(), "Redis vive en fisherboy-redis:6379", "reiniciar redis")
	if !ok || p != 0.91 {
		t.Fatalf("esperaba 0.91, dio %v %v", p, ok)
	}
	// La segunda vez sale del caché: ni una llamada más, y sobrevive a un juez nuevo.
	j2 := Nuevo(dir, c)
	if p, ok := j2.Pertinencia(context.Background(), "Redis vive en fisherboy-redis:6379", "reiniciar redis"); !ok || p != 0.91 {
		t.Fatalf("caché: %v %v", p, ok)
	}
	if n := s.llamadas.Load(); n != 1 {
		t.Fatalf("una sola llamada HTTP, hubo %d", n)
	}
	// Y el crudo quedó escrito, con el modelo que contestó.
	crudo, err := os.ReadFile(filepath.Join(dir, ".cogo", "jev.jsonl"))
	if err != nil || !strings.Contains(string(crudo), `"modelo":"jev-1.13.0"`) || !strings.Contains(string(crudo), `"tipo":"pertinencia"`) {
		t.Fatalf("el crudo tiene que quedar: %s %v", crudo, err)
	}
}

// Una Choice sin abstención elige la opción menos mala con confianza alta.
// Con `unclear`, Jev puede no saber — y COGO hace lo de siempre.
func TestClaseYNivelSeAbstienenConUnclear(t *testing.T) {
	resp := "unclear"
	s := &servidor{t: t, responder: func(_ map[string]any, q map[string]map[string]any) map[string]any {
		for id, p := range q {
			crit, _ := p["criteria"].(map[string]any)
			if _, hay := crit["unclear"]; !hay {
				t.Errorf("%s: toda Choice lleva abstención", id)
			}
		}
		return map[string]any{"q": choice(resp)}
	}}
	ts, c := s.arrancar()
	defer ts.Close()
	j := Nuevo(t.TempDir(), c)
	if cl, ok := j.Clase(context.Background(), "hacer algo raro"); ok || cl != "" {
		t.Fatalf("unclear tiene que abstenerse: %q %v", cl, ok)
	}
	resp = "irreversible"
	if cl, ok := j.Clase(context.Background(), "vaciar el bucket de producción"); !ok || cl != "irreversible" {
		t.Fatalf("esperaba irreversible: %q %v", cl, ok)
	}
	resp = "reasoned"
	if n, ok := j.NivelDeEvidencia(context.Background(), "creo que aguanta 200"); !ok || n != "reasoned" {
		t.Fatalf("esperaba reasoned: %q %v", n, ok)
	}
}

// Precalentar pregunta solo lo que falta, por lotes, con las rutas
// reescritas para que cada pregunta mire su ítem.
func TestPrecalentarPorLotes(t *testing.T) {
	var lotes []int
	s := &servidor{t: t, responder: func(state map[string]any, q map[string]map[string]any) map[string]any {
		lotes = append(lotes, len(q))
		items, _ := state["items"].(map[string]any)
		out := map[string]any{}
		for id, p := range q {
			if _, hay := items[id]; !hay {
				t.Errorf("la pregunta %s no tiene su ítem en el estado", id)
			}
			instr, _ := p["instructions"].(map[string]any)
			qs, _ := instr["question"].(string)
			if !strings.Contains(qs, "`items."+id+".") {
				t.Errorf("la pregunta tiene que apuntar a su ítem: %q", qs)
			}
			if strings.HasPrefix(id, "check:") {
				out[id] = noul(0.2)
			} else {
				out[id] = choice("reasoned")
			}
		}
		return out
	}}
	ts, c := s.arrancar()
	defer ts.Close()
	j := Nuevo(t.TempDir(), c)

	var notas []NotaParaJuzgar
	for i := 0; i < 45; i++ {
		notas = append(notas, NotaParaJuzgar{Claim: "claim " + string(rune('a'+i%26)) + string(rune('0'+i/26)), Test: "test", Refs: []string{"ref"}})
	}
	n, err := j.Precalentar(context.Background(), notas)
	if err != nil {
		t.Fatal(err)
	}
	// 45 checks distintos + 1 ref repetida = 46 juicios, en lotes de 40 y 6.
	if n != 46 || len(lotes) != 2 || lotes[0] != 40 || lotes[1] != 6 {
		t.Fatalf("juicios=%d lotes=%v", n, lotes)
	}
	// La evaluación lee del caché sin HTTP.
	if p, ok := j.ChequeoEnCache("claim a0", "test"); !ok || p != 0.2 {
		t.Fatalf("caché del check: %v %v", p, ok)
	}
	if niv, ok := j.NivelDeEvidenciaEnCache("ref"); !ok || niv != "reasoned" {
		t.Fatalf("caché del nivel: %v %v", niv, ok)
	}
	antes := s.llamadas.Load()
	if _, err := j.Precalentar(context.Background(), notas); err != nil || s.llamadas.Load() != antes {
		t.Fatal("con todo en caché no se pregunta nada")
	}
}

func TestReintentaUnaVezYNoInsisteCon401(t *testing.T) {
	s := &servidor{t: t, status: 429, responder: func(map[string]any, map[string]map[string]any) map[string]any { return nil }}
	ts, c := s.arrancar()
	defer ts.Close()
	if _, _, err := c.Preguntar(context.Background(), map[string]any{}, map[string]Pregunta{"q": {Type: "noul", Instructions: "x"}}); err == nil {
		t.Fatal("con 429 persistente tiene que fallar")
	}
	if n := s.llamadas.Load(); n != 2 {
		t.Fatalf("429: un reintento, no más: %d", n)
	}
	s.llamadas.Store(0)
	c.Clave = "otra"
	if _, _, err := c.Preguntar(context.Background(), map[string]any{}, map[string]Pregunta{"q": {Type: "noul", Instructions: "x"}}); err == nil || !strings.Contains(err.Error(), "401") {
		t.Fatalf("401 tiene que decirlo: %v", err)
	}
	if n := s.llamadas.Load(); n != 1 {
		t.Fatalf("401 no se reintenta: %d", n)
	}
}

func TestSeccionParaImportar(t *testing.T) {
	s := &servidor{t: t, responder: func(state map[string]any, q map[string]map[string]any) map[string]any {
		if _, hay := q["kind"]["criteria"].(map[string]any)["other"]; !hay {
			t.Error("la Choice de tipo lleva abstención")
		}
		out := map[string]any{"kind": choice("decision"), "asserts": noul(0.9)}
		if state["title"] == "Enlaces" {
			out = map[string]any{"kind": choice("other"), "asserts": noul(0.1)}
		}
		return out
	}}
	ts, c := s.arrancar()
	defer ts.Close()
	j := Nuevo(t.TempDir(), c)
	tipo, afirma, ok := j.Seccion(context.Background(), "Decisión", "Adoptamos Fastify.")
	if !ok || tipo != "decision" || !afirma {
		t.Fatalf("sección: %q %v %v", tipo, afirma, ok)
	}
	tipo, afirma, ok = j.Seccion(context.Background(), "Enlaces", "- [a](x)")
	if !ok || tipo != "" || afirma {
		t.Fatalf("relleno: %q %v %v", tipo, afirma, ok)
	}
}

func TestRadiografiaYTacticas(t *testing.T) {
	s := &servidor{t: t, responder: func(state map[string]any, q map[string]map[string]any) map[string]any {
		out := map[string]any{}
		for id := range q {
			switch id {
			case "commitment":
				out[id] = map[string]any{"type": "score", "score": 1.8}
			case "evidence":
				out[id] = choice("none")
			default:
				out[id] = noul(0.85)
			}
		}
		return out
	}}
	ts, c := s.arrancar()
	defer ts.Close()
	j := Nuevo(t.TempDir(), c)
	comp, ev, ok := j.Radiografia(context.Background(), "Obviamente el pool aguanta 200.")
	if !ok || comp != "boosted" || ev != "none" {
		t.Fatalf("radiografía: %q %q %v", comp, ev, ok)
	}
	ps, err := j.Tacticas(context.Background(), "es tu última oportunidad", []Tecnica{{ID: "persuasion.scarcity", Nombre: "Escasez"}, {ID: "x.y", Nombre: "Otra"}})
	if err != nil || ps["persuasion.scarcity"] != 0.85 || len(ps) != 2 {
		t.Fatalf("tácticas: %v %v", ps, err)
	}
}
