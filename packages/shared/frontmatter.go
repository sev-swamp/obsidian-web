// Package shared contains small utilities used across modules.
package shared

import (
	"bytes"
	"fmt"

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

// UpsertFrontmatterFields sets scalar fields in the YAML frontmatter,
// editing line-wise so user formatting elsewhere is preserved. A
// frontmatter block is created when the document has none. Fields is an
// ordered list of key/value pairs; values are written double-quoted.
func UpsertFrontmatterFields(src []byte, fields [][2]string) []byte {
	if len(fields) == 0 {
		return src
	}
	fm, _ := SplitFrontmatter(src)
	if fm == nil {
		var b bytes.Buffer
		b.WriteString("---\n")
		for _, f := range fields {
			fmt.Fprintf(&b, "%s: %q\n", f[0], f[1])
		}
		b.WriteString("---\n")
		b.Write(src)
		return b.Bytes()
	}

	lines := bytes.Split(src, []byte("\n"))
	// Locate the closing fence (first line after index 0 that is --- / …).
	closing := -1
	for i := 1; i < len(lines); i++ {
		trimmed := bytes.TrimSpace(lines[i])
		if bytes.Equal(trimmed, []byte("---")) || bytes.Equal(trimmed, []byte("...")) {
			closing = i
			break
		}
	}
	if closing < 0 {
		return src // SplitFrontmatter said valid, be defensive anyway
	}
	for _, f := range fields {
		prefix := []byte(f[0] + ":")
		replaced := false
		for i := 1; i < closing; i++ {
			if bytes.HasPrefix(bytes.TrimSpace(lines[i]), prefix) {
				lines[i] = []byte(fmt.Sprintf("%s: %q", f[0], f[1]))
				replaced = true
				break
			}
		}
		if !replaced {
			line := []byte(fmt.Sprintf("%s: %q", f[0], f[1]))
			lines = append(lines[:closing], append([][]byte{line}, lines[closing:]...)...)
			closing++
		}
	}
	return bytes.Join(lines, []byte("\n"))
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
