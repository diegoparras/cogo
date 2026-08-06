package artifact

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"testing"
)

func TestFSStoreRoundTrip(t *testing.T) {
	s := NewFSStore(t.TempDir())
	ctx := context.Background()
	content := []byte("the command failed:\nexit 1\npanic: nil map")

	want := sha256.Sum256(content)
	wantHex := hex.EncodeToString(want[:])

	sha, err := s.Put(ctx, content, "text/plain")
	if err != nil {
		t.Fatal(err)
	}
	if sha != wantHex {
		t.Fatalf("key = %s, want the content sha %s", sha, wantHex)
	}

	// Dedup: same bytes → same key, and Put is idempotent.
	if sha2, err := s.Put(ctx, content, "text/plain"); err != nil || sha2 != sha {
		t.Fatalf("re-put: sha=%s err=%v (want stable dedup)", sha2, err)
	}

	if ok, err := s.Has(ctx, sha); err != nil || !ok {
		t.Fatalf("Has = %v,%v want true", ok, err)
	}

	got, err := s.Get(ctx, sha)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(content) {
		t.Fatalf("Get returned different bytes")
	}

	// Missing key.
	if _, err := s.Get(ctx, "deadbeef"); err != ErrNotFound {
		t.Fatalf("Get(missing) = %v, want ErrNotFound", err)
	}
	if ok, _ := s.Has(ctx, "deadbeef"); ok {
		t.Fatal("Has(missing) = true")
	}

	// Delete, then it's gone (and re-deleting is not an error).
	if err := s.Delete(ctx, sha); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Get(ctx, sha); err != ErrNotFound {
		t.Fatalf("Get after delete = %v, want ErrNotFound", err)
	}
	if err := s.Delete(ctx, sha); err != nil {
		t.Fatalf("re-delete should be a no-op, got %v", err)
	}
}

// TestFSStoreIntegrity: if the stored bytes are tampered under a key, Get must
// refuse to return them — the whole point of content addressing.
func TestFSStoreIntegrity(t *testing.T) {
	dir := t.TempDir()
	s := NewFSStore(dir)
	ctx := context.Background()
	sha, err := s.Put(ctx, []byte("original"), "")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(s.path(sha), []byte("tampered!"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Get(ctx, sha); err == nil {
		t.Fatal("Get returned tampered content instead of an integrity error")
	}
}

func TestSha256Hex(t *testing.T) {
	if got := Sha256Hex(nil); got != emptyHash {
		t.Fatalf("Sha256Hex(nil) = %s, want the empty-input hash %s", got, emptyHash)
	}
}

// TestFromEnvSelectsDisk: with no R2 endpoint, FromEnv falls back to disk.
func TestFromEnvSelectsDisk(t *testing.T) {
	t.Setenv("COGO_R2_ENDPOINT", "")
	if b := FromEnv(t.TempDir()).Backend(); b != "disk" {
		t.Fatalf("Backend without R2 = %q, want disk", b)
	}
}
