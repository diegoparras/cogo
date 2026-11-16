package artifact

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"
)

// TestR2Integration exercises the real S3/R2 backend end to end: PUT, HEAD, GET
// (with the integrity recompute), dedup, and DELETE. It is opt-in — it runs only
// when COGO_R2_ENDPOINT is set, so CI and normal `go test ./...` skip it. Never
// hardcode credentials; pass them via the environment.
func TestR2Integration(t *testing.T) {
	if os.Getenv("COGO_R2_ENDPOINT") == "" {
		t.Skip("set COGO_R2_* to run the live R2 integration test")
	}
	s := FromEnv(t.TempDir())
	if s.Backend() != "r2" {
		t.Fatalf("expected r2 backend, got %s", s.Backend())
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Unique content per run so parallel runs / leftovers don't interfere.
	content := []byte(fmt.Sprintf("cogo r2 self-test @ %d\n%s", time.Now().UnixNano(),
		"the failing command output that today would be lost"))

	sha, err := s.Put(ctx, content, "text/plain")
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	t.Logf("stored key=%s", sha)

	// Always try to clean up, even if a later step fails.
	defer func() {
		if err := s.Delete(ctx, sha); err != nil {
			t.Errorf("cleanup Delete: %v", err)
		}
	}()

	if ok, err := s.Has(ctx, sha); err != nil || !ok {
		t.Fatalf("Has after Put = %v,%v want true", ok, err)
	}

	// Dedup: second Put of identical bytes is a no-op returning the same key.
	if sha2, err := s.Put(ctx, content, "text/plain"); err != nil || sha2 != sha {
		t.Fatalf("dedup Put: sha=%s err=%v", sha2, err)
	}

	got, err := s.Get(ctx, sha)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if string(got) != string(content) {
		t.Fatal("Get returned different bytes than Put")
	}

	// A key that can't exist → ErrNotFound, not an integrity/auth error.
	absent := "0000000000000000000000000000000000000000000000000000000000000000"
	if _, err := s.Get(ctx, absent); err != ErrNotFound {
		t.Fatalf("Get(absent) = %v, want ErrNotFound", err)
	}
	if ok, err := s.Has(ctx, absent); err != nil || ok {
		t.Fatalf("Has(absent) = %v,%v want false", ok, err)
	}
}
