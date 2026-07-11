// Package markdown renders Obsidian-flavoured markdown to HTML.
// Supported: CommonMark, GFM (tables, task lists, strikethrough,
// autolinks), YAML frontmatter, wiki-links, callouts, Mermaid
// (client-side), MathJax and syntax highlighting.
package markdown

import (
	"bytes"
	"net/url"
	"strings"

	mathjax "github.com/litao91/goldmark-mathjax"
	"github.com/yuin/goldmark"
	highlighting "github.com/yuin/goldmark-highlighting/v2"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer/html"
	"github.com/yuin/goldmark/util"
	"go.abhg.dev/goldmark/mermaid"
	"go.abhg.dev/goldmark/wikilink"

	"github.com/obsidianweb/obsidianweb/packages/core"
	"github.com/obsidianweb/obsidianweb/packages/shared"
)

// LinkResolver resolves wiki-link targets to vault paths.
type LinkResolver interface {
	Resolve(target string) (string, bool)
}

// Renderer implements core.Renderer using goldmark.
type Renderer struct {
	md goldmark.Markdown
}

var _ core.Renderer = (*Renderer)(nil)

// NewRenderer builds the markdown pipeline. Internal note links are
// rendered as /n/<path> and embeds as /api/attachment/<path> so the
// frontend can route them.
func NewRenderer(resolver LinkResolver) *Renderer {
	md := goldmark.New(
		goldmark.WithExtensions(
			extension.GFM,
			extension.Footnote,
			highlighting.NewHighlighting(
				highlighting.WithStyle("github"),
			),
			&mermaid.Extender{RenderMode: mermaid.RenderModeClient, NoScript: true},
			mathjax.MathJax,
			&wikilink.Extender{Resolver: &wikiResolver{resolver: resolver}},
			&calloutExtension{},
		),
		goldmark.WithParserOptions(
			parser.WithAutoHeadingID(),
		),
		goldmark.WithRendererOptions(
			html.WithUnsafe(), // vault content is trusted, allow raw HTML like Obsidian does
		),
	)
	return &Renderer{md: md}
}

// Render converts markdown to HTML and returns parsed frontmatter.
func (r *Renderer) Render(path string, source []byte) (string, map[string]any, error) {
	fm, body := shared.SplitFrontmatter(source)
	var buf bytes.Buffer
	if err := r.md.Convert(body, &buf); err != nil {
		return "", fm, err
	}
	return buf.String(), fm, nil
}

// wikiResolver adapts LinkResolver to the goldmark-wikilink API.
type wikiResolver struct {
	resolver LinkResolver
}

func (w *wikiResolver) ResolveWikilink(n *wikilink.Node) ([]byte, error) {
	target := string(n.Target)
	fragment := string(n.Fragment)

	if w.resolver != nil {
		if p, ok := w.resolver.Resolve(target); ok {
			var u string
			if n.Embed || !core.IsMarkdown(p) && !isNoteTarget(p) {
				u = "/api/attachment/" + escapePath(p)
			} else {
				u = "/n/" + escapePath(strings.TrimSuffix(p, ".md"))
			}
			if fragment != "" {
				u += "#" + url.PathEscape(fragment)
			}
			return []byte(u), nil
		}
	}
	if target == "" && fragment != "" {
		return []byte("#" + url.PathEscape(fragment)), nil
	}
	// Unresolved: link to the would-be note so it can be created later.
	return []byte("/n/" + escapePath(strings.TrimSuffix(target, ".md")) + "?missing=1"), nil
}

func isNoteTarget(p string) bool {
	return !strings.Contains(p[strings.LastIndex(p, "/")+1:], ".")
}

func escapePath(p string) string {
	segs := strings.Split(p, "/")
	for i, s := range segs {
		segs[i] = url.PathEscape(s)
	}
	return strings.Join(segs, "/")
}

// Ensure util import is used (goldmark extensions often need it).
var _ = util.Prioritized
