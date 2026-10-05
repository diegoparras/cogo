package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/diegoparras/cogo/internal/contra"
	"github.com/diegoparras/cogo/internal/core"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// registrarOpen registra el tool `open`. Ver tools.go para lo que comparten.
func registrarOpen(s *mcp.Server, d *deps) {
	dir, loadVault := d.dir, d.loadVault
	mcp.AddTool(s, &mcp.Tool{
		Name:        "open",
		Description: "Return one note by id, with its freshly computed color.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in openIn) (*mcp.CallToolResult, any, error) {
		vault, err := loadVault()
		if err != nil {
			return errResult(err), nil, nil
		}
		Consultadas(in.ID) // abrir una nota por su id también la despierta
		n, ok := vault[in.ID]
		if !ok {
			return errResult(fmt.Errorf("no note with id %q", in.ID)), nil, nil
		}
		cstore := contra.Open(dir)
		n.Apply(core.Evaluate(n, vault, cstore.OpenNoteSet(), today()))
		md, err := core.MarshalNote(n)
		if err != nil {
			return errResult(err), nil, nil
		}
		out := string(md)
		// The trace behind a red-by-contradiction verdict: name the clashing
		// note(s) and why, so the agent can resolve instead of just seeing "red".
		if cs := cstore.ForNote(in.ID); len(cs) > 0 {
			var b strings.Builder
			b.WriteString(out)
			b.WriteString("\n## ⚠ Contradicciones abiertas\n\nEsta nota es roja porque choca con otra(s). Resolvé el conflicto antes de apoyarte en ella:\n\n")
			for _, c := range cs {
				fmt.Fprintf(&b, "- contradice `%s`", c.Other)
				if c.Reason != "" {
					fmt.Fprintf(&b, " — %s", c.Reason)
				}
				b.WriteByte('\n')
			}
			out = b.String()
		}
		return textResult(out), nil, nil
	})
}
