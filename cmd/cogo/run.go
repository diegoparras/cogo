package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/diegoparras/cogo/internal/atomico"
	"github.com/diegoparras/cogo/internal/core"
	"github.com/diegoparras/cogo/internal/journal"
	"github.com/diegoparras/cogo/internal/runner"
)

// El runner donde vive el código.
//
// En el despliegue real nada llega a `verified`: la imagen es scratch. El
// check se corre acá, en la máquina que tiene el repo, contra el vault local:
//
//	cogo run <nota> <check>      ejecuta un check de .cogo/runner.yaml y lo asienta
//	cogo sync -url ... -token ... empuja las ejecuciones al COGO hosteado
//
// El hosteado recibe solo eventos del runner, solo de un administrador, y los
// encadena con su propio hash. Es el único camino a `verified` en producción,
// y sigue sin shell adentro.

func cmdRun(args []string) error {
	fs := flag.NewFlagSet("run", flag.ExitOnError)
	dir := vaultFlag(fs)
	reanchor := fs.Bool("reanchor", false, "confirmá que comprobaste la afirmación contra el contenido ACTUAL de la evidencia que cambió")
	_ = fs.Parse(args)
	conVault(dir)
	rest := fs.Args()
	if len(rest) != 2 {
		return fmt.Errorf("uso: cogo run <nota> <check>")
	}
	noteID, checkID := rest[0], rest[1]

	vault, err := core.LoadVault(*dir)
	if err != nil {
		return err
	}
	n, ok := vault[noteID]
	if !ok {
		return fmt.Errorf("no note with id %q", noteID)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(pars.Entero("runner.timeout_maximo"))*time.Minute)
	defer cancel()
	res, err := ejecutarCheck(ctx, *dir, noteID, checkID)
	if err != nil {
		return err
	}
	if err := aplicarEjecucion(n, res, core.LoadEvidenceRoots(*dir), *reanchor); err != nil {
		return err
	}
	v := core.Evaluate(n, vault, nil, today())
	n.Apply(v)
	path := n.Path
	if path == "" {
		path = filepath.Join(*dir, noteID+".md")
	}
	if err := core.WriteNoteFile(path, n); err != nil {
		return err
	}
	_ = regenIndex(*dir, vault)
	_ = appendLog(*dir, fmt.Sprintf("run %s %s exit=%d %s", noteID, checkID, res.ExitCode, v.Color))
	fmt.Printf("%s %s — check %q exit %d in %s\n", colorTag(v.Color), noteID, res.CheckID, res.ExitCode, res.Duracion.Round(time.Millisecond))
	if !res.OK() && strings.TrimSpace(res.Stderr) != "" {
		fmt.Println(strings.TrimSpace(res.Stderr))
	}
	return nil
}

// aplicarEjecucion deja en la nota lo que dijo el runner. Es lo mismo que
// hace `verify(check:)` por MCP: un pase pasa por Verificar como ejecutado; una
// falla se asienta como ejecutada y fallida, sin pasar por Verificar, que solo
// sabe marcar pases.
func aplicarEjecucion(n *core.Note, res runner.Resultado, roots core.EvidenceRoots, reanchor bool) error {
	if res.OK() {
		return core.Verificar(n, roots, today(), core.Verificacion{
			Por: journal.EmisorEjecucion, Reanclar: reanchor, Ejecutado: true,
		})
	}
	if derivadas := core.DriftedRefs(n); len(derivadas) > 0 && !reanchor {
		return &core.ErrDeriva{Refs: derivadas}
	}
	n.Check.Status, n.Check.Attested, n.Check.AttestedBy = "failed", core.AttestExecuted, journal.EmisorEjecucion
	n.LastVerified = today()
	return nil
}

// ── sync ────────────────────────────────────────────────────────────────────

type cursorSync struct {
	UltimoSeq uint64 `json:"ultimo_seq"`
}

func rutaCursor(dir string) string { return filepath.Join(dir, ".cogo", "sync.json") }

func cmdSync(args []string) error {
	fs := flag.NewFlagSet("sync", flag.ExitOnError)
	dir := vaultFlag(fs)
	url := fs.String("url", os.Getenv("COGO_URL"), "COGO hosteado (la base, o su /mcp)")
	token := fs.String("token", os.Getenv("COGO_TOKEN"), "Bearer de ADMINISTRADOR (la raíz): un token emitido no puede importar ejecuciones")
	todo := fs.Bool("todo", false, "ignorar el cursor y empujar todas las ejecuciones (el hosteado deduplica)")
	_ = fs.Parse(args)
	conVault(dir)
	if strings.TrimSpace(*url) == "" {
		return fmt.Errorf("sync necesita -url (o COGO_URL)")
	}
	j, err := journalDe(*dir)
	if err != nil {
		return err
	}
	var cur cursorSync
	if !*todo {
		if b, err := os.ReadFile(rutaCursor(*dir)); err == nil {
			_ = json.Unmarshal(b, &cur)
		}
	}
	evs, err := j.Ejecuciones(cur.UltimoSeq)
	if err != nil {
		return err
	}
	if len(evs) == 0 {
		fmt.Println("nada que empujar: no hay ejecuciones nuevas desde el último sync")
		return nil
	}
	n, err := empujarEjecuciones(context.Background(), *url, *token, evs)
	if err != nil {
		return err
	}
	cur.UltimoSeq = evs[len(evs)-1].Seq
	b, _ := json.Marshal(cur)
	_ = atomico.Escribir(rutaCursor(*dir), b, 0o644)
	fmt.Printf("empujadas %d ejecuciones (%d nuevas en el hosteado); cursor en el evento %d\n", len(evs), n, cur.UltimoSeq)
	return nil
}

// empujarEjecuciones hace el POST. Es HTTP plano y no MCP porque importar es
// una operación de administración, no un tool de agente.
func empujarEjecuciones(ctx context.Context, url, token string, evs []journal.Event) (int, error) {
	base := strings.TrimSuffix(strings.TrimSuffix(strings.TrimSpace(url), "/"), "/mcp")
	cuerpo, err := json.Marshal(map[string]any{"eventos": evs})
	if err != nil {
		return 0, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/api/journal/importar", bytes.NewReader(cuerpo))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	res, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		return 0, err
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if res.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("%s: %d %s", base, res.StatusCode, strings.TrimSpace(string(b)))
	}
	var out struct {
		Importados int `json:"importados"`
	}
	_ = json.Unmarshal(b, &out)
	return out.Importados, nil
}
