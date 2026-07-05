package core

import (
	"fmt"
	"hash/fnv"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Evidence resolution: the "teeth" the color engine was missing. Until now a
// note went green as long as its evidence ref was non-empty — COGO never checked
// that the citation pointed at anything real. ResolveEvidence closes that gap for
// the case COGO can verify deterministically and locally: a file reference.
//
// The rule is conservative on purpose — COGO penalizes evidence it can PROVE is
// broken, never evidence it merely can't see:
//
//	resolved  — a checkable file ref that exists on disk
//	broken    — a checkable file ref that does NOT exist (stops counting; can sink green)
//	unchecked — anything COGO can't verify offline (a log line, a command, a URL,
//	            a repo-relative path with no root, an elided "...", etc.)
const (
	EvResolved  = "resolved"
	EvBroken    = "broken"
	EvUnchecked = "unchecked"
	EvDrifted   = "drifted" // resolves, but the file changed since the note was verified
)

// a trailing :line, :line-line, #Lnn or "line 33-41" locator, stripped before stat.
var (
	lineSuffixRe = regexp.MustCompile(`([:#]L?\d+(-\d+)?)$`)
	lineWordRe   = regexp.MustCompile(`(?i)\s+lines?\s+\d+(-\d+)?$`)
)

// ResolveEvidence annotates every evidence item in the vault with a runtime
// Status (see the constants). roots supplies the base directory repo-relative
// refs resolve against, per project (empty roots disables relative checking). It
// mutates the notes in place; call it after LoadVault and before evaluating color
// if you want the teeth on.
func ResolveEvidence(vault map[string]*Note, roots EvidenceRoots) {
	for _, n := range vault {
		root := roots.Root(n.Project)
		for i := range n.Evidence {
			status, path := resolveRefPath(n.Evidence[i].Ref, root)
			// Drift: a resolvable file that changed since the stamped baseline no
			// longer supports the note the way it did when verified.
			if status == EvResolved && n.Evidence[i].Hash != "" && path != "" {
				if cur := fileHash(path); cur != "" && cur != n.Evidence[i].Hash {
					status = EvDrifted
				}
			}
			n.Evidence[i].Status = status
		}
	}
}

// StampEvidenceHashes records the current content hash of each resolvable file
// citation as the drift baseline. Call it when a note is (re)verified — that is
// the moment "this is the evidence I confirmed against". Non-file evidence is
// left untouched (empty hash = never drifts).
func StampEvidenceHashes(n *Note, roots EvidenceRoots) {
	root := roots.Root(n.Project)
	for i := range n.Evidence {
		if status, path := resolveRefPath(n.Evidence[i].Ref, root); status == EvResolved && path != "" {
			if h := fileHash(path); h != "" {
				n.Evidence[i].Hash = h
			}
		}
	}
}

// fileHash is a fast, NON-cryptographic content hash (FNV-64a) — enough to detect
// that a file changed. Deliberately not sha256: keeps the core package free of
// crypto imports (and of the antivirus false positives that dogged the crypto ones).
func fileHash(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	h := fnv.New64a()
	_, _ = h.Write(b)
	return fmt.Sprintf("%016x", h.Sum64())
}

func resolveRef(ref, root string) string { s, _ := resolveRefPath(ref, root); return s }

// resolveRefPath classifies a ref and, for a checkable file, also returns the
// resolved filesystem path (so callers can hash it for drift). path is "" for
// anything not locatable as a local file.
func resolveRefPath(ref, root string) (status, path string) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return EvUnchecked, ""
	}

	// Take the locator token: everything before a prose separator, then the first
	// whitespace-delimited field. "docker-compose.yml:164 — REDIS_URL: ..." -> "docker-compose.yml:164".
	p := ref
	for _, sep := range []string{" — ", " – ", " - ", " (", ", "} {
		if i := strings.Index(p, sep); i >= 0 {
			p = p[:i]
		}
	}
	if fields := strings.Fields(p); len(fields) > 0 {
		p = fields[0]
	}
	// Strip a trailing line locator so the path can be stat'd.
	p = lineWordRe.ReplaceAllString(p, "")
	p = lineSuffixRe.ReplaceAllString(p, "")

	// An elided path ("file://.../x", "src/.../y") is not something COGO can locate.
	if strings.Contains(p, "...") {
		return EvUnchecked, ""
	}

	low := strings.ToLower(p)
	switch {
	case strings.HasPrefix(low, "http://"), strings.HasPrefix(low, "https://"):
		return EvUnchecked, "" // a URL needs the network; COGO stays offline by default
	case strings.HasPrefix(low, "file://"):
		if fp := fileURIPath(p); fp != "" {
			return existsStatus(fp), fp
		}
		return EvUnchecked, ""
	case filepath.IsAbs(p):
		return existsStatus(p), p
	case looksLikePath(p) && root != "":
		fp := filepath.Join(root, filepath.FromSlash(p))
		return existsStatus(fp), fp
	default:
		return EvUnchecked, ""
	}
}

// looksLikePath keeps bare prose ("connect OK to redis") from being treated as a
// relative path: it must carry a separator or a file extension to be checkable.
func looksLikePath(p string) bool {
	return strings.ContainsAny(p, "/\\") || filepath.Ext(p) != ""
}

func fileURIPath(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	p := u.Path
	if p == "" {
		p = strings.TrimPrefix(raw, "file://")
	}
	// Windows "file:///C:/x" -> "/C:/x" -> "C:/x".
	if len(p) >= 3 && p[0] == '/' && p[2] == ':' {
		p = p[1:]
	}
	return filepath.FromSlash(p)
}

func existsStatus(path string) string {
	if _, err := os.Stat(path); err == nil {
		return EvResolved
	} else if os.IsNotExist(err) {
		return EvBroken
	}
	return EvUnchecked // permission/other: don't punish what we couldn't read
}
