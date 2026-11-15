package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/diegoparras/cogo/internal/decision"
	"github.com/diegoparras/cogo/internal/recibo"
)

// Lo que la persona decidió cuando COGO no autorizó se ve en la sala de
// guerra: cuántas aprobó, rechazó, siguen pendientes o vencieron, y en cada
// fila de recibos, qué decidió.
func TestLaSalaDeGuerraMuestraLoQueDecidioLaPersona(t *testing.T) {
	s := testServer(t)
	st := recibo.Abrir(s.dir)
	dec := decision.Abrir(s.dir)
	ahora := time.Now().UTC()
	agregar := func(accion string) string {
		r, err := st.Agregar(recibo.Recibo{Quien: "token:claude", Accion: accion, Clase: "irreversible", Autoriza: false})
		if err != nil {
			t.Fatal(err)
		}
		return r.ID
	}
	aprobado, rechazado, pendiente, vencido := agregar("drop table a"), agregar("drop table b"), agregar("drop table c"), agregar("drop table d")
	_ = dec.Pedir(decision.Pedido{Recibo: aprobado, Accion: "drop table a", Via: decision.ViaHook, Cuando: ahora})
	_ = dec.Resolver(aprobado, decision.Aprobada, "user:diego", ahora)
	_ = dec.Pedir(decision.Pedido{Recibo: rechazado, Accion: "drop table b", Via: decision.ViaPregunta, Cuando: ahora})
	_ = dec.Resolver(rechazado, decision.Rechazada, "user:diego", ahora)
	_ = dec.Pedir(decision.Pedido{Recibo: pendiente, Accion: "drop table c", Via: decision.ViaHook, Cuando: ahora})
	_ = dec.Pedir(decision.Pedido{Recibo: vencido, Accion: "drop table d", Via: decision.ViaHook, Cuando: ahora.Add(-2 * decision.Vence)})

	mux := http.NewServeMux()
	s.Mount(mux)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", "/api/salaguerra", nil))
	var out struct {
		Evitado struct {
			Decisiones map[string]int `json:"decisiones"`
		} `json:"evitado"`
		Recibos struct {
			Ultimos []struct {
				ID       string `json:"id"`
				Decision string `json:"decision"`
			} `json:"ultimos"`
		} `json:"recibos"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	quiero := map[string]int{"aprobada": 1, "rechazada": 1, "pendiente": 1, "vencida": 1}
	for k, n := range quiero {
		if out.Evitado.Decisiones[k] != n {
			t.Fatalf("decisiones: %v", out.Evitado.Decisiones)
		}
	}
	porID := map[string]string{}
	for _, r := range out.Recibos.Ultimos {
		porID[r.ID] = r.Decision
	}
	if porID[aprobado] != "aprobada" || porID[rechazado] != "rechazada" || porID[pendiente] != "pendiente" || porID[vencido] != "vencida" {
		t.Fatalf("cada fila dice qué decidió la persona: %v", porID)
	}
}
