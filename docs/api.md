# REST API & WebSocket

Base URL: `http://<host>:8787`. All payloads are JSON. Note paths are
vault-relative; the `.md` extension is optional in requests.

When auth is enabled, send `Authorization: Bearer <token>` (or `?token=`
for media/WebSocket). Roles: `viewer` < `editor` < `admin`.

## Auth

| Method | Path               | Description                                  |
| ------ | ------------------ | -------------------------------------------- |
| POST   | `/api/auth/login`  | `{username, password}` → `{token, role}`     |
| GET    | `/api/auth/status` | `{authEnabled}`                              |

## Notes (viewer / editor)

| Method | Path                 | Role   | Description                                   |
| ------ | -------------------- | ------ | --------------------------------------------- |
| GET    | `/api/notes`         | viewer | All note metadata                             |
| GET    | `/api/note/{path}`   | viewer | Note: content, rendered `html`, frontmatter, backlinks |
| GET    | `/api/raw/{path}`    | viewer | Raw markdown                                  |
| POST   | `/api/note`          | editor | Create note (body below) → created note       |
| PUT    | `/api/note/{path}`   | editor | `{content}` — save note                       |
| DELETE | `/api/note/{path}`   | editor | Delete note                                   |

`POST /api/note` body:

```json
{
  "title": "Weekly sync",          // required
  "folder": "Meetings",            // optional; else type/default rules
  "type": "meeting",               // optional; maps via notes.typeFolders
  "template": "Meeting",           // optional; template name
  "variables": {"project": "X"},   // optional custom template variables
  "content": "..."                 // optional; ignored when template set
}
```

## Vault

| Method | Path                     | Role   | Description                          |
| ------ | ------------------------ | ------ | ------------------------------------ |
| GET    | `/api/tree`              | viewer | Directory tree                       |
| GET    | `/api/search?q=&limit=`  | viewer | Full-text search (`tag:x`, `path:x`) |
| GET    | `/api/recent?limit=`     | viewer | Recently modified notes              |
| GET    | `/api/templates`         | viewer | Available template names             |
| GET    | `/api/attachment/{path}` | viewer | Raw file (images, PDF, audio, video; supports Range) |
| POST   | `/api/upload`            | editor | multipart `file` (+ optional `folder`) → `{path}` |

## Settings & meta

| Method | Path                    | Role   | Description                             |
| ------ | ----------------------- | ------ | --------------------------------------- |
| GET    | `/api/settings`         | viewer | Note rules + vault dirs                 |
| PUT    | `/api/settings`         | admin  | `{notes: NoteRules}` — persisted to config |
| GET    | `/api/health`           | —      | Liveness                                |
| GET    | `/api/obsidian/plugins` | viewer | Installed Obsidian community plugins    |
| GET    | `/api/plugins/{id}/…`   | viewer | Routes registered by platform plugins   |

## WebSocket `/ws`

The server pushes JSON events; the UI updates without page reloads.

```json
{ "type": "file.changed", "path": "Projects/Roadmap.md" }
```

| Event           | Meaning                                    |
| --------------- | ------------------------------------------ |
| `file.created`  | Note or attachment appeared                |
| `file.changed`  | File content changed                       |
| `file.deleted`  | File removed (also fired on rename)        |
| `tree.changed`  | Directory structure changed                |
| `index.updated` | Link/search indexes refreshed              |

Errors are returned as `{"error": "message"}` with an appropriate HTTP
status (400, 401, 403, 404, 500).
