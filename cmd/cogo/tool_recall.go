package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/diegoparras/cogo/internal/core"
	"github.com/diegoparras/cogo/internal/history"
	"github.com/diegoparras/cogo/internal/suasion"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// registrarRecall registra el tool `recall`. Ver tools.go para lo que comparten.
func registrarRecall(s *mcp.Server, d *deps) {
	dir, loadVault, contradictions := d.dir, d.loadVault, d.contradictions
	mcp.AddTool(s, &mcp.Tool{
		Name:        "recall",
		Description: "Re-anchor after a context compaction, or catch up on another agent's work. With no argument it returns the load-bearing memory you must not lose (the user's mandate/red lines and the verified decisions and constraints) plus a cursor. Pass that cursor back as `since` and it returns ONLY what changed since then — the delta a second agent (or this one, post-compaction) needs to sync without re-reading the whole vault. Call it at the start of a session and again after any auto-compaction.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in recallIn) (*mcp.CallToolResult, any, error) {
		now := time.Now().UTC().Format(time.RFC3339Nano)
		var b strings.Builder
		if since := strings.TrimSpace(in.Since); since != "" {
			// Delta mode: only what moved since the caller's cursor.
			b.WriteString("# Recall — what changed\n")
			fmt.Fprintf(&b, "_Delta since %s._\n", since)
			if mandateChangedSince(dir, in.Project, since) {
				b.WriteString("\n⚠ The mandate changed since then — run a full `recall` (no `since`) to re-read the red lines.\n")
			}
			changes := history.ChangedSince(dir, since)
			if len(changes) == 0 {
				b.WriteString("\nNothing new.\n")
			} else {
				fmt.Fprintf(&b, "\n## %d note(s) changed\n", len(changes))
				for _, c := range changes {
					fmt.Fprintf(&b, "- **%s** [%s] — %s\n", c.ID, c.Color, firstLine(c.Claim))
				}
			}
			fmt.Fprintf(&b, "\n---\n_cursor: %s (pass as `since` next time)_\n", now)
			return textResult(b.String()), nil, nil
		}
		b.WriteString("# Recall — do not lose these\n")
		if in.Project != "" {
			fmt.Fprintf(&b, "_Project: %s._\n", in.Project)
		}
		if m := suasion.LoadMandateResolved(dir, in.Project); m != nil && (m.Goal != "" || len(m.RedLines) > 0) {
			b.WriteString("\n## Mandate (red lines)\n")
			if m.Goal != "" {
				fmt.Fprintf(&b, "- goal: %s\n", m.Goal)
			}
			for _, rl := range m.RedLines {
				fmt.Fprintf(&b, "- 🔴 %s\n", rl)
			}
		}
		if vault, err := loadVault(); err == nil {
			if c := core.BuildConstraints(vault, contradictions(), today(), in.Project); c != "" {
				b.WriteString("\n## Verified decisions & constraints\n")
				b.WriteString(c)
				b.WriteString("\n")
			}
		}
		if b.Len() < 40 {
			b.WriteString("\n_No mandate declared and no verified decisions/constraints yet._\n")
		}
		fmt.Fprintf(&b, "\n---\n_cursor: %s (pass as `since` next time to get only what changed)_\n", now)
		return textResult(b.String()), nil, nil
	})
}
