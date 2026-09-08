package main

import (
	"context"
	"fmt"

	"github.com/diegoparras/cogo/internal/core"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// registrarRemove registra el tool `remove`. Ver tools.go para lo que comparten.
func registrarRemove(s *mcp.Server, d *deps) {
	dir := d.dir
	mcp.AddTool(s, &mcp.Tool{
		Name:        "remove",
		Description: "Delete a note from disk for good. Only for genuine garbage (wrong project, leaked secret, duplicate) — prefer archive, which keeps the record. Leaves a tombstone in the log.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in openIn) (*mcp.CallToolResult, any, error) {
		vault, err := core.LoadVault(dir)
		if err != nil {
			return errResult(err), nil, nil
		}
		n, ok := vault[in.ID]
		if !ok {
			return errResult(fmt.Errorf("no note with id %q", in.ID)), nil, nil
		}
		if _, err := core.TrashNote(dir, n); err != nil {
			return errResult(err), nil, nil
		}
		delete(vault, in.ID)
		_ = regenIndex(dir, vault)
		_ = appendLog(dir, "delete "+in.ID)
		return textResult(fmt.Sprintf("deleted %q (moved to .cogo/trash)", in.ID)), nil, nil
	})
}
