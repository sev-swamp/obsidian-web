// Package search provides an embedded, incrementally updated in-memory
// full-text index over the vault. No external database is required.
package search

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/obsidianweb/obsidianweb/packages/core"
)

// Index implements core.SearchIndex.
type Index struct {
	mu       sync.RWMutex
	docs     map[string]core.SearchDoc
	inverted map[string]map[string]int // token -> path -> term frequency
}

// NewIndex returns an empty search index.
func NewIndex() *Index {
	return &Index{
		docs:     map[string]core.SearchDoc{},
		inverted: map[string]map[string]int{},
	}
}

var _ core.SearchIndex = (*Index)(nil)

// Index adds or replaces a document.
func (idx *Index) Index(doc core.SearchDoc) {
	tokens := tokenize(doc.Body)
	for t := range tokenize(doc.Title) {
		tokens[t] += 5 // title terms rank higher
	}
	for _, tag := range doc.Tags {
		for t := range tokenize(tag) {
			tokens[t] += 5
		}
	}
	for _, a := range doc.Aliases {
		for t := range tokenize(a) {
			tokens[t] += 5
		}
	}

	idx.mu.Lock()
	defer idx.mu.Unlock()
	idx.removeLocked(doc.Path)
	idx.docs[doc.Path] = doc
	for token, tf := range tokens {
		m, ok := idx.inverted[token]
		if !ok {
			m = map[string]int{}
			idx.inverted[token] = m
		}
		m[doc.Path] = tf
	}
}

// Remove drops a document from the index.
func (idx *Index) Remove(path string) {
	idx.mu.Lock()
	defer idx.mu.Unlock()
	idx.removeLocked(path)
}

func (idx *Index) removeLocked(path string) {
	if _, ok := idx.docs[path]; !ok {
		return
	}
	delete(idx.docs, path)
	for token, m := range idx.inverted {
		delete(m, path)
		if len(m) == 0 {
			delete(idx.inverted, token)
		}
	}
}

// Search supports free text plus `tag:x`, `path:x` and `prop:` filters.
// The last term matches as a prefix so search-as-you-type works.
func (idx *Index) Search(query string, limit int) []core.SearchResult {
	var terms []string
	var tagFilters, pathFilters []string
	var propertyFilters []propertyFilter
	for _, field := range splitQuery(query) {
		switch {
		case strings.HasPrefix(field, "tag:"):
			tagFilters = append(tagFilters, strings.TrimPrefix(field, "tag:"))
		case strings.HasPrefix(field, "path:"):
			pathFilters = append(pathFilters, strings.TrimPrefix(field, "path:"))
		case strings.HasPrefix(field, "prop:"):
			if filter, ok := parsePropertyFilter(strings.TrimPrefix(field, "prop:")); ok {
				propertyFilters = append(propertyFilters, filter)
			}
		default:
			terms = append(terms, normalizeToken(field))
		}
	}

	idx.mu.RLock()
	defer idx.mu.RUnlock()

	scores := map[string]float64{}
	if len(terms) == 0 {
		if len(tagFilters) == 0 && len(pathFilters) == 0 && len(propertyFilters) == 0 {
			return nil
		}
		for path := range idx.docs {
			scores[path] = 1
		}
	} else {
		for i, term := range terms {
			prefix := i == len(terms)-1 // search-as-you-type on the last term
			termScores := idx.termScores(term, prefix)
			if i == 0 {
				scores = termScores
				continue
			}
			// Intersect: every term must match.
			for path := range scores {
				if extra, ok := termScores[path]; ok {
					scores[path] += extra
				} else {
					delete(scores, path)
				}
			}
		}
	}

	var results []core.SearchResult
	for path, score := range scores {
		doc := idx.docs[path]
		if !matchesTags(doc, tagFilters) || !matchesPath(doc, pathFilters) || !matchesProperties(doc, propertyFilters) {
			continue
		}
		results = append(results, core.SearchResult{
			Path:    path,
			Title:   doc.Title,
			Tags:    doc.Tags,
			Score:   score,
			Snippet: snippet(doc.Body, terms),
		})
	}
	sort.Slice(results, func(i, j int) bool {
		if results[i].Score != results[j].Score {
			return results[i].Score > results[j].Score
		}
		return results[i].Path < results[j].Path
	})
	if len(results) > limit {
		results = results[:limit]
	}
	return results
}

