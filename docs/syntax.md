# Note syntax reference

Everything the markdown pipeline renders. The same reference is
available in the web UI via the **?** button in the header (with
built-in search). The in-app version lives in
[apps/web/src/help/content.ts](../apps/web/src/help/content.ts) — keep
both in sync when the pipeline gains features.

## Headings

```
# H1
## H2
### H3 … up to ######
```

## Text formatting

| Syntax               | Result          |
| -------------------- | --------------- |
| `**bold**`           | **bold**        |
| `*italic*`           | *italic*        |
| `***bold italic***`  | ***bold italic*** |
| `~~strikethrough~~`  | ~~strikethrough~~ |
| `` `inline code` ``  | `inline code`   |
| `---`                | horizontal rule |

## Lists and tasks

```
- bullet item
1. numbered item
- [ ] open task
- [x] done task
```

Nest with a 4-space indent.

## Links

| Syntax                       | Meaning                              |
| ---------------------------- | ------------------------------------ |
| `[[Note]]`                   | wiki-link by note name               |
| `[[Folder/Note]]`            | wiki-link by path                    |
| `[[Note\|custom text]]`      | link with an alias                   |
| `[[Note#Heading]]`           | link to a heading                    |
| `[[Note#^block-id]]`         | link to a block                      |
| `![[image.png]]`             | embed an attachment (image/PDF/audio/video) |
| `[text](https://example.com)`| external link                        |

Frontmatter `aliases` make a note reachable under other names.

## Quotes and callouts

```
> plain quote

> [!note] Title
> Callout body
```

Callout types: `note`/`info` (blue), `tip`/`success`/`done` (green),
`warning`/`caution` (yellow), `danger`/`error`/`bug` (red),
`question`/`help` (orange), `example` (purple), `quote`, `abstract`,
`todo`.

## Tables

```
| Name | Role  |
| ---- | ----- |
| Ivan | admin |
```

## Code blocks

````
```go
func main() {}
```
````

The language after ``` enables syntax highlighting.

## Mermaid diagrams

````
```mermaid
graph LR
  A --> B
```
````

## Math (MathJax)

```
Inline: $e^{i\pi} + 1 = 0$

Block:
$$
\int_0^1 x^2\,dx
$$
```

## Tags

```
Inline #tag and nested #area/subarea
```

or in frontmatter: `tags: [project, idea]`.

## Frontmatter

```yaml
---
title: Display title
tags: [project]
aliases: [Other name]
---
```

## Template variables

| Variable              | Value                          |
| --------------------- | ------------------------------ |
| `{{title}}`           | new note title                 |
| `{{filename}}`        | sanitized file name            |
| `{{date}}`            | current date (`2026-07-12`)    |
| `{{time}}`            | current time (`14:30`)         |
| `{{datetime}}`        | ISO timestamp                  |
| `{{date:YYYY-MM-DD}}` | custom format (YYYY MM DD HH mm ss) |
| `{{anything}}`        | custom variable passed via API |

## Search operators

| Query               | Meaning                                |
| ------------------- | -------------------------------------- |
| `word1 word2`       | notes containing both words            |
| `tag:project report`| "report" only in notes tagged project  |
| `path:Projects plan`| "plan" only under the Projects folder  |
