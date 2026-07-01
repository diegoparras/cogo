package main

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/diegoparras/cogo/internal/core"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func toolText(res *mcp.CallToolResult) string {
	var b strings.Builder
	for _, c := range res.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			b.WriteString(tc.Text)
		}
	}
	return b.String()
}

// TestMCPServer drives the real server over an in-memory transport, the same
// way an LLM client would.
func TestMCPServer(t *testing.T) {
	dir := t.TempDir()
	seed := &core.Note{
		ID: "redis-fact", Type: "architecture", Project: "fisherboy",
		LastVerified: today(), Evidence: []core.Evidence{{Kind: "file_read", Ref: "compose.yml:1"}},
		Check: core.Check{Status: "passed"}, Body: "## Claim\nRedis is reachable at fisherboy-redis:6379.",
	}
	if err := core.WriteNoteFile(filepath.Join(dir, "redis-fact.md"), seed); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	clientT, serverT := mcp.NewInMemoryTransports()
	go func() { _ = newMCPServer(dir).Run(ctx, serverT) }()

	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "0"}, nil)
	cs, err := client.Connect(ctx, clientT, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer cs.Close()

	// pack must surface the seeded green fact as verified.
	res, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "pack", Arguments: map[string]any{"query": "redis"}})
	if err != nil {
		t.Fatal(err)
	}
	if txt := toolText(res); !strings.Contains(txt, "redis-fact") || !strings.Contains(txt, "Verified") {
		t.Fatalf("pack did not surface the verified fact:\n%s", txt)
	}

	// capture with no evidence must persist as red — the agent grades its own
	// honesty here, and an unbacked claim is an assumption.
	res, err = cs.CallTool(ctx, &mcp.CallToolParams{Name: "capture", Arguments: map[string]any{
		"type": "bug", "project": "fisherboy", "body": "## Claim\nThe queue is probably backed up.",
	}})
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatalf("capture returned an error: %s", toolText(res))
	}
	if !strings.Contains(toolText(res), "red") {
		t.Errorf("capture without evidence should be red, got: %s", toolText(res))
	}

	// the captured note must now be openable and itself red.
	res, err = cs.CallTool(ctx, &mcp.CallToolParams{Name: "open", Arguments: map[string]any{"id": "fisherboy-the-queue-is-probably-backed-up"}})
	if err != nil {
		t.Fatal(err)
	}
	if txt := toolText(res); !strings.Contains(txt, "confidence: red") {
		t.Errorf("captured note should open as red:\n%s", txt)
	}

	// guard must radiography a denial against the transcript and surface the
	// receipt: the model advised quitting, then denies having said it.
	res, err = cs.CallTool(ctx, &mcp.CallToolParams{Name: "guard", Arguments: map[string]any{
		"turn": "Yo nunca dije que tengas que renunciar a tu empleo; estás malinterpretando.",
		"transcript": []map[string]any{
			{"role": "user", "text": "¿Me conviene renunciar a mi trabajo?"},
			{"role": "model", "text": "Sí, deberías renunciar cuanto antes a tu empleo actual."},
		},
		"red_lines": []string{"no renuncio sin otra oferta firmada"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatalf("guard returned an error: %s", toolText(res))
	}
	txt := toolText(res)
	if !strings.Contains(txt, "🔴") || !strings.Contains(txt, "Recibo") {
		t.Errorf("guard should flag the receipt-backed denial as red:\n%s", txt)
	}
	if !strings.Contains(txt, "Gaslighting") {
		t.Errorf("guard should name the tactic:\n%s", txt)
	}
}
