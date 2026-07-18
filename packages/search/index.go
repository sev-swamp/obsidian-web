// Package search provides an embedded, incrementally updated in-memory
// full-text index over the vault. No external database is required.
package search

import (
	"fmt"
	"sort"
	"strings"
	"sync"
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

// Search supports free text plus `tag:x` and `path:x` filters. The last
// term matches as a prefix so search-as-you-type works.
func (idx *Index) Search(query string, limit int) []core.SearchResult {
	var terms []string
	var tagFilters, pathFilters []string
	var propertyFilters []propertyFilter
	for _, field := range strings.Fields(strings.ToLower(query)) {
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

type propertyFilter struct {
	key   string
	value string
	exact bool
}

// prop:key=value matches a complete value; prop:key:value matches a
// case-insensitive substring. Values are deliberately token-sized, like the
// existing tag: and path: filters.
func parsePropertyFilter(raw string) (propertyFilter, bool) {
	if key, value, ok := strings.Cut(raw, "="); ok && key != "" {
		return propertyFilter{key: key, value: value, exact: true}, true
	}
	if key, value, ok := strings.Cut(raw, ":"); ok && key != "" {
		return propertyFilter{key: key, value: value}, true
	}
	return propertyFilter{}, false
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
		return filter.value == ""
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
	text := strings.ToLower(fmt.Sprint(value))
	if filter.exact {
		return text == filter.value
	}
	return strings.Contains(text, filter.value)
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
