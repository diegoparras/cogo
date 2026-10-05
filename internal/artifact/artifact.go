// Package artifact is COGO's content-addressed blob store: the key of every
// object IS the SHA-256 of its bytes. That single choice buys three things at
// once — a reference proves the content wasn't edited, identical bytes dedupe to
// one object, and `verify` can recompute the hash instead of trusting a claim.
//
// It is the storage half of the roadmap's "artefactos en R2": today an
// evidence ref points at something that rots (a commit line, a log timestamp, a
// closed session); with this store COGO keeps the artifact itself, addressed by
// content, so veracity becomes something recomputed rather than asserted.
//
// Standalone-first, like the rest of COGO: with no R2 configured it stores on
// disk under <vault>/.cogo/artifacts; set COGO_R2_* and the same interface
// speaks S3 to Cloudflare R2. One interface, two backends.
package artifact

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"time"
)

// ErrNotFound is returned by Get/Has when the object is absent.
var ErrNotFound = errors.New("artifact: not found")

// Store is a content-addressed blob store. Keys are always the lowercase hex
// SHA-256 of the content; callers never choose a key.
type Store interface {
	// Put stores content and returns its SHA-256 hex key. Idempotent: storing
	// the same bytes twice is one object (dedup).
	Put(ctx context.Context, content []byte, contentType string) (sha string, err error)
	// Get returns the bytes for a key and re-verifies them: if what comes back
	// doesn't hash to the key, it returns an integrity error, never the bytes.
	Get(ctx context.Context, sha string) ([]byte, error)
	// Has reports whether a key exists without downloading it.
	Has(ctx context.Context, sha string) (bool, error)
	// Delete removes an object. Deleting a missing object is not an error.
	Delete(ctx context.Context, sha string) error
	// Backend names the active backend ("disk" or "r2") for diagnostics.
	Backend() string
}

// Sha256Hex is the canonical key for a blob.
func Sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// FromEnv returns the R2 store when COGO_R2_ENDPOINT is set, otherwise a disk
// store under <vaultDir>/.cogo/artifacts. Missing R2 credentials with an
// endpoint set is a configuration error surfaced on first use, not here.
func FromEnv(vaultDir string) Store {
	if ep := os.Getenv("COGO_R2_ENDPOINT"); ep != "" {
		return &S3Store{
			endpoint:  ep,
			bucket:    getenvOr("COGO_R2_BUCKET", "cogo"),
			prefix:    "artifacts/",
			accessKey: os.Getenv("COGO_R2_ACCESS_KEY_ID"),
			secretKey: os.Getenv("COGO_R2_SECRET_ACCESS_KEY"),
			region:    getenvOr("COGO_R2_REGION", "auto"),
			client:    newHTTPClient(),
		}
	}
	return &FSStore{root: filepath.Join(vaultDir, ".cogo", "artifacts")}
}

func getenvOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

// FSStore is the disk backend: objects live at root/<sha[:2]>/<sha>, sharded so
// a directory never holds the whole vault. Keyed by content, so writes dedupe.
type FSStore struct{ root string }

// NewFSStore builds a disk store rooted at dir.
func NewFSStore(dir string) *FSStore { return &FSStore{root: dir} }

func (s *FSStore) path(sha string) string {
	if len(sha) < 2 {
		return filepath.Join(s.root, sha)
	}
	return filepath.Join(s.root, sha[:2], sha)
}

func (s *FSStore) Put(_ context.Context, content []byte, _ string) (string, error) {
	sha := Sha256Hex(content)
	p := s.path(sha)
	if _, err := os.Stat(p); err == nil {
		return sha, nil // already stored — dedup
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return "", err
	}
	// Write via a temp file + rename so a crash never leaves a half object under
	// a hash that claims to be complete.
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, content, 0o644); err != nil {
		return "", err
	}
	if err := os.Rename(tmp, p); err != nil {
		_ = os.Remove(tmp)
		return "", err
	}
	return sha, nil
}

func (s *FSStore) Get(_ context.Context, sha string) ([]byte, error) {
	b, err := os.ReadFile(s.path(sha))
	if os.IsNotExist(err) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if Sha256Hex(b) != sha {
		return nil, errors.New("artifact: integrity check failed (content does not match key)")
	}
	return b, nil
}

func (s *FSStore) Has(_ context.Context, sha string) (bool, error) {
	_, err := os.Stat(s.path(sha))
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

func (s *FSStore) Delete(_ context.Context, sha string) error {
	err := os.Remove(s.path(sha))
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func (s *FSStore) Backend() string { return "disk" }

// emptyHash is the SHA-256 of no bytes — the payload hash for GET/HEAD/DELETE.
const emptyHash = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"

// nowUTC is a seam for tests; production uses the wall clock.
var nowUTC = func() time.Time { return time.Now().UTC() }
