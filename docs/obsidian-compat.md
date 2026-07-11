# Obsidian compatibility

The platform works on an unmodified Obsidian vault and never changes its
structure. Compatibility beyond markdown is provided by
[packages/obsidian](../packages/obsidian/compat.go) and grows gradually.

## Supported today

| Area                     | Status                                             |
| ------------------------ | -------------------------------------------------- |
| Markdown + frontmatter   | ✅ CommonMark, GFM, YAML frontmatter               |
| Wiki-links               | ✅ `[[Note]]`, aliases, `#heading`, `#^block`, embeds |
| Backlinks                | ✅ computed from the live link index               |
| Callouts                 | ✅ common types incl. foldable markers (`+`/`-` parsed, rendered static) |
| Mermaid / MathJax        | ✅ client-side rendering                           |
| Tags                     | ✅ frontmatter + inline `#tags`                    |
| Templates                | ✅ `{{title}}`, `{{date}}`, `{{time}}`, `{{date:YYYY-MM-DD}}`, custom vars |
| `.obsidian/manifest` read| ✅ community plugin manifests + enabled state (`GET /api/obsidian/plugins`) |
| `.obsidian/app.json` read| ✅ attachment folder, new-note location            |

## Known limitations

- **Community plugins do not execute.** Obsidian plugins are JavaScript
  written against Obsidian's desktop API (DOM, workspace, editor); the
  server only reads their manifests and reports them. A future adapter
  service (TypeScript, out of process) could emulate a subset of the
  Obsidian API — the plugin runtime is deliberately isolated behind the
  SDK so this can be added without core changes.
- Dataview/Templater syntax is rendered as plain text.
- Graph view, canvas (`.canvas`) and themes are not implemented yet.
- Block reference *targets* (`^block-id`) are linkable but not
  highlighted at destination.
