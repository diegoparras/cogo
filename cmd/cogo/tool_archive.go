package main

import (
	"context"

	"github.com/diegoparras/cogo/internal/core"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// registrarArchive registra el tool `archive`. Ver tools.go para lo que comparten.
func registrarArchive(s *mcp.Server, d *deps) {
	dir := d.dir
	mcp.AddTool(s, &mcp.Tool{
		Name:        "archive",
		Description: "Put a note away: keep it on disk but drop it from the graph, pack and search. For findings that are done or obsolete. Lifecycle is a separate axis from color — archiving never changes a note's confidence, and it is restorable.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in openIn) (*mcp.CallToolResult, any, error) {
		return setNoteStatus(dir, in.ID, core.StateArchived)
	})
}
