package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/diegoparras/cogo/internal/auth"
	"github.com/diegoparras/cogo/internal/contra"
	"github.com/diegoparras/cogo/internal/core"
	"github.com/diegoparras/cogo/internal/history"
	"github.com/diegoparras/cogo/internal/llm"
	"github.com/diegoparras/cogo/internal/scrub"
	"github.com/diegoparras/cogo/internal/suasion"
	"github.com/diegoparras/cogo/internal/tokens"
	"github.com/diegoparras/cogo/internal/web"
	"github.com/diegoparras/cogo/internal/xray"
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
	// Record a per-note history line on every write (stdio and HTTP both go
	// through core.WriteNoteFile). The vault dir is derived from the note path.
	core.SetWriteHook(func(path string, n *core.Note) {
		history.Record(filepath.Dir(path), n.ID, n.Confidence, n.ColorReason, core.Claim(n))
	})
	srv := newMCPServer(*dir)

	// stdio: the local default, launched per session by the LLM client.
	if *httpAddr == "" {
		return srv.Run(context.Background(), &mcp.StdioTransport{})
	}

	// HTTP: the long-running container service. Remote clients reach it behind a
	// proxy with OIDC (Lockatus); locally it is loopback.
	handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return srv }, nil)
	authn, err := auth.FromEnv(context.Background())
	if err != nil {
		return err
	}
	// Issued-token store: multiple named Bearer tokens for MCP clients, each
	// revocable, with optional expiry and read-only scope. Adds an authorization
	// path to the gate; the root COGO_MCP_TOKEN / OIDC still bootstraps it.
	store := tokens.Open(*dir)
	authn.SetVerifier(func(secret string) (string, bool, bool) {
		t, ok := store.Verify(secret, today().String())
		return t.Label, t.ReadOnly, ok
	})

	// Fail-safe: never put an unauthenticated vault + MCP on a public interface.
	if err := checkExposure(*httpAddr, authn); err != nil {
		return err
	}
	mux := http.NewServeMux()
	mux.Handle("/mcp", handler)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("ok")) })
	web.New(*dir, today, store).Mount(mux) // human face: visor at /, JSON API at /api
	authn.RegisterRoutes(mux)              // accessory: OIDC login (federated only)

	tls := os.Getenv("COOKIE_SECURE") == "1"
	var h http.Handler = enforceReadOnly(mux) // read-only tokens can't write
	h = auditMiddleware(*dir)(h)              // audit trail (who called which tool)
	h = authn.Gate(h)                         // auth (cookie or Bearer), stamps caller+scope
	h = newIPLimiter(20, 60).middleware(h)    // per-IP rate limit
	h = securityHeaders(h, tls)               // conservative headers

	insecure := !authn.Enabled() && !isLoopback(*httpAddr)
	fmt.Fprintf(os.Stderr, "cogo: serving on %s [auth=%s] — visor at /, MCP at /mcp (vault %s)\n", *httpAddr, authn.Mode(), *dir)
	if insecure {
		fmt.Fprintf(os.Stderr, "  ⚠ WARNING: public interface with no auth (COGO_ALLOW_INSECURE=1). Set COGO_MCP_TOKEN for a VPS.\n")
	}
	return http.ListenAndServe(*httpAddr, h)
}

