// Package secretscan is the guard that runs BEFORE an artifact is stored. Once a
// blob is written under an immutable content hash it can't be un-said: a
// credential that slips into a stored command log or config dump is there for
// good (and deduped, so it may back several notes). So COGO scans first and, by
// default, REFUSES to store anything that looks like a secret — the caller can
// opt into redaction instead. This is the roadmap's "Guard antes de subir": the
// cost of a leaked secret is paid late, so pay attention early.
//
// The scanner is deliberately high-precision: it matches well-known credential
// shapes (keys with fixed prefixes, private-key headers, credentials embedded in
// URLs) plus a guarded "secret-ish assignment" rule, and skips obvious
// placeholders. It is not a guarantee — it's a tripwire that catches the usual
// accidents.
package secretscan

import (
	"regexp"
	"strings"
)

// Finding is one detected secret, with a masked preview (never the full secret).
type Finding struct {
	Rule    string `json:"rule"`    // human name, e.g. "AWS access key"
	Preview string `json:"preview"` // masked: first/last few chars only
}

type rule struct {
	name  string
	re    *regexp.Regexp
	group int // submatch index that holds the secret (0 = whole match)
}

var rules = []rule{
	{"private key", regexp.MustCompile(`-----BEGIN (?:RSA |EC |DSA |OPENSSH |PGP )?PRIVATE KEY-----`), 0},
	{"AWS access key id", regexp.MustCompile(`\b(?:AKIA|ASIA)[0-9A-Z]{16}\b`), 0},
	{"Google API key", regexp.MustCompile(`\bAIza[0-9A-Za-z_\-]{35}\b`), 0},
	{"GitHub token", regexp.MustCompile(`\bgh[pousr]_[0-9A-Za-z]{36,}\b`), 0},
	{"Slack token", regexp.MustCompile(`\bxox[baprs]-[0-9A-Za-z\-]{10,}\b`), 0},
	{"OpenAI/OpenRouter key", regexp.MustCompile(`\bsk-(?:or-v1-)?[0-9A-Za-z]{20,}\b`), 0},
	{"Cloudflare API token", regexp.MustCompile(`\bcfut_[0-9A-Za-z]{20,}\b`), 0},
	{"JWT", regexp.MustCompile(`\beyJ[0-9A-Za-z_\-]{8,}\.eyJ[0-9A-Za-z_\-]{8,}\.[0-9A-Za-z_\-]{8,}\b`), 0},
	{"bearer token", regexp.MustCompile(`(?i)\bbearer\s+([0-9A-Za-z._\-]{20,})`), 1},
	{"credentials in URL", regexp.MustCompile(`\b[a-z][a-z0-9+.\-]*://[^\s:/@]+:([^\s@/]{3,})@`), 1},
	// Guarded generic: a secret-ish key assigned a long opaque value.
	{"secret assignment", regexp.MustCompile(`(?i)\b(?:secret|password|passwd|pwd|api[_-]?key|access[_-]?key|secret[_-]?key|private[_-]?key|token|auth[_-]?token)\b\s*[:=]\s*["']?([0-9A-Za-z/+_\-]{16,})["']?`), 1},
}

// placeholderRe skips obvious non-secrets so refuse-by-default doesn't fire on
// examples ("password: changeme", "token: <your-token>", "key: xxxxxxxx").
var placeholderRe = regexp.MustCompile(`(?i)^(?:x{4,}|\.{3,}|<[^>]*>|your[_\-]|example|changeme|placeholder|redacted|dummy|test[_\-]?token|none|null|todo)`)

func isPlaceholder(secret string) bool {
	if placeholderRe.MatchString(secret) {
		return true
	}
	// A single repeated character (aaaa…, 0000…) isn't a real secret.
	if len(secret) > 0 {
		first := secret[0]
		same := true
		for i := 1; i < len(secret); i++ {
			if secret[i] != first {
				same = false
				break
			}
		}
		if same {
			return true
		}
	}
	return false
}

// mask shows only the shape: first 3 and last 2 characters, the rest hidden.
func mask(s string) string {
	if len(s) <= 6 {
		return strings.Repeat("•", len(s))
	}
	return s[:3] + strings.Repeat("•", 4) + s[len(s)-2:]
}

// Scan returns findings for likely secrets in content (empty slice if clean).
func Scan(content []byte) []Finding {
	s := string(content)
	var out []Finding
	seen := map[string]bool{}
	for _, r := range rules {
		for _, m := range r.re.FindAllStringSubmatch(s, -1) {
			secret := m[r.group]
			if secret == "" || isPlaceholder(secret) {
				continue
			}
			key := r.name + "\x00" + secret
			if seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, Finding{Rule: r.name, Preview: mask(secret)})
		}
	}
	return out
}

// Redact returns content with every detected secret span replaced by a marker,
// plus the findings. Used when the caller opts into "redact and store".
func Redact(content []byte) ([]byte, []Finding) {
	s := string(content)
	findings := Scan(content)
	if len(findings) == 0 {
		return content, nil
	}
	for _, r := range rules {
		s = r.re.ReplaceAllStringFunc(s, func(match string) string {
			sub := r.re.FindStringSubmatch(match)
			secret := sub[r.group]
			if secret == "" || isPlaceholder(secret) {
				return match // leave placeholders as-is
			}
			return strings.Replace(match, secret, "[REDACTED:"+r.name+"]", 1)
		})
	}
	return []byte(s), findings
}

// Guard implements COGO's "refuse by default, redact on opt-in" policy:
//   - clean content            → (content, nil, blocked=false)
//   - secrets, redact = false  → (content, findings, blocked=TRUE) — caller must refuse
//   - secrets, redact = true   → (redacted, findings, blocked=false)
func Guard(content []byte, redact bool) (out []byte, findings []Finding, blocked bool) {
	findings = Scan(content)
	if len(findings) == 0 {
		return content, nil, false
	}
	if !redact {
		return content, findings, true
	}
	red, _ := Redact(content)
	return red, findings, false
}

// Summary renders findings as a short human line for an error/warning message.
func Summary(findings []Finding) string {
	parts := make([]string, len(findings))
	for i, f := range findings {
		parts[i] = f.Rule + " (" + f.Preview + ")"
	}
	return strings.Join(parts, ", ")
}
