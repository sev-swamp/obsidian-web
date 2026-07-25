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
| `history:read`   | view note history and diffs                   | —      | ✅     | ✅    |
| `trash:read`     | list the trash                                | —      | ✅     | ✅    |
| `trash:purge`    | remove entries from the trash                 | —      | —      | ✅    |
| `files:upload`   | upload attachments                            | —      | ✅     | ✅    |
| `settings:write` | settings UI: users, groups, ACL, SSO          | —      | —      | ✅    |

The mapping lives in `packages/auth` (`rolePermissions`). A response to
a request lacking a permission is `403 {"error":"missing permission: …"}`.

## Auth

| Method | Path                  | Description                                              |
| ------ | --------------------- | -------------------------------------------------------- |
| POST   | `/api/auth/login`     | `{username, password}` → `{token, role, permissions}`    |
| GET    | `/api/auth/status`    | `{authEnabled}`                                          |
| POST   | `/api/auth/code`      | `{code}` — exchange a one-time login code (minted by an SSO plugin via the service API) for a session: `{token, username, role, permissions}`. Codes are single-use with a 30 s TTL; invalid/expired → 401 |
| GET    | `/api/auth/providers` | `{providers: [{id, name, url}]}` — external sign-in buttons for the login page (empty when auth is off) |

## Service API (external auth plugins)

Machine endpoints for SSO plugins (see docs/sso-plugin.md). They accept
only **service tokens** — `Authorization: Bearer svc_<id>_<secret>` —
never user JWTs. A token's permissions are explicit (not derived from a
role); its `roleCeiling` caps every role it may assign. Secrets are
bcrypt-hashed in users.yaml, so revocation applies on the next request.
Every call is audit-logged with the actor `svc:<id>`.

| Method   | Path                          | Permission              | Description |
| -------- | ----------------------------- | ----------------------- | ----------- |
| POST/PUT | `/api/service/users`          | `users:provision`       | Upsert `{username, role, groups}` as an SSO-only account (no password, sign-in only via the provider). Role above the ceiling, users already outranking the ceiling and statically configured accounts → 403; passwords are never touched, deletion is impossible |
| POST     | `/api/service/login-code`     | `session:code`          | `{username}` → `{code, expiresAt}` — one-time login code for an **existing** user (unknown → 404). Existence and tokenVersion are re-checked at exchange time |
| PUT      | `/api/service/login-providers`| `login-providers:write` | `{id, name, url}` — the plugin registers/updates its own login button |

Admin management (permission `settings:write`):

| Method | Path                            | Description |
| ------ | ------------------------------- | ----------- |
| GET    | `/api/admin/service-tokens`     | `{tokens, permissions}` — records (hashes never leave the server) + assignable catalog |
| POST   | `/api/admin/service-tokens`     | `{id, name, permissions, roleCeiling}` → `{token, record}` — the full token is shown exactly once |
| DELETE | `/api/admin/service-tokens/{id}`| Revoke (immediate, no restart) |
| GET    | `/api/admin/login-providers`    | `{providers}` |
| PUT    | `/api/admin/login-providers`    | `{providers: [{id, name, url}]}` — replace the list |

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

## History & trash

| Method | Path                     | Permission     | Description                                   |
| ------ | ------------------------ | -------------- | --------------------------------------------- |
| GET    | `/api/history/{path}?limit=` | `history:read` | Revisions, newest first (`sourceRev` marks what a restore was taken from) |
| GET    | `/api/diff/{path}?rev=` or `?from=&to=` | `history:read` | Line diff: what `rev` changed, or between two revisions |
| POST   | `/api/restore/{path}`    | `notes:edit`   | `{rev}` → `{status: "restored"}`, or `{status: "unchanged"}` when the content already matches |
| GET    | `/api/trash?limit=`      | `trash:read`   | Deleted notes (`restoreRev`, `deleteRev`), filtered by the caller's folder ACL |
| POST   | `/api/trash/restore`     | `notes:edit`   | `{path}` — restore from trash (needs write access to the path) |
| POST   | `/api/trash/purge`       | `trash:purge`  | `{path}` — remove the entry from the trash (needs write access; content stays in git history) |
| POST   | `/api/trash/purge-all`   | `trash:purge`  | Remove every entry the caller can see in their trash |

Restore commits carry a `Restored-From: <rev>` trailer; the trash is an
explicit index in `.git/obsidianweb-trash.json` maintained on every
delete/restore, so entries never age out (see ADR-0005). Purging only
hides the entry — the note's content remains reachable through history.

## Settings & meta

| Method | Path                    | Permission       | Description                             |
| ------ | ----------------------- | ---------------- | --------------------------------------- |
| GET    | `/api/settings`         | `notes:read`     | Note rules + vault dirs + history `{enabled, mode}` |
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
