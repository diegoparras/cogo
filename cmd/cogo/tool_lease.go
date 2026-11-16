package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/diegoparras/cogo/internal/auth"
	"github.com/diegoparras/cogo/internal/lease"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// registrarLease registra el tool `lease`. Ver tools.go para lo que comparten.
func registrarLease(s *mcp.Server, d *deps) {
	dir := d.dir
	mcp.AddTool(s, &mcp.Tool{
		Name:        "lease",
		Description: "Coordinate with other agents on a shared vault: take a time-bounded lease on a resource before a risky, non-idempotent job (a migration, a deploy, a bulk edit). `acquire` grants it unless someone else holds it (you're told who, and until when); `release` frees it; `list` shows what's held. Leases expire on their own, so a crashed holder never wedges the vault. Advisory like git — it makes the collision visible, it doesn't physically block.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in leaseIn) (*mcp.CallToolResult, any, error) {
		ls := lease.Open(dir)
		holder := strings.TrimSpace(in.Holder)
		if holder == "" {
			if c := auth.CallerCtx(ctx); c != "" {
				holder = c
			} else {
				holder = "local"
			}
		}
		now := time.Now()
		switch strings.TrimSpace(in.Action) {
		case "acquire":
			ttl := time.Duration(in.TTLSeconds) * time.Second
			if in.TTLSeconds <= 0 {
				ttl = 15 * time.Minute
			}
			l, err := ls.Acquire(in.Name, holder, in.Note, ttl, now)
			if err != nil {
				return textResult("⛔ " + err.Error() + "\nWait for it to free or expire, or coordinate with the holder before proceeding."), nil, nil
			}
			return textResult(fmt.Sprintf("✓ Lease %q acquired by %q until %s. Release it when done.", l.Name, l.Holder, l.Expires)), nil, nil
		case "release":
			if ls.Release(in.Name, holder) {
				return textResult(fmt.Sprintf("Released %q.", in.Name)), nil, nil
			}
			return textResult(fmt.Sprintf("Nothing to release: %q isn't held by %q.", in.Name, holder)), nil, nil
		case "list", "":
			held := ls.List(now)
			if len(held) == 0 {
				return textResult("No leases held."), nil, nil
			}
			var b strings.Builder
			b.WriteString("# Leases held\n")
			for _, l := range held {
				fmt.Fprintf(&b, "- **%s** — %s (until %s)", l.Name, l.Holder, l.Expires)
				if l.Note != "" {
					fmt.Fprintf(&b, " · %s", l.Note)
				}
				b.WriteString("\n")
			}
			return textResult(b.String()), nil, nil
		default:
			return errResult(fmt.Errorf("action must be acquire, release or list")), nil, nil
		}
	})
}
