# ADR-0004: Server-side markdown rendering with goldmark

- Status: accepted
- Date: 2026-07-11

## Context

Obsidian-flavoured markdown (wiki-links, callouts, embeds) needs the
link index to resolve targets. Rendering could happen client-side
(remark/markdown-it in React) or server-side (goldmark in Go).

## Decision

The server renders markdown to HTML via a goldmark pipeline
(`packages/markdown`): GFM + footnotes + syntax highlighting +
frontmatter split + wiki-links (resolved against the live `LinkIndex`)
+ Obsidian callouts (custom AST transformer) + Mermaid and MathJax
emitted as client-renderable markup. The frontend displays the HTML and
activates Mermaid/MathJax/link routing.

## Rationale

- Wiki-link resolution, backlinks and future plugins need the vault
  index — which lives on the server; duplicating it in the browser
  would violate the single-source-of-truth rule.
- One rendering implementation serves web, CLI export and future
  clients identically.
- goldmark is CommonMark-compliant and designed for AST-level
  extensions (callouts took ~100 lines).

## Consequences

- Interactive rendering (Mermaid, MathJax) stays client-side by design:
  the server emits `<pre class="mermaid">` and TeX delimiters.
- Raw HTML in notes is allowed (`html.WithUnsafe`), matching Obsidian's
  trust model of "your own vault". Multi-tenant deployments must add
  sanitization before exposing untrusted vaults.
- Plugin-driven markdown transforms hook into the goldmark pipeline on
  the server, keeping clients dumb.
