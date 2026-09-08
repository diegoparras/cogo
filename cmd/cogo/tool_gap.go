package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/diegoparras/cogo/internal/auth"
	"github.com/diegoparras/cogo/internal/core"
	"github.com/diegoparras/cogo/internal/scrub"
	"github.com/diegoparras/cogo/internal/secretscan"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// registrarGap registra el tool `gap`. Ver tools.go para lo que comparten.
func registrarGap(s *mcp.Server, d *deps) {
	dir, scrubber, loadVault := d.dir, d.scrubber, d.loadVault
	mcp.AddTool(s, &mcp.Tool{
		Name:        "gap",
		Description: "Record something the project does NOT know, as an open question. Use it when you hit a decision you cannot make because a fact is missing, when you had to assume something to keep going, or when an investigation came back inconclusive. This is NOT a low-confidence note: a note claims something and might be wrong, a gap claims nothing and says so. Gaps carry no color and never enter the confidence graph; the pack returns them in their own section, ordered by how many decisions each one blocks. If you find yourself about to guess, record the gap instead.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in gapIn) (*mcp.CallToolResult, any, error) {
		q := strings.TrimSpace(in.Question)
		if q == "" {
			return errResult(fmt.Errorf("a gap needs a question: what is it that nobody knows?")), nil, nil
		}
		id := in.ID
		if id == "" {
			id = core.DeriveID(in.Project, q)
		}
		ruta, err := core.RutaDeNota(dir, id)
		if err != nil {
			return errResult(err), nil, nil
		}
		body := strings.TrimSpace(in.Body)
		if body == "" {
			body = "## Claim\n" + q
		}
		note := &core.Note{
			ID: id, Type: core.TipoBrecha, Project: in.Project, Body: body,
			LastVerified: today(), Question: q, Blocks: in.Blocks,
			CostToResolve: in.Cost, Attempted: in.Attempted,
			Author: auth.CallerCtx(ctx),
		}
		if resumen, hay := secretscan.Nota(note); hay {
			return textResult("⛔ Not stored — possible secret(s) detected: " + resumen + "."), nil, nil
		}
		if err := scrub.Note(ctx, scrubber, note); err != nil {
			return errResult(fmt.Errorf("scrub failed: %w", err)), nil, nil
		}
		vault, err := loadVault()
		if err != nil {
			return errResult(err), nil, nil
		}
		vault[id] = note
		if err := core.WriteNoteFile(ruta, note); err != nil {
			return errResult(err), nil, nil
		}
		_ = regenIndex(dir, vault)
		_ = appendLog(dir, fmt.Sprintf("gap %s", id))
		msg := fmt.Sprintf("gap %s registrada: %s", id, q)
		if k := len(in.Blocks); k > 0 {
			msg += fmt.Sprintf(" — traba %d decisión(es)", k)
		}
		return textResult(msg), nil, nil
	})
}
