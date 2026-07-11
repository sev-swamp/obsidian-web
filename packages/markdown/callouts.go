package markdown

import (
	"regexp"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

// calloutExtension turns Obsidian callouts (`> [!note] Title`) into
// blockquotes annotated with CSS classes; styling happens client-side.
type calloutExtension struct{}

func (e *calloutExtension) Extend(m goldmark.Markdown) {
	m.Parser().AddOptions(parser.WithASTTransformers(
		util.Prioritized(&calloutTransformer{}, 500),
	))
}

var calloutMarker = regexp.MustCompile(`^\[!([\w-]+)\]([+-]?)\s*(.*)$`)

type calloutTransformer struct{}

func (t *calloutTransformer) Transform(doc *ast.Document, reader text.Reader, pc parser.Context) {
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		bq, ok := n.(*ast.Blockquote)
		if !ok {
			return ast.WalkContinue, nil
		}
		para, ok := bq.FirstChild().(*ast.Paragraph)
		if !ok {
			return ast.WalkContinue, nil
		}
		// The marker "[!note] Title" is split across several Text nodes
		// ("[" starts a potential link), so reassemble the first line.
		var lineNodes []*ast.Text
		var line strings.Builder
		for child := para.FirstChild(); child != nil; child = child.NextSibling() {
			t, isText := child.(*ast.Text)
			if !isText {
				break
			}
			lineNodes = append(lineNodes, t)
			line.Write(t.Segment.Value(reader.Source()))
			if t.SoftLineBreak() || t.HardLineBreak() {
				break
			}
		}
		m := calloutMarker.FindStringSubmatch(line.String())
		if m == nil {
			return ast.WalkContinue, nil
		}
		kind := strings.ToLower(m[1])
		title := strings.TrimSpace(m[3])
		if title == "" {
			title = strings.ToUpper(kind[:1]) + kind[1:]
		}

		bq.SetAttributeString("class", []byte("callout callout-"+kind))
		bq.SetAttributeString("data-callout", []byte(kind))

		// Replace the marker line with a styled title element.
		for _, t := range lineNodes {
			para.RemoveChild(para, t)
		}
		titleHTML := ast.NewString([]byte(
			`<span class="callout-title" data-callout="` + kind + `">` + escapeHTML(title) + `</span>`))
		titleHTML.SetCode(true)
		if para.FirstChild() != nil {
			para.InsertBefore(para, para.FirstChild(), titleHTML)
		} else {
			para.AppendChild(para, titleHTML)
		}
		return ast.WalkSkipChildren, nil
	})
}

func escapeHTML(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;")
	return r.Replace(s)
}
