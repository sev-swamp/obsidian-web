package search

import (
	"testing"

	"github.com/obsidianweb/obsidianweb/packages/core"
)

func doc(path, title, body string, tags ...string) core.SearchDoc {
	return core.SearchDoc{Path: path, Title: title, Body: body, Tags: tags}
}

func TestSearchRankingAndFilters(t *testing.T) {
	idx := NewIndex()
	idx.Index(doc("a.md", "Kubernetes guide", "deploying clusters", "devops"))
	idx.Index(doc("b.md", "Cooking", "kubernetes mentioned once in passing"))
	idx.Index(doc("notes/c.md", "Recipes", "pasta and sauce", "cooking"))

	results := idx.Search("kubernetes", 10)
	if len(results) != 2 {
		t.Fatalf("expected 2 hits, got %d", len(results))
	}
	if results[0].Path != "a.md" {
		t.Errorf("title match must rank first, got %s", results[0].Path)
	}

	if r := idx.Search("tag:devops kubernetes", 10); len(r) != 1 || r[0].Path != "a.md" {
		t.Errorf("tag filter: %+v", r)
	}
	if r := idx.Search("path:notes pasta", 10); len(r) != 1 || r[0].Path != "notes/c.md" {
		t.Errorf("path filter: %+v", r)
	}
	// Prefix matching on the last term (search-as-you-type).
	if r := idx.Search("kuber", 10); len(r) == 0 {
		t.Error("prefix search returned nothing")
	}

	idx.Remove("a.md")
	if r := idx.Search("clusters", 10); len(r) != 0 {
		t.Errorf("removed doc still found: %+v", r)
	}
}

func TestSearchPropertyFilters(t *testing.T) {
	idx := NewIndex()
	idx.Index(core.SearchDoc{Path: "daily.md", Title: "Daily", Body: "entry", Frontmatter: map[string]any{
		"created": "2026-07-18 16:00", "author": "Ivan", "tags": []any{"daily", "work"}, "empty": nil,
	}})
	idx.Index(core.SearchDoc{Path: "other.md", Title: "Other", Body: "entry", Frontmatter: map[string]any{"author": "Petr"}})

	for _, query := range []string{"prop:author=ivan", "prop:created:2026-07", "prop:tags=daily", "prop:empty="} {
		if r := idx.Search(query, 10); len(r) != 1 || r[0].Path != "daily.md" {
			t.Errorf("%q: %+v", query, r)
		}
	}
	if r := idx.Search("prop:author=ivan entry", 10); len(r) != 1 || r[0].Path != "daily.md" {
		t.Errorf("combined filter: %+v", r)
	}
}
