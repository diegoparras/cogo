// Package web is the human face: a small SPA, embedded in the binary, served
// over the same HTTP server as the MCP face. It is a thin client over core —
// every endpoint just loads the vault and asks core. It also holds two pieces
// of optional runtime state: the LLM provider (configurable from the GUI) and
// the last contradiction scan (which paints notes red across every view).
package web

import (
	"archive/zip"
	"embed"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/diegoparras/cogo/internal/agentdocs"
	"github.com/diegoparras/cogo/internal/agentsmd"
	"github.com/diegoparras/cogo/internal/artifact"
	"github.com/diegoparras/cogo/internal/auth"
	"github.com/diegoparras/cogo/internal/contra"
	"github.com/diegoparras/cogo/internal/core"
	"github.com/diegoparras/cogo/internal/ghsource"
	"github.com/diegoparras/cogo/internal/history"
	"github.com/diegoparras/cogo/internal/journal"
	"github.com/diegoparras/cogo/internal/lease"
	"github.com/diegoparras/cogo/internal/lint"
	"github.com/diegoparras/cogo/internal/llm"
	"github.com/diegoparras/cogo/internal/parametros"
	"github.com/diegoparras/cogo/internal/savings"
	"github.com/diegoparras/cogo/internal/scrub"
	"github.com/diegoparras/cogo/internal/secretscan"
	"github.com/diegoparras/cogo/internal/tokens"
	"github.com/diegoparras/cogo/internal/xray"
)

//go:embed assets
var assetsFS embed.FS

// Version is shown in the "Acerca de" modal.
const Version = "0.1.0"

type Server struct {
	dir    string
	today  func() core.Date
	tokens *tokens.Store
	contra *contra.Store
	cache  *core.VaultCache // mtime-keyed vault reads (scale past a few thousand notes)

	// pars son los parámetros del vault, compartidos con el motor. Ver
	// parametros.go: el panel los edita y el motor los lee, sobre la misma
	// instancia, para que un cambio valga en la evaluación siguiente.
	pars *parametros.Set
	// registro es el journal compartido del proceso (ver parametros.go).
	registro *journal.Journal
	// anotarUso registra qué notas se consultaron (ver internal/uso).
	anotarUso func(ids ...string)

	mu             sync.RWMutex
	provider       llm.Provider
	contradictions map[string]bool
	scrubber       scrub.Scrubber
}

func New(dir string, today func() core.Date, store *tokens.Store) *Server {
	s := &Server{dir: dir, today: today, tokens: store, contradictions: map[string]bool{}, cache: core.NewVaultCache(dir)}
	s.provider = s.loadProvider()
	s.scrubber = scrub.FromEnv()
	if u, err := readUsage(dir); err == nil {
		llm.SeedUsage(u) // resume the cumulative token tally across restarts
	}
	s.contra = contra.Open(dir)               // persisted contradictions
	s.contradictions = s.contra.OpenNoteSet() // survive a restart: red from the start
	return s
}

func usagePath(dir string) string { return filepath.Join(dir, ".cogo", "usage.json") }

func readUsage(dir string) (llm.TokenUsage, error) {
	var u llm.TokenUsage
	b, err := os.ReadFile(usagePath(dir))
	if err != nil {
		return u, err
	}
	return u, json.Unmarshal(b, &u)
}

// flushUsage persists the running token tally next to the vault (best-effort),
// so the counter is cumulative across restarts. Called after model-using calls.
func (s *Server) flushUsage() {
	b, err := json.Marshal(llm.Usage())
	if err != nil {
		return
	}
	_ = os.MkdirAll(filepath.Join(s.dir, ".cogo"), 0o755)
	_ = os.WriteFile(usagePath(s.dir), b, 0o644)
}

// Mount registers the SPA and the JSON API on the given mux.
func (s *Server) Mount(mux *http.ServeMux) {
	sub, err := fs.Sub(assetsFS, "assets")
	if err != nil {
		panic(err)
	}
	mux.Handle("/", servidorDeAssets(sub)) // con ETag por contenido: ver assets_http.go
	mux.HandleFunc("/api/config", s.handleConfig)
	mux.HandleFunc("/api/notes", s.handleNotes)
	mux.HandleFunc("/api/pack", s.handlePack)
	mux.HandleFunc("/api/graph", s.handleGraph)
	mux.HandleFunc("/api/note", s.handleNote)
	mux.HandleFunc("/api/note/history", s.handleHistory)
	mux.HandleFunc("/api/parametros", s.handleParametros)
	mux.HandleFunc("/api/salud", s.handleSalud)
	mux.HandleFunc("/api/salaguerra", s.handleSalaGuerra)
	mux.HandleFunc("/api/verify", s.handleVerify)
	mux.HandleFunc("/api/archive", s.handleArchive)
	mux.HandleFunc("/api/restore", s.handleRestore)
	mux.HandleFunc("/api/delete", s.handleDelete)
	mux.HandleFunc("/api/trash", s.handleTrash)
	mux.HandleFunc("/api/preview", s.handlePreview)
	mux.HandleFunc("/api/capture", s.handleCapture)
	mux.HandleFunc("/api/lint", s.handleLint)
	mux.HandleFunc("/api/contradictions", s.handleContradictions)
	mux.HandleFunc("/api/settings", s.handleSettings)
	mux.HandleFunc("/api/settings/test", s.handleTestLLM)
	mux.HandleFunc("/api/settings/test-embed", s.handleTestEmbed)
	mux.HandleFunc("/api/settings/models", s.handleModels)
	mux.HandleFunc("/api/guard", s.handleGuard)
	mux.HandleFunc("/api/xray", s.handleXray)
	mux.HandleFunc("/api/guard/label", s.handleGuardLabel)
	mux.HandleFunc("/api/mandate", s.handleMandate)
	mux.HandleFunc("/api/tokens", s.handleTokens)
	mux.HandleFunc("/api/audit", s.handleAudit)
	mux.HandleFunc("/api/artifact", s.handleArtifact)
	mux.HandleFunc("/api/leases", s.handleLeases)
	mux.HandleFunc("/api/github", s.handleGitHub)
	mux.HandleFunc("/api/github/map", s.handleGitHubMap)
	mux.HandleFunc("/api/export", s.handleExport)
	mux.HandleFunc("/api/evidence-roots", s.handleEvidenceRoots)
	mux.HandleFunc("/api/agents-md", s.handleAgentsMD)
	mux.HandleFunc("/api/agent-blocks", s.handleAgentBlocks)
	mux.HandleFunc("/api/agent-docs", s.handleAgentDocs)
}

// handleAgentDocs manages the agent-instruction files a user authors (AGENTS.md,
// CLAUDE.md, …), stored in the vault with a version history. GET lists them, or
// with ?name= returns one doc + its history; POST saves; DELETE removes.
func (s *Server) handleAgentDocs(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		if name := r.URL.Query().Get("name"); name != "" {
			content, err := agentdocs.Load(s.dir, name)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			writeJSON(w, map[string]any{"name": agentdocs.SafeName(name), "content": content, "history": agentdocs.History(s.dir, name)})
			return
		}
		writeJSON(w, map[string]any{"docs": agentdocs.List(s.dir), "known": agentdocs.Known})
	case http.MethodPost:
		var in struct {
			Name    string `json:"name"`
			Content string `json:"content"`
		}
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if agentdocs.SafeName(in.Name) == "" {
			writeJSON(w, map[string]any{"ok": false, "error": "nombre inválido (usá algo como CLAUDE.md)"})
			return
		}
		if err := agentdocs.Save(s.dir, in.Name, in.Content, s.today().String()+"T00:00:00Z"); err != nil {
			writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		writeJSON(w, map[string]any{"ok": true})
	case http.MethodDelete:
		if err := agentdocs.Delete(s.dir, r.URL.Query().Get("name")); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, map[string]any{"ok": true})
	default:
		http.Error(w, "GET, POST or DELETE", http.StatusMethodNotAllowed)
	}
}

