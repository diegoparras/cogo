package core

import (
	"regexp"
	"sort"
	"strings"
)

// GraphNode is one note painted by confidence. GraphEdge is a typed relation.
type GraphNode struct {
	ID    string `json:"id"`
	Type  string `json:"type"`
	Color string `json:"color"`
}

type GraphEdge struct {
	From string `json:"from"`
	To   string `json:"to"`
	Kind string `json:"kind"` // depends_on | supersedes | caused_by | wikilink
}

type GraphData struct {
	Nodes []GraphNode `json:"nodes"`
	Edges []GraphEdge `json:"edges"`
}

var wikilinkRe = regexp.MustCompile(`\[\[([^\]]+)\]\]`)

// BuildGraph resolves the vault into nodes (painted by computed color) and typed
// edges. Frontmatter gives the strong edges (depends_on, supersedes, caused_by);
// [[wikilinks]] in the body give the weak "relates to" edges. Only edges to
// notes that exist in the vault are kept.
func BuildGraph(vault map[string]*Note, contradictions map[string]bool, today Date) GraphData {
	verdicts := EvaluateVault(vault, contradictions, today)

	g := GraphData{}
	for id, n := range vault {
		g.Nodes = append(g.Nodes, GraphNode{ID: id, Type: n.Type, Color: verdicts[id].Color.String()})
	}
	sort.Slice(g.Nodes, func(i, j int) bool { return g.Nodes[i].ID < g.Nodes[j].ID })

	seen := map[string]bool{}
	add := func(from, to, kind string) {
		if from == to || seen[from+"\x00"+to] {
			return
		}
		if _, ok := vault[to]; !ok {
			return
		}
		g.Edges = append(g.Edges, GraphEdge{From: from, To: to, Kind: kind})
		seen[from+"\x00"+to] = true
	}

	// Strong, typed edges first so a wikilink never shadows them.
	for id, n := range vault {
		for _, d := range n.DependsOn {
			add(id, d, "depends_on")
		}
		if n.Supersedes != "" {
			add(id, n.Supersedes, "supersedes")
		}
		if n.CausedBy != "" {
			add(id, n.CausedBy, "caused_by")
		}
	}
	for id, n := range vault {
		for _, m := range wikilinkRe.FindAllStringSubmatch(n.Body, -1) {
			add(id, strings.TrimSpace(m[1]), "wikilink")
		}
	}

	sort.Slice(g.Edges, func(i, j int) bool {
		if g.Edges[i].From != g.Edges[j].From {
			return g.Edges[i].From < g.Edges[j].From
		}
		return g.Edges[i].To < g.Edges[j].To
	})
	return g
}