// splitQuery lowercases the query and splits it into fields, keeping
// double-quoted sections together (quotes are dropped) so filter values
// may contain spaces: prop:created="2026-07-18 16:00".
func splitQuery(query string) []string {
	var fields []string
	var b strings.Builder
	inQuotes := false
	flush := func() {
		if b.Len() > 0 {
			fields = append(fields, b.String())
			b.Reset()
		}
	}
	for _, r := range strings.ToLower(query) {
		switch {
		case r == '"':
			inQuotes = !inQuotes
		case unicode.IsSpace(r) && !inQuotes:
			flush()
		default:
			b.WriteRune(r)
		}
	}
	flush()
	return fields
}

type propertyFilter struct {
	key   string
	value string
	op    string // "=" exact, ":" substring, or ">", ">=", "<", "<="
}

// Two-character operators are listed before their one-character prefixes
// so ">=" wins over ">" at the same position.
var propertyOps = []string{">=", "<=", "=", ">", "<", ":"}

// parsePropertyFilter splits "key<op>value". "=" matches a complete value,
// ":" a case-insensitive substring; comparisons order numerically when both
// sides are numbers and lexicographically otherwise (ISO dates compare
// correctly that way: prop:created>=2026-07-01).
func parsePropertyFilter(raw string) (propertyFilter, bool) {
	at, op := -1, ""
	for _, candidate := range propertyOps {
		if i := strings.Index(raw, candidate); i > 0 && (at < 0 || i < at) {
			at, op = i, candidate
		}
	}
	if at < 0 {
		return propertyFilter{}, false
	}
	return propertyFilter{key: raw[:at], value: raw[at+len(op):], op: op}, true
}

func matchesProperties(doc core.SearchDoc, filters []propertyFilter) bool {
	for _, filter := range filters {
		value, ok := doc.Frontmatter[filter.key]
		if !ok {
			return false
		}
		if !matchesPropertyValue(value, filter) {
			return false
		}
	}
	return true
}

func matchesPropertyValue(value any, filter propertyFilter) bool {
	if value == nil {
		return filter.op == "=" && filter.value == ""
	}
	if list, ok := value.([]any); ok {
		for _, item := range list {
			if matchesPropertyValue(item, filter) {
				return true
			}
		}
		return false
	}
	if list, ok := value.([]string); ok {
		for _, item := range list {
			if matchesPropertyValue(item, filter) {
				return true
			}
		}
		return false
	}
	text := strings.ToLower(propertyString(value))
	switch filter.op {
	case "=":
		return text == filter.value
	case ":":
		return strings.Contains(text, filter.value)
	default:
		return comparePropertyValues(text, filter.value, filter.op)
	}
}

func comparePropertyValues(a, b, op string) bool {
	cmp := strings.Compare(a, b)
	if fa, err := strconv.ParseFloat(a, 64); err == nil {
		if fb, err := strconv.ParseFloat(b, 64); err == nil {
			switch {
			case fa < fb:
				cmp = -1
			case fa > fb:
				cmp = 1
			default:
				cmp = 0
			}
		}
	}
	switch op {
	case ">":
		return cmp > 0
	case ">=":
		return cmp >= 0
	case "<":
		return cmp < 0
	case "<=":
		return cmp <= 0
	}
	return false
}

// propertyString renders a frontmatter scalar for matching and display.
// yaml.v3 decodes unquoted ISO dates into time.Time; format those back
// into the shape people type in queries.
func propertyString(value any) string {
	if t, ok := value.(time.Time); ok {
		if t.Hour() == 0 && t.Minute() == 0 && t.Second() == 0 {
			return t.Format("2006-01-02")
		}
		return t.Format("2006-01-02 15:04")
	}
	return fmt.Sprint(value)
}

// maxPropertyValues caps the per-key value list returned by Properties;
// it feeds autocomplete, not analytics.
const maxPropertyValues = 50