// handleAgentsMD generates the bootstrap file (AGENTS.md/CLAUDE.md) that teaches
// a coding agent the COGO protocol and how to connect over MCP. ?tool=claude
// names it CLAUDE.md; ?digest=1 embeds a static snapshot of the green/yellow
// notes. The connection snippet points at this server's own /mcp URL.
func (s *Server) handleAgentsMD(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "GET only", http.StatusMethodNotAllowed)
		return
	}
	name := "AGENTS.md"
	if r.URL.Query().Get("tool") == "claude" {
		name = "CLAUDE.md"
	}
	scheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	opts := agentsmd.Options{Filename: name, HTTPURL: scheme + "://" + r.Host + "/mcp"}
	if r.URL.Query().Get("digest") == "1" {
		vault, ok := s.load(w)
		if !ok {
			return
		}
		verdicts := core.EvaluateVault(vault, s.contras(), s.today())
		items := make([]agentsmd.DigestItem, 0, len(vault))
		for id, n := range vault {
			if n.Status != "" {
				continue // archived/retracted are not part of the live memory
			}
			items = append(items, agentsmd.DigestItem{Color: verdicts[id].Color.String(), ID: id, Claim: core.Claim(n)})
		}
		sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })
		opts.Digest = agentsmd.RenderDigest(items)
		opts.Date = s.today().String()
	}
	writeJSON(w, map[string]any{"filename": name, "markdown": agentsmd.Generate(opts)})
}

// handleAgentBlocks serves the reusable pieces an instruction file is composed
// of: COGO's curated blocks (the wording stays canonical here instead of being
// retyped into every AGENTS.md), the recommended presets, and the user's OWN
// blocks — so a recurring instruction becomes a one-click piece too.
//
//	GET    ?project=&token=  → {blocks, presets}
//	POST   {id,title,desc,markdown} → saves a custom block
//	DELETE ?id=              → removes a custom block
func (s *Server) handleAgentBlocks(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		scheme := "http"
		if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
			scheme = "https"
		}
		opts := agentsmd.BlockOptions{
			HTTPURL: scheme + "://" + r.Host + "/mcp",
			Project: r.URL.Query().Get("project"),
			Token:   r.URL.Query().Get("token"),
		}
		blocks := append(agentsmd.Curated(opts), agentsmd.LoadCustom(s.dir)...)
		// Token labels help the operator pick which agent this file is for; the
		// secret itself is hashed and unrecoverable, so it is never returned.
		labels := []string{}
		if s.tokens != nil {
			for _, t := range s.tokens.List() {
				labels = append(labels, t.Label)
			}
		}
		writeJSON(w, map[string]any{"blocks": blocks, "presets": agentsmd.Presets(), "token_labels": labels})
	case http.MethodPost:
		var b agentsmd.Block
		if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := agentsmd.SaveCustom(s.dir, b); err != nil {
			writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		writeJSON(w, map[string]any{"ok": true, "blocks": agentsmd.LoadCustom(s.dir)})
	case http.MethodDelete:
		if err := agentsmd.DeleteCustom(s.dir, r.URL.Query().Get("id")); err != nil {
			writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		writeJSON(w, map[string]any{"ok": true, "blocks": agentsmd.LoadCustom(s.dir)})
	default:
		http.Error(w, "GET, POST or DELETE", http.StatusMethodNotAllowed)
	}
}

// handleExport streams the whole vault as a zip so a user can back it up or move
// it to another machine. Every note plus the human catalog (index.md, log.md) is
// included; .cogo (local state — usage counters, hashed token secrets, history)
// is deliberately excluded, so the archive is portable and carries no secrets.
func (s *Server) handleExport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "GET only", http.StatusMethodNotAllowed)
		return
	}
	name := "cogo-vault-" + s.today().String() + ".zip"
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
	zw := zip.NewWriter(w)
	defer func() { _ = zw.Close() }()
	_ = filepath.WalkDir(s.dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == ".cogo" {
				return fs.SkipDir
			}
			return nil
		}
		rel, err := filepath.Rel(s.dir, path)
		if err != nil {
			return err
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		fw, err := zw.Create(filepath.ToSlash(rel))
		if err != nil {
			return err
		}
		_, err = fw.Write(b)
		return err
	})
}

// handleEvidenceRoots reads or updates the per-project evidence roots. GET also
// returns the distinct project names present in the vault, so the UI can offer
// them without the user retyping. Admin-only (blocked for read-only tokens).
func (s *Server) handleEvidenceRoots(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		roots := s.evRoots()
		vault, ok := s.load(w)
		if !ok {
			return
		}
		set := map[string]bool{}
		for _, n := range vault {
			if n.Project != "" {
				set[n.Project] = true
			}
		}
		known := make([]string, 0, len(set))
		for p := range set {
			known = append(known, p)
		}
		sort.Strings(known)
		writeJSON(w, map[string]any{"default": roots.Default(), "projects": roots.Projects(), "known_projects": known})
	case http.MethodPost:
		var in struct {
			Default  string            `json:"default"`
			Projects map[string]string `json:"projects"`
		}
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := core.SaveEvidenceRoots(s.dir, in.Default, in.Projects); err != nil {
			writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		writeJSON(w, map[string]any{"ok": true})
	default:
		http.Error(w, "GET or POST", http.StatusMethodNotAllowed)
	}
}

