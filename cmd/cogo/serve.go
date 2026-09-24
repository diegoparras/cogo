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

	"github.com/diegoparras/cogo/internal/auth"
	"github.com/diegoparras/cogo/internal/core"
	"github.com/diegoparras/cogo/internal/embed"
	"github.com/diegoparras/cogo/internal/llm"
	"github.com/diegoparras/cogo/internal/suasion"
	"github.com/diegoparras/cogo/internal/tokens"
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
	// Record a per-note history line on every write (stdio and HTTP both go
	// through core.WriteNoteFile). The vault dir is derived from the note path.
	engancharEscrituras()
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
	mux.HandleFunc("/healthz", salud(*dir))
	visor := web.New(*dir, today, store)
	visor.UsarParametros(pars) // el panel edita el mismo Set que lee el motor
	visor.UsarRegistroDeUso(Consultadas)
	visor.UsarJuez(juezParaGuard(), juezParaXray(), func() float64 { return umbral("jev.umbral_tactica") })
	if j, err := journalDe(*dir); err == nil {
		visor.UsarJournal(j) // y lee el mismo registro, con su caché ya caliente
	}
	visor.Mount(mux)          // human face: visor at /, JSON API at /api
	authn.RegisterRoutes(mux) // accessory: OIDC login (federated only)

	tls := os.Getenv("COOKIE_SECURE") == "1"
	var h http.Handler = enforceReadOnly(mux) // read-only tokens can't write
	h = enforceAdmin(h)                       // issued tokens can't administer
	h = auditMiddleware(*dir)(h)              // audit trail (who called which tool); rejects batches
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
	Check    string `json:"check,omitempty" jsonschema:"id of a check declared by the vault owner in .cogo/runner.yaml. When given, COGO EXECUTES it and records the real exit code: this is the only way a note reaches attested: executed. You choose which declared check applies; you cannot supply the command"`
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
	Origin     string            `json:"origin,omitempty" jsonschema:"human|agent|instrument — who ORIGINATED the claim, which is not who is writing the note. Set 'human' only when the person actually said it; 'agent' when you chose, proposed or inferred it; 'instrument' when it came out of a command, a test or a file. It matters most for decisions and constraints: those assert that somebody CHOSE, and no evidence can prove a choice. If you decided it yourself, say 'agent' — a proposal recorded honestly is more useful than a decision nobody made."`
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
