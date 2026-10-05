package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/diegoparras/cogo/internal/core"
	"github.com/diegoparras/cogo/internal/secretscan"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// registrarStash registra el tool `stash`. Ver tools.go para lo que comparten.
func registrarStash(s *mcp.Server, d *deps) {
	store := d.store
	mcp.AddTool(s, &mcp.Tool{
		Name:        "stash",
		Description: "Store an artifact by its content hash and get back a ref to cite as evidence: `artifact://<sha256>`. Use it for the things that prove a claim but rot away today — a failed command's full output, a config dump, a CSV, a small file. COGO keeps the bytes, so `verify` can later RECOMPUTE (the object exists and its hash still matches) instead of trusting a reference that has since disappeared. A secret guard runs FIRST: if the content looks like it holds credentials, stash REFUSES by default — nothing secret should become an immutable hash — so clean it, or pass redact:true to store a masked copy. `content` for text, `content_base64` for binary. Do NOT stash long documentation; that belongs in the repo.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in stashIn) (*mcp.CallToolResult, any, error) {
		data, err := stashBytes(in)
		if err != nil {
			return errResult(fmt.Errorf("stash: bad content_base64: %w", err)), nil, nil
		}
		if len(data) == 0 {
			return errResult(fmt.Errorf("stash needs `content` or `content_base64`")), nil, nil
		}
		out, findings, blocked := secretscan.Guard(data, in.Redact)
		if blocked {
			return textResult(fmt.Sprintf("⛔ Not stored — possible secret(s) detected: %s.\nClean the content, or call again with redact:true to store a masked copy.", secretscan.Summary(findings))), nil, nil
		}
		sha, err := store.Put(ctx, out, in.ContentType)
		if err != nil {
			return errResult(err), nil, nil
		}
		var b strings.Builder
		fmt.Fprintf(&b, "Stored in %s. Cite it as evidence with:\n\n    %s\n", store.Backend(), core.ArtifactRef(sha))
		if len(findings) > 0 {
			fmt.Fprintf(&b, "\n⚠ Redacted before storing: %s\n", secretscan.Summary(findings))
		}
		return textResult(b.String()), nil, nil
	})
}