func newMCPServer(dir string) *mcp.Server {
	s := mcp.NewServer(&mcp.Implementation{Name: "cogo", Version: version}, nil)
	scrubber := scrub.FromEnv()
	cache := core.NewVaultCache(dir) // mtime-keyed reads: the MCP is a long-running server
	// loadVault reads the vault and checks that evidence refs resolve, so the
	// color an agent consumes reflects broken citations (see core.ResolveEvidence).
	// Evidence roots are re-read each call (tiny file) so UI edits take effect live.
	loadVault := func() (map[string]*core.Note, error) {
		v, err := cache.Load()
		if err != nil {
			return nil, err
		}
		core.ResolveEvidence(v, core.LoadEvidenceRoots(dir))
		return v, nil
	}
	// contradictions is the set of note ids under an OPEN contradiction, read fresh
	// from the persisted store each call (tiny file). Feeding it to the color engine
	// is what makes an agent over MCP see red-by-contradiction — the same paint the
	// visor shows — instead of a color blind to the store the human curates.
	contradictions := func() map[string]bool { return contra.Open(dir).OpenNoteSet() }

	mcp.AddTool(s, &mcp.Tool{
		Name:        "pack",
		Description: "Get colored context for a topic before acting. Green=verified, yellow=probable; red is quarantined as do-not-rely.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in packIn) (*mcp.CallToolResult, any, error) {
		vault, err := loadVault()
		if err != nil {
			return errResult(err), nil, nil
		}
		p := core.BuildPack(vault, contradictions(), core.PackOptions{Query: in.Query, Project: in.Project, Budget: in.Budget, Today: today()})
		return textResult(p.Markdown), nil, nil
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "recall",
		Description: "Re-anchor after a context compaction. Returns the load-bearing memory you must not lose: the user's mandate (red lines) and the verified decisions and constraints. Call it at the start of a session and again after any auto-compaction, so binding constraints don't silently disappear from context.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in recallIn) (*mcp.CallToolResult, any, error) {
		var b strings.Builder
		b.WriteString("# Recall — do not lose these\n")
		if m := suasion.LoadMandate(suasion.MandatePath(dir)); m != nil && (m.Goal != "" || len(m.RedLines) > 0) {
			b.WriteString("\n## Mandate (red lines)\n")
			if m.Goal != "" {
				fmt.Fprintf(&b, "- goal: %s\n", m.Goal)
			}
			for _, rl := range m.RedLines {
				fmt.Fprintf(&b, "- 🔴 %s\n", rl)
			}
		}
		if vault, err := loadVault(); err == nil {
			if c := core.BuildConstraints(vault, contradictions(), today()); c != "" {
				b.WriteString("\n## Verified decisions & constraints\n")
				b.WriteString(c)
				b.WriteString("\n")
			}
		}
		if b.Len() < 40 {
			b.WriteString("\n_No mandate declared and no verified decisions/constraints yet._\n")
		}
		return textResult(b.String()), nil, nil
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "reflect",
		Description: "After finishing a task, hand a short summary of what you did and verified. If a model is configured, COGO proposes graded notes worth capturing (claim + evidence + a check) so real findings persist instead of being re-derived next session — you still decide what to `capture`.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in reflectIn) (*mcp.CallToolResult, any, error) {
		if strings.TrimSpace(in.Summary) == "" {
			return errResult(fmt.Errorf("reflect needs a `summary` of what you did/learned")), nil, nil
		}
		p := guardProvider(dir)
		if !p.Available() {
			return textResult("No model configured (Ajustes → Modelo IA). `reflect` needs a model to score what's worth keeping; capture findings by hand with `capture`."), nil, nil
		}
		out, err := p.Complete(ctx, reflectPrompt(in.Summary))
		if err != nil {
			return errResult(fmt.Errorf("reflect model call failed: %w", err)), nil, nil
		}
		return textResult("# Capturables — revisá y guardá lo que valga con `capture`\n\n" + strings.TrimSpace(out)), nil, nil
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "search",
		Description: "List notes matching a query: id, color and a one-line summary (no bodies).",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in searchIn) (*mcp.CallToolResult, any, error) {
		vault, err := loadVault()
		if err != nil {
			return errResult(err), nil, nil
		}
		hits := core.Search(vault, contradictions(), in.Query, in.Project, today(), in.Limit, in.IncludeArchived)
		if len(hits) == 0 {
			return textResult("no matching notes"), nil, nil
		}
		var b strings.Builder
		for _, h := range hits {
			fmt.Fprintf(&b, "- %s `%s` — %s", h.Color, h.ID, h.Summary)
			if h.State != "" {
				fmt.Fprintf(&b, " [%s]", h.State)
			}
			b.WriteString("\n")
		}
		return textResult(b.String()), nil, nil
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "open",
		Description: "Return one note by id, with its freshly computed color.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in openIn) (*mcp.CallToolResult, any, error) {
		vault, err := loadVault()
		if err != nil {
			return errResult(err), nil, nil
		}
		n, ok := vault[in.ID]
		if !ok {
			return errResult(fmt.Errorf("no note with id %q", in.ID)), nil, nil
		}
		cstore := contra.Open(dir)
		n.Apply(core.Evaluate(n, vault, cstore.OpenNoteSet(), today()))
		md, err := core.MarshalNote(n)
		if err != nil {
			return errResult(err), nil, nil
		}
		out := string(md)
		// The trace behind a red-by-contradiction verdict: name the clashing
		// note(s) and why, so the agent can resolve instead of just seeing "red".
		if cs := cstore.ForNote(in.ID); len(cs) > 0 {
			var b strings.Builder
			b.WriteString(out)
			b.WriteString("\n## ⚠ Contradicciones abiertas\n\nEsta nota es roja porque choca con otra(s). Resolvé el conflicto antes de apoyarte en ella:\n\n")
			for _, c := range cs {
				fmt.Fprintf(&b, "- contradice `%s`", c.Other)
				if c.Reason != "" {
					fmt.Fprintf(&b, " — %s", c.Reason)
				}
				b.WriteByte('\n')
			}
			out = b.String()
		}
		return textResult(out), nil, nil
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
			DependsOn:    in.DependsOn, Supersedes: in.Supersedes, CausedBy: in.CausedBy,
		}
		for _, e := range in.Evidence {
			note.Evidence = append(note.Evidence, core.Evidence{Kind: e.Kind, Ref: e.Ref})
		}
		if err := scrub.Note(ctx, scrubber, note); err != nil {
			return errResult(fmt.Errorf("scrub failed: %w", err)), nil, nil
		}

		vault, err := loadVault()
		if err != nil {
			return errResult(err), nil, nil
		}
		cx := contradictions()
		existing, had := vault[id]
		if had {
			if ev := core.Evaluate(existing, vault, cx, today()); ev.Color == core.Green {
				return errResult(fmt.Errorf("note %q exists and is green; not overwritten — verify it or use a new id", id)), nil, nil
			}
		}
		vault[id] = note
		core.ResolveEvidence(vault, core.LoadEvidenceRoots(dir)) // resolve the new note's own refs
		v := core.Evaluate(note, vault, cx, today())
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
		vault, err := loadVault()
		if err != nil {
			return errResult(err), nil, nil
		}
		n, ok := vault[in.ID]
		if !ok {
			return errResult(fmt.Errorf("no note with id %q", in.ID)), nil, nil
		}
		n.Check.Status = "passed"
		n.LastVerified = today()
		core.StampEvidenceHashes(n, core.LoadEvidenceRoots(dir)) // re-baseline drift on verify
		v := core.Evaluate(n, vault, contradictions(), today())
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

	mcp.AddTool(s, &mcp.Tool{
		Name:        "archive",
		Description: "Put a note away: keep it on disk but drop it from the graph, pack and search. For findings that are done or obsolete. Lifecycle is a separate axis from color — archiving never changes a note's confidence, and it is restorable.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in openIn) (*mcp.CallToolResult, any, error) {
		return setNoteStatus(dir, in.ID, core.StateArchived)
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "restore",
		Description: "Bring an archived or retracted note back to active — visible again in the graph, pack and search.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in openIn) (*mcp.CallToolResult, any, error) {
		return setNoteStatus(dir, in.ID, "")
	})

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
		rep := xray.Analyze(in.Answer)
		var b strings.Builder
		icon := map[string]string{"red": "🔴", "yellow": "🟡", "ungraded": "⚪"}
		fmt.Fprintf(&b, "Radiografía de veracidad — %s\n%s\n\n", icon[rep.Overall]+" "+rep.Overall, rep.Summary)
		for _, c := range rep.Claims {
			fmt.Fprintf(&b, "%s %q\n   %s (compromiso: %s · evidencia: %s)\n", icon[c.Color], c.Text, c.Reason, c.Commitment, c.Evidence)
		}
		return textResult(b.String()), nil, nil
	})

	return s
}

