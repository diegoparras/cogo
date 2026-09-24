package motor

import (
	"testing"

	"github.com/diegoparras/cogo/internal/confidence"
	"github.com/diegoparras/cogo/internal/core"
)

// Una nota que el registro no conoce no está "en cuarentena": está donde
// arranca toda nota. El cero del tipo Estado es quarantined, y sin este
// default una captura reciente aparecía como excluida a propósito.
func TestSinEventosArrancaEnElEstadoInicial(t *testing.T) {
	hoy := core.NewDate(2026, 9, 23)
	n := &core.Note{ID: "nueva", Type: "bug", LastVerified: hoy,
		Evidence: []core.Evidence{{Kind: "command_output", Ref: "x.log:1"}},
		Check:    core.Check{Test: "go test ./..."}}
	final, local := Estados(map[string]*core.Note{"nueva": n}, nil, hoy, nil)
	if local["nueva"] == confidence.Quarantined || final["nueva"] == confidence.Quarantined {
		t.Fatalf("sin eventos no puede ser cuarentena: local=%s final=%s", local["nueva"], final["nueva"])
	}
	if final["nueva"] != confidence.Inicial {
		t.Fatalf("esperaba el estado inicial (%s), dio %s", confidence.Inicial, final["nueva"])
	}
}
