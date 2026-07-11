package shared

import "testing"

func TestSplitFrontmatter(t *testing.T) {
	src := []byte("---\ntitle: Hello\ntags: [a, b]\n---\n\nBody here")
	fm, body := SplitFrontmatter(src)
	if fm == nil {
		t.Fatal("expected frontmatter")
	}
	if fm["title"] != "Hello" {
		t.Errorf("title = %v", fm["title"])
	}
	if string(body) != "\nBody here" {
		t.Errorf("body = %q", body)
	}
}

func TestSplitFrontmatterAbsent(t *testing.T) {
	src := []byte("# Just a heading\n")
	fm, body := SplitFrontmatter(src)
	if fm != nil {
		t.Errorf("expected nil frontmatter, got %v", fm)
	}
	if string(body) != string(src) {
		t.Errorf("body changed: %q", body)
	}
}

func TestSplitFrontmatterUnclosed(t *testing.T) {
	src := []byte("---\ntitle: broken\nno closing fence")
	fm, body := SplitFrontmatter(src)
	if fm != nil || string(body) != string(src) {
		t.Errorf("unclosed frontmatter must be treated as body")
	}
}

func TestStringList(t *testing.T) {
	if got := StringList("solo"); len(got) != 1 || got[0] != "solo" {
		t.Errorf("string: %v", got)
	}
	if got := StringList([]any{"a", "b"}); len(got) != 2 {
		t.Errorf("list: %v", got)
	}
	if got := StringList(nil); got != nil {
		t.Errorf("nil: %v", got)
	}
}
