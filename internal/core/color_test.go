package core

import "testing"

// day is a terse alias for MustDate in tests.
func day(s string) Date { return MustDate(s) }

// Fixed "today" so every freshness case is deterministic.
var today = day("2026-06-29")

func note(id, typ, lastVerified string, ev []Evidence, check string, deps ...string) *Note {
	return &Note{
		ID:           id,
		Type:         typ,
		LastVerified: day(lastVerified),
		Evidence:     ev,
		Check:        Check{Status: check},
		DependsOn:    deps,
	}
}

// obs is one piece of observed evidence with a real ref.
func obs(ref string) []Evidence { return []Evidence{{Kind: "file_read", Ref: ref}} }

func TestEvaluateSingleNote(t *testing.T) {
	cases := []struct {
		name string
		n    *Note
		want Color
	}{
		// bug window = 60d. stale at +60, expires at +120.
		{"green: observed + passed + fresh", note("g", "bug", "2026-06-19", obs("api.go:40"), "passed"), Green},
		{"yellow: reported evidence caps at yellow", note("y1", "bug", "2026-06-19", []Evidence{{"doc", "README#redis"}}, "passed"), Yellow},
		{"yellow: observed but check not run", note("y2", "bug", "2026-06-19", obs("api.go:40"), "not_run"), Yellow},
		{"yellow: stale but not expired", note("y3", "bug", "2026-04-20", obs("api.go:40"), "passed"), Yellow},
		{"red: expired", note("r1", "bug", "2026-02-19", obs("api.go:40"), "passed"), Red},
		{"red: no evidence", note("r2", "bug", "2026-06-19", nil, "passed"), Red},
		{"red: evidence without a ref counts as none", note("r3", "bug", "2026-06-19", []Evidence{{"file_read", ""}}, "passed"), Red},
		{"ungraded: mistakes never decay", note("m", "mistake", "2020-01-01", obs("x"), "passed"), Ungraded},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			vault := map[string]*Note{c.n.ID: c.n}
			got := Evaluate(c.n, vault, nil, today)
			if got.Color != c.want {
				t.Fatalf("got %s (%s), want %s", got.Color, got.Reason, c.want)
			}
		})
	}
}

func TestContradictionForcesRed(t *testing.T) {
	n := note("c", "bug", "2026-06-19", obs("api.go:40"), "passed") // green on its own
	vault := map[string]*Note{n.ID: n}
	if got := Evaluate(n, vault, map[string]bool{"c": true}, today); got.Color != Red {
		t.Fatalf("got %s (%s), want red", got.Color, got.Reason)
	}
}

func TestDependencyPropagation(t *testing.T) {
	redDep := note("b", "bug", "2026-06-19", nil, "passed")                         // no evidence -> red
	yellowDep := note("c", "bug", "2026-06-19", []Evidence{{"doc", "r"}}, "passed") // reported -> yellow

	t.Run("depends on red -> red", func(t *testing.T) {
		a := note("a", "bug", "2026-06-19", obs("api.go:40"), "passed", "b") // green on its own
		vault := map[string]*Note{"a": a, "b": redDep}
		if got := Evaluate(a, vault, nil, today); got.Color != Red {
			t.Fatalf("got %s (%s), want red", got.Color, got.Reason)
		}
	})

	t.Run("depends on yellow -> yellow", func(t *testing.T) {
		a := note("a", "bug", "2026-06-19", obs("api.go:40"), "passed", "c") // green on its own
		vault := map[string]*Note{"a": a, "c": yellowDep}
		if got := Evaluate(a, vault, nil, today); got.Color != Yellow {
			t.Fatalf("got %s (%s), want yellow", got.Color, got.Reason)
		}
	})
}

func TestDependencyCycleIsRed(t *testing.T) {
	a := note("a", "bug", "2026-06-19", obs("x"), "passed", "b")
	b := note("b", "bug", "2026-06-19", obs("y"), "passed", "a")
	vault := map[string]*Note{"a": a, "b": b}
	if got := Evaluate(a, vault, nil, today); got.Color != Red {
		t.Fatalf("got %s (%s), want red", got.Color, got.Reason)
	}
}
