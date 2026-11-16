package runner

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/diegoparras/cogo/internal/confidence"
	"github.com/diegoparras/cogo/internal/journal"
)

// I1 de punta a punta: solo una ejecución real lleva una nota a `verified`, y el
// rastro de qué se corrió queda en el journal.
func TestSoloLaEjecucionRealLlegaAVerified(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("los comandos del test son de shell POSIX")
	}
	vault := t.TempDir()
	wd := t.TempDir()
	if err := os.MkdirAll(filepath.Join(vault, ".cogo"), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := "enabled: true\nchecks:\n" +
		"  - id: pasa\n    command: [\"sh\", \"-c\", \"exit 0\"]\n    workdir: " + wd + "\n" +
		"  - id: falla\n    command: [\"sh\", \"-c\", \"exit 1\"]\n    workdir: " + wd + "\n"
	if err := os.WriteFile(filepath.Join(vault, ".cogo", "runner.yaml"), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}

	c, err := Cargar(vault)
	if err != nil {
		t.Fatal(err)
	}
	j, err := journal.Open(vault)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := j.Append(journal.Event{NoteID: "n", Kind: "CheckDeclared", Emitter: "agent"}); err != nil {
		t.Fatal(err)
	}
	if _, err := Verificar(context.Background(), j, c, "n", "pasa"); err != nil {
		t.Fatal(err)
	}
	evs, _ := j.All()
	if got := journal.EstadoDe(evs, "n"); got != confidence.Verified {
		t.Errorf("tras ejecutar un check que pasa, la nota quedó en %s", got)
	}

	// y un check que falla la refuta
	if _, err := j.Append(journal.Event{NoteID: "m", Kind: "CheckDeclared", Emitter: "agent"}); err != nil {
		t.Fatal(err)
	}
	if _, err := Verificar(context.Background(), j, c, "m", "falla"); err != nil {
		t.Fatal(err)
	}
	evs, _ = j.All()
	if got := journal.EstadoDe(evs, "m"); got != confidence.Refuted {
		t.Errorf("tras ejecutar un check que falla, la nota quedó en %s", got)
	}

	// el rastro tiene que decir qué se corrió y con qué resultado
	var hay bool
	for _, e := range evs {
		if e.Kind == "CheckExecuted" && e.NoteID == "n" {
			hay = true
			if e.Emitter != EmisorRunner {
				t.Errorf("el evento de ejecución lo emitió %q", e.Emitter)
			}
			if !strings.Contains(string(e.Payload), "exit_code") {
				t.Errorf("el rastro no guarda el código de salida: %s", e.Payload)
			}
		}
	}
	if !hay {
		t.Error("no quedó rastro de la ejecución en el journal")
	}
}

// I1 defendido en runtime, no solo por convención: el journal RECHAZA el emisor
// reservado por la puerta común. Grepear el árbol solo detecta lo que ya está
// escrito; esto detiene lo que se escriba mañana.
func TestElJournalRechazaElEmisorReservadoPorLaPuertaComun(t *testing.T) {
	j, err := journal.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	_, err = j.Append(journal.Event{
		NoteID: "n", Kind: "CheckExecuted", Emitter: EmisorRunner, Guard: "ejecucion_ok",
	})
	if err == nil {
		t.Fatal("se aceptó el emisor reservado por Append: cualquiera podría fabricar una verificación")
	}
	if !errors.Is(err, journal.ErrEmisorReservado) {
		t.Errorf("rechazó, pero con otro error: %v", err)
	}
	if evs, _ := j.All(); len(evs) != 0 {
		t.Errorf("el evento rechazado igual se escribió: %d eventos", len(evs))
	}
}

// La otra mitad: fuera del runner nadie llama a la puerta especial. Un grep de
// AppendEjecucion tiene que dar exactamente los lugares que producen
// verificaciones — ese es el punto de que tenga nombre propio en vez de ser una
// cadena esparcida.
func TestSoloElRunnerUsaLaPuertaDeEjecucion(t *testing.T) {
	sep := string(filepath.Separator)
	var infractores []string
	err := filepath.Walk(filepath.Join("..", ".."), func(p string, fi os.FileInfo, err error) error {
		if err != nil || fi.IsDir() || !strings.HasSuffix(p, ".go") {
			return nil
		}
		if strings.Contains(p, sep+"runner"+sep) || strings.Contains(p, sep+"journal"+sep) {
			return nil // el dueño, y el paquete que la define
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return nil
		}
		for _, ln := range strings.Split(string(b), "\n") {
			if strings.Contains(ln, "AppendEjecucion") {
				infractores = append(infractores, p+": "+strings.TrimSpace(ln))
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, i := range infractores {
		t.Errorf("alguien fuera del runner produce verificaciones:\n  %s", i)
	}
}
