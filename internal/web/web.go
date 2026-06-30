// Package web is the human face: a small SPA, embedded in the binary, served
// over the same HTTP server as the MCP face. It is a thin client over core —
// every endpoint just loads the vault and asks core.
package web

import (
	"embed"
	"encoding/json"
	"io/fs"
	"net/http"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/diegoparras/cogo/internal/core"
)

//go:embed assets
var assetsFS embed.FS

// Version is shown in the "Acerca de" modal.
const Version = "0.1.0"

type Server struct {
	dir   string
	today func() core.Date
}

func New(dir string, today func() core.Date) *Server {
	return &Server{dir: dir, today: today}
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
}

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
	writeJSON(w, map[string]any{"version": Version, "projects": projects, "count": len(vault)})
}

func (s *Server) handleNotes(w http.ResponseWriter, r *http.Request) {
	vault, ok := s.load(w)
	if !ok {
		return
	}
	writeJSON(w, core.Overview(vault, nil, s.today()))
}

func (s *Server) handlePack(w http.ResponseWriter, r *http.Request) {
	vault, ok := s.load(w)
	if !ok {
		return
	}
	budget, _ := strconv.Atoi(r.URL.Query().Get("budget"))
	p := core.BuildPack(vault, nil, core.PackOptions{
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
	writeJSON(w, core.BuildGraph(vault, nil, s.today()))
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
	v := core.Evaluate(n, vault, nil, s.today())
	writeJSON(w, map[string]any{
		"id": n.ID, "type": n.Type, "project": n.Project, "body": n.Body,
		"evidence": n.Evidence, "check_test": n.Check.Test,
		"color": v.Color.String(), "reason": v.Reason, "stale_at": v.StaleAt.String(),
	})
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
	// Editing resets verification: a changed claim must be re-checked. The note
	// drops to yellow/red until the human re-verifies it.
	return &core.Note{
		ID: id, Type: d.Type, Project: d.Project, Body: strings.TrimSpace(d.Body),
		LastVerified: s.today(),
		Evidence:     clean,
		Check:        core.Check{Test: d.CheckTest, Status: "not_run"},
	}
}

// handlePreview computes the color of a draft WITHOUT saving — powers the live
// color preview as the user types.
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
	v := core.Evaluate(n, vault, nil, s.today())
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
	v := core.Evaluate(n, vault, nil, s.today())
	n.Apply(v)
	if err := core.WriteNoteFile(path, n); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{"id": n.ID, "color": v.Color.String(), "reason": v.Reason, "stale_at": v.StaleAt.String()})
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
	v := core.Evaluate(n, vault, nil, s.today())
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

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(v)
}
