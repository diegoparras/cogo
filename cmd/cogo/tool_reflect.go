package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// registrarReflect registra el tool `reflect`. Ver tools.go para lo que comparten.
func registrarReflect(s *mcp.Server, d *deps) {
	dir := d.dir
	mcp.AddTool(s, &mcp.Tool{
		Name:        "reflect",
		Description: "After finishing a task, hand a short summary of what you did and verified. If a model is configured, COGO proposes graded notes worth capturing (claim + evidence + a check) so real findings persist instead of being re-derived next session — you still decide what to `capture`.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in reflectIn) (*mcp.CallToolResult, any, error) {
		if strings.TrimSpace(in.Summary) == "" {
			return errResult(fmt.Errorf("reflect needs a `summary` of what you did/learned")), nil, nil
		}
		p := guardProvider(dir)
		if !p.Available() {
			return textResult("No model configured (Ajustes → Modelo IA). `reflect` needs a model to score what's worth keeping; capture findings by hand with `capture`."), nil, nil
		}
		out, err := p.Complete(ctx, reflectPrompt(in.Summary))
		if err != nil {
			return errResult(fmt.Errorf("reflect model call failed: %w", err)), nil, nil
		}
		return textResult("# Capturables — revisá y guardá lo que valga con `capture`\n\n" + strings.TrimSpace(out)), nil, nil
	})
}
