package core

import "testing"

func TestArchiveHidesFromDefaultViews(t *testing.T) {
	v := packVault()
	v["green-redis"].Status = StateArchived

	if !Hidden(v)["green-redis"] {
		t.Fatal("archived note should be hidden")
	}

	// Overview: excluded by default, included (with state) when asked.
	if got := len(Overview(v, nil, packToday, false)); got != 4 {
		t.Errorf("default overview should drop the archived note: want 4, got %d", got)
	}
	all := Overview(v, nil, packToday, true)
	if len(all) != 5 {
		t.Fatalf("includeArchived overview should keep all 5, got %d", len(all))
	}
	var sawState bool
	for _, nv := range all {
		if nv.ID == "green-redis" {
			sawState = nv.State == StateArchived
		}
	}
	if !sawState {
		t.Error("archived note should carry State=archived when shown")
	}

	// Pack never includes an archived note, regardless of options.
	p := BuildPack(v, nil, PackOptions{Project: "fisherboy", Today: packToday})
	if p.Greens != 0 {
		t.Errorf("archived green must not reach the pack: greens=%d", p.Greens)
	}

	// Graph drops the node and any edge touching it.
	v["yellow-worker"].DependsOn = []string{"green-redis"}
	g := BuildGraph(v, nil, packToday, false)
	for _, n := range g.Nodes {
		if n.ID == "green-redis" {
			t.Error("archived node should not appear in the default graph")
		}
	}
	for _, e := range g.Edges {
		if e.To == "green-redis" || e.From == "green-redis" {
			t.Errorf("edge to an archived node should be dropped: %+v", e)
		}
	}
}

func TestSupersedesBuriesTheOldNote(t *testing.T) {
	v := packVault()
	// A new active note supersedes the old red one.
	v["red-guess-v2"] = &Note{
		ID: "red-guess-v2", Type: "bug", Project: "fisherboy",
		LastVerified: MustDate("2026-06-20"), Evidence: obs("queue.go:12"),
		Check: Check{Test: "drain the queue", Status: "passed"}, Supersedes: "red-guess",
		Body: "## Claim\nThe queue backlog is fixed.",
	}

	state := Lifecycle(v)
	if state["red-guess"] != StateSuperseded {
		t.Errorf("superseded note should be buried: got %q", state["red-guess"])
	}
	if state["red-guess-v2"] != StateActive {
		t.Errorf("the superseding note stays active: got %q", state["red-guess-v2"])
	}
	if !Hidden(v)["red-guess"] || Hidden(v)["red-guess-v2"] {
		t.Error("only the old note should be hidden")
	}

	// If the superseding note is itself archived, it no longer buries the old one.
	v["red-guess-v2"].Status = StateArchived
	if Lifecycle(v)["red-guess"] != StateActive {
		t.Error("an archived note must not keep another buried")
	}
}

func TestRestoreReactivates(t *testing.T) {
	v := packVault()
	v["green-redis"].Status = StateArchived
	v["green-redis"].Status = "" // restore
	if Hidden(v)["green-redis"] {
		t.Error("restored note should be active again")
	}
	if Lifecycle(v)["green-redis"] != StateActive {
		t.Error("restored note state should be active")
	}
}