// Properties aggregates frontmatter keys across indexed notes visible to
// the caller: note count, dominant value type and most frequent values.
func (idx *Index) Properties(allow core.AllowFunc) []core.PropertyInfo {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	type agg struct {
		count  int
		types  map[string]int
		values map[string]int
	}
	keys := map[string]*agg{}
	for path, doc := range idx.docs {
		if allow != nil && !allow(path) {
			continue
		}
		for key, value := range doc.Frontmatter {
			a := keys[key]
			if a == nil {
				a = &agg{types: map[string]int{}, values: map[string]int{}}
				keys[key] = a
			}
			a.count++
			a.types[inferPropertyType(value)]++
			for _, v := range flattenPropertyValue(value) {
				a.values[v]++
			}
		}
	}

	out := make([]core.PropertyInfo, 0, len(keys))
	for key, a := range keys {
		out = append(out, core.PropertyInfo{
			Key:    key,
			Type:   dominantType(a.types),
			Count:  a.count,
			Values: topValues(a.values),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out
}

var (
	dateRe     = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
	datetimeRe = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}[T ]\d{2}:\d{2}`)
)

func inferPropertyType(value any) string {
	switch v := value.(type) {
	case bool:
		return "checkbox"
	case int, int64, uint64, float64:
		return "number"
	case []any, []string:
		return "list"
	case time.Time:
		if v.Hour() == 0 && v.Minute() == 0 && v.Second() == 0 {
			return "date"
		}
		return "datetime"
	case string:
		switch {
		case dateRe.MatchString(v):
			return "date"
		case datetimeRe.MatchString(v):
			return "datetime"
		case strings.HasPrefix(v, "[[") && strings.HasSuffix(v, "]]"):
			return "link"
		}
	}
	return "text"
}

func flattenPropertyValue(value any) []string {
	switch v := value.(type) {
	case nil:
		return nil
	case []any:
		var out []string
		for _, item := range v {
			out = append(out, flattenPropertyValue(item)...)
		}
		return out
	case []string:
		return v
	default:
		s := propertyString(v)
		if s == "" {
			return nil
		}
		return []string{s}
	}
}

func dominantType(counts map[string]int) string {
	best, bestCount := "text", 0
	for typ, count := range counts {
		if count > bestCount || (count == bestCount && typ < best) {
			best, bestCount = typ, count
		}
	}
	return best
}

func topValues(counts map[string]int) []core.PropertyValue {
	out := make([]core.PropertyValue, 0, len(counts))
	for v, c := range counts {
		out = append(out, core.PropertyValue{Value: v, Count: c})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Value < out[j].Value
	})
	if len(out) > maxPropertyValues {
		out = out[:maxPropertyValues]
	}
	return out
}

func (idx *Index) termScores(term string, prefix bool) map[string]float64 {
	out := map[string]float64{}
	if m, ok := idx.inverted[term]; ok {
		for path, tf := range m {
			out[path] += float64(tf)
		}
	}
	if prefix && len(term) >= 2 {
		for token, m := range idx.inverted {
			if token != term && strings.HasPrefix(token, term) {
				for path, tf := range m {
					out[path] += float64(tf) * 0.5
				}
			}
		}
	}
	return out
}

func matchesTags(doc core.SearchDoc, filters []string) bool {
	for _, f := range filters {
		found := false
		for _, tag := range doc.Tags {
			if strings.EqualFold(tag, f) || strings.HasPrefix(strings.ToLower(tag), f+"/") {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func matchesPath(doc core.SearchDoc, filters []string) bool {
	lower := strings.ToLower(doc.Path)
	for _, f := range filters {
		if !strings.Contains(lower, f) {
			return false
		}
	}
	return true
}

// snippet returns a short body excerpt around the first matched term.
func snippet(body string, terms []string) string {
	const window = 90
	lower := strings.ToLower(body)
	pos := -1
	for _, t := range terms {
		if i := strings.Index(lower, t); i >= 0 && (pos < 0 || i < pos) {
			pos = i
		}
	}
	if pos < 0 {
		pos = 0
	}
	start := pos - window/2
	if start < 0 {
		start = 0
	}
	end := start + window
	if end > len(body) {
		end = len(body)
	}
	s := strings.TrimSpace(strings.ReplaceAll(body[start:end], "\n", " "))
	if start > 0 {
		s = "…" + s
	}
	if end < len(body) {
		s += "…"
	}
	return s
}

func tokenize(text string) map[string]int {
	tokens := map[string]int{}
	var b strings.Builder
	flush := func() {
		if b.Len() >= 2 {
			tokens[strings.ToLower(b.String())]++
		}
		b.Reset()
	}
	for _, r := range text {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(unicode.ToLower(r))
		} else {
			flush()
		}
	}
	flush()
	return tokens
}

func normalizeToken(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return unicode.ToLower(r)
		}
		return -1
	}, s)
}
