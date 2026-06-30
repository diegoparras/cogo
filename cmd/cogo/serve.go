package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/diegoparras/cogo/internal/core"
	"github.com/diegoparras/cogo/internal/scrub"
	"github.com/diegoparras/cogo/internal/web"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const version = "0.1.0"

// cmdServe runs cogo as an MCP server over stdio: the same binary, the same
// core, exposed to any LLM client. Side-effect-free by construction — it only
// reads the vault and writes notes; no shell, no outbound network.
func cmdServe(args []string) error {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	dir := vaultFlag(fs)
	httpAddr := fs.String("http", "", "serve MCP over HTTP on this address (e.g. :8080); empty = stdio")
	_ = fs.Parse(args)

	if err := os.MkdirAll(*dir, 0o755); err != nil {
		return err
	}
	srv := newMCPServer(*dir)

	// stdio: the local default, launched per session by the LLM client.
	if *httpAddr == "" {
		return srv.Run(context.Background(), &mcp.StdioTransport{})
	}

	// HTTP: the long-running container service. Remote clients reach it behind a
	// proxy with OIDC (Lockatus); locally it is loopback.
	handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return srv }, nil)
	mux := http.NewServeMux()
	mux.Handle("/mcp", handler)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("ok")) })
	web.New(*dir, today).Mount(mux) // human face: visor at /, JSON API at /api
	fmt.Fprintf(os.Stderr, "cogo: serving on %s — web visor at /, MCP at /mcp (vault %s)\n", *httpAddr, *dir)
	return http.ListenAndServe(*httpAddr, mux)
}

func newMCPServer(dir string) *mcp.Server {
	s := mcp.NewServer(&mcp.Implementation{Name: "cogo", Version: version}, nil)
	scrubber := scrub.FromEnv()

	mcp.AddTool(s, &mcp.Tool{
		Name:        "pack",
		Description: "Get colored context for a topic before acting. Green=verified, yellow=probable; red is quarantined as do-not-rely.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in packIn) (*mcp.CallToolResult, any, error) {
		vault, err := core.LoadVault(dir)
		if err != nil {
			return errResult(err), nil, nil
		}
		p := core.BuildPack(vault, nil, core.PackOptions{Query: in.Query, Project: in.Project, Budget: in.Budget, Today: today()})
		return textResult(p.Markdown), nil, nil
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "search",
		Description: "List notes matching a query: id, color and a one-line summary (no bodies).",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in searchIn) (*mcp.CallToolResult, any, error) {
		vault, err := core.LoadVault(dir)
		if err != nil {
			return errResult(err), nil, nil
		}
		hits := core.Search(vault, nil, in.Query, in.Project, today(), in.Limit)
		if len(hits) == 0 {
			return textResult("no matching notes"), nil, nil
		}
		var b strings.Builder
		for _, h := range hits {
			fmt.Fprintf(&b, "- %s `%s` — %s\n", h.Color, h.ID, h.Summary)
		}
		return textResult(b.String()), nil, nil
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "open",
		Description: "Return one note by id, with its freshly computed color.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in openIn) (*mcp.CallToolResult, any, error) {
		vault, err := core.LoadVault(dir)
		if err != nil {
			return errResult(err), nil, nil
		}
		n, ok := vault[in.ID]
		if !ok {
			return errResult(fmt.Errorf("no note with id %q", in.ID)), nil, nil
		}
		n.Apply(core.Evaluate(n, vault, nil, today()))
		md, err := core.MarshalNote(n)
		if err != nil {
			return errResult(err), nil, nil
		}
		return textResult(string(md)), nil, nil
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "capture",
		Description: "Record a finding as a note. Always include evidence and a minimal check. Never set the color — COGO computes it.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in captureIn) (*mcp.CallToolResult, any, error) {
		if in.Type == "" || strings.TrimSpace(in.Body) == "" {
			return errResult(fmt.Errorf("capture needs at least a type and a body")), nil, nil
		}
		id := in.ID
		if id == "" {
			id = core.DeriveID(in.Project, in.Body)
		}
		note := &core.Note{
			ID: id, Type: in.Type, Project: in.Project, Body: strings.TrimSpace(in.Body),
			LastVerified: today(),
			Check:        core.Check{Test: in.CheckTest, Status: "not_run"},
		}
		for _, e := range in.Evidence {
			note.Evidence = append(note.Evidence, core.Evidence{Kind: e.Kind, Ref: e.Ref})
		}
		if err := scrub.Note(ctx, scrubber, note); err != nil {
			return errResult(fmt.Errorf("scrub failed: %w", err)), nil, nil
		}

		vault, err := core.LoadVault(dir)
		if err != nil {
			return errResult(err), nil, nil
		}
		existing, had := vault[id]
		if had {
			if ev := core.Evaluate(existing, vault, nil, today()); ev.Color == core.Green {
				return errResult(fmt.Errorf("note %q exists and is green; not overwritten — verify it or use a new id", id)), nil, nil
			}
		}
		vault[id] = note
		v := core.Evaluate(note, vault, nil, today())
		note.Apply(v)

		path := filepath.Join(dir, id+".md")
		if had && existing.Path != "" {
			path = existing.Path
		}
		if err := core.WriteNoteFile(path, note); err != nil {
			return errResult(err), nil, nil
		}
		_ = regenIndex(dir, vault)
		_ = appendLog(dir, fmt.Sprintf("capture %s %s — %s", id, v.Color, v.Reason))
		return textResult(fmt.Sprintf("captured %q as %s — %s", id, v.Color, v.Reason)), nil, nil
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "verify",
		Description: "Mark a note's check as passed as of today and re-color it.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in openIn) (*mcp.CallToolResult, any, error) {
		vault, err := core.LoadVault(dir)
		if err != nil {
			return errResult(err), nil, nil
		}
		n, ok := vault[in.ID]
		if !ok {
			return errResult(fmt.Errorf("no note with id %q", in.ID)), nil, nil
		}
		n.Check.Status = "passed"
		n.LastVerified = today()
		v := core.Evaluate(n, vault, nil, today())
		n.Apply(v)

		path := n.Path
		if path == "" {
			path = filepath.Join(dir, in.ID+".md")
		}
		if err := core.WriteNoteFile(path, n); err != nil {
			return errResult(err), nil, nil
		}
		_ = regenIndex(dir, vault)
		_ = appendLog(dir, fmt.Sprintf("verify %s %s", in.ID, v.Color))
		return textResult(fmt.Sprintf("%s %s — %s", v.Color, in.ID, v.Reason)), nil, nil
	})

	return s
}

