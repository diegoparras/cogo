package core

import (
	"sort"
	"strings"
)

// ScopeString renders a scope map as "commit=abc123 os=windows" in stable key
// order, for showing the conditions a claim held under.
func ScopeString(scope map[string]string) string {
	if len(scope) == 0 {
		return ""
	}
	keys := make([]string, 0, len(scope))
	for k := range scope {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, len(keys))
	for i, k := range keys {
		parts[i] = k + "=" + scope[k]
	}
	return strings.Join(parts, " ")
}

// ScopeConflict reports the keys where a note's declared scope disagrees with an
// environment. Only keys present in BOTH are compared (a note that declares
// os=windows conflicts with env os=linux, but says nothing about keys the env
// doesn't carry). The returned map holds the env's differing values; nil means
// compatible (or nothing to compare). Case-insensitive.
func ScopeConflict(scope, env map[string]string) map[string]string {
	if len(scope) == 0 || len(env) == 0 {
		return nil
	}
	var out map[string]string
	for k, v := range scope {
		if ev, ok := env[k]; ok && !strings.EqualFold(strings.TrimSpace(ev), strings.TrimSpace(v)) {
			if out == nil {
				out = map[string]string{}
			}
			out[k] = ev
		}
	}
	return out
}
