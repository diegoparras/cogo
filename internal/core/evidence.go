package core

import (
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
			n.Evidence[i].Status = resolveRef(n.Evidence[i].Ref, root)
		}
	}
}

func resolveRef(ref, root string) string {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return EvUnchecked
	}

	// Take the locator token: everything before a prose separator, then the first
	// whitespace-delimited field. "docker-compose.yml:164 — REDIS_URL: ..." -> "docker-compose.yml:164".
	path := ref
	for _, sep := range []string{" — ", " – ", " - ", " (", ", "} {
		if i := strings.Index(path, sep); i >= 0 {
			path = path[:i]
		}
	}
	if fields := strings.Fields(path); len(fields) > 0 {
		path = fields[0]
	}
	// Strip a trailing line locator so the path can be stat'd.
	path = lineWordRe.ReplaceAllString(path, "")
	path = lineSuffixRe.ReplaceAllString(path, "")

	// An elided path ("file://.../x", "src/.../y") is not something COGO can locate.
	if strings.Contains(path, "...") {
		return EvUnchecked
	}

	low := strings.ToLower(path)
	switch {
	case strings.HasPrefix(low, "http://"), strings.HasPrefix(low, "https://"):
		return EvUnchecked // a URL needs the network; COGO stays offline by default
	case strings.HasPrefix(low, "file://"):
		if fp := fileURIPath(path); fp != "" {
			return existsStatus(fp)
		}
		return EvUnchecked
	case filepath.IsAbs(path):
		return existsStatus(path)
	case looksLikePath(path) && root != "":
		return existsStatus(filepath.Join(root, filepath.FromSlash(path)))
	default:
		return EvUnchecked
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
