// Package web is the human face: a small SPA, embedded in the binary, served
// over the same HTTP server as the MCP face. It is a thin client over core —
// every endpoint just loads the vault and asks core. It also holds two pieces
// of optional runtime state: the LLM provider (configurable from the GUI) and
// the last contradiction scan (which paints notes red across every view).
package web

import (
	"embed"
	"encoding/json"
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
)

//go:embed assets
var assetsFS embed.FS

// Version is shown in the "Acerca de" modal.
const Version = "0.1.0"

type Server struct {
	dir   string
	today func() core.Date

	mu             sync.RWMutex
	provider       llm.Provider
	contradictions map[string]bool
}

func New(dir string, today func() core.Date) *Server {
	s := &Server{dir: dir, today: today, contradictions: map[string]bool{}}
	s.provider = s.loadProvider()
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
	mux.HandleFunc("/api/preview", s.handlePreview)
	mux.HandleFunc("/api/capture", s.handleCapture)
	mux.HandleFunc("/api/lint", s.handleLint)
	mux.HandleFunc("/api/settings", s.handleSettings)
	mux.HandleFunc("/api/settings/test", s.handleTestLLM)
}

func (s *Server) contras() map[string]bool { s.mu.RLock(); defer s.mu.RUnlock(); return s.contradictions }
func (s *Server) prov() llm.Provider       { s.mu.RLock(); defer s.mu.RUnlock(); return s.provider }

func (s *Server) load(w http.ResponseWriter) (map[string]*core.Note, bool) {
	vault, err := core.LoadVault(s.dir)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return nil, false
	}
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
	})
}

func (s *Server) handleNotes(w http.ResponseWriter, r *http.Request) {
	vault, ok := s.load(w)
	if !ok {
		return
	}
	writeJSON(w, core.Overview(vault, s.contras(), s.today()))
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
	writeJSON(w, core.BuildGraph(vault, s.contras(), s.today()))
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
	v := core.Evaluate(n, vault, s.contras(), s.today())
	writeJSON(w, map[string]any{"id": n.ID, "color": v.Color.String(), "reason": v.Reason, "stale_at": v.StaleAt.String()})
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
	path := filepath.Join(s.dir, n.ID+".md")
	if existing, ok := vault[n.ID]; ok && existing.Path != "" {
		path = existing.Path
	}
	vault[n.ID] = n
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

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(v)
}
