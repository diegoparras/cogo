package main

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/diegoparras/cogo/internal/auth"
	"github.com/diegoparras/cogo/internal/core"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// registrarVerify registra el tool `verify`. Ver tools.go para lo que comparten.
func registrarVerify(s *mcp.Server, d *deps) {
	dir, loadVault, contradictions := d.dir, d.loadVault, d.contradictions
	mcp.AddTool(s, &mcp.Tool{
		Name:        "verify",
		Description: "Record that a note's check passes, as of today, and re-color it. Without `check`, this is a DECLARATION, not an execution: it is stored as such, with your identity, and the note is marked `attested: declared`. With `check: <id>`, COGO runs that check — one the vault owner declared in .cogo/runner.yaml — and records the real exit code; that is the only path to `attested: executed` and to `verified`. If the cited evidence CHANGED since the note was last verified, this refuses — re-verifying must not be a way to make a drift warning disappear. Pass reanchor:true only if you actually re-checked the claim against the current content.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in verifyIn) (*mcp.CallToolResult, any, error) {
		vault, err := loadVault()
		if err != nil {
			return errResult(err), nil, nil
		}
		n, ok := vault[in.ID]
		if !ok {
			return errResult(fmt.Errorf("no note with id %q", in.ID)), nil, nil
		}
		var corrida string
		if strings.TrimSpace(in.Check) != "" {
			// El agente elige QUÉ check declarado corre; el comando lo puso el
			// dueño del vault. Los eventos de la ejecución los escribe el runner
			// por su puerta reservada, y son el único camino a `verified`.
			res, err := ejecutarCheck(ctx, dir, in.ID, in.Check)
			if err != nil {
				return errResult(err), nil, nil
			}
			corrida = fmt.Sprintf(" — check %q exit %d in %s", res.CheckID, res.ExitCode, res.Duracion.Round(time.Millisecond))
			// Lo que dijo el runner se asienta igual que desde la CLI (cogo run).
			if err := aplicarEjecucion(n, res, core.LoadEvidenceRoots(dir), in.Reanchor); err != nil {
				return errResult(err), nil, nil
			}
		} else {
			// Sin check, es una declaración. Se declara aunque no haya nadie
			// identificado: stdio y un COGO local sin autenticación no tienen
			// identidad, y core.Verificar lo asienta como "sin identificar".
			// Antes esto se decidía comparando contra una Verificacion vacía, y
			// sin identidad ni reanchor la Verificacion ES vacía: verify no
			// declaraba nada y contestaba como si lo hubiera hecho.
			ver := core.Verificacion{Por: auth.CallerCtx(ctx), Reanclar: in.Reanchor}
			if err := core.Verificar(n, core.LoadEvidenceRoots(dir), today(), ver); err != nil {
				return errResult(err), nil, nil
			}
		}
		path := n.Path
		if path == "" {
			path = filepath.Join(dir, in.ID+".md")
		}
		v, err := core.EscribirConColor(path, n, vault, contradictions(), today())
		if err != nil {
			return errResult(err), nil, nil
		}
		_ = regenIndex(dir, vault)
		_ = appendLog(dir, fmt.Sprintf("verify %s %s%s", in.ID, v.Color, corrida))
		return textResult(fmt.Sprintf("%s %s — %s%s", v.Color, in.ID, v.Reason, corrida)), nil, nil
	})
}
