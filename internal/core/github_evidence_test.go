package core

import "testing"

func TestParseGitHubRef(t *testing.T) {
	cases := []struct {
		in                        string
		owner, repo, gitRef, path string
		ok                        bool
	}{
		{"github://diegoparras/cogo@main/internal/core/pack.go:37", "diegoparras", "cogo", "main", "internal/core/pack.go", true},
		{"github://o/r/a/b.go", "o", "r", "", "a/b.go", true},                                      // sin ref: rama por defecto
		{"github://o/r@abc123/x.go#L12", "o", "r", "abc123", "x.go", true},                         // locator #L
		{"github://o/r@v1.2.0/dir/sub/f.txt lines 3-9", "o", "r", "v1.2.0", "dir/sub/f.txt", true}, // "lines n-m"
		{"github://o/r", "", "", "", "", false},                                                    // sin path
		{"github://", "", "", "", "", false},
	}
	for _, c := range cases {
		o, r, g, p, ok := ParseGitHubRef(c.in)
		if ok != c.ok || o != c.owner || r != c.repo || g != c.gitRef || p != c.path {
			t.Errorf("ParseGitHubRef(%q) = (%q,%q,%q,%q,%v), want (%q,%q,%q,%q,%v)",
				c.in, o, r, g, p, ok, c.owner, c.repo, c.gitRef, c.path, c.ok)
		}
	}
}

// TestGitHubEvidenceStatuses cubre la semántica completa con un resolver falso:
// existe -> resolved; no existe -> broken (hunde el color); no se pudo chequear
// -> unchecked (nunca castiga); cambió desde la verificación -> drifted.
func TestGitHubEvidenceStatuses(t *testing.T) {
	defer SetGitHubResolver(nil)

	// Sin resolver: unchecked, jamás roto.
	SetGitHubResolver(nil)
	if s, _ := resolveRefPath("github://o/r@main/a.go", ""); s != EvUnchecked {
		t.Fatalf("sin resolver = %s, want unchecked", s)
	}

	SetGitHubResolver(func(owner, repo, ref, path string) (string, bool, bool) {
		switch path {
		case "existe.go":
			return "sha-actual", true, true
		case "falta.go":
			return "", false, true
		default:
			return "", false, false // no se pudo chequear (rate limit / red)
		}
	})

	if s, _ := resolveRefPath("github://o/r@main/existe.go:10", ""); s != EvResolved {
		t.Errorf("archivo existente = %s, want resolved", s)
	}
	if s, _ := resolveRefPath("github://o/r@main/falta.go", ""); s != EvBroken {
		t.Errorf("archivo ausente = %s, want broken", s)
	}
	if s, _ := resolveRefPath("github://o/r@main/error.go", ""); s != EvUnchecked {
		t.Errorf("error de API = %s, want unchecked (no castigar lo que no se pudo ver)", s)
	}

	// Drift: la nota fue verificada contra otro contenido del mismo archivo.
	n := &Note{
		ID: "n", Type: "bug", LastVerified: MustDate("2026-07-31"),
		Evidence: []Evidence{{Kind: "file_read", Ref: "github://o/r@main/existe.go", Hash: "sha-viejo"}},
		Check:    Check{Status: "passed"}, Body: "## Claim\nx",
	}
	vault := map[string]*Note{"n": n}
	ResolveEvidence(vault, EvidenceRoots{})
	if n.Evidence[0].Status != EvDrifted {
		t.Errorf("archivo cambiado = %s, want drifted", n.Evidence[0].Status)
	}

	// Re-verificar re-estampa el hash actual y deja de driftear.
	StampEvidenceHashes(n, EvidenceRoots{})
	if n.Evidence[0].Hash != "sha-actual" {
		t.Fatalf("StampEvidenceHashes no re-estampó: %q", n.Evidence[0].Hash)
	}
	ResolveEvidence(vault, EvidenceRoots{})
	if n.Evidence[0].Status != EvResolved {
		t.Errorf("tras re-verificar = %s, want resolved", n.Evidence[0].Status)
	}
}