// auditCap mirrors the writer-side cap (cmd/cogo defaultAuditMax) so the visor
// can tell the operator how many entries the log retains. Kept in sync by hand.
func auditCap() int {
	if v := os.Getenv("COGO_AUDIT_MAX"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return 5000
}

// auditLine is the parsed shape of one audit entry, used to match a single row
// for deletion. All fields are strings so two entries compare with ==.
type auditLine struct {
	Time   string `json:"time"`
	Caller string `json:"caller"`
	Tool   string `json:"tool"`
	Method string `json:"method"`
	Path   string `json:"path"`
	IP     string `json:"ip"`
}

// handleAudit serves and manages the MCP/API audit trail:
//
//	GET               → the most recent 300 entries + total + cap
//	GET  ?download=1  → the raw audit.jsonl as a file attachment
//	DELETE            → clear the whole log (no body)
//	DELETE  +body     → remove the single entry matching the posted row
func (s *Server) handleAudit(w http.ResponseWriter, r *http.Request) {
	path := filepath.Join(s.dir, ".cogo", "audit.jsonl")
	switch r.Method {
	case http.MethodGet:
		b, _ := os.ReadFile(path)
		if r.URL.Query().Get("download") != "" {
			w.Header().Set("Content-Type", "application/x-ndjson")
			w.Header().Set("Content-Disposition", `attachment; filename="cogo-audit.jsonl"`)
			_, _ = w.Write(b)
			return
		}
		lines := auditLines(b)
		entries := []json.RawMessage{}
		for i := len(lines) - 1; i >= 0 && len(entries) < 300; i-- {
			entries = append(entries, json.RawMessage(lines[i]))
		}
		writeJSON(w, map[string]any{"entries": entries, "total": len(lines), "cap": auditCap()})
	case http.MethodDelete:
		var target auditLine
		hasTarget := json.NewDecoder(r.Body).Decode(&target) == nil &&
			(target.Time != "" || target.Path != "" || target.Tool != "")
		if !hasTarget { // no body → clear everything
			_ = os.Remove(path)
			writeJSON(w, map[string]any{"ok": true, "total": 0})
			return
		}
		b, err := os.ReadFile(path)
		if err != nil {
			writeJSON(w, map[string]any{"ok": true, "removed": false, "total": 0})
			return
		}
		lines := auditLines(b)
		kept := make([]string, 0, len(lines))
		removed := false
		for _, ln := range lines {
			if !removed {
				var e auditLine
				if json.Unmarshal([]byte(ln), &e) == nil && e == target {
					removed = true
					continue
				}
			}
			kept = append(kept, ln)
		}
		out := ""
		if len(kept) > 0 {
			out = strings.Join(kept, "\n") + "\n"
		}
		_ = os.WriteFile(path, []byte(out), 0o644)
		writeJSON(w, map[string]any{"ok": true, "removed": removed, "total": len(kept)})
	default:
		http.Error(w, "GET or DELETE", http.StatusMethodNotAllowed)
	}
}

// auditLines splits a jsonl blob into its non-empty lines.
func auditLines(b []byte) []string {
	if len(b) == 0 {
		return nil
	}
	raw := strings.Split(strings.TrimRight(string(b), "\n"), "\n")
	out := make([]string, 0, len(raw))
	for _, ln := range raw {
		if strings.TrimSpace(ln) != "" {
			out = append(out, ln)
		}
	}
	return out
}

// handleArtifact stores and serves content-addressed artifacts. POST stashes
// content (JSON: content or content_base64) behind the secret guard and returns
// its `artifact://<sha>` ref to cite as evidence; GET ?sha= streams the bytes
// back (with integrity re-checked by the store). The store is R2 when COGO_R2_*
// is set, else disk under the vault.
func (s *Server) handleArtifact(w http.ResponseWriter, r *http.Request) {
	store := artifact.FromEnv(s.dir)
	switch r.Method {
	case http.MethodGet:
		sha := strings.TrimSpace(r.URL.Query().Get("sha"))
		if sha == "" {
			http.Error(w, "sha required", http.StatusBadRequest)
			return
		}
		b, err := store.Get(r.Context(), sha)
		if err == artifact.ErrNotFound {
			http.Error(w, "artifact not found", http.StatusNotFound)
			return
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		_, _ = w.Write(b)
	case http.MethodPost:
		var in struct {
			Content       string `json:"content"`
			ContentBase64 string `json:"content_base64"`
			ContentType   string `json:"content_type"`
			Redact        bool   `json:"redact"`
		}
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		data := []byte(in.Content)
		if in.ContentBase64 != "" {
			d, err := base64.StdEncoding.DecodeString(in.ContentBase64)
			if err != nil {
				http.Error(w, "bad content_base64", http.StatusBadRequest)
				return
			}
			data = d
		}
		if len(data) == 0 {
			http.Error(w, "content required", http.StatusBadRequest)
			return
		}
		out, findings, blocked := secretscan.Guard(data, in.Redact)
		if blocked {
			writeJSON(w, map[string]any{"ok": false, "blocked": true, "findings": findings})
			return
		}
		sha, err := store.Put(r.Context(), out, in.ContentType)
		if err != nil {
			writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		writeJSON(w, map[string]any{"ok": true, "sha": sha, "ref": core.ArtifactRef(sha), "backend": store.Backend(), "findings": findings})
	default:
		http.Error(w, "GET or POST", http.StatusMethodNotAllowed)
	}
}

// handleLeases lists the currently-held coordination leases (who holds what,
// until when). Read-only visibility for the visor; agents acquire/release via
// the MCP `lease` tool.
func (s *Server) handleLeases(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, map[string]any{"leases": lease.Open(s.dir).List(time.Now())})
	case http.MethodDelete: // operator override: force-release a lease by name
		name := strings.TrimSpace(r.URL.Query().Get("name"))
		if name == "" {
			http.Error(w, "name required", http.StatusBadRequest)
			return
		}
		writeJSON(w, map[string]any{"ok": true, "released": lease.Open(s.dir).ForceRelease(name)})
	default:
		http.Error(w, "GET or DELETE", http.StatusMethodNotAllowed)
	}
}

// handleGitHub is the repository explorer: it lists directories and shows files
// so you can find what you want to cite WITHOUT leaving COGO and hunting the
// path on github.com. Read-only and stateless — nothing of the repo is stored;
// what COGO persists is the citation you build from here.
//
//	GET ?repo=owner/name&ref=&path=        → listado del directorio
//	GET ?repo=owner/name&ref=&path=&file=1 → contenido del archivo
func (s *Server) handleGitHub(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "GET only", http.StatusMethodNotAllowed)
		return
	}
	full := strings.TrimSpace(r.URL.Query().Get("repo"))
	owner, repo, ok := strings.Cut(strings.TrimSuffix(strings.TrimPrefix(full, "https://github.com/"), ".git"), "/")
	if !ok || owner == "" || repo == "" {
		writeJSON(w, map[string]any{"ok": false, "error": "escribí el repo como owner/nombre"})
		return
	}
	repo = strings.TrimSuffix(repo, "/")
	ref := strings.TrimSpace(r.URL.Query().Get("ref"))
	path := strings.Trim(strings.TrimSpace(r.URL.Query().Get("path")), "/")
	gh := ghsource.FromEnv()

	if r.URL.Query().Get("file") != "" {
		content, sha, htmlURL, err := gh.FileContent(r.Context(), owner, repo, ref, path)
		if err != nil {
			writeJSON(w, map[string]any{"ok": false, "error": err.Error(), "html_url": htmlURL})
			return
		}
		writeJSON(w, map[string]any{
			"ok": true, "kind": "file", "path": path, "sha": sha, "html_url": htmlURL,
			"lines": strings.Split(strings.ReplaceAll(string(content), "\r\n", "\n"), "\n"),
		})
		return
	}
	items, err := gh.Tree(r.Context(), owner, repo, ref, path)
	if err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, map[string]any{"ok": true, "kind": "dir", "path": path, "entries": items,
		"authenticated": gh.Authenticated()})
}

// handleGitHubMap builds the repository's "confidence map": the folder tree
// crossed with the vault's citations. Each cited file takes the color of the
// notes that cite it (the worst one wins — a red note is the one you need to
// see), and each folder reports how many of its files have NO memory at all.
//
// That last number is the point of the whole view: a file browser tells you what
// exists, this tells you where your knowledge is solid and where you are blind.
func (s *Server) handleGitHubMap(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "GET only", http.StatusMethodNotAllowed)
		return
	}
	full := strings.TrimSpace(r.URL.Query().Get("repo"))
	owner, repo, ok := strings.Cut(strings.TrimSuffix(strings.TrimPrefix(full, "https://github.com/"), ".git"), "/")
	if !ok || owner == "" || repo == "" {
		writeJSON(w, map[string]any{"ok": false, "error": "escribí el repo como owner/nombre"})
		return
	}
	repo, ref := strings.TrimSuffix(repo, "/"), strings.TrimSpace(r.URL.Query().Get("ref"))

	// 1. Qué archivos de ESTE repo cita el vault, y con qué color.
	vault, okv := s.load(w)
	if !okv {
		return
	}
	verdicts := core.EvaluateVault(vault, s.contras(), s.today())
	hidden := core.Hidden(vault)
	type cite struct {
		ID    string `json:"id"`
		Color string `json:"color"`
		Line  string `json:"line,omitempty"`
	}
	cited := map[string][]cite{}
	for id, n := range vault {
		if hidden[id] {
			continue
		}
		for _, e := range n.Evidence {
			o, rp, _, path, okr := core.ParseGitHubRef(e.Ref)
			if !okr || !strings.EqualFold(o, owner) || !strings.EqualFold(rp, repo) {
				continue
			}
			cited[path] = append(cited[path], cite{ID: id, Color: verdicts[id].Color.String()})
		}
	}

	// 2. El árbol del repo, en una sola llamada.
	paths, truncated, err := ghsource.FromEnv().FullTree(r.Context(), owner, repo, ref)
	if err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
		return
	}

	// 3. Nodos: TODAS las carpetas (el esqueleto del repo) y solo los archivos
	// citados. Dibujar cada archivo volvería el mapa ilegible en cualquier repo
	// real; lo que importa por carpeta es cuántos quedaron sin memoria.
	worst := func(cs []cite) string {
		rank := map[string]int{"red": 0, "yellow": 1, "green": 2, "ungraded": 3}
		out := "ungraded"
		for _, c := range cs {
			if rank[c.Color] < rank[out] {
				out = c.Color
			}
		}
		return out
	}
	type node struct {
		ID      string `json:"id"`
		Type    string `json:"type"`
		Color   string `json:"color"`
		Claim   string `json:"claim,omitempty"`
		Files   int    `json:"files,omitempty"`   // archivos dentro (solo carpetas)
		Blind   int    `json:"blind,omitempty"`   // …sin ninguna nota que los cite
		Notes   []cite `json:"notes,omitempty"`   // notas que citan este archivo
		Project string `json:"project,omitempty"` // reutiliza el tooltip del motor
		Size    int    `json:"size,omitempty"`    // bytes (archivos)
		Ext     string `json:"ext,omitempty"`     // extensión, para agrupar en el árbol
	}
	// Dibujar TODO el repo (no solo lo citado) es lo que hace legible el mapa: se
	// ve la forma real del proyecto y las islas de memoria resaltan sobre lo
	// neutro. En un repo enorme eso es ilegible e inmanejable, así que por encima
	// del tope se cae al esqueleto (carpetas + archivos citados) y se avisa.
	const maxNodes = 700
	fileCount := 0
	for _, p := range paths {
		if p.Type != "dir" {
			fileCount++
		}
	}
	dense := fileCount > maxNodes

	files, blind := map[string]int{}, map[string]int{}
	dirs := map[string]bool{"": true}
	var nodes []node
	for _, p := range paths {
		if p.Type == "dir" {
			dirs[p.Path] = true
			continue
		}
		d := ""
		if i := strings.LastIndex(p.Path, "/"); i >= 0 {
			d = p.Path[:i]
		}
		files[d]++
		cs, hit := cited[p.Path]
		if !hit {
			blind[d]++
		}
		if !hit && dense {
			continue // repo grande: solo el esqueleto
		}
		n := node{ID: p.Path, Type: "file", Color: "ungraded", Size: p.Size, Ext: ext(p.Path)}
		if hit {
			n.Color, n.Notes = worst(cs), cs
			n.Claim = fmt.Sprintf("%d nota(s) lo citan", len(cs))
		} else {
			n.Claim = "sin memoria: ninguna nota lo cita"
		}
		nodes = append(nodes, n)
	}
	for d := range dirs {
		name := d
		if name == "" {
			name = repo
		}
		color := "ungraded"
		if files[d] > 0 && blind[d] == 0 {
			color = "green" // toda la carpeta tiene memoria
		} else if files[d] > blind[d] {
			color = "yellow" // parcialmente cubierta
		}
		nodes = append(nodes, node{ID: name, Type: "dir", Color: color, Files: files[d], Blind: blind[d],
			Claim: fmt.Sprintf("%d archivo(s), %d sin memoria", files[d], blind[d])})
	}

	// 4. Aristas padre → hijo.
	edges := []map[string]string{}
	parentOf := func(p string) string {
		if i := strings.LastIndex(p, "/"); i >= 0 {
			return p[:i]
		}
		return repo // la raíz lleva el nombre del repo
	}
	for _, n := range nodes {
		if n.ID == repo {
			continue
		}
		from := parentOf(n.ID)
		if from == "" {
			from = repo
		}
		if !dirs[from] && from != repo {
			continue
		}
		edges = append(edges, map[string]string{"from": from, "to": n.ID, "kind": "contains"})
	}
	writeJSON(w, map[string]any{"ok": true, "nodes": nodes, "edges": edges,
		"truncated": truncated, "cited": len(cited), "repo": owner + "/" + repo,
		"dense": dense, "total_files": fileCount, "ref": ref})
}

