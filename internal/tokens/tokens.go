// Package tokens is the MCP access-token store: multiple named Bearer tokens an
// operator hands out to apps/agents, each revocable on its own. Secrets are
// stored HASHED (sha256), never in plaintext — a leaked tokens.json reveals
// nothing usable. It lives next to the vault at .cogo/tokens.json (gitignored),
// the same place as llm.json / usage.json.
package tokens

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// Token is one issued access token. Hash is the sha256 of the secret; the secret
// itself is shown once at creation and never persisted.
type Token struct {
	ID       string `json:"id"`
	Label    string `json:"label"`
	Hash     string `json:"hash,omitempty"` // omitted from List() output
	Created  string `json:"created"`
	LastUsed string `json:"last_used,omitempty"`
	Expires  string `json:"expires,omitempty"`  // "" = never
	ReadOnly bool   `json:"readonly,omitempty"` // only pack/search/open
}

// Store is the in-memory + on-disk set of tokens. One instance per COGO process,
// shared by the auth verifier and the management endpoints.
type Store struct {
	dir  string
	mu   sync.Mutex
	toks []Token
}

// Open reads the store from <dir>/.cogo/tokens.json (missing file = empty).
func Open(dir string) *Store {
	s := &Store{dir: dir}
	if b, err := os.ReadFile(s.path()); err == nil {
		var wrap struct {
			Tokens []Token `json:"tokens"`
		}
		if json.Unmarshal(b, &wrap) == nil {
			s.toks = wrap.Tokens
		}
	}
	return s
}

func (s *Store) path() string { return filepath.Join(s.dir, ".cogo", "tokens.json") }

func (s *Store) saveLocked() error {
	if err := os.MkdirAll(filepath.Join(s.dir, ".cogo"), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(struct {
		Tokens []Token `json:"tokens"`
	}{s.toks}, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path(), b, 0o600)
}

// List returns every token WITHOUT its hash (safe to send to the UI).
func (s *Store) List() []Token {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Token, len(s.toks))
	copy(out, s.toks)
	for i := range out {
		out[i].Hash = ""
	}
	return out
}

// Create issues a new token: it generates the secret, stores only its hash, and
// returns the plaintext secret ONCE (the caller shows it and then forgets it).
// expires is an absolute YYYY-MM-DD ("" = never); today stamps Created.
func (s *Store) Create(label, expires string, readOnly bool, today string) (secret string, t Token, err error) {
	label = strings.TrimSpace(label)
	if label == "" {
		return "", Token{}, errors.New("la etiqueta no puede estar vacía")
	}
	secret = newSecret()
	t = Token{ID: newID(), Label: label, Hash: hashSecret(secret), Created: today, Expires: expires, ReadOnly: readOnly}
	s.mu.Lock()
	s.toks = append(s.toks, t)
	err = s.saveLocked()
	s.mu.Unlock()
	if err != nil {
		return "", Token{}, err
	}
	out := t
	out.Hash = ""
	return secret, out, nil
}

// Revoke deletes a token by id. Returns false if it wasn't there.
func (s *Store) Revoke(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.toks {
		if s.toks[i].ID == id {
			s.toks = append(s.toks[:i], s.toks[i+1:]...)
			_ = s.saveLocked()
			return true
		}
	}
	return false
}

// Verify matches a presented secret (constant-time on the hash), rejecting
// expired tokens, and records "last used" at most once per day. Returns the
// matched token (whether it is read-only matters to the caller) and ok.
func (s *Store) Verify(secret, today string) (Token, bool) {
	h := hashSecret(secret)
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.toks {
		if subtle.ConstantTimeCompare([]byte(s.toks[i].Hash), []byte(h)) != 1 {
			continue
		}
		if s.toks[i].Expires != "" && today > s.toks[i].Expires {
			return Token{}, false // expired (ISO dates compare lexicographically)
		}
		if s.toks[i].LastUsed != today {
			s.toks[i].LastUsed = today
			_ = s.saveLocked() // throttled: one write per token per day
		}
		t := s.toks[i]
		t.Hash = ""
		return t, true
	}
	return Token{}, false
}

func newSecret() string {
	b := make([]byte, 24)
	_, _ = rand.Read(b)
	return "cogo_" + base64.RawURLEncoding.EncodeToString(b)
}

func newID() string {
	b := make([]byte, 5)
	_, _ = rand.Read(b)
	return "tk_" + base64.RawURLEncoding.EncodeToString(b)
}

func hashSecret(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}
