package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/diegoparras/cogo/internal/llm"
	"github.com/diegoparras/cogo/internal/suasion"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// registrarGuard registra el tool `guard`. Ver tools.go para lo que comparten.
func registrarGuard(s *mcp.Server, d *deps) {
	dir := d.dir
	mcp.AddTool(s, &mcp.Tool{
		Name: "guard",
		Description: "Radiography a model turn for manipulation pressure: names influence/coercion " +
			"tactics with quoted evidence, checks denials against the transcript (receipts), and " +
			"measures drift against the user's declared red lines. Deterministic. It informs the " +
			"human and never censors the model.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in guardIn) (*mcp.CallToolResult, any, error) {
		eng, err := suasion.Default()
		if err != nil {
			return errResult(err), nil, nil
		}
		if strings.TrimSpace(in.Turn) == "" {
			return errResult(fmt.Errorf("guard needs the model turn to analyze")), nil, nil
		}
		var transcript []suasion.Turn
		for _, t := range in.Transcript {
			transcript = append(transcript, suasion.Turn{Role: t.Role, Text: t.Text})
		}
		var mandate *suasion.Mandate
		if in.Goal != "" || len(in.RedLines) > 0 {
			mandate = &suasion.Mandate{Goal: in.Goal, RedLines: in.RedLines}
		} else {
			// The call declared nothing: fall back to the mandate persisted in
			// the vault (shared with the visor's Guard tab).
			mandate = suasion.LoadMandate(suasion.MandatePath(dir))
		}
		report := eng.AnalyzeWith(ctx, in.Turn, transcript, mandate, suasion.Opts{
			Tier1:    guardProvider(dir),
			Tier2:    llm.StrongFromEnv(guardProvider(dir)),
			Steelman: in.Steelman,
		})
		return textResult(eng.Render(report)), nil, nil
	})
}