// handleTokens manages the issued MCP access tokens: GET lists them (no
// secrets), POST creates one (returns the plaintext ONCE), DELETE revokes by id.
func (s *Server) handleTokens(w http.ResponseWriter, r *http.Request) {
	if s.tokens == nil {
		http.Error(w, "token store unavailable", http.StatusServiceUnavailable)
		return
	}
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, map[string]any{"tokens": s.tokens.List()})
	case http.MethodPost:
		var in struct {
			Label       string `json:"label"`
			ExpiresDays int    `json:"expires_days"`
			ReadOnly    bool   `json:"readonly"`
		}
		_ = json.NewDecoder(r.Body).Decode(&in)
		expires := ""
		if in.ExpiresDays > 0 {
			expires = s.today().AddDays(in.ExpiresDays).String()
		}
		secret, t, err := s.tokens.Create(in.Label, expires, in.ReadOnly, s.today().String())
		if err != nil {
			writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		writeJSON(w, map[string]any{"ok": true, "token": secret, "item": t})
	case http.MethodDelete:
		id := r.URL.Query().Get("id")
		if !s.tokens.Revoke(id) {
			http.Error(w, "no such token", http.StatusNotFound)
			return
		}
		writeJSON(w, map[string]any{"ok": true, "id": id})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) contras() map[string]bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.contradictions
}
func (s *Server) prov() llm.Provider { s.mu.RLock(); defer s.mu.RUnlock(); return s.provider }

func (s *Server) load(w http.ResponseWriter) (map[string]*core.Note, bool) {
	vault, err := s.cache.Load()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return nil, false
	}
	core.ResolveEvidence(vault, s.evRoots()) // the teeth: check that evidence refs resolve
	return vault, true
}

// evRoots reads the per-project evidence roots fresh each call (tiny file), so a
// change made in the UI takes effect on the next request without a restart.
func (s *Server) evRoots() core.EvidenceRoots { return core.LoadEvidenceRoots(s.dir) }

func (s *Server) handleConfig(w http.ResponseWriter, r *http.Request) {
	vault, ok := s.load(w)
	if !ok {
		return
	}
	set := map[string]bool{}
	for _, n := range vault {
		if n.Project != "" {
			set[n.Project] = true
		}
	}
	projects := make([]string, 0, len(set))
	for p := range set {
		projects = append(projects, p)
	}
	sort.Strings(projects)
	u := llm.Usage()
	sv := savings.Read(s.dir)
	writeJSON(w, map[string]any{
		"version": Version, "projects": projects, "count": len(vault),
		"llm_configured": s.prov().Available(),
		"scrub_enabled":  s.scrubber.Enabled(),
		"evidence_root":  s.evRoots().Configured(),
		"tokens":         u.Total, "token_calls": u.Calls,
		"saved_tokens": sv.Total, "saved_packs": sv.Packs,
		"artifact_backend": artifact.FromEnv(s.dir).Backend(),
	})
}

