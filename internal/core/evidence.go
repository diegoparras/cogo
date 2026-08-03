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

// artifactExists, when set, reports whether a content-addressed artifact (an
// "artifact://<sha256>" ref) is present in the configured store. It is injected
// by the server (SetArtifactChecker) so core stays storage-agnostic, mirroring
// the writeHook seam. When nil, artifact refs are left unchecked — the same
// conservative default as any evidence COGO can't verify offline.
var artifactExists func(sha string) bool

// SetArtifactChecker installs the artifact-existence probe used to resolve
// "artifact://" evidence. Pass nil to disable (standalone with no store).
func SetArtifactChecker(f func(sha string) bool) { artifactExists = f }

// artifactPrefix marks an evidence ref whose bytes COGO stores by content hash.
const artifactPrefix = "artifact://"

// githubPrefix marks evidence that lives in a GitHub repository, cited as
// "github://owner/repo@ref/path/to/file.go:42" (the "@ref" and the line are
// optional). It is what makes file evidence checkable from a HOSTED COGO, which
// has no working copy on disk — and, because GitHub returns the file's git blob
// SHA, it is also how a note stays green only while the cited file hasn't moved.
const githubPrefix = "github://"

// githubResolver, when set, checks a GitHub citation: it returns the file's
// current content hash and whether it exists. ok=false means COGO could not
// check (network, rate limit, no access) — the caller must degrade to unchecked,
// never to broken. Injected by the server so core keeps no network code.
var githubResolver func(owner, repo, ref, path string) (sha string, found bool, ok bool)

// SetGitHubResolver installs the GitHub evidence resolver. Pass nil to disable
// (the default: COGO stays offline unless told otherwise).
func SetGitHubResolver(f func(owner, repo, ref, path string) (sha string, found bool, ok bool)) {
	githubResolver = f
}

// ParseGitHubRef splits "github://owner/repo@ref/path/file.go:42" into its
// parts. ref is "" when not pinned (the repo's default branch); the trailing
// line locator is dropped. ok=false when the ref is not a usable citation.
func ParseGitHubRef(ref string) (owner, repo, gitRef, path string, ok bool) {
	rest := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(ref), githubPrefix))
	parts := strings.SplitN(rest, "/", 3)
	if len(parts) < 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
		return "", "", "", "", false
	}
	owner = parts[0]
	repo = parts[1]
	if i := strings.Index(repo, "@"); i >= 0 {
		gitRef = repo[i+1:]
		repo = repo[:i]
	}
	path = parts[2]
	// Drop a trailing line locator so the path can be fetched.
	path = lineWordRe.ReplaceAllString(path, "")
	path = lineSuffixRe.ReplaceAllString(path, "")
	if repo == "" || path == "" {
		return "", "", "", "", false
	}
	return owner, repo, gitRef, path, true
}

// githubStatusHash resolves a GitHub citation to a status and its current
// content hash ("" when unknown).
func githubStatusHash(ref string) (status, hash string) {
	if githubResolver == nil {
		return EvUnchecked, ""
	}
	owner, repo, gitRef, path, ok := ParseGitHubRef(ref)
	if !ok {
		return EvUnchecked, ""
	}
	sha, found, ok := githubResolver(owner, repo, gitRef, path)
	switch {
	case !ok:
		return EvUnchecked, "" // couldn't check: never punish
	case !found:
		return EvBroken, ""
	default:
		return EvResolved, sha
	}
}

// ArtifactRef builds the evidence ref for a stored artifact.
func ArtifactRef(sha string) string { return artifactPrefix + sha }

// ArtifactRefs returns the content hashes cited by a note's "artifact://"
// evidence (empty if none). Used to garbage-collect the store after a delete.
func ArtifactRefs(n *Note) []string {
	var out []string
	for i := range n.Evidence {
		if ref := strings.TrimSpace(n.Evidence[i].Ref); strings.HasPrefix(ref, artifactPrefix) {
			out = append(out, strings.TrimSpace(ref[len(artifactPrefix):]))
		}
	}
	return out
}