// --- tool I/O ---

type xrayIn struct {
	Answer string `json:"answer" jsonschema:"the AI answer text to radiograph for veracity"`
}

type packIn struct {
	Query   string `json:"query" jsonschema:"the topic to build context for"`
	Project string `json:"project,omitempty" jsonschema:"optional project filter"`
	Budget  int    `json:"token_budget,omitempty" jsonschema:"approximate token ceiling; 0 means unlimited"`
}

type recallIn struct{} // no input: recall returns the whole load-bearing bundle

type reflectIn struct {
	Summary string `json:"summary" jsonschema:"a short summary of what you did and verified this session"`
}

// reflectPrompt asks the model to distil a session summary into capturable notes.
// Conservative: only concrete, verifiable findings; no invention; caller decides.
func reflectPrompt(summary string) string {
	return "Ayudás a decidir qué vale la pena guardar en la memoria de un proyecto (COGO).\n" +
		"A partir de este resumen de lo que hizo un agente, extraé 0 a 5 HALLAZGOS que valga la pena capturar como notas.\n" +
		"Incluí SOLO cosas concretas y verificables (una decisión tomada, un bug confirmado, una restricción, un comando que anduvo). Descartá corazonadas sin fundamento.\n" +
		"Respondé en el idioma del resumen. Para cada hallazgo:\n" +
		"- **claim**: afirmación declarativa y testeable, una línea.\n" +
		"- type: decision|bug|runbook|architecture|constraint|command|mistake\n" +
		"- evidence: el archivo/comando/log que lo respalda (si lo hay).\n" +
		"- check: el test mínimo que lo confirmaría.\n" +
		"Si no hay nada que valga la pena, respondé exactamente: (nada que capturar).\n" +
		"NO inventes nada que no esté en el resumen.\n\nRESUMEN:\n" + summary
}

