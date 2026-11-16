package web

import (
	"encoding/json"
	"path/filepath"
	"sync"
	"testing"

	"github.com/diegoparras/cogo/internal/core"
)

// El botón de verificar del visor contesta con el color que deja la
// verificación, no con el de antes.
//
// En producción el color sale del registro de eventos, y el registro se entera
// de la verificación por el hook de escritura, o sea recién cuando la nota se
// escribe. El motor de acá reproduce esa dependencia y nada más: una nota está
// verde para él sólo si ya vio escrita su verificación.
func TestVerifyContestaElColorDeDespues(t *testing.T) {
	s := testServer(t)

	var mu sync.Mutex
	escrito := map[string]string{} // id → status del check que vio el hook
	core.SetWriteHook(func(_ string, _, n *core.Note) {
		mu.Lock()
		escrito[n.ID] = n.Check.Status
		mu.Unlock()
	})
	core.SetMotor(func(vault map[string]*core.Note, contras map[string]bool, hoy core.Date) map[string]core.Verdict {
		out := core.EvaluateVaultCore(vault, contras, hoy)
		mu.Lock()
		defer mu.Unlock()
		for id, v := range out {
			if escrito[id] != "passed" {
				out[id] = core.Verdict{Color: core.Yellow, Reason: "el registro todavía no vio la verificación", StaleAt: v.StaleAt}
			}
		}
		return out
	})
	t.Cleanup(func() { core.SetMotor(nil); core.SetWriteHook(nil) })

	n := &core.Note{
		ID: "build-compila", Type: "runbook", Project: "x",
		LastVerified: core.MustDate("2026-06-29"),
		Evidence:     []core.Evidence{{Kind: "command_output", Ref: "go build ./... → ok"}},
		Check:        core.Check{Test: "go build ./...", Status: "not_run"},
		Body:         "## Claim\n`go build ./...` compila sin errores.",
	}
	if err := core.WriteNoteFile(filepath.Join(s.dir, "build-compila.md"), n); err != nil {
		t.Fatal(err)
	}

	rec := call(s.handleVerify, "POST", "/api/verify?id=build-compila", nil)
	var r map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &r); err != nil {
		t.Fatalf("verify respondió %d: %s", rec.Code, rec.Body.String())
	}

	vault, err := core.LoadVault(s.dir)
	if err != nil {
		t.Fatal(err)
	}
	ahora := core.Evaluate(vault["build-compila"], vault, nil, s.today())
	if ahora.Color != core.Green {
		t.Fatalf("precondición: verificada, la nota tiene que estar verde; está %s (%s)", ahora.Color, ahora.Reason)
	}
	if r["color"] != "green" {
		t.Errorf("el visor contestó %v (%v), y la nota ya está verde", r["color"], r["reason"])
	}
	if got := vault["build-compila"].Confidence; got != "green" {
		t.Errorf("el archivo quedó con confidence %q después de verificar", got)
	}
}
