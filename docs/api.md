# REST API & WebSocket

Base URL: `http://<host>:8787`. All payloads are JSON. Note paths are
vault-relative; the `.md` extension is optional in requests.

When auth is enabled, send `Authorization: Bearer <token>` (or `?token=`
for media/WebSocket).

## Roles and permissions

Each JWT carries a `permissions` claim derived from the user's role at
login. The API enforces these permissions per endpoint and the frontend
uses the same list to show or hide actions (edit, delete, new note).

| Permission       | Grants                                        | viewer | editor | admin |
| ---------------- | --------------------------------------------- | :----: | :----: | :---: |
| `notes:read`     | read notes, tree, search, attachments, templates | ✅  | ✅     | ✅    |
| `notes:edit`     | create and save notes                         | —      | ✅     | ✅    |
| `notes:delete`   | delete notes                                  | —      | ✅     | ✅    |
| `files:upload`   | upload attachments                            | —      | ✅     | ✅    |
| `settings:write` | change platform settings                      | —      | —      | ✅    |

The mapping lives in `packages/auth` (`rolePermissions`). A response to
a request lacking a permission is `403 {"error":"missing permission: …"}`.

## Auth

| Method | Path               | Description                                              |
| ------ | ------------------ | -------------------------------------------------------- |
| POST   | `/api/auth/login`  | `{username, password}` → `{token, role, permissions}`    |
| GET    | `/api/auth/status` | `{authEnabled}`                                          |

## Notes

| Method | Path                 | Permission     | Description                                   |
| ------ | -------------------- | -------------- | --------------------------------------------- |
| GET    | `/api/notes`         | `notes:read`   | All note metadata                             |
| GET    | `/api/note/{path}`   | `notes:read`   | Note: content, rendered `html`, frontmatter, backlinks |
| GET    | `/api/raw/{path}`    | `notes:read`   | Raw markdown                                  |
| POST   | `/api/note`          | `notes:edit`   | Create note (body below) → created note       |
| PUT    | `/api/note/{path}`   | `notes:edit`   | `{content}` — save note                       |
| DELETE | `/api/note/{path}`   | `notes:delete` | Delete note                                   |

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

| Method | Path                     | Permission     | Description                          |
| ------ | ------------------------ | -------------- | ------------------------------------ |
| GET    | `/api/tree`              | `notes:read`   | Directory tree                       |
| GET    | `/api/search?q=&limit=`  | `notes:read`   | Full-text search (`tag:x`, `path:x`) |
| GET    | `/api/recent?limit=`     | `notes:read`   | Recently modified notes              |
| GET    | `/api/templates`         | `notes:read`   | Available template names             |
| GET    | `/api/attachment/{path}` | `notes:read`   | Raw file (images, PDF, audio, video; supports Range) |
| POST   | `/api/upload`            | `files:upload` | multipart `file` (+ optional `folder`) → `{path}` |

## Settings & meta

| Method | Path                    | Permission       | Description                             |
| ------ | ----------------------- | ---------------- | --------------------------------------- |
| GET    | `/api/settings`         | `notes:read`     | Note rules + vault dirs                 |
| PUT    | `/api/settings`         | `settings:write` | `{notes: NoteRules}` — persisted to config |
| GET    | `/api/health`           | —                | Liveness                                |
| GET    | `/api/obsidian/plugins` | `notes:read`     | Installed Obsidian community plugins    |
| GET    | `/api/plugins/{id}/…`   | `notes:read`     | Routes registered by platform plugins   |

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
