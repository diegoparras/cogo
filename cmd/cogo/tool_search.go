package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/diegoparras/cogo/internal/core"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// registrarSearch registra el tool `search`. Ver tools.go para lo que comparten.
func registrarSearch(s *mcp.Server, d *deps) {
	dir, loadVault, contradictions := d.dir, d.loadVault, d.contradictions
	mcp.AddTool(s, &mcp.Tool{
		Name:        "search",
		Description: "List notes matching a query: id, color and a one-line summary (no bodies). Ranks by MEANING (embeddings) when an embedding model is configured, else keyword BM25.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in searchIn) (*mcp.CallToolResult, any, error) {
		vault, err := loadVault()
		if err != nil {
			return errResult(err), nil, nil
		}
		// Optional semantic ranking; on any failure it falls through to keyword.
		if in.Query != "" {
			if ep := embedProvider(dir); ep != nil && ep.EmbedAvailable() {
				if out, ok := semanticSearch(ctx, dir, vault, contradictions(), in, ep); ok {
					return textResult(out), nil, nil
				}
			}
		}
		hits := core.Search(vault, contradictions(), in.Query, in.Project, today(), in.Limit, in.IncludeArchived)
		if len(hits) == 0 {
			return textResult("no matching notes"), nil, nil
		}
		var b strings.Builder
		for _, h := range hits {
			fmt.Fprintf(&b, "- %s `%s` — %s", h.Color, h.ID, h.Summary)
			if h.State != "" {
				fmt.Fprintf(&b, " [%s]", h.State)
			}
			if h.Latent {
				// Aparece en la búsqueda pero NO en el pack. Decirlo evita que un
				// agente la dé por parte de su contexto: si la quiere usar tiene
				// que abrirla, y abrirla es exactamente lo que la devuelve.
				b.WriteString(" [dormant — not in your pack; open it to bring it back]")
			}
			b.WriteString("\n")
		}
		return textResult(b.String()), nil, nil
	})
}