// artifactStatus resolves an artifact ref: because the key IS the content hash,
// it can only be present (resolved) or gone (broken) — it never drifts.
func artifactStatus(sha string) string {
	if artifactExists == nil {
		return EvUnchecked
	}
	if artifactExists(sha) {
		return EvResolved
	}
	return EvBroken
}

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
			// Drift: evidence that still resolves but whose content changed since
			// the stamped baseline no longer supports the note the way it did when
			// verified. The current hash is the local file's, or GitHub's blob SHA.
			if status == EvResolved && n.Evidence[i].Hash != "" {
				cur := ""
				if path != "" {
					cur = fileHash(path)
				} else if isGitHubRef(n.Evidence[i].Ref) {
					_, cur = githubStatusHash(n.Evidence[i].Ref)
				}
				if cur != "" && cur != n.Evidence[i].Hash {
					status = EvDrifted
				}
			}
			n.Evidence[i].Status = status
		}
	}
}

// DriftedRefs devuelve las citas cuya evidencia cambió desde la última vez que
// se verificó la nota. Es lo que hay que mostrarle a alguien antes de dejarlo
// re-verificar: son exactamente las afirmaciones que ya no descansan sobre lo
// mismo que descansaban.
func DriftedRefs(n *Note) []string {
	var out []string
	for _, e := range n.Evidence {
		if e.Status == EvDrifted {
			out = append(out, e.Ref)
		}
	}
	return out
}

// StampNewEvidenceHashes establece la línea base SOLO donde todavía no había
// una. Es lo que corresponde al verificar: fija el punto de comparación de la
// evidencia nueva sin tocar la que ya derivó.
//
// La diferencia con StampEvidenceHashes es el defecto que arregla: re-estampar
// todo al verificar BORRA la señal de deriva. Una nota cuyo archivo citado
// cambió volvía a verde y perdía el rastro de que había cambiado — el sistema
// olvidaba, en el mismo acto, aquello de lo que debía avisar.
func StampNewEvidenceHashes(n *Note, roots EvidenceRoots) {
	for i := range n.Evidence {
		if n.Evidence[i].Hash != "" {
			continue // ya tiene línea base: dejarla es lo que conserva el drift
		}
		stampOne(n, i, roots)
	}
}

// StampEvidenceHashes records the current content hash of each resolvable file
// citation as the drift baseline. Re-baselines EVERYTHING, so it erases any
// pending drift: only call it when the caller explicitly confirmed the claim
// against the current content (re-anclaje deliberado).
func StampEvidenceHashes(n *Note, roots EvidenceRoots) {
	for i := range n.Evidence {
		stampOne(n, i, roots)
	}
}

// stampOne fija la línea base de una cita: el SHA del blob para GitHub, el hash
// del contenido para un archivo local. La evidencia que no es un archivo queda
// sin hash, y por eso nunca deriva.
func stampOne(n *Note, i int, roots EvidenceRoots) {
	if isGitHubRef(n.Evidence[i].Ref) {
		if status, h := githubStatusHash(n.Evidence[i].Ref); status == EvResolved && h != "" {
			n.Evidence[i].Hash = h
		}
		return
	}
	if status, path := resolveRefPath(n.Evidence[i].Ref, roots.Root(n.Project)); status == EvResolved && path != "" {
		if h := fileHash(path); h != "" {
			n.Evidence[i].Hash = h
		}
	}
}

// isGitHubRef reports whether a citation points into a GitHub repository.
func isGitHubRef(ref string) bool {
	return strings.HasPrefix(strings.TrimSpace(ref), githubPrefix)
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
	// A content-addressed artifact: check the store, never the filesystem. Handled
	// first so the locator/line-suffix logic below can't mangle the hash.
	if strings.HasPrefix(ref, artifactPrefix) {
		return artifactStatus(strings.TrimSpace(ref[len(artifactPrefix):])), ""
	}
	// A GitHub citation: checked against the API, not the filesystem — this is
	// what gives a hosted COGO teeth over file evidence.
	if strings.HasPrefix(ref, githubPrefix) {
		s, _ := githubStatusHash(ref)
		return s, ""
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
