package tokens

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCreateVerifyRevoke(t *testing.T) {
	dir := t.TempDir()
	s := Open(dir)

	secret, tok, err := s.Create("Claude Code", "", false, "2026-07-04")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(secret, "cogo_") {
		t.Errorf("secret should be prefixed: %q", secret)
	}
	if tok.Hash != "" {
		t.Error("Create must not return the hash to the caller")
	}

	// The plaintext verifies; a wrong one doesn't.
	if got, ok := s.Verify(secret, "2026-07-04"); !ok || got.Label != "Claude Code" {
		t.Errorf("valid secret should verify, got %+v ok=%v", got, ok)
	}
	if _, ok := s.Verify("cogo_wrong", "2026-07-04"); ok {
		t.Error("wrong secret must not verify")
	}

	// List never leaks the hash.
	for _, l := range s.List() {
		if l.Hash != "" {
			t.Error("List() leaked a hash")
		}
	}

	// It persists hashed (never plaintext) and reloads.
	raw, _ := os.ReadFile(filepath.Join(dir, ".cogo", "tokens.json"))
	if strings.Contains(string(raw), secret) {
		t.Error("plaintext secret must NOT be on disk")
	}
	if _, ok := Open(dir).Verify(secret, "2026-07-04"); !ok {
		t.Error("token should survive a reload")
	}

	// Revoke kills it.
	if !s.Revoke(tok.ID) {
		t.Error("revoke should succeed")
	}
	if _, ok := s.Verify(secret, "2026-07-04"); ok {
		t.Error("revoked token must not verify")
	}
}

func TestExpiryAndReadOnly(t *testing.T) {
	s := Open(t.TempDir())

	sec, _, _ := s.Create("temporal", "2026-08-01", true, "2026-07-04")
	// Before expiry: verifies, and carries the read-only flag.
	got, ok := s.Verify(sec, "2026-07-15")
	if !ok || !got.ReadOnly {
		t.Errorf("pre-expiry read-only token should verify with ReadOnly=true, got %+v ok=%v", got, ok)
	}
	// After expiry: rejected.
	if _, ok := s.Verify(sec, "2026-08-02"); ok {
		t.Error("expired token must not verify")
	}
}
