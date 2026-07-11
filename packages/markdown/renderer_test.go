package markdown

import (
	"strings"
	"testing"
)

type fakeResolver map[string]string

func (f fakeResolver) Resolve(target string) (string, bool) {
	p, ok := f[target]
	return p, ok
}

func TestRenderWikilinksAndFrontmatter(t *testing.T) {
	r := NewRenderer(fakeResolver{"Target": "Folder/Target.md", "img.png": "attachments/img.png"})
	src := []byte("---\ntitle: T\n---\n\nSee [[Target]] and [[Missing]] and ![[img.png]]\n")
	html, fm, err := r.Render("note.md", src)
	if err != nil {
		t.Fatal(err)
	}
	if fm["title"] != "T" {
		t.Errorf("frontmatter title = %v", fm["title"])
	}
	if !strings.Contains(html, `href="/n/Folder/Target"`) {
		t.Errorf("resolved wikilink missing:\n%s", html)
	}
	if !strings.Contains(html, "missing=1") {
		t.Errorf("unresolved link marker missing:\n%s", html)
	}
	if !strings.Contains(html, `/api/attachment/attachments/img.png`) {
		t.Errorf("embed URL missing:\n%s", html)
	}
}

func TestRenderCallout(t *testing.T) {
	r := NewRenderer(nil)
	html, _, err := r.Render("note.md", []byte("> [!warning] Careful\n> Body\n"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(html, `class="callout callout-warning"`) {
		t.Errorf("callout class missing:\n%s", html)
	}
	if !strings.Contains(html, `<span class="callout-title"`) || !strings.Contains(html, "Careful") {
		t.Errorf("callout title missing:\n%s", html)
	}
}

func TestRenderTaskListAndMermaid(t *testing.T) {
	r := NewRenderer(nil)
	html, _, err := r.Render("note.md", []byte("- [x] done\n\n```mermaid\ngraph TD; A-->B\n```\n"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(html, `type="checkbox"`) {
		t.Errorf("task list checkbox missing:\n%s", html)
	}
	if !strings.Contains(html, `<pre class="mermaid">`) {
		t.Errorf("mermaid block missing:\n%s", html)
	}
}
