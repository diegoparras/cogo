package core

import "testing"

// TestArtifactEvidenceResolution: an "artifact://<sha>" ref resolves via the
// injected checker — present → resolved, absent → broken, no checker → unchecked.
func TestArtifactEvidenceResolution(t *testing.T) {
	defer SetArtifactChecker(nil) // don't leak the hook into other tests

	// No checker wired: unchecked (conservative — never punishes what it can't see).
	SetArtifactChecker(nil)
	if s, _ := resolveRefPath(ArtifactRef("abc123"), ""); s != EvUnchecked {
		t.Fatalf("no checker: got %s, want unchecked", s)
	}

	present := map[string]bool{"deadbeef": true}
	SetArtifactChecker(func(sha string) bool { return present[sha] })

	if s, _ := resolveRefPath(ArtifactRef("deadbeef"), ""); s != EvResolved {
		t.Fatalf("present artifact: got %s, want resolved", s)
	}
	if s, _ := resolveRefPath(ArtifactRef("00000000"), ""); s != EvBroken {
		t.Fatalf("absent artifact: got %s, want broken", s)
	}

	// It flows through a full note evaluation: a green-worthy note whose only
	// evidence is a MISSING artifact must not stay green.
	n := &Note{
		ID: "n", Type: "bug", LastVerified: MustDate("2026-07-07"),
		Evidence: []Evidence{{Kind: "command_output", Ref: ArtifactRef("00000000")}},
		Check:    Check{Status: "passed"},
		Body:     "## Claim\nthe thing broke",
	}
	vault := map[string]*Note{"n": n}
	ResolveEvidence(vault, EvidenceRoots{})
	if n.Evidence[0].Status != EvBroken {
		t.Fatalf("missing artifact evidence status = %s, want broken", n.Evidence[0].Status)
	}
}
