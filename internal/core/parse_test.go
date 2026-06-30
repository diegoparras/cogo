package core

import (
	"reflect"
	"strings"
	"testing"
)

// The brief's own example note (§3).
const sampleNote = `---
id: fisherboy-redis-hostname
type: bug
project: fisherboy
evidence:
  - kind: direct_log
    ref: "api log 2026-06-27T14:03Z: connect OK to redis:6379"
  - kind: absence
    ref: "no worker logs since 2026-06-27T13:50Z"
check:
  test: "read worker effective env; test internal connectivity to redis:6379"
  status: not_run
last_verified: 2026-06-27
depends_on: [fisherboy-redis-topology]
supersedes: null
caused_by: null
---

## Claim
The worker likely fails because it can't resolve the internal Redis hostname.

## Minimal check
Read the worker's effective env before editing code.
`

func TestParseNote(t *testing.T) {
	n, err := ParseNote([]byte(sampleNote))
	if err != nil {
		t.Fatal(err)
	}
	if n.ID != "fisherboy-redis-hostname" {
		t.Errorf("id = %q", n.ID)
	}
	if n.Type != "bug" || n.Project != "fisherboy" {
		t.Errorf("type/project = %q/%q", n.Type, n.Project)
	}
	if len(n.Evidence) != 2 || n.Evidence[0].Kind != "direct_log" || n.Evidence[1].Kind != "absence" {
		t.Errorf("evidence = %+v", n.Evidence)
	}
	if n.Check.Status != "not_run" {
		t.Errorf("check status = %q", n.Check.Status)
	}
	if n.LastVerified != MustDate("2026-06-27") {
		t.Errorf("last_verified = %s", n.LastVerified)
	}
	if !reflect.DeepEqual(n.DependsOn, []string{"fisherboy-redis-topology"}) {
		t.Errorf("depends_on = %v", n.DependsOn)
	}
	if n.Supersedes != "" || n.CausedBy != "" {
		t.Errorf("null fields not empty: %q %q", n.Supersedes, n.CausedBy)
	}
	if !strings.Contains(n.Body, "## Claim") || !strings.Contains(n.Body, "Minimal check") {
		t.Errorf("body lost: %q", n.Body)
	}
}

func TestRoundTrip(t *testing.T) {
	n, err := ParseNote([]byte(sampleNote))
	if err != nil {
		t.Fatal(err)
	}
	out, err := MarshalNote(n)
	if err != nil {
		t.Fatal(err)
	}
	n2, err := ParseNote(out)
	if err != nil {
		t.Fatalf("re-parse failed: %v\n---\n%s", err, out)
	}
	if n2.ID != n.ID || n2.Type != n.Type || n2.Project != n.Project ||
		n2.Check != n.Check || n2.LastVerified != n.LastVerified ||
		n2.Supersedes != n.Supersedes || n2.CausedBy != n.CausedBy {
		t.Errorf("scalar fields differ after round trip:\n%+v\n%+v", n, n2)
	}
	if !reflect.DeepEqual(n2.Evidence, n.Evidence) {
		t.Errorf("evidence differs: %+v vs %+v", n.Evidence, n2.Evidence)
	}
	if !reflect.DeepEqual(n2.DependsOn, n.DependsOn) {
		t.Errorf("depends_on differs: %v vs %v", n.DependsOn, n2.DependsOn)
	}
	if !strings.Contains(n2.Body, "## Claim") {
		t.Errorf("body lost after round trip: %q", n2.Body)
	}
}

func TestMarshalFencesComputedBlock(t *testing.T) {
	n, _ := ParseNote([]byte(sampleNote))
	n.Apply(Verdict{Color: Yellow, Reason: "observed evidence but check not passed", StaleAt: MustDate("2026-08-26")})
	out, err := MarshalNote(n)
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	if !strings.Contains(s, "computed by COGO · do not edit") {
		t.Errorf("computed block not fenced:\n%s", s)
	}
	if !strings.Contains(s, "confidence: yellow") {
		t.Errorf("confidence not written:\n%s", s)
	}
}

// The parser and the color engine meet: parse a real note and grade it.
func TestParseThenEvaluate(t *testing.T) {
	n, err := ParseNote([]byte(sampleNote))
	if err != nil {
		t.Fatal(err)
	}
	topo := &Note{
		ID:           "fisherboy-redis-topology",
		Type:         "architecture",
		LastVerified: MustDate("2026-06-20"),
		Evidence:     []Evidence{{Kind: "file_read", Ref: "docker-compose.yml:30"}},
		Check:        Check{Status: "passed"},
	}
	vault := map[string]*Note{n.ID: n, topo.ID: topo}

	got := Evaluate(n, vault, nil, MustDate("2026-06-29"))
	// Strongest evidence is observed (direct_log) but the check is not_run,
	// and the dependency is green -> yellow.
	if got.Color != Yellow {
		t.Fatalf("got %s (%s), want yellow", got.Color, got.Reason)
	}
}
