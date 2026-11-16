package suasion

import "testing"

// Expected technique count per discipline. This is the contract of the
// ontology: a redactor dropping or inventing a technique fails the build.
var wantCounts = map[string]int{
	"persuasion":      19,
	"interrogation":   18,
	"negotiation":     18,
	"coercion":        19,
	"dark_psychology": 13,
	"rhetoric":        21,
}

func mustLoad(t *testing.T) *Ontology {
	t.Helper()
	o, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	return o
}

func TestLoadAndValidate(t *testing.T) {
	o := mustLoad(t)
	if err := o.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestCounts(t *testing.T) {
	o := mustLoad(t)
	if len(o.Disciplines) != len(wantCounts) {
		t.Fatalf("got %d disciplines, want %d", len(o.Disciplines), len(wantCounts))
	}
	total := 0
	for _, d := range o.Disciplines {
		want, ok := wantCounts[d.Discipline]
		if !ok {
			t.Errorf("unexpected discipline %q", d.Discipline)
			continue
		}
		if got := len(d.Techniques); got != want {
			t.Errorf("%s: got %d techniques, want %d", d.Discipline, got, want)
		}
		total += len(d.Techniques)
	}
	if o.Len() != total {
		t.Errorf("index has %d ids, files have %d techniques (duplicate ids?)", o.Len(), total)
	}
}

// Spot-checks on load-bearing techniques: the ones whose detection design the
// engine depends on most (receipts, false binary, language redefinition).
func TestKeystoneTechniques(t *testing.T) {
	o := mustLoad(t)

	gas := o.Get("dark_psychology.gaslighting")
	if gas == nil {
		t.Fatal("dark_psychology.gaslighting missing")
	}
	hasNLI := false
	for _, d := range gas.Detectors {
		if d.Type == "nli_transcript" {
			hasNLI = true
		}
	}
	if !hasNLI {
		t.Error("gaslighting must carry an nli_transcript detector (the receipts)")
	}

	alt := o.Get("interrogation.alternative_question")
	if alt == nil {
		t.Fatal("interrogation.alternative_question missing")
	}
	if alt.Severity != "high" && alt.Severity != "critical" {
		t.Errorf("alternative_question severity = %s, want high|critical (documented false-confession driver)", alt.Severity)
	}

	if o.Get("coercion.loading_the_language") == nil {
		t.Fatal("coercion.loading_the_language missing")
	}
	if o.Get("rhetoric.bullshit") == nil {
		t.Fatal("rhetoric.bullshit missing")
	}
}
