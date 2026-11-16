package main

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/diegoparras/cogo/internal/auth"
	"github.com/diegoparras/cogo/internal/core"
	"github.com/diegoparras/cogo/internal/scrub"
	"github.com/diegoparras/cogo/internal/secretscan"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// registrarCapture registra el tool `capture`. Ver tools.go para lo que comparten.
func registrarCapture(s *mcp.Server, d *deps) {
	dir, scrubber, loadVault, contradictions := d.dir, d.scrubber, d.loadVault, d.contradictions
	mcp.AddTool(s, &mcp.Tool{
		Name:        "capture",
		Description: "Record a finding as a note. Always include evidence and a minimal check. Never set the color — COGO computes it.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in captureIn) (*mcp.CallToolResult, any, error) {
		if in.Type == "" || strings.TrimSpace(in.Body) == "" {
			return errResult(fmt.Errorf("capture needs at least a type and a body")), nil, nil
		}
		id := in.ID
		if id == "" {
			id = core.DeriveID(in.Project, in.Body)
		}
		if err := core.ValidarID(id); err != nil {
			return errResult(err), nil, nil
		}
		note := &core.Note{
			ID: id, Type: in.Type, Project: in.Project, Body: strings.TrimSpace(in.Body),
			LastVerified: today(),
			Check:        core.Check{Test: in.CheckTest, Status: "not_run"},
			DependsOn:    in.DependsOn, Supersedes: in.Supersedes, CausedBy: in.CausedBy,
			Scope: in.Scope,
			// El origen se normaliza acá y no se toma crudo: un valor que no
			// existe cae en `agent`, que es la suposición conservadora — quien
			// está capturando ES un agente. Un typo no puede ser una forma de
			// que una propuesta pase por decisión.
			Origin: string(core.NormalizarOrigen(in.Origin)),
		}
		// Y si no declaró nada en una normativa, también es del agente. Dejarlo
		// vacío lo mostraría como "no consta", que es la marca reservada para las
		// notas escritas antes de que este campo existiera: usarla acá borraría
		// la diferencia entre no haber podido declarar y no haber querido.
		if note.Origin == "" && core.EsNormativa(note) {
			note.Origin = string(core.OrigenAgente)
		}
		for _, e := range in.Evidence {
			note.Evidence = append(note.Evidence, core.Evidence{Kind: e.Kind, Ref: e.Ref})
		}
		if resumen, hay := secretscan.Nota(note); hay {
			return textResult("⛔ Not stored — possible secret(s) detected: " + resumen +
				". A note is memory that other agents will read: clean the content first."), nil, nil
		}
		if err := scrub.Note(ctx, scrubber, note); err != nil {
			return errResult(fmt.Errorf("scrub failed: %w", err)), nil, nil
		}

		vault, err := loadVault()
		if err != nil {
			return errResult(err), nil, nil
		}
		cx := contradictions()
		existing, had := vault[id]
		if had {
			if ev := core.Evaluate(existing, vault, cx, today()); ev.Color == core.Green {
				return errResult(fmt.Errorf("note %q exists and is green; not overwritten — verify it or use a new id", id)), nil, nil
			}
		}
		// Author = who captured it (the authenticated caller); preserve the original
		// creator across edits.
		if had && existing.Author != "" {
			note.Author = existing.Author
		} else {
			note.Author = auth.CallerCtx(ctx)
		}
		vault[id] = note
		core.ResolveEvidence(vault, core.LoadEvidenceRoots(dir)) // resolve the new note's own refs
		v := core.Evaluate(note, vault, cx, today())
		note.Apply(v)

		path := filepath.Join(dir, id+".md")
		if had && existing.Path != "" {
			path = existing.Path
		}
		if err := core.WriteNoteFile(path, note); err != nil {
			return errResult(err), nil, nil
		}
		_ = regenIndex(dir, vault)
		_ = appendLog(dir, fmt.Sprintf("capture %s %s — %s", id, v.Color, v.Reason))
		return textResult(fmt.Sprintf("captured %q as %s — %s", id, v.Color, v.Reason)), nil, nil
	})
}
