package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/diegoparras/cogo/internal/recibo"
)

// El veredicto del humano sobre un recibo es lo que alimenta "Lo que COGO
// evitó" en la sala de guerra: un bloqueo confirmado cuenta; uno sin juzgar, no.
func TestElVeredictoAlimentaLoQueCOGOEvito(t *testing.T) {
	s := testServer(t)
	st := recibo.Abrir(s.dir)
	bloqueado, err := st.Agregar(recibo.Recibo{Quien: "token:claude", Accion: "drop table usuarios", Clase: "irreversible", Autoriza: false})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.Agregar(recibo.Recibo{Quien: "token:claude", Accion: "ls", Clase: "reversible", Autoriza: true}); err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	s.Mount(mux)

	post := func(body string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("POST", "/api/recibos/veredicto", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		mux.ServeHTTP(rec, req)
		return rec
	}
	if rec := post(`{"recibo":"r-no-existe","acertado":true}`); rec.Code != http.StatusNotFound {
		t.Fatalf("recibo inexistente: %d %s", rec.Code, rec.Body.String())
	}
	if rec := post(`{"recibo":"` + bloqueado.ID + `","acertado":true,"nota":"era producción"}`); rec.Code != http.StatusOK {
		t.Fatalf("veredicto: %d %s", rec.Code, rec.Body.String())
	}

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", "/api/salaguerra", nil))
	var out struct {
		Evitado struct {
			Balance struct {
				TotalDecidido int `json:"total_decidido"`
				Juzgados      int `json:"juzgados"`
				BloqueosBien  int `json:"bloqueos_bien"`
				SinJuzgar     int `json:"sin_juzgar"`
			} `json:"balance"`
			Bloqueos int `json:"bloqueos"`
		} `json:"evitado"`
		Recibos struct {
			Ultimos []struct {
				ID       string `json:"id"`
				Juzgado  bool   `json:"juzgado"`
				Acertado bool   `json:"acertado"`
			} `json:"ultimos"`
		} `json:"recibos"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	b := out.Evitado.Balance
	if b.TotalDecidido != 2 || b.Juzgados != 1 || b.BloqueosBien != 1 || b.SinJuzgar != 1 || out.Evitado.Bloqueos != 1 {
		t.Fatalf("balance: %+v bloqueos=%d", b, out.Evitado.Bloqueos)
	}
	juzgados := 0
	for _, r := range out.Recibos.Ultimos {
		if r.ID == bloqueado.ID && (!r.Juzgado || !r.Acertado) {
			t.Fatalf("la fila del recibo juzgado tiene que decirlo: %+v", r)
		}
		if r.Juzgado {
			juzgados++
		}
	}
	if juzgados != 1 {
		t.Fatalf("un solo recibo juzgado en la tabla: %d", juzgados)
	}
	// GET no es un veredicto.
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", "/api/recibos/veredicto", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET: %d", rec.Code)
	}
}