type searchIn struct {
	Query           string `json:"query" jsonschema:"search terms"`
	Project         string `json:"project,omitempty" jsonschema:"optional project filter"`
	Limit           int    `json:"limit,omitempty" jsonschema:"max results; 0 means all"`
	IncludeArchived bool   `json:"include_archived,omitempty" jsonschema:"also list archived/retracted/superseded notes (hidden by default)"`
}

// setNoteStatus flips a note's lifecycle state (archive/restore) and persists it.
// Color is untouched — lifecycle is a separate axis from confidence.
func setNoteStatus(dir, id, status string) (*mcp.CallToolResult, any, error) {
	vault, err := core.LoadVault(dir)
	if err != nil {
		return errResult(err), nil, nil
	}
	n, ok := vault[id]
	if !ok {
		return errResult(fmt.Errorf("no note with id %q", id)), nil, nil
	}
	n.Status = status
	path := n.Path
	if path == "" {
		path = filepath.Join(dir, id+".md")
	}
	if err := core.WriteNoteFile(path, n); err != nil {
		return errResult(err), nil, nil
	}
	_ = regenIndex(dir, vault)
	st := core.Lifecycle(vault)[id]
	_ = appendLog(dir, fmt.Sprintf("status %s %s", id, st))
	return textResult(fmt.Sprintf("%s is now %s", id, st)), nil, nil
}

type openIn struct {
	ID string `json:"id" jsonschema:"the note id"`
}

type evidenceIn struct {
	Kind string `json:"kind" jsonschema:"direct_log|command_output|test_result|file_read|doc|testimony|inference|hypothesis|absence"`
	Ref  string `json:"ref" jsonschema:"reference to the real artifact: commit+line, log timestamp, command+output, URL+date"`
}

type captureIn struct {
	Type       string       `json:"type" jsonschema:"one of decision|bug|runbook|architecture|constraint|command|mistake"`
	Body       string       `json:"body" jsonschema:"the note in markdown: a Claim, optional Refutation, and a Minimal check"`
	Project    string       `json:"project,omitempty" jsonschema:"the project this note belongs to"`
	ID         string       `json:"id,omitempty" jsonschema:"stable id; if omitted it is derived from the claim"`
	Evidence   []evidenceIn `json:"evidence,omitempty" jsonschema:"supporting artifacts; each needs a kind and a ref to a real artifact"`
	CheckTest  string       `json:"check_test,omitempty" jsonschema:"the minimal test that would verify the claim"`
	DependsOn  []string     `json:"depends_on,omitempty" jsonschema:"ids of notes this one hard-depends on; a red dependency makes this note red too"`
	Supersedes string       `json:"supersedes,omitempty" jsonschema:"id of a note this one replaces; the old note is archived (buried)"`
	CausedBy   string       `json:"caused_by,omitempty" jsonschema:"id of the note that caused this finding"`
}

type transcriptTurnIn struct {
	Role string `json:"role" jsonschema:"user or model"`
	Text string `json:"text" jsonschema:"the message text"`
}

type guardIn struct {
	Turn       string             `json:"turn" jsonschema:"the model turn to analyze"`
	Transcript []transcriptTurnIn `json:"transcript,omitempty" jsonschema:"prior conversation oldest-first, for checking denials against what was actually said"`
	Goal       string             `json:"goal,omitempty" jsonschema:"the user's declared goal for this conversation"`
	RedLines   []string           `json:"red_lines,omitempty" jsonschema:"what the user declared they are NOT willing to do or believe; drift is measured against these"`
	Steelman   bool               `json:"steelman,omitempty" jsonschema:"true to add an adversarial second opinion: the strongest case for the side the turn does not show (needs a configured model; never changes the verdict)"`
}

// guardProvider mirrors the visor's rule: a saved GUI setting wins, then env,
// otherwise off — Tier 1 is optional and guard stays deterministic without it.
func guardProvider(dir string) llm.Provider {
	var set struct {
		BaseURL string `json:"base_url"`
		Model   string `json:"model"`
		APIKey  string `json:"api_key"`
	}
	if b, err := os.ReadFile(filepath.Join(dir, ".cogo", "llm.json")); err == nil {
		if json.Unmarshal(b, &set) == nil && set.BaseURL != "" && set.Model != "" {
			return &llm.OpenAICompatible{BaseURL: set.BaseURL, Model: set.Model, APIKey: set.APIKey, Referer: os.Getenv("COGO_LLM_REFERER")}
		}
	}
	return llm.FromEnv()
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