// --- tool I/O ---

type packIn struct {
	Query   string `json:"query" jsonschema:"the topic to build context for"`
	Project string `json:"project,omitempty" jsonschema:"optional project filter"`
	Budget  int    `json:"token_budget,omitempty" jsonschema:"approximate token ceiling; 0 means unlimited"`
}

type searchIn struct {
	Query   string `json:"query" jsonschema:"search terms"`
	Project string `json:"project,omitempty" jsonschema:"optional project filter"`
	Limit   int    `json:"limit,omitempty" jsonschema:"max results; 0 means all"`
}

type openIn struct {
	ID string `json:"id" jsonschema:"the note id"`
}

type evidenceIn struct {
	Kind string `json:"kind" jsonschema:"direct_log|command_output|test_result|file_read|doc|testimony|inference|hypothesis|absence"`
	Ref  string `json:"ref" jsonschema:"reference to the real artifact: commit+line, log timestamp, command+output, URL+date"`
}

type captureIn struct {
	Type      string       `json:"type" jsonschema:"one of decision|bug|runbook|architecture|constraint|command|mistake"`
	Body      string       `json:"body" jsonschema:"the note in markdown: a Claim, optional Refutation, and a Minimal check"`
	Project   string       `json:"project,omitempty" jsonschema:"the project this note belongs to"`
	ID        string       `json:"id,omitempty" jsonschema:"stable id; if omitted it is derived from the claim"`
	Evidence  []evidenceIn `json:"evidence,omitempty" jsonschema:"supporting artifacts; each needs a kind and a ref to a real artifact"`
	CheckTest string       `json:"check_test,omitempty" jsonschema:"the minimal test that would verify the claim"`
}

// --- helpers ---

func textResult(s string) *mcp.CallToolResult {
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: s}}}
}

// errResult reports a tool error inside the result (IsError), so the LLM sees
// it — not as a protocol-level failure.
func errResult(err error) *mcp.CallToolResult {
	return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "cogo: " + err.Error()}}}
}

