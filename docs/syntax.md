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
> Callout body — can span
> multiple lines.
```

The type goes in `[!brackets]`; the title after it is optional (the
capitalized type name is used when omitted).

| Color     | Types                                          | Example                  |
| --------- | ---------------------------------------------- | ------------------------ |
| 🔵 blue   | `note`, `info`, `todo`                         | `> [!note] Heads up`     |
| 🟢 green  | `tip`, `hint`, `success`, `check`, `done`      | `> [!done] Shipped`      |
| 🟡 yellow | `warning`, `caution`, `attention`              | `> [!warning] Careful`   |
| 🔴 red    | `danger`, `error`, `bug`, `fail`, `failure`    | `> [!bug] Known issue`   |
| 🟠 orange | `question`, `help`, `faq`                      | `> [!question] Why?`     |
| 🟣 purple | `example`                                      | `> [!example] Sample`    |
| ⚪ gray   | `quote`, `cite`                                | `> [!quote] — Author`    |
| 🩵 cyan   | `abstract`, `summary`, `tldr`                  | `> [!tldr] In short`     |

Unknown types still render as callouts with the default (blue) color:

```
> [!my-type] Custom block
```

### Adding a custom callout color

Colors are plain CSS in
[apps/web/src/index.css](../apps/web/src/index.css): each type sets the
`--callout-color` variable. To give `[!my-type]` its own color, add one
rule and rebuild the frontend:

```css
.markdown blockquote.callout-my-type { --callout-color: #e11d48; }
```

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
