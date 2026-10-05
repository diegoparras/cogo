package main

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// registrarRestore registra el tool `restore`. Ver tools.go para lo que comparten.
func registrarRestore(s *mcp.Server, d *deps) {
	dir := d.dir
	mcp.AddTool(s, &mcp.Tool{
		Name:        "restore",
		Description: "Bring an archived or retracted note back to active — visible again in the graph, pack and search.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in openIn) (*mcp.CallToolResult, any, error) {
		return setNoteStatus(dir, in.ID, "")
	})
}