// handleNotes lista el vault como un ÍNDICE, no como un volcado: busca, filtra,
// ordena y pagina del lado del servidor. Con un puñado de notas da igual, pero
// una lista plana de todo deja de servir mucho antes de lo que uno cree — no se
// encuentra nada y el navegador dibuja cientos de tarjetas que nadie mira.
//
//	?q=      texto (ranking BM25/semántico, el mismo del tool `search`)
//	?project= ?author= ?color=   filtros
//	?sort=   atencion (default, rojo primero) | reciente | antigua
//	?limit= ?offset=             paginación (default 50)
//	?archived=1                  incluir archivadas/retractadas
//
// Devuelve además las facetas (proyectos y autores con su conteo) para que la
// barra de filtros muestre lo que hay, sin tener que traerse el vault entero.
func (s *Server) handleNotes(w http.ResponseWriter, r *http.Request) {
	vault, ok := s.load(w)
	if !ok {
		return
	}
	q := r.URL.Query()
	views := core.Overview(vault, s.contras(), s.today(), archivedParam(r))

	// La fecha de creación no vive en la nota: es la primera línea de su historial.
	for i := range views {
		views[i].Created = history.CreatedAt(s.dir, views[i].ID)
	}

	// Facetas sobre el conjunto completo (antes de filtrar): la barra tiene que
	// ofrecer todos los proyectos y autores, no solo los del filtro activo.
	countBy := func(get func(core.NoteView) string) []map[string]any {
		n := map[string]int{}
		for _, v := range views {
			if k := get(v); k != "" {
				n[k]++
			}
		}
		keys := make([]string, 0, len(n))
		for k := range n {
			keys = append(keys, k)
		}
		sort.Slice(keys, func(i, j int) bool {
			if n[keys[i]] != n[keys[j]] {
				return n[keys[i]] > n[keys[j]]
			}
			return keys[i] < keys[j]
		})
		out := make([]map[string]any, 0, len(keys))
		for _, k := range keys {
			out = append(out, map[string]any{"name": k, "count": n[k]})
		}
		return out
	}
	facets := map[string]any{
		"projects": countBy(func(v core.NoteView) string { return v.Project }),
		"authors":  countBy(func(v core.NoteView) string { return v.Author }),
		"colors":   countBy(func(v core.NoteView) string { return v.Color }),
		// Rango real de fechas de creación, para que el selector de fechas ofrezca
		// el período que EXISTE en vez de un calendario infinito.
		"dates": datesFacet(views),
	}

	// Búsqueda: reusa el ranking del tool `search` (BM25) y respeta su orden.
	if query := strings.TrimSpace(q.Get("q")); query != "" {
		rank := map[string]int{}
		for i, res := range core.Search(vault, s.contras(), query, q.Get("project"), s.today(), 0, archivedParam(r)) {
			rank[res.ID] = i + 1
		}
		kept := views[:0]
		for _, v := range views {
			if rank[v.ID] > 0 {
				kept = append(kept, v)
			}
		}
		views = kept
		sort.SliceStable(views, func(i, j int) bool { return rank[views[i].ID] < rank[views[j].ID] })
	}

	// Filtros simples.
	keep := func(f func(core.NoteView) bool) {
		out := views[:0]
		for _, v := range views {
			if f(v) {
				out = append(out, v)
			}
		}
		views = out
	}
	if p := q.Get("project"); p != "" {
		keep(func(v core.NoteView) bool { return v.Project == p })
	}
	if a := q.Get("author"); a != "" {
		keep(func(v core.NoteView) bool { return v.Author == a })
	}
	// ?ids=a,b,c — resuelve un puñado de notas concretas de una sola vez. Lo usa el
	// editor para pintar las relaciones YA elegidas con su color, sin pedir una por
	// una ni (como antes) bajarse el vault entero para encontrarlas.
	if ids := q.Get("ids"); ids != "" {
		set := map[string]bool{}
		for _, x := range strings.Split(ids, ",") {
			if x = strings.TrimSpace(x); x != "" {
				set[x] = true
			}
		}
		keep(func(v core.NoteView) bool { return set[v.ID] })
	}
	if c := q.Get("color"); c != "" {
		set := map[string]bool{}
		for _, x := range strings.Split(c, ",") {
			set[strings.TrimSpace(x)] = true
		}
		keep(func(v core.NoteView) bool { return set[v.Color] })
	}
	// Rango de fechas de CREACIÓN (YYYY-MM-DD, ambos extremos incluidos). Las
	// fechas ISO se comparan como texto sin ambigüedad. Una nota sin historial no
	// tiene fecha conocida: si hay filtro activo queda fuera, porque "no sé cuándo
	// se creó" no es lo mismo que "se creó en este rango".
	from, to := q.Get("from"), q.Get("to")
	if from != "" || to != "" {
		keep(func(v core.NoteView) bool {
			if len(v.Created) < 10 {
				return false
			}
			d := v.Created[:10]
			return (from == "" || d >= from) && (to == "" || d <= to)
		})
	}

	// Orden. El default es la opinión de COGO: primero lo que necesita atención.
	// Por fecha se usa la verificación, que es lo que de verdad envejece.
	switch q.Get("sort") {
	case "reciente":
		sort.SliceStable(views, func(i, j int) bool { return views[i].Verified > views[j].Verified })
	case "antigua":
		sort.SliceStable(views, func(i, j int) bool { return views[i].Verified < views[j].Verified })
	case "nueva": // por creación: la fecha que el visor ahora muestra en cada tarjeta
		sort.SliceStable(views, func(i, j int) bool { return views[i].Created > views[j].Created })
	case "vieja":
		sort.SliceStable(views, func(i, j int) bool { return views[i].Created < views[j].Created })
	}

	total := len(views)
	offset, _ := strconv.Atoi(q.Get("offset"))
	// "todas" es una opción explícita del paginador. Sigue siendo UNA petición: el
	// servidor decide cuánto manda, y el navegador nunca improvisa un "traé todo".
	limit := total
	if q.Get("limit") != "all" {
		limit, _ = strconv.Atoi(q.Get("limit"))
		if limit <= 0 {
			limit = 50
		}
	}
	if offset < 0 || offset > total {
		offset = total
	}
	end := offset + limit
	if end > total {
		end = total
	}
	page := views[offset:end]
	if page == nil {
		page = []core.NoteView{}
	}
	writeJSON(w, map[string]any{
		"notes": page, "total": total, "offset": offset, "limit": limit, "facets": facets,
	})
}

// datesFacet devuelve la fecha de creación más vieja y la más nueva del vault.
// El selector de fechas las usa para acotarse a lo que realmente existe: ofrecer
// un calendario abierto donde no hay notas es prometer resultados que no hay.
func datesFacet(views []core.NoteView) map[string]any {
	min, max := "", ""
	for _, v := range views {
		if len(v.Created) < 10 {
			continue
		}
		d := v.Created[:10]
		if min == "" || d < min {
			min = d
		}
		if max == "" || d > max {
			max = d
		}
	}
	return map[string]any{"min": min, "max": max}
}

// archivedParam reads the "?archived=1" toggle used by views that can optionally
// show the notes that are normally hidden (archived, retracted, superseded).
// normalizarCuerpo colapsa los espacios en blanco para que reformatear no
// cuente como cambio de contenido. No normaliza nada más: mayúsculas, signos y
// palabras son contenido.
func normalizarCuerpo(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// truthy lee un parámetro de query como booleano, con las grafías que la gente
// escribe de verdad.
func truthy(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes", "si", "sí":
		return true
	}
	return false
}

func archivedParam(r *http.Request) bool {
	switch strings.ToLower(r.URL.Query().Get("archived")) {
	case "1", "true", "yes":
		return true
	}
	return false
}

func (s *Server) handlePack(w http.ResponseWriter, r *http.Request) {
	vault, ok := s.load(w)
	if !ok {
		return
	}
	budget, _ := strconv.Atoi(r.URL.Query().Get("budget"))
	p := core.BuildPack(vault, s.contras(), core.PackOptions{
		Query:   r.URL.Query().Get("query"),
		Project: r.URL.Query().Get("project"),
		Budget:  budget,
		Today:   s.today(),
	})
	s.consultadas(p.Incluidas...) // el humano también consume, y también despierta
	savings.Add(s.dir, p.RawTokens-p.Tokens, s.today().String())
	writeJSON(w, map[string]any{
		"markdown": p.Markdown, "tokens": p.Tokens, "raw_tokens": p.RawTokens,
		"greens": p.Greens, "yellows": p.Yellows, "reds": p.Reds,
		"mistakes": p.Mistakes, "dropped": p.Dropped,
	})
}

func (s *Server) handleGraph(w http.ResponseWriter, r *http.Request) {
	vault, ok := s.load(w)
	if !ok {
		return
	}
	writeJSON(w, core.BuildGraph(vault, s.contras(), s.today(), archivedParam(r)))
}

// handleNote returns one note's editable inputs (plus its computed color), so
// the web editor can prefill its form.
func (s *Server) handleNote(w http.ResponseWriter, r *http.Request) {
	vault, ok := s.load(w)
	if !ok {
		return
	}
	n, found := vault[r.URL.Query().Get("id")]
	if !found {
		http.Error(w, "no such note", http.StatusNotFound)
		return
	}
	v := core.Evaluate(n, vault, s.contras(), s.today())
	var conflicts []contra.Conflict
	if s.contra != nil {
		conflicts = s.contra.ForNote(n.ID) // the trace behind a red-by-contradiction verdict
	}
	writeJSON(w, map[string]any{
		"id": n.ID, "type": n.Type, "project": n.Project, "body": n.Body,
		"evidence": n.Evidence, "check_test": n.Check.Test,
		"depends_on": n.DependsOn, "supersedes": n.Supersedes, "caused_by": n.CausedBy,
		"color": v.Color.String(), "reason": v.Reason, "stale_at": v.StaleAt.String(),
		"contradictions": conflicts, "author": n.Author, "scope": n.Scope,
		// Todo lo que el editor puede escribir tiene que poder leerse de vuelta, o
		// abrir una nota para tocarle una coma le borra el resto. Es la simetría
		// que hay que revisar cada vez que se agrega un campo.
		"origin": n.Origin, "pinned": n.Pinned,
		"question": n.Question, "blocks": n.Blocks,
		"cost_to_resolve": n.CostToResolve, "attempted": n.Attempted,
	})
}

