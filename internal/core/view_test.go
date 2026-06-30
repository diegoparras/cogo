package core

import "testing"

func TestOverviewOrdersRedFirst(t *testing.T) {
	ov := Overview(packVault(), nil, packToday)
	if len(ov) != 5 {
		t.Fatalf("want 5 views, got %d", len(ov))
	}
	if ov[0].Color != "red" {
		t.Errorf("expected red first (most attention), got %s", ov[0].Color)
	}
	if ov[0].Claim == "" {
		t.Errorf("view is missing a claim: %+v", ov[0])
	}
}

func TestBuildGraphTypedEdges(t *testing.T) {
	v := packVault()
	v["yellow-worker"].DependsOn = []string{"green-redis"}
	v["yellow-worker"].Body += "\n\nrelated to [[red-guess]]."

	g := BuildGraph(v, nil, packToday)
	if len(g.Nodes) != 5 {
		t.Fatalf("want 5 nodes, got %d", len(g.Nodes))
	}

	var dep, wiki bool
	for _, e := range g.Edges {
		if e.From == "yellow-worker" && e.To == "green-redis" && e.Kind == "depends_on" {
			dep = true
		}
		if e.From == "yellow-worker" && e.To == "red-guess" && e.Kind == "wikilink" {
			wiki = true
		}
	}
	if !dep {
		t.Errorf("missing depends_on edge: %+v", g.Edges)
	}
	if !wiki {
		t.Errorf("missing wikilink edge: %+v", g.Edges)
	}
}
