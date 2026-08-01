// Package ghsource lets COGO check evidence that lives in a GitHub repository.
//
// It closes a hole that only shows up once COGO is hosted: a note citing
// "src/foo.go:42" can be verified on the laptop that has the repo checked out,
// but the deployed instance has no files, so every file citation degrades to
// "unchecked" and the color engine loses its teeth exactly where several agents
// share the memory. Pointing at GitHub instead of a local path fixes that for
// everyone at once.
//
// It also enables the freshness rule COGO wanted from the start — "green while
// the cited commit hasn't changed": GitHub's contents API returns the file's git
// blob SHA, which is a content hash, so the existing drift machinery works
// unchanged (stamp it on verify, compare it later).
//
// Deliberately hand-rolled over three REST endpoints instead of pulling in a
// GitHub SDK: the same call COGO makes everywhere else (see the SigV4 signer in
// internal/artifact) — a scratch image with no dependencies beats convenience.
package ghsource

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

// Client talks to the GitHub REST API. The zero value is unusable; use FromEnv.
type Client struct {
	token string
	api   string // base, e.g. https://api.github.com (overridable for GHES/tests)
	http  *http.Client

	mu    sync.Mutex
	cache map[string]entry
	ttl   time.Duration
}

type entry struct {
	sha   string
	found bool
	at    time.Time
}

// FromEnv builds a client from the environment. COGO_GITHUB_TOKEN authenticates
// (5000 req/h and access to private repos); without it the client still works
// for public repos at GitHub's anonymous rate limit. COGO_GITHUB_API overrides
// the base URL for GitHub Enterprise.
func FromEnv() *Client {
	return &Client{
		token: strings.TrimSpace(os.Getenv("COGO_GITHUB_TOKEN")),
		api:   strings.TrimRight(getenvOr("COGO_GITHUB_API", "https://api.github.com"), "/"),
		http:  &http.Client{Timeout: 12 * time.Second},
		cache: map[string]entry{},
		ttl:   10 * time.Minute,
	}
}

func getenvOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

// Authenticated reports whether a token is configured (private repos + the high
// rate limit).
func (c *Client) Authenticated() bool { return c != nil && c.token != "" }