// handleGuardLabel captures a HUMAN judgment of a Guard analysis into a
// human-labeled corpus (.cogo/guard-labels.jsonl). This is the honest answer to
// the "circular corpus" problem: the eval corpus was model-labeled; genuine
// human labels can only come from humans, so we collect them as a by-product of
// use. GET returns how many have been gathered.
func (s *Server) handleGuardLabel(w http.ResponseWriter, r *http.Request) {
	path := filepath.Join(s.dir, ".cogo", "guard-labels.jsonl")
	switch r.Method {
	case http.MethodGet:
		n := 0
		if b, err := os.ReadFile(path); err == nil {
			n = strings.Count(strings.TrimRight(string(b), "\n"), "\n") + 1
			if len(strings.TrimSpace(string(b))) == 0 {
				n = 0
			}
		}
		writeJSON(w, map[string]any{"count": n})
	case http.MethodPost:
		var in struct {
			Turn         string `json:"turn"`
			GuardVerdict string `json:"guard_verdict"`
			Label        string `json:"label"` // manipulative | benign
		}
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil || strings.TrimSpace(in.Turn) == "" || in.Label == "" {
			http.Error(w, "turn and label are required", http.StatusBadRequest)
			return
		}
		_ = os.MkdirAll(filepath.Join(s.dir, ".cogo"), 0o755)
		f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer f.Close()
		rec := map[string]any{"date": s.today().String(), "turn": in.Turn, "guard_verdict": in.GuardVerdict, "human_label": in.Label}
		b, _ := json.Marshal(rec)
		_, _ = f.Write(append(b, '\n'))
		writeJSON(w, map[string]any{"ok": true})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleXray radiographs an AI answer for veracity (deterministic gap meter).
func (s *Server) handleXray(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	var in struct {
		Answer string `json:"answer"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, xray.Analyze(in.Answer))
}

// handleTrash lists the deleted notes (GET) and restores or purges one (POST).
func (s *Server) handleTrash(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, map[string]any{"trash": core.ListTrash(s.dir)})
	case http.MethodPost:
		id, action := r.URL.Query().Get("id"), r.URL.Query().Get("action")
		var err error
		switch action {
		case "restore":
			err = core.RestoreTrash(s.dir, id)
		case "purge":
			// Grab the note's artifacts before it's gone, purge, then GC any that no
			// other note (live or trashed) still cites — the store is deduplicated,
			// so a shared blob must survive until its last citer is purged.
			var shas []string
			if target, e := core.ReadTrashNote(s.dir, id); e == nil {
				shas = core.ArtifactRefs(target)
			}
			err = core.PurgeTrash(s.dir, id)
			if err == nil && len(shas) > 0 {
				keep := core.ReferencedArtifacts(s.dir)
				store := artifact.FromEnv(s.dir)
				for _, sha := range shas {
					if !keep[sha] {
						_ = store.Delete(r.Context(), sha)
					}
				}
			}
		default:
			http.Error(w, "action must be restore or purge", http.StatusBadRequest)
			return
		}
		if err != nil {
			writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		writeJSON(w, map[string]any{"ok": true, "id": id, "action": action})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleHistory returns a note's recorded versions (when/why its color changed).
func (s *Server) handleHistory(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if id == "" {
		http.Error(w, "id required", http.StatusBadRequest)
		return
	}
	writeJSON(w, map[string]any{"versions": history.Load(s.dir, id)})
}

// handleVerify is the "revalidate" action: the check passed as of today.
func (s *Server) handleVerify(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	vault, ok := s.load(w)
	if !ok {
		return
	}
	id := r.URL.Query().Get("id")
	n, found := vault[id]
	if !found {
		http.Error(w, "no such note", http.StatusNotFound)
		return
	}
	// ?reanchor=1 confirma que se comprobó la afirmación contra el contenido
	// actual de la evidencia que derivó. Sin eso, verificar una nota derivada se
	// rechaza — antes la re-verificación borraba el aviso de deriva en silencio.
	err := core.Verificar(n, s.evRoots(), s.today(), core.Verificacion{
		Por:      auth.Caller(r),
		Reanclar: truthy(r.URL.Query().Get("reanchor")),
	})
	if err != nil {
		var d *core.ErrDeriva
		if errors.As(err, &d) {
			w.WriteHeader(http.StatusConflict)
			writeJSON(w, map[string]any{"error": d.Error(), "drifted": d.Refs, "needs": "reanchor"})
			return
		}
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	v := core.Evaluate(n, vault, s.contras(), s.today())
	n.Apply(v)
	path := n.Path
	if path == "" {
		path = filepath.Join(s.dir, id+".md")
	}
	if err := core.WriteNoteFile(path, n); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{"id": id, "color": v.Color.String(), "reason": v.Reason, "stale_at": v.StaleAt.String()})
}

// setStatus is the shared body of archive/restore: it flips a note's lifecycle
// state and rewrites it. The color is untouched — lifecycle is a separate axis.
func (s *Server) setStatus(w http.ResponseWriter, r *http.Request, status string) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	vault, ok := s.load(w)
	if !ok {
		return
	}
	id := r.URL.Query().Get("id")
	n, found := vault[id]
	if !found {
		http.Error(w, "no such note", http.StatusNotFound)
		return
	}
	n.Status = status
	path := n.Path
	if path == "" {
		path = filepath.Join(s.dir, id+".md")
	}
	if err := core.WriteNoteFile(path, n); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	state := core.Lifecycle(vault)
	writeJSON(w, map[string]any{"id": id, "state": stateOrActive(state[id])})
}

// handleArchive puts a note away (still on disk, restorable, out of the graph).
func (s *Server) handleArchive(w http.ResponseWriter, r *http.Request) {
	status := core.StateArchived
	if q := r.URL.Query().Get("status"); q == core.StateRetracted {
		status = core.StateRetracted // "retract" = withdrawn as wrong, vs merely obsolete
	}
	s.setStatus(w, r, status)
}

// handleRestore brings an archived/retracted note back to active.
func (s *Server) handleRestore(w http.ResponseWriter, r *http.Request) {
	s.setStatus(w, r, "")
}

// handleDelete removes a note from disk for good — for genuine garbage (wrong
// project, leaked secret, duplicate). It leaves a tombstone line in log.md so
// the deletion itself is on the record.
func (s *Server) handleDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	vault, ok := s.load(w)
	if !ok {
		return
	}
	id := r.URL.Query().Get("id")
	n, found := vault[id]
	if !found {
		http.Error(w, "no such note", http.StatusNotFound)
		return
	}
	if _, err := core.TrashNote(s.dir, n); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.tombstone(id)
	writeJSON(w, map[string]any{"id": id, "deleted": true})
}

// tombstone appends a deletion record to the vault's log.md (best-effort).
func (s *Server) tombstone(id string) {
	f, err := os.OpenFile(filepath.Join(s.dir, "log.md"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintf(f, "- %s delete %s\n", s.today().String(), id)
}

func stateOrActive(st string) string {
	if st == "" {
		return core.StateActive
	}
	return st
}

// draft is what the editor sends: a note's inputs. COGO computes the color.
type draft struct {
	ID         string            `json:"id"`
	Type       string            `json:"type"`
	Project    string            `json:"project"`
	Body       string            `json:"body"`
	Evidence   []core.Evidence   `json:"evidence"`
	CheckTest  string            `json:"check_test"`
	DependsOn  []string          `json:"depends_on"`
	Supersedes string            `json:"supersedes"`
	CausedBy   string            `json:"caused_by"`
	Scope      map[string]string `json:"scope,omitempty"`

	// Origin y Pinned son los dos campos que el humano tiene que poder tocar
	// desde SU interfaz. `pinned` sobre todo: es la excepción por nota al
	// olvido automático, y una excepción que hay que editar el .md a mano para
	// activar no es una excepción, es un obstáculo.
	Origin string `json:"origin,omitempty"`
	Pinned bool   `json:"pinned,omitempty"`

	// Notas de brecha (type: gap). No tienen evidencia ni check porque no
	// afirman nada: lo que llevan es la pregunta y qué está trabando.
	Question  string   `json:"question,omitempty"`
	Blocks    []string `json:"blocks,omitempty"`
	Cost      string   `json:"cost_to_resolve,omitempty"`
	Attempted []string `json:"attempted,omitempty"`
}

func (s *Server) noteFromDraft(d draft) *core.Note {
	id := d.ID
	if id == "" {
		id = core.DeriveID(d.Project, d.Body)
	}
	clean := d.Evidence[:0]
	for _, e := range d.Evidence {
		if strings.TrimSpace(e.Kind) != "" && strings.TrimSpace(e.Ref) != "" {
			clean = append(clean, e)
		}
	}
	// Editing resets verification: a changed claim must be re-checked.
	n := &core.Note{
		ID: id, Type: d.Type, Project: d.Project, Body: strings.TrimSpace(d.Body),
		LastVerified: s.today(),
		Evidence:     clean,
		Check:        core.Check{Test: d.CheckTest, Status: "not_run"},
		DependsOn:    cleanIDs(d.DependsOn),
		Supersedes:   strings.TrimSpace(d.Supersedes),
		CausedBy:     strings.TrimSpace(d.CausedBy),
		Scope:        d.Scope,
		Pinned:       d.Pinned,
	}
	// El origen solo se guarda donde significa algo. Escribirlo en un `bug`
	// dejaría un campo que nadie mira y que el pack no muestra: ruido en el
	// frontmatter, que es lo que este formato existe para no tener.
	if core.EsNormativa(n) {
		n.Origin = string(core.NormalizarOrigen(d.Origin))
	}
	if core.EsBrecha(n) {
		// Una brecha no lleva evidencia ni check: no hay nada que respaldar
		// porque no afirma nada. Lo que lleva es la pregunta y qué traba.
		n.Question = strings.TrimSpace(d.Question)
		if n.Question == "" {
			n.Question = core.Claim(n) // el cuerpo, si no se declaró aparte
		}
		n.Blocks = cleanIDs(d.Blocks)
		n.CostToResolve = strings.TrimSpace(d.Cost)
		n.Attempted = cleanIDs(d.Attempted)
		n.Evidence, n.Check = nil, core.Check{}
	}
	return n
}

// cleanIDs drops blank entries from a relation list.
func cleanIDs(ids []string) []string {
	out := ids[:0]
	for _, id := range ids {
		if s := strings.TrimSpace(id); s != "" {
			out = append(out, s)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// handlePreview computes the color of a draft WITHOUT saving — the live preview.
func (s *Server) handlePreview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	var d draft
	if err := json.NewDecoder(r.Body).Decode(&d); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	vault, ok := s.load(w)
	if !ok {
		return
	}
	n := s.noteFromDraft(d)
	vault[n.ID] = n
	core.ResolveEvidence(vault, s.evRoots()) // resolve the draft's own refs so the preview is honest
	v := core.Evaluate(n, vault, s.contras(), s.today())
	writeJSON(w, map[string]any{"id": n.ID, "color": v.Color.String(), "reason": v.Reason, "stale_at": v.StaleAt.String(), "evidence": n.Evidence})
}

// handleCapture validates a draft, colors it and writes it to the vault.
func (s *Server) handleCapture(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	var d draft
	if err := json.NewDecoder(r.Body).Decode(&d); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if d.Type == "" || strings.TrimSpace(d.Body) == "" {
		http.Error(w, "type and body are required", http.StatusBadRequest)
		return
	}
	vault, ok := s.load(w)
	if !ok {
		return
	}
	n := s.noteFromDraft(d)
	if err := scrub.Note(r.Context(), s.scrubber, n); err != nil {
		http.Error(w, "scrub (Anonimal) failed; note not saved: "+err.Error(), http.StatusBadGateway)
		return
	}
	path := filepath.Join(s.dir, n.ID+".md")
	n.Author = auth.Caller(r) // who captured it (authenticated identity)
	if existing, ok := vault[n.ID]; ok {
		if existing.Path != "" {
			path = existing.Path
		}
		if existing.Author != "" {
			n.Author = existing.Author // preserve the original creator across edits
		}
		// A cosmetic edit (claim, evidence and check all unchanged) keeps the
		// verification — fixing a typo shouldn't cost the green. A material edit
		// (the claim/evidence/check changed) resets to not_run, as before.
		if cosmeticEdit(existing, n) {
			n.Check.Status = existing.Check.Status
			n.LastVerified = existing.LastVerified
			// Evidence is unchanged (kind+ref), so carry the drift baseline over —
			// a typo fix must not silently re-confirm the evidence.
			for i := range n.Evidence {
				n.Evidence[i].Hash = existing.Evidence[i].Hash
			}
		}
	}
	vault[n.ID] = n
	core.ResolveEvidence(vault, s.evRoots())
	v := core.Evaluate(n, vault, s.contras(), s.today())
	n.Apply(v)
	if err := core.WriteNoteFile(path, n); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{"id": n.ID, "color": v.Color.String(), "reason": v.Reason, "stale_at": v.StaleAt.String()})
}

// cosmeticEdit reports whether the new version leaves the CLAIM, the evidence and
// the check test unchanged — i.e. nothing that the verification was about moved,
// so the note's passed check and last_verified date can carry over.
// cosmeticEdit dice si una edición conserva la verificación de la nota.
//
// Comparaba `core.Claim`, que es un RESUMEN recortado a 280 caracteres. Con eso,
// cualquier cambio más allá de ese corte —o en cualquier sección que no fuera el
// claim— pasaba por cosmético: una nota podía invertir lo que afirmaba y
// conservar el verde de la afirmación anterior. Usar una función de resumen como
// función de identidad es el error, y era explotable en 17 de las 25 notas de un
// vault real.
//
// Ahora compara el cuerpo ENTERO, normalizando solo espacios en blanco. Re-
// indentar o reordenar saltos de línea sigue sin costar la verificación;
// cambiar una palabra, sí. Es deliberadamente conservador: el costo de
// re-verificar de más es bajo, y el de afirmar de más es todo el producto.
func cosmeticEdit(a, b *core.Note) bool {
	if normalizarCuerpo(a.Body) != normalizarCuerpo(b.Body) ||
		a.Check.Test != b.Check.Test || len(a.Evidence) != len(b.Evidence) {
		return false
	}
	for i := range a.Evidence {
		if a.Evidence[i].Kind != b.Evidence[i].Kind || a.Evidence[i].Ref != b.Evidence[i].Ref {
			return false
		}
	}
	return true
}

// handleLint runs the maintenance pass and remembers any contradictions so they
// paint red across the visor until the next scan.
func (s *Server) handleLint(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	vault, ok := s.load(w)
	if !ok {
		return
	}
	rep := lint.Run(r.Context(), vault, s.today(), s.prov())
	// Fold the fresh findings into the persisted store (new ones open, dismissed
	// ones stay dismissed, nothing open is auto-cleared), then repaint from it.
	var found []contra.Found
	for _, is := range rep.Issues {
		if is.Kind == "contradiction" && len(is.IDs) == 2 {
			found = append(found, contra.Found{A: is.IDs[0], B: is.IDs[1], Reason: is.Msg})
		}
	}
	exists := func(id string) bool { _, ok := vault[id]; return ok }
	s.contra.Merge(found, s.today().String(), exists)
	s.mu.Lock()
	s.contradictions = s.contra.OpenNoteSet()
	s.mu.Unlock()
	s.flushUsage()
	writeJSON(w, map[string]any{
		"issues": rep.Issues, "llm_used": rep.LLMUsed,
		"pairs_checked": rep.PairsChecked, "candidate_pairs": rep.CandidatePairs,
		"contradictions": len(s.contra.OpenNoteSet()),
	})
}

// handleContradictions lists the persisted open contradictions (with each note's
// claim, for a side-by-side view) and lets a human resolve or dismiss one.
func (s *Server) handleContradictions(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		vault, ok := s.load(w)
		if !ok {
			return
		}
		type view struct {
			ID       string `json:"id"`
			A        string `json:"a"`
			B        string `json:"b"`
			AClaim   string `json:"a_claim"`
			BClaim   string `json:"b_claim"`
			Reason   string `json:"reason"`
			Detected string `json:"detected"`
		}
		out := []view{}
		for _, c := range s.contra.List() {
			if c.Status != contra.StatusOpen {
				continue
			}
			claim := func(id string) string {
				if n, ok := vault[id]; ok {
					return core.Claim(n)
				}
				return "(nota eliminada)"
			}
			out = append(out, view{ID: c.ID, A: c.A, B: c.B, AClaim: claim(c.A), BClaim: claim(c.B), Reason: c.Reason, Detected: c.Detected})
		}
		writeJSON(w, map[string]any{"contradictions": out})
	case http.MethodPost:
		id, action := r.URL.Query().Get("id"), r.URL.Query().Get("action")
		var ok bool
		switch action {
		case "resolve":
			ok = s.contra.Resolve(id)
		case "dismiss":
			ok = s.contra.Dismiss(id)
		default:
			http.Error(w, "action must be resolve or dismiss", http.StatusBadRequest)
			return
		}
		if !ok {
			http.Error(w, "no such contradiction", http.StatusNotFound)
			return
		}
		s.mu.Lock()
		s.contradictions = s.contra.OpenNoteSet()
		s.mu.Unlock()
		writeJSON(w, map[string]any{"ok": true, "id": id, "action": action})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// ---- LLM settings (configurable from the GUI, persisted next to the vault) ----

type llmSettings struct {
	BaseURL    string `json:"base_url"`
	Model      string `json:"model"`
	EmbedModel string `json:"embed_model,omitempty"` // optional; enables semantic search
	APIKey     string `json:"api_key"`
}

func (s *Server) settingsPath() string { return filepath.Join(s.dir, ".cogo", "llm.json") }

func (s *Server) readSettings() (llmSettings, error) {
	var set llmSettings
	b, err := os.ReadFile(s.settingsPath())
	if err != nil {
		return set, err
	}
	return set, json.Unmarshal(b, &set)
}

func (s *Server) writeSettings(set llmSettings) error {
	if err := os.MkdirAll(filepath.Dir(s.settingsPath()), 0o700); err != nil {
		return err
	}
	b, _ := json.MarshalIndent(set, "", "  ")
	return os.WriteFile(s.settingsPath(), b, 0o600)
}

// loadProvider: a saved GUI setting wins; otherwise fall back to env. Off if neither.
func (s *Server) loadProvider() llm.Provider {
	if set, err := s.readSettings(); err == nil && set.BaseURL != "" && set.Model != "" {
		return &llm.OpenAICompatible{BaseURL: set.BaseURL, Model: set.Model, APIKey: set.APIKey, Referer: os.Getenv("COGO_LLM_REFERER")}
	}
	return llm.FromEnv()
}

func providerName(p llm.Provider) string {
	if p.Available() {
		return p.Name()
	}
	return ""
}

func (s *Server) handleSettings(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		set, _ := s.readSettings()
		p := s.prov()
		writeJSON(w, map[string]any{
			"base_url": set.BaseURL, "model": set.Model, "embed_model": set.EmbedModel, "has_key": set.APIKey != "",
			"configured": p.Available(), "name": providerName(p),
		})
	case http.MethodPost:
		var set llmSettings
		if err := json.NewDecoder(r.Body).Decode(&set); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if strings.TrimSpace(set.BaseURL) == "" || (strings.TrimSpace(set.Model) == "" && strings.TrimSpace(set.EmbedModel) == "") {
			_ = os.Remove(s.settingsPath()) // clearing turns the LLM (and embeddings) off
		} else {
			if set.APIKey == "" { // blank key on save means "keep the existing one"
				if old, err := s.readSettings(); err == nil {
					set.APIKey = old.APIKey
				}
			}
			if err := s.writeSettings(set); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		}
		s.mu.Lock()
		s.provider = s.loadProvider()
		s.mu.Unlock()
		p := s.prov()
		writeJSON(w, map[string]any{"configured": p.Available(), "name": providerName(p)})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleTestLLM(w http.ResponseWriter, r *http.Request) {
	p := s.prov()
	if !p.Available() {
		writeJSON(w, map[string]any{"ok": false, "error": "no hay modelo configurado"})
		return
	}
	if _, err := p.Complete(r.Context(), "Reply with the single word: ok"); err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	s.flushUsage()
	writeJSON(w, map[string]any{"ok": true, "name": p.Name()})
}

// handleTestEmbed checks the embeddings model separately from the chat one:
// it embeds a tiny text and reports the vector dimension. base/key/embed_model
// come from the request so it works before saving (blank base/key reuse saved).
func (s *Server) handleTestEmbed(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	var in llmSettings
	_ = json.NewDecoder(r.Body).Decode(&in)
	base, key, em := strings.TrimSpace(in.BaseURL), in.APIKey, strings.TrimSpace(in.EmbedModel)
	if saved, err := s.readSettings(); err == nil {
		if base == "" {
			base = saved.BaseURL
		}
		if key == "" && base == saved.BaseURL {
			key = saved.APIKey
		}
		if em == "" {
			em = saved.EmbedModel
		}
	}
	if base == "" || em == "" {
		writeJSON(w, map[string]any{"ok": false, "error": "falta el servidor y/o el modelo de embeddings"})
		return
	}
	p := &llm.OpenAICompatible{BaseURL: base, EmbedModel: em, APIKey: key, Referer: os.Getenv("COGO_LLM_REFERER")}
	vecs, err := p.Embed(r.Context(), []string{"cogo embeddings connectivity test"})
	if err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	dim := 0
	if len(vecs) > 0 {
		dim = len(vecs[0])
	}
	if dim == 0 {
		writeJSON(w, map[string]any{"ok": false, "error": "el endpoint respondió pero sin vector"})
		return
	}
	s.flushUsage()
	writeJSON(w, map[string]any{"ok": true, "dim": dim, "model": em})
}

// handleModels lists the models an endpoint exposes and flags which are a good
// fit for COGO's jobs (contradiction detection, Guard's structural analysis,
// the steelman) — i.e. capable instruct/chat models, not embeddings or audio.
// base_url + api_key come from the request (so it works before saving); a blank
// key reuses the saved one for the same server.
func (s *Server) handleModels(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	var in llmSettings
	_ = json.NewDecoder(r.Body).Decode(&in)
	base, key := strings.TrimSpace(in.BaseURL), in.APIKey
	if saved, err := s.readSettings(); err == nil {
		if base == "" {
			base = saved.BaseURL
		}
		if key == "" && base == saved.BaseURL {
			key = saved.APIKey
		}
	}
	if base == "" {
		writeJSON(w, map[string]any{"ok": false, "error": "falta el servidor (base URL)"})
		return
	}
	p := &llm.OpenAICompatible{BaseURL: base, Model: "-", APIKey: key, Referer: os.Getenv("COGO_LLM_REFERER")}
	ids, err := p.Models(r.Context())
	if err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	sort.Strings(ids)
	type m struct {
		ID          string `json:"id"`
		Recommended bool   `json:"recommended"`
	}
	out := make([]m, 0, len(ids))
	rec := 0
	for _, id := range ids {
		ok := recommendModel(id)
		if ok {
			rec++
		}
		out = append(out, m{ID: id, Recommended: ok})
	}
	writeJSON(w, map[string]any{"ok": true, "models": out, "count": len(ids), "recommended": rec})
}

// recommendModel is a heuristic: a capable instruct/chat model from a strong
// family, sized 7B+ for local ones — and NOT an embedding/audio/image/rerank
// model, which cannot do COGO's judgment tasks.
func recommendModel(id string) bool {
	s := strings.ToLower(id)
	for _, bad := range []string{"embed", "whisper", "tts", "audio", "moderation", "rerank", "dall-e", "stable-diffusion", "flux", "clip", "bge", "e5-", "guard", "llava", "vl:", "-vl", "-v:", "vision"} {
		if strings.Contains(s, bad) {
			return false
		}
	}
	for _, k := range []string{"claude", "gpt-4", "gpt-4o", "o1-", "o3-", "o4-", "deepseek", "qwen2.5", "qwen-2.5", "qwen2", "qwen3", "qwen-3", "llama-3", "llama3", "gemma2", "gemma-2", "mistral-large", "mixtral", "command-r", "grok", "gemini-1.5", "gemini-2", "phi-4"} {
		if strings.Contains(s, k) {
			return true
		}
	}
	if strings.Contains(s, "instruct") || strings.Contains(s, "chat") {
		for _, sz := range []string{"70b", "72b", "32b", "27b", "14b", "9b", "8b", "7b"} {
			if strings.Contains(s, sz) {
				return true
			}
		}
	}
	return false
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(v)
}

// ext devuelve la extensión de un path ("go", "md", ""), para que el árbol pueda
// agrupar y el grafo diferenciar sin inventar colores nuevos.
func ext(p string) string {
	i := strings.LastIndex(p, ".")
	j := strings.LastIndex(p, "/")
	if i < 0 || i < j || i == len(p)-1 {
		return ""
	}
	return strings.ToLower(p[i+1:])
}
