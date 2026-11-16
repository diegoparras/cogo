package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/diegoparras/cogo/internal/auth"
	"github.com/diegoparras/cogo/internal/confidence"
	"github.com/diegoparras/cogo/internal/core"
	"github.com/diegoparras/cogo/internal/journal"
	"github.com/diegoparras/cogo/internal/motor"
	"github.com/diegoparras/cogo/internal/tokens"
	"github.com/diegoparras/cogo/internal/web"
)

func vaultConRunner(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	_ = os.MkdirAll(filepath.Join(dir, ".cogo"), 0o755)
	// `go version` existe en cualquier máquina que corra estos tests.
	cfg := "enabled: true\nchecks:\n  - id: go-version\n    command: [\"go\", \"version\"]\n    workdir: " + filepath.ToSlash(dir) + "\n    timeout: 1m\n"
	if err := os.WriteFile(filepath.Join(dir, ".cogo", "runner.yaml"), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	n := &core.Note{ID: "go-anda", Type: "command", Project: "p", LastVerified: today(),
		Evidence: []core.Evidence{{Kind: "command_output", Ref: "go version → go1.26"}},
		Check:    core.Check{Test: "go version sale 0", Status: "not_run"},
		Body:     "## Claim\nEl toolchain de Go está instalado y responde."}
	if err := core.WriteNoteFile(filepath.Join(dir, "go-anda.md"), n); err != nil {
		t.Fatal(err)
	}
	return dir
}

func estadoDe(t *testing.T, dir, id string) confidence.Estado {
	t.Helper()
	vault, err := core.LoadVault(dir)
	if err != nil {
		t.Fatal(err)
	}
	j, err := journal.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	evs, _ := j.All()
	final, _ := motor.Estados(vault, nil, today(), evs)
	return final[id]
}

// El circuito real: el check corre acá, el hosteado lo importa, y la nota
// llega a verified allá — sin que el hosteado haya ejecutado nada.
func TestRunLocalYSyncAlHosteado(t *testing.T) {
	soltarRegistro()
	t.Cleanup(soltarRegistro)
	local := vaultConRunner(t)
	if err := cmdRun([]string{"-vault", local, "go-anda", "go-version"}); err != nil {
		t.Fatalf("cogo run: %v", err)
	}
	if got := estadoDe(t, local, "go-anda"); got != confidence.Verified {
		t.Fatalf("local: esperaba verified, es %s", got)
	}

	// El hosteado: mismo vault de partida, sin runner, detrás del gate de
	// administración.
	remoto := t.TempDir()
	n, _ := core.ReadNoteFile(filepath.Join(local, "go-anda.md"))
	n.Check = core.Check{Test: "go version sale 0", Status: "not_run"}
	if err := core.WriteNoteFile(filepath.Join(remoto, "go-anda.md"), n); err != nil {
		t.Fatal(err)
	}
	s := web.New(remoto, today, tokens.Open(remoto))
	mux := http.NewServeMux()
	s.Mount(mux)
	// El gate: raíz con "Bearer raiz", token emitido con cualquier otro.
	gate := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		quien := "token:agente"
		if r.Header.Get("Authorization") == "Bearer raiz" {
			quien = "root"
		}
		enforceAdmin(mux).ServeHTTP(w, r.WithContext(auth.ConIdentidad(r.Context(), quien, false)))
	})
	ts := httptest.NewServer(gate)
	defer ts.Close()

	// Un token emitido no puede importar ejecuciones: sería fabricar verdes.
	soltarRegistro()
	if err := cmdSync([]string{"-vault", local, "-url", ts.URL + "/mcp", "-token", "cogo_agente", "-todo"}); err == nil || !strings.Contains(err.Error(), "403") {
		t.Fatalf("un token emitido tiene que recibir 403: %v", err)
	}
	if err := cmdSync([]string{"-vault", local, "-url", ts.URL, "-token", "raiz", "-todo"}); err != nil {
		t.Fatalf("cogo sync: %v", err)
	}
	if got := estadoDe(t, remoto, "go-anda"); got != confidence.Verified {
		t.Fatalf("hosteado: esperaba verified, es %s", got)
	}
	nr, _ := core.ReadNoteFile(filepath.Join(remoto, "go-anda.md"))
	if nr.Check.Status != "passed" || nr.Check.Attested != core.AttestExecuted || nr.Check.AttestedBy != journal.EmisorEjecucion {
		t.Fatalf("la nota hosteada tiene que quedar como la dejó el runner: %+v", nr.Check)
	}
	// El cursor avanzó: un segundo sync no tiene nada que empujar.
	if _, err := os.Stat(filepath.Join(local, ".cogo", "sync.json")); err != nil {
		t.Fatal("el cursor tiene que quedar escrito")
	}
	if err := cmdSync([]string{"-vault", local, "-url", ts.URL, "-token", "raiz"}); err != nil {
		t.Fatalf("segundo sync: %v", err)
	}
}
