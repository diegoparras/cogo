package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/diegoparras/cogo/internal/xray"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// registrarXray registra el tool `xray`. Ver tools.go para lo que comparten.
func registrarXray(s *mcp.Server, d *deps) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "xray",
		Description: "Radiography an answer for VERACITY (the twin of guard's manipulation check): per " +
			"claim, expose the gap between how strongly it is asserted and how much grounding it declares. " +
			"Deterministic — no model. Flags claims asserted hard with no basis, opinions dressed as facts, " +
			"and un-sourced factual claims. It never says 'true'; green needs an executed test (Phase 2).",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in xrayIn) (*mcp.CallToolResult, any, error) {
		if strings.TrimSpace(in.Answer) == "" {
			return errResult(fmt.Errorf("xray needs the answer text to analyze")), nil, nil
		}
		rep := xray.AnalyzeCon(ctx, in.Answer, juezParaXray())
		var b strings.Builder
		icon := map[string]string{"red": "🔴", "yellow": "🟡", "ungraded": "⚪"}
		fmt.Fprintf(&b, "Radiografía de veracidad — %s\n%s\n\n", icon[rep.Overall]+" "+rep.Overall, rep.Summary)
		for _, c := range rep.Claims {
			fmt.Fprintf(&b, "%s %q\n   %s (compromiso: %s · evidencia: %s)\n", icon[c.Color], c.Text, c.Reason, c.Commitment, c.Evidence)
		}
		return textResult(b.String()), nil, nil
	})
}
