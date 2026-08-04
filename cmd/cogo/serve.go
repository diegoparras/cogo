package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/diegoparras/cogo/internal/accion"
	"github.com/diegoparras/cogo/internal/artifact"
	"github.com/diegoparras/cogo/internal/auth"
	"github.com/diegoparras/cogo/internal/confidence"
	"github.com/diegoparras/cogo/internal/contra"
	"github.com/diegoparras/cogo/internal/core"
	"github.com/diegoparras/cogo/internal/embed"
	"github.com/diegoparras/cogo/internal/ghsource"
	"github.com/diegoparras/cogo/internal/history"
	"github.com/diegoparras/cogo/internal/journal"
	"github.com/diegoparras/cogo/internal/lease"
	"github.com/diegoparras/cogo/internal/llm"
	"github.com/diegoparras/cogo/internal/motor"
	"github.com/diegoparras/cogo/internal/savings"
	"github.com/diegoparras/cogo/internal/scrub"
	"github.com/diegoparras/cogo/internal/secretscan"
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
	instalarParametros(*dir)
	if err := instalarMotor(*dir); err != nil {
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
	visor := web.New(*dir, today, store)
	visor.UsarParametros(pars) // el panel edita el mismo Set que lee el motor
	visor.Mount(mux)           // human face: visor at /, JSON API at /api
	authn.RegisterRoutes(mux)  // accessory: OIDC login (federated only)

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
	store := artifact.FromEnv(dir) // R2 if COGO_R2_* is set, else disk under .cogo/artifacts
	// Resolve "artifact://<sha>" evidence against the store: present → the citation
	// holds; gone → it breaks and the color degrades. Content-addressed, so it can
	// only exist or not — verify recomputes instead of trusting a stale claim.
	core.SetArtifactChecker(func(sha string) bool {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		ok, _ := store.Has(ctx, sha)
		return ok
	})
	// Resolve "github://owner/repo@ref/path" evidence against the GitHub API, so a
	// hosted COGO (no working copy on disk) can still check file citations — and
	// so a note stays green only while the cited file's blob SHA hasn't moved.
	gh := ghsource.FromEnv()
	core.SetGitHubResolver(func(owner, repo, ref, path string) (string, bool, bool) {
		ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
		defer cancel()
		sha, found, err := gh.FileSHA(ctx, owner, repo, ref, path)
		if err != nil {
			return "", false, false // couldn't check: stays unchecked
		}
		return sha, found, true
	})
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
		p := core.BuildPack(vault, contradictions(), core.PackOptions{Query: in.Query, Project: in.Project, Budget: in.Budget, Today: today(), Env: in.Env})
		savings.Add(dir, p.RawTokens-p.Tokens, today().String())
		return textResult(p.Markdown), nil, nil
	})

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

	mcp.AddTool(s, &mcp.Tool{
		Name:        "search",
		Description: "List notes matching a query: id, color and a one-line summary (no bodies). Ranks by MEANING (embeddings) when an embedding model is configured, else keyword BM25.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in searchIn) (*mcp.CallToolResult, any, error) {
		vault, err := loadVault()
		if err != nil {
			return errResult(err), nil, nil
		}
		// Optional semantic ranking; on any failure it falls through to keyword.
		if in.Query != "" {
			if ep := embedProvider(dir); ep != nil && ep.EmbedAvailable() {
				if out, ok := semanticSearch(ctx, dir, vault, contradictions(), in, ep); ok {
					return textResult(out), nil, nil
				}
			}
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
		Name:        "gap",
		Description: "Record something the project does NOT know, as an open question. Use it when you hit a decision you cannot make because a fact is missing, when you had to assume something to keep going, or when an investigation came back inconclusive. This is NOT a low-confidence note: a note claims something and might be wrong, a gap claims nothing and says so. Gaps carry no color and never enter the confidence graph; the pack returns them in their own section, ordered by how many decisions each one blocks. If you find yourself about to guess, record the gap instead.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in gapIn) (*mcp.CallToolResult, any, error) {
		q := strings.TrimSpace(in.Question)
		if q == "" {
			return errResult(fmt.Errorf("a gap needs a question: what is it that nobody knows?")), nil, nil
		}
		id := in.ID
		if id == "" {
			id = core.DeriveID(in.Project, q)
		}
		body := strings.TrimSpace(in.Body)
		if body == "" {
			body = "## Claim\n" + q
		}
		note := &core.Note{
			ID: id, Type: core.TipoBrecha, Project: in.Project, Body: body,
			LastVerified: today(), Question: q, Blocks: in.Blocks,
			CostToResolve: in.Cost, Attempted: in.Attempted,
			Author: auth.CallerCtx(ctx),
		}
		if err := scrub.Note(ctx, scrubber, note); err != nil {
			return errResult(fmt.Errorf("scrub failed: %w", err)), nil, nil
		}
		vault, err := loadVault()
		if err != nil {
			return errResult(err), nil, nil
		}
		vault[id] = note
		if err := core.WriteNoteFile(filepath.Join(dir, id+".md"), note); err != nil {
			return errResult(err), nil, nil
		}
		_ = regenIndex(dir, vault)
		_ = appendLog(dir, fmt.Sprintf("gap %s", id))
		msg := fmt.Sprintf("gap %s registrada: %s", id, q)
		if k := len(in.Blocks); k > 0 {
			msg += fmt.Sprintf(" — traba %d decisión(es)", k)
		}
		return textResult(msg), nil, nil
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "authorize",
		Description: "Ask whether what you know is enough for what you are about to do. Call it BEFORE any action that changes something outside your own answer — writing files, running migrations, deploying, deleting, sending, publishing. Not every action needs the same backing: explaining something from a yellow note is fine, dropping a table from the same note is not. COGO classifies the action, looks up the confidence this vault requires for that class, and checks the notes you say you are relying on. A NOT AUTHORIZED answer is not an obstacle to route around: report it to the human and let them decide. Declaring a lower class does not lower the bar — the text of the action is classified too and the stricter of the two wins.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in authorizeIn) (*mcp.CallToolResult, any, error) {
		if strings.TrimSpace(in.Action) == "" {
			return errResult(fmt.Errorf("authorize needs to know what you are about to do")), nil, nil
		}
		vault, err := loadVault()
		if err != nil {
			return errResult(err), nil, nil
		}
		estados := map[string]confidence.Estado{}
		if j, err := journal.Open(dir); err == nil {
			if evs, err := j.All(); err == nil {
				estados, _ = motor.Estados(vault, contradictions(), today(), evs)
			}
		}
		v := accion.Autorizar(
			accion.Peticion{Accion: in.Action, Clase: in.Class, Notas: in.Notes},
			fuenteVault{estados: estados, vault: vault}, pars)
		// Toda consulta queda registrada, autorice o no. Un control que solo deja
		// rastro cuando bloquea no sirve para auditar: lo que se quiere poder
		// reconstruir es en qué se apoyó cada acción, sobre todo las que pasaron.
		_ = appendLog(dir, fmt.Sprintf("authorize %s [%s] %v -> %v",
			v.Clase, v.Necesita, in.Notes, v.Autoriza))
		return textResult(textoAutorizacion(v)), v, nil
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
			Scope: in.Scope,
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
		// Author = who captured it (the authenticated caller); preserve the original
		// creator across edits.
		if had && existing.Author != "" {
			note.Author = existing.Author
		} else {
			note.Author = auth.CallerCtx(ctx)
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
		Description: "Record that a note's check passes, as of today, and re-color it. This is a DECLARATION, not an execution: it is stored as such, with your identity, and the note is marked `attested: declared`. Only COGO's own runner can produce `attested: executed`. If the cited evidence CHANGED since the note was last verified, this refuses — re-verifying must not be a way to make a drift warning disappear. Pass reanchor:true only if you actually re-checked the claim against the current content.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in verifyIn) (*mcp.CallToolResult, any, error) {
		vault, err := loadVault()
		if err != nil {
			return errResult(err), nil, nil
		}
		n, ok := vault[in.ID]
		if !ok {
			return errResult(fmt.Errorf("no note with id %q", in.ID)), nil, nil
		}
		if err := core.Verificar(n, core.LoadEvidenceRoots(dir), today(), core.Verificacion{
			Por:      auth.CallerCtx(ctx),
			Reanclar: in.Reanchor,
		}); err != nil {
			return errResult(err), nil, nil
		}
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
	Query   string            `json:"query" jsonschema:"the topic to build context for"`
	Project string            `json:"project,omitempty" jsonschema:"optional project filter"`
	Budget  int               `json:"token_budget,omitempty" jsonschema:"approximate token ceiling; 0 means unlimited"`
	Env     map[string]string `json:"env,omitempty" jsonschema:"your current environment (os, commit, runtime…), e.g. {\"os\":\"linux\"}. Notes whose declared scope conflicts get flagged so a claim isn't trusted blind on a machine it wasn't made for."`
}

type recallIn struct {
	Since   string `json:"since,omitempty" jsonschema:"optional cursor from a previous recall (RFC3339 UTC). If set, recall returns ONLY the notes that changed since then — the delta to sync another agent (or this one, post-compaction) without re-reading everything. Omit for the full load-bearing bundle. The reply always ends with a fresh cursor to pass next time."`
	Project string `json:"project,omitempty" jsonschema:"optional project. If set, recall re-anchors on THAT project's rules only: its mandate (its own red lines, or the vault-wide ones if it declared none) and its verified decisions/constraints — no noise from other projects."`
}

type stashIn struct {
	Content       string `json:"content,omitempty" jsonschema:"the artifact as text (a command's output, a log, a CSV, a config dump)"`
	ContentBase64 string `json:"content_base64,omitempty" jsonschema:"the artifact as base64, for binary content (a PDF, a screenshot)"`
	ContentType   string `json:"content_type,omitempty" jsonschema:"optional MIME type, e.g. text/plain or application/pdf"`
	Redact        bool   `json:"redact,omitempty" jsonschema:"if true, store a copy with any detected secrets masked instead of refusing"`
}

type leaseIn struct {
	Action     string `json:"action" jsonschema:"acquire | release | list"`
	Name       string `json:"name,omitempty" jsonschema:"the resource to lock, e.g. 'migrate-db' or a repo path"`
	Holder     string `json:"holder,omitempty" jsonschema:"who holds it; defaults to your authenticated identity (token label), else 'local'"`
	TTLSeconds int    `json:"ttl_seconds,omitempty" jsonschema:"how long to hold it in seconds (default 900 = 15 min)"`
	Note       string `json:"note,omitempty" jsonschema:"what you're doing while holding it"`
}

// stashBytes pulls the artifact bytes from a stash request (base64 wins if both
// are given).
func stashBytes(in stashIn) ([]byte, error) {
	if in.ContentBase64 != "" {
		return base64.StdEncoding.DecodeString(in.ContentBase64)
	}
	return []byte(in.Content), nil
}

// mandateChangedSince reports whether the mandate governing a project was
// written after the given RFC3339 cursor — a cheap signal the red lines moved.
func mandateChangedSince(dir, project, since string) bool {
	cut, err := time.Parse(time.RFC3339, since)
	if err != nil {
		return false
	}
	fi, err := os.Stat(suasion.MandatePathFor(dir, project))
	if err != nil {
		return false
	}
	return fi.ModTime().UTC().After(cut)
}

// firstLine is the note's claim trimmed to one short line for the recall delta.
func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = strings.TrimSpace(s[:i])
	}
	if r := []rune(s); len(r) > 100 {
		return string(r[:100]) + "…"
	}
	return s
}

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

// verifyIn es el input de `verify`. Lleva reanchor aparte porque re-anclar es
// una afirmación distinta de verificar: dice "comprobé la afirmación contra el
// contenido NUEVO de la evidencia que cambió".
type verifyIn struct {
	ID       string `json:"id" jsonschema:"the note id"`
	Reanchor bool   `json:"reanchor,omitempty" jsonschema:"set only if you re-checked the claim against the CURRENT content of evidence that has drifted"`
}

type evidenceIn struct {
	Kind string `json:"kind" jsonschema:"direct_log|command_output|test_result|file_read|doc|testimony|inference|hypothesis|absence"`
	Ref  string `json:"ref" jsonschema:"reference to the real artifact: commit+line, log timestamp, command+output, URL+date"`
}

type captureIn struct {
	Type       string            `json:"type" jsonschema:"one of decision|bug|runbook|architecture|constraint|command|mistake"`
	Body       string            `json:"body" jsonschema:"the note in markdown: a Claim, optional Refutation, and a Minimal check"`
	Project    string            `json:"project,omitempty" jsonschema:"the project this note belongs to"`
	ID         string            `json:"id,omitempty" jsonschema:"stable id; if omitted it is derived from the claim"`
	Evidence   []evidenceIn      `json:"evidence,omitempty" jsonschema:"supporting artifacts; each needs a kind and a ref to a real artifact"`
	CheckTest  string            `json:"check_test,omitempty" jsonschema:"the minimal test that would verify the claim"`
	DependsOn  []string          `json:"depends_on,omitempty" jsonschema:"ids of notes this one hard-depends on; a red dependency makes this note red too"`
	Supersedes string            `json:"supersedes,omitempty" jsonschema:"id of a note this one replaces; the old note is archived (buried)"`
	CausedBy   string            `json:"caused_by,omitempty" jsonschema:"id of the note that caused this finding"`
	Scope      map[string]string `json:"scope,omitempty" jsonschema:"the conditions under which the claim holds, on a vault shared across machines — e.g. {\"os\":\"windows\",\"commit\":\"abc123\",\"go\":\"1.25\"}. A claim true here may be false elsewhere; recording the scope keeps another machine from trusting it blindly."`
}

// gapIn es el input de `gap`. Deliberadamente NO tiene evidencia ni check: una
// brecha no afirma nada, así que no hay nada que respaldar. Lo que necesita es
// la pregunta y qué está trabando.
type gapIn struct {
	Question  string   `json:"question" jsonschema:"what the project does NOT know, written as a question"`
	Project   string   `json:"project,omitempty" jsonschema:"the project this gap belongs to"`
	ID        string   `json:"id,omitempty" jsonschema:"stable id; derived from the question if omitted"`
	Blocks    []string `json:"blocks,omitempty" jsonschema:"ids of the decisions waiting on this answer; the count is what orders the list"`
	Cost      string   `json:"cost_to_resolve,omitempty" jsonschema:"bajo|medio|alto — how expensive it is to find out"`
	Attempted []string `json:"attempted,omitempty" jsonschema:"what was already tried and why it fell short, so the next person does not hit the same wall"`
	Body      string   `json:"body,omitempty" jsonschema:"optional detail in markdown"`
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

// embedProvider builds the OPTIONAL embeddings client for semantic search. It
// reuses the base URL + key from .cogo/llm.json (GUI) or env, and the embedding
// model from `embed_model` in that file or COGO_EMBED_MODEL. nil if not set up.
func embedProvider(dir string) *llm.OpenAICompatible {
	var set struct {
		BaseURL    string `json:"base_url"`
		APIKey     string `json:"api_key"`
		EmbedModel string `json:"embed_model"`
	}
	em := os.Getenv("COGO_EMBED_MODEL")
	if b, err := os.ReadFile(filepath.Join(dir, ".cogo", "llm.json")); err == nil {
		_ = json.Unmarshal(b, &set)
	}
	if em == "" {
		em = set.EmbedModel
	}
	base, key := set.BaseURL, set.APIKey
	if base == "" {
		base, key = os.Getenv("COGO_LLM_BASE_URL"), os.Getenv("COGO_LLM_API_KEY")
	}
	if base == "" || em == "" {
		return nil
	}
	return &llm.OpenAICompatible{BaseURL: base, EmbedModel: em, APIKey: key, Referer: os.Getenv("COGO_LLM_REFERER")}
}

// semanticSearch ranks notes by meaning (embedding cosine). Returns (output,true)
// on success; (_, false) to signal the caller to fall back to keyword search.
func semanticSearch(ctx context.Context, dir string, vault map[string]*core.Note, cx map[string]bool, in searchIn, ep *llm.OpenAICompatible) (string, bool) {
	verdicts := core.EvaluateVault(vault, cx, today())
	state := core.Lifecycle(vault)
	var docs []embed.Doc
	for id, n := range vault {
		if !in.IncludeArchived && state[id] != core.StateActive {
			continue
		}
		if in.Project != "" && n.Project != in.Project {
			continue
		}
		docs = append(docs, embed.Doc{ID: id, Text: core.Claim(n)})
	}
	if len(docs) == 0 {
		return "", false
	}
	ids, err := embed.Rank(ctx, dir, docs, in.Query, ep)
	if err != nil {
		return "", false // any embed error → caller falls back to BM25
	}
	if in.Limit > 0 && len(ids) > in.Limit {
		ids = ids[:in.Limit]
	}
	var b strings.Builder
	b.WriteString("🔎 semántico (por significado)\n")
	for _, id := range ids {
		fmt.Fprintf(&b, "- %s `%s` — %s", verdicts[id].Color, id, clip(core.Claim(vault[id]), 100))
		if st := state[id]; st != core.StateActive {
			fmt.Fprintf(&b, " [%s]", st)
		}
		b.WriteString("\n")
	}
	return b.String(), true
}

func clip(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n-1]) + "…"
	}
	return s
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
