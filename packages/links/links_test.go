package links

import "testing"

func TestParseLinks(t *testing.T) {
	src := []byte("See [[Note]] and [[Folder/Other|alias]] plus [[Ref#Heading]] and ![[img.png]].\n" +
		"Ignored in code: `[[NotALink]]`\n```\n[[AlsoNot]]\n```\n")
	links := ParseLinks(src)
	if len(links) != 4 {
		t.Fatalf("expected 4 links, got %d: %+v", len(links), links)
	}
	if links[0].Raw != "Note" {
		t.Errorf("raw[0] = %q", links[0].Raw)
	}
	if links[1].Alias != "alias" || links[1].Raw != "Folder/Other" {
		t.Errorf("link[1] = %+v", links[1])
	}
	if links[2].Fragment != "Heading" {
		t.Errorf("fragment = %q", links[2].Fragment)
	}
	if !links[3].Embed {
		t.Errorf("embed flag missing: %+v", links[3])
	}
}

func TestParseTags(t *testing.T) {
	tags := ParseTags([]byte("Text #project and #area/work but not#this or `#code`"))
	want := map[string]bool{"project": true, "area/work": true}
	if len(tags) != len(want) {
		t.Fatalf("tags = %v", tags)
	}
	for _, tag := range tags {
		if !want[tag] {
			t.Errorf("unexpected tag %q", tag)
		}
	}
}

func TestResolveAndBacklinks(t *testing.T) {
	idx := NewIndex()
	idx.Update("Folder/Target.md", []byte("---\naliases: [Tgt]\n---\ncontent"))
	idx.Update("Source.md", []byte("Links to [[Target]] and [[Tgt]]"))
	idx.RegisterFile("attachments/pic.png")

	for _, target := range []string{"Target", "Folder/Target", "Tgt", "target"} {
		if p, ok := idx.Resolve(target); !ok || p != "Folder/Target.md" {
			t.Errorf("Resolve(%q) = %q, %v", target, p, ok)
		}
	}
	if p, ok := idx.Resolve("pic.png"); !ok || p != "attachments/pic.png" {
		t.Errorf("attachment resolve = %q, %v", p, ok)
	}

	bl := idx.Backlinks("Folder/Target.md")
	if len(bl) != 1 || bl[0] != "Source.md" {
		t.Errorf("backlinks = %v", bl)
	}

	idx.Update("Broken.md", []byte("[[Nowhere]]"))
	if broken := idx.BrokenLinks(); len(broken["Broken.md"]) != 1 {
		t.Errorf("broken = %v", broken)
	}

	idx.Remove("Source.md")
	if bl := idx.Backlinks("Folder/Target.md"); len(bl) != 0 {
		t.Errorf("backlinks after remove = %v", bl)
	}
}
