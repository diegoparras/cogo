package core

import "strings"

// DeriveID builds a stable id from a project and the first claim line of a body.
// Shared by every capture path (web form, MCP) so ids are consistent.
func DeriveID(project, body string) string {
	claim := ""
	for _, ln := range strings.Split(body, "\n") {
		t := strings.TrimSpace(ln)
		if t == "" || strings.HasPrefix(t, "#") {
			continue
		}
		claim = t
		break
	}
	slug := Slugify(claim)
	if project != "" {
		return Slugify(project) + "-" + slug
	}
	return slug
}

// Slugify turns text into a lowercase a-z0-9 dash slug, capped at 48 runes.
func Slugify(s string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(s) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			dash = false
		case b.Len() > 0 && !dash:
			b.WriteByte('-')
			dash = true
		}
		if b.Len() >= 48 {
			break
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "note"
	}
	return out
}
