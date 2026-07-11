package links

import (
	"path"
	"strings"
	"sync"

	"github.com/obsidianweb/obsidianweb/packages/core"
	"github.com/obsidianweb/obsidianweb/packages/shared"
)

// Index implements core.LinkIndex. It is safe for concurrent use and is
// updated incrementally as files change.
type Index struct {
	mu sync.RWMutex
	// byName maps a lowercase note name (basename without .md) to its path.
	byName map[string]string
	// byAlias maps a lowercase frontmatter alias to a note path.
	byAlias map[string]string
	// files is the set of all indexed vault paths (notes and attachments).
	files map[string]bool
	// outgoing holds parsed wiki-links per note.
	outgoing map[string][]core.Link
}

// NewIndex returns an empty link index.
func NewIndex() *Index {
	return &Index{
		byName:   map[string]string{},
		byAlias:  map[string]string{},
		files:    map[string]bool{},
		outgoing: map[string][]core.Link{},
	}
}

var _ core.LinkIndex = (*Index)(nil)

// Update (re)indexes a markdown note: name, aliases and outgoing links.
func (idx *Index) Update(p string, content []byte) {
	fm, _ := shared.SplitFrontmatter(content)
	aliases := shared.StringList(fmVal(fm, "aliases"))
	links := ParseLinks(content)

	idx.mu.Lock()
	defer idx.mu.Unlock()
	idx.removeLocked(p)
	idx.files[p] = true
	idx.byName[nameKey(p)] = p
	for _, a := range aliases {
		idx.byAlias[strings.ToLower(a)] = p
	}
	idx.outgoing[p] = links
}

// RegisterFile indexes a non-markdown file so embeds can resolve it.
func (idx *Index) RegisterFile(p string) {
	idx.mu.Lock()
	defer idx.mu.Unlock()
	idx.files[p] = true
	idx.byName[strings.ToLower(path.Base(p))] = p
}

// Remove drops a file from the index.
func (idx *Index) Remove(p string) {
	idx.mu.Lock()
	defer idx.mu.Unlock()
	idx.removeLocked(p)
}

func (idx *Index) removeLocked(p string) {
	delete(idx.files, p)
	delete(idx.outgoing, p)
	if idx.byName[nameKey(p)] == p {
		delete(idx.byName, nameKey(p))
	}
	if idx.byName[strings.ToLower(path.Base(p))] == p {
		delete(idx.byName, strings.ToLower(path.Base(p)))
	}
	for alias, target := range idx.byAlias {
		if target == p {
			delete(idx.byAlias, alias)
		}
	}
}

// Resolve maps a wiki-link target to a vault path. Supported forms:
// "Note", "folder/Note", "Note.md", an alias, or an attachment name.
func (idx *Index) Resolve(target string) (string, bool) {
	target = strings.TrimSpace(target)
	if target == "" {
		return "", false
	}
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	return idx.resolveLocked(target)
}

func (idx *Index) resolveLocked(target string) (string, bool) {
	lower := strings.ToLower(target)
	// Exact vault path (with or without .md).
	if idx.files[target] {
		return target, true
	}
	if idx.files[target+".md"] {
		return target + ".md", true
	}
	// Name of a note or attachment.
	if p, ok := idx.byName[strings.ToLower(path.Base(strings.TrimSuffix(lower, ".md")))]; ok {
		// For "folder/Note" style targets require the path suffix to match.
		if !strings.Contains(target, "/") || strings.HasSuffix(strings.ToLower(p), lower+".md") || strings.HasSuffix(strings.ToLower(p), lower) {
			return p, true
		}
	}
	if p, ok := idx.byName[lower]; ok {
		return p, true
	}
	if p, ok := idx.byAlias[lower]; ok {
		return p, true
	}
	return "", false
}

// Backlinks returns the paths of notes that link to p.
func (idx *Index) Backlinks(p string) []string {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	var sources []string
	for src, links := range idx.outgoing {
		if src == p {
			continue
		}
		for _, l := range links {
			if resolved, ok := idx.resolveLocked(l.Raw); ok && resolved == p {
				sources = append(sources, src)
				break
			}
		}
	}
	return sources
}

// BrokenLinks returns unresolved links grouped by source note.
func (idx *Index) BrokenLinks() map[string][]core.Link {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	broken := map[string][]core.Link{}
	for src, links := range idx.outgoing {
		for _, l := range links {
			if l.Raw == "" {
				continue // pure fragment link within the same note
			}
			if _, ok := idx.resolveLocked(l.Raw); !ok {
				broken[src] = append(broken[src], l)
			}
		}
	}
	return broken
}

// Stats returns total and broken link counts.
func (idx *Index) Stats() (int, int) {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	total, broken := 0, 0
	for _, links := range idx.outgoing {
		total += len(links)
		for _, l := range links {
			if l.Raw == "" {
				continue
			}
			if _, ok := idx.resolveLocked(l.Raw); !ok {
				broken++
			}
		}
	}
	return total, broken
}

func nameKey(p string) string {
	base := path.Base(p)
	return strings.ToLower(strings.TrimSuffix(base, path.Ext(base)))
}

func fmVal(fm map[string]any, key string) any {
	if fm == nil {
		return nil
	}
	return fm[key]
}
