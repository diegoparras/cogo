// Package web is the human face: a small SPA, embedded in the binary, served
// over the same HTTP server as the MCP face. It is a thin client over core —
// every endpoint just loads the vault and asks core. It also holds two pieces
// of optional runtime state: the LLM provider (configurable from the GUI) and
// the last contradiction scan (which paints notes red across every view).
package web

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/diegoparras/cogo/internal/core"
	"github.com/diegoparras/cogo/internal/lint"
	"github.com/diegoparras/cogo/internal/llm"
	"github.com/diegoparras/cogo/internal/scrub"
)

//go:embed assets
var assetsFS embed.FS

// Version is shown in the "Acerca de" modal.
const Version = "0.1.0"

type Server struct {
	dir          string
	evidenceRoot string // base dir for resolving repo-relative evidence refs (COGO_EVIDENCE_ROOT)
	today        func() core.Date

	mu             sync.RWMutex
	provider       llm.Provider
	contradictions map[string]bool
	scrubber       scrub.Scrubber
}

func New(dir string, today func() core.Date) *Server {
	s := &Server{dir: dir, today: today, contradictions: map[string]bool{}, evidenceRoot: os.Getenv("COGO_EVIDENCE_ROOT")}
	s.provider = s.loadProvider()
	s.scrubber = scrub.FromEnv()
	return s
}

// Mount registers the SPA and the JSON API on the given mux.
func (s *Server) Mount(mux *http.ServeMux) {
	sub, err := fs.Sub(assetsFS, "assets")
	if err != nil {
		panic(err)
	}
	mux.Handle("/", http.FileServer(http.FS(sub)))
	mux.HandleFunc("/api/config", s.handleConfig)
	mux.HandleFunc("/api/notes", s.handleNotes)
	mux.HandleFunc("/api/pack", s.handlePack)
	mux.HandleFunc("/api/graph", s.handleGraph)
	mux.HandleFunc("/api/note", s.handleNote)
	mux.HandleFunc("/api/verify", s.handleVerify)
	mux.HandleFunc("/api/archive", s.handleArchive)
	mux.HandleFunc("/api/restore", s.handleRestore)
	mux.HandleFunc("/api/delete", s.handleDelete)
	mux.HandleFunc("/api/preview", s.handlePreview)
	mux.HandleFunc("/api/capture", s.handleCapture)
	mux.HandleFunc("/api/lint", s.handleLint)
	mux.HandleFunc("/api/settings", s.handleSettings)
	mux.HandleFunc("/api/settings/test", s.handleTestLLM)
	mux.HandleFunc("/api/settings/models", s.handleModels)
	mux.HandleFunc("/api/guard", s.handleGuard)
	mux.HandleFunc("/api/mandate", s.handleMandate)
}

func (s *Server) contras() map[string]bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.contradictions
}
func (s *Server) prov() llm.Provider { s.mu.RLock(); defer s.mu.RUnlock(); return s.provider }

func (s *Server) load(w http.ResponseWriter) (map[string]*core.Note, bool) {
	vault, err := core.LoadVault(s.dir)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return nil, false
	}
	core.ResolveEvidence(vault, s.evidenceRoot) // the teeth: check that evidence refs resolve
	return vault, true
}

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
	writeJSON(w, map[string]any{
		"version": Version, "projects": projects, "count": len(vault),
		"llm_configured": s.prov().Available(),
		"scrub_enabled":  s.scrubber.Enabled(),
		"evidence_root":  s.evidenceRoot != "",
	})
}

func (s *Server) handleNotes(w http.ResponseWriter, r *http.Request) {
	vault, ok := s.load(w)
	if !ok {
		return
	}
	writeJSON(w, core.Overview(vault, s.contras(), s.today(), archivedParam(r)))
}

// archivedParam reads the "?archived=1" toggle used by views that can optionally
// show the notes that are normally hidden (archived, retracted, superseded).
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
	writeJSON(w, map[string]any{
		"markdown": p.Markdown, "tokens": p.Tokens,
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
	writeJSON(w, map[string]any{
		"id": n.ID, "type": n.Type, "project": n.Project, "body": n.Body,
		"evidence": n.Evidence, "check_test": n.Check.Test,
		"color": v.Color.String(), "reason": v.Reason, "stale_at": v.StaleAt.String(),
	})
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
	n.Check.Status = "passed"
	n.LastVerified = s.today()
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
	ID        string          `json:"id"`
	Type      string          `json:"type"`
	Project   string          `json:"project"`
	Body      string          `json:"body"`
	Evidence  []core.Evidence `json:"evidence"`
	CheckTest string          `json:"check_test"`
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
	return &core.Note{
		ID: id, Type: d.Type, Project: d.Project, Body: strings.TrimSpace(d.Body),
		LastVerified: s.today(),
		Evidence:     clean,
		Check:        core.Check{Test: d.CheckTest, Status: "not_run"},
	}
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
	core.ResolveEvidence(vault, s.evidenceRoot) // resolve the draft's own refs so the preview is honest
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
	if existing, ok := vault[n.ID]; ok && existing.Path != "" {
		path = existing.Path
	}
	vault[n.ID] = n
	core.ResolveEvidence(vault, s.evidenceRoot)
	v := core.Evaluate(n, vault, s.contras(), s.today())
	n.Apply(v)
	if err := core.WriteNoteFile(path, n); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{"id": n.ID, "color": v.Color.String(), "reason": v.Reason, "stale_at": v.StaleAt.String()})
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
	s.mu.Lock()
	s.contradictions = rep.Contradictions()
	s.mu.Unlock()
	writeJSON(w, map[string]any{
		"issues": rep.Issues, "llm_used": rep.LLMUsed,
		"pairs_checked": rep.PairsChecked, "candidate_pairs": rep.CandidatePairs,
		"contradictions": len(rep.Contradictions()),
	})
}

// ---- LLM settings (configurable from the GUI, persisted next to the vault) ----

type llmSettings struct {
	BaseURL string `json:"base_url"`
	Model   string `json:"model"`
	APIKey  string `json:"api_key"`
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
			"base_url": set.BaseURL, "model": set.Model, "has_key": set.APIKey != "",
			"configured": p.Available(), "name": providerName(p),
		})
	case http.MethodPost:
		var set llmSettings
		if err := json.NewDecoder(r.Body).Decode(&set); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if strings.TrimSpace(set.BaseURL) == "" || strings.TrimSpace(set.Model) == "" {
			_ = os.Remove(s.settingsPath()) // clearing turns the LLM off
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
	writeJSON(w, map[string]any{"ok": true, "name": p.Name()})
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