// FileSHA returns the git blob SHA of a file at a ref — a content hash COGO can
// compare later to detect drift. found=false means the citation points at
// nothing (a broken ref). An error means COGO could not check (network, rate
// limit, permissions): callers must treat that as "unchecked", never as broken.
//
// Results are cached briefly because evidence is re-resolved on every request;
// without it, one page load with 50 notes would be 50 API calls.
func (c *Client) FileSHA(ctx context.Context, owner, repo, ref, path string) (sha string, found bool, err error) {
	if c == nil {
		return "", false, fmt.Errorf("ghsource: no client")
	}
	key := owner + "/" + repo + "@" + ref + "/" + path
	c.mu.Lock()
	if e, ok := c.cache[key]; ok && time.Since(e.at) < c.ttl {
		c.mu.Unlock()
		return e.sha, e.found, nil
	}
	c.mu.Unlock()

	u := fmt.Sprintf("%s/repos/%s/%s/contents/%s", c.api, url.PathEscape(owner), url.PathEscape(repo), escapePath(path))
	if ref != "" {
		u += "?ref=" + url.QueryEscape(ref)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return "", false, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return "", false, err
	}
	defer resp.Body.Close()

	switch {
	case resp.StatusCode == http.StatusNotFound:
		c.remember(key, "", false)
		return "", false, nil
	case resp.StatusCode == http.StatusForbidden, resp.StatusCode == http.StatusUnauthorized:
		// Rate limited or no access: COGO cannot judge, so it must not punish.
		return "", false, fmt.Errorf("github: %s (sin acceso o límite de rate)", resp.Status)
	case resp.StatusCode/100 != 2:
		return "", false, fmt.Errorf("github: %s", resp.Status)
	}
	// A file answers with an object; a DIRECTORY answers with an array. Peek at
	// the first byte so a directory reads as "not a citable file" instead of
	// blowing up as an unmarshal error (which would look like "couldn't check").
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", false, err
	}
	if t := strings.TrimLeft(string(raw), " \t\r\n"); strings.HasPrefix(t, "[") {
		c.remember(key, "", false)
		return "", false, nil
	}
	var out struct {
		SHA  string `json:"sha"`
		Type string `json:"type"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", false, err
	}
	if out.Type != "file" || out.SHA == "" {
		c.remember(key, "", false)
		return "", false, nil
	}
	c.remember(key, out.SHA, true)
	return out.SHA, true, nil
}

// TreeEntry is one item of a repository directory listing.
type TreeEntry struct {
	Name string `json:"name"`
	Path string `json:"path"`
	Type string `json:"type"` // "file" | "dir"
	Size int    `json:"size,omitempty"`
}

// Tree lists a directory of the repository so you can find the file you want to
// cite without leaving COGO. Reading the repo is not the same as storing it: the
// listing is fetched live and kept nowhere — what COGO persists is the citation.
func (c *Client) Tree(ctx context.Context, owner, repo, ref, path string) ([]TreeEntry, error) {
	if c == nil {
		return nil, fmt.Errorf("ghsource: no client")
	}
	u := fmt.Sprintf("%s/repos/%s/%s/contents/%s", c.api, url.PathEscape(owner), url.PathEscape(repo), escapePath(path))
	if ref != "" {
		u += "?ref=" + url.QueryEscape(ref)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("no encontré ese repositorio o esa ruta")
	}
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return nil, fmt.Errorf("sin acceso (¿repo privado sin token, o límite de rate?)")
	}
	if resp.StatusCode/100 != 2 {
		return nil, fmt.Errorf("github: %s", resp.Status)
	}
	var items []TreeEntry
	if err := json.NewDecoder(resp.Body).Decode(&items); err != nil {
		return nil, fmt.Errorf("esa ruta es un archivo, no una carpeta")
	}
	sort.Slice(items, func(i, j int) bool {
		if (items[i].Type == "dir") != (items[j].Type == "dir") {
			return items[i].Type == "dir" // carpetas primero
		}
		return strings.ToLower(items[i].Name) < strings.ToLower(items[j].Name)
	})
	return items, nil
}

// FileContent fetches the file's text at a ref, so the visor can SHOW the cited
// evidence instead of asking you to take the citation on faith. Returns the
// decoded content, its blob SHA and the canonical GitHub URL. Binary or very
// large files come back as an error — COGO shows source fragments, it is not a
// file browser (that is what the repository itself is for).
func (c *Client) FileContent(ctx context.Context, owner, repo, ref, path string) (content []byte, sha, htmlURL string, err error) {
	if c == nil {
		return nil, "", "", fmt.Errorf("ghsource: no client")
	}
	u := fmt.Sprintf("%s/repos/%s/%s/contents/%s", c.api, url.PathEscape(owner), url.PathEscape(repo), escapePath(path))
	if ref != "" {
		u += "?ref=" + url.QueryEscape(ref)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, "", "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, "", "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, "", "", fmt.Errorf("el archivo citado no existe en el repositorio")
	}
	if resp.StatusCode/100 != 2 {
		return nil, "", "", fmt.Errorf("github: %s", resp.Status)
	}
	var out struct {
		SHA      string `json:"sha"`
		Type     string `json:"type"`
		Encoding string `json:"encoding"`
		Content  string `json:"content"`
		HTMLURL  string `json:"html_url"`
		Size     int    `json:"size"`
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, "", "", err
	}
	if strings.HasPrefix(strings.TrimLeft(string(raw), " \t\r\n"), "[") {
		return nil, "", "", fmt.Errorf("esa ruta es una carpeta, no un archivo citable")
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, "", "", err
	}
	if out.Type != "file" {
		return nil, "", "", fmt.Errorf("esa ruta no es un archivo")
	}
	if out.Encoding != "base64" || out.Content == "" {
		// GitHub omits the body for large files; the citation is still valid.
		return nil, out.SHA, out.HTMLURL, fmt.Errorf("el archivo es demasiado grande para mostrarlo acá")
	}
	dec, err := base64.StdEncoding.DecodeString(strings.ReplaceAll(out.Content, "\n", ""))
	if err != nil {
		return nil, out.SHA, out.HTMLURL, err
	}
	if !utf8.Valid(dec) {
		return nil, out.SHA, out.HTMLURL, fmt.Errorf("el archivo es binario")
	}
	return dec, out.SHA, out.HTMLURL, nil
}

func (c *Client) remember(key, sha string, found bool) {
	c.mu.Lock()
	c.cache[key] = entry{sha: sha, found: found, at: time.Now()}
	c.mu.Unlock()
}

// escapePath escapes each segment of a repo path, keeping the slashes.
func escapePath(p string) string {
	segs := strings.Split(strings.TrimPrefix(p, "/"), "/")
	for i, s := range segs {
		segs[i] = url.PathEscape(s)
	}
	return strings.Join(segs, "/")
}
