// Package links parses Obsidian wiki-links and maintains the link graph
// of the vault: resolution, backlinks and broken-link detection.
package links

import (
	"regexp"
	"strings"

	"github.com/obsidianweb/obsidianweb/packages/core"
)

// [[Target]] / [[Target|Alias]] / [[Target#Heading]] / [[Target#^block]] / ![[embed]]
var wikilinkRe = regexp.MustCompile(`(!?)\[\[([^\[\]\|#]*)(#[^\[\]\|]*)?(?:\|([^\[\]]*))?\]\]`)

var inlineTagRe = regexp.MustCompile(`(?:^|\s)#([\p{L}\p{N}_/-]+)`)

var codeBlockRe = regexp.MustCompile("(?s)```.*?```|`[^`\n]*`")

// ParseLinks extracts wiki-links from markdown, ignoring code blocks.
func ParseLinks(content []byte) []core.Link {
	text := codeBlockRe.ReplaceAllString(string(content), "")
	matches := wikilinkRe.FindAllStringSubmatch(text, -1)
	links := make([]core.Link, 0, len(matches))
	for _, m := range matches {
		raw := strings.TrimSpace(m[2])
		fragment := strings.TrimPrefix(strings.TrimSpace(m[3]), "#")
		if raw == "" && fragment == "" {
			continue
		}
		links = append(links, core.Link{
			Raw:      raw,
			Fragment: fragment,
			Alias:    strings.TrimSpace(m[4]),
			Embed:    m[1] == "!",
		})
	}
	return links
}

// ParseTags extracts inline #tags from markdown, ignoring code blocks.
func ParseTags(content []byte) []string {
	text := codeBlockRe.ReplaceAllString(string(content), "")
	matches := inlineTagRe.FindAllStringSubmatch(text, -1)
	seen := map[string]bool{}
	var tags []string
	for _, m := range matches {
		tag := m[1]
		if !seen[tag] {
			seen[tag] = true
			tags = append(tags, tag)
		}
	}
	return tags
}
