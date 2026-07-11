// Package shared contains small utilities used across modules.
package shared

import (
	"bytes"

	"gopkg.in/yaml.v3"
)

// SplitFrontmatter separates YAML frontmatter from the markdown body.
// Returns nil map when the document has no (or invalid) frontmatter;
// the body is always returned.
func SplitFrontmatter(src []byte) (map[string]any, []byte) {
	if !bytes.HasPrefix(src, []byte("---")) {
		return nil, src
	}
	nl := bytes.IndexByte(src, '\n')
	if nl < 0 || len(bytes.TrimSpace(src[3:nl])) != 0 {
		return nil, src
	}
	rest := src[nl+1:]
	idx := 0
	for {
		lineEnd := bytes.IndexByte(rest[idx:], '\n')
		var line []byte
		var next int
		if lineEnd < 0 {
			line = rest[idx:]
			next = len(rest)
		} else {
			line = rest[idx : idx+lineEnd]
			next = idx + lineEnd + 1
		}
		trimmed := bytes.TrimSpace(line)
		if bytes.Equal(trimmed, []byte("---")) || bytes.Equal(trimmed, []byte("...")) {
			var fm map[string]any
			if err := yaml.Unmarshal(rest[:idx], &fm); err != nil {
				return nil, src
			}
			return fm, rest[next:]
		}
		if lineEnd < 0 {
			return nil, src
		}
		idx = next
	}
}

// StringList coerces a frontmatter value (string, []any or []string)
// into a list of strings. YAML allows `tags: a` and `tags: [a, b]`.
func StringList(v any) []string {
	switch t := v.(type) {
	case nil:
		return nil
	case string:
		if t == "" {
			return nil
		}
		return []string{t}
	case []string:
		return t
	case []any:
		out := make([]string, 0, len(t))
		for _, item := range t {
			if s, ok := item.(string); ok && s != "" {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}
