# Access control (teams)

Implements [plans/02-access-control.md](../plans/02-access-control.md).

Two layers:

1. **Global role** (viewer/editor/admin) → permissions embedded in the
   JWT (see [api.md](api.md)). This is the ceiling.
2. **Folder ACL** — ordered glob rules that can only *narrow* access
   per path. Enforced across every channel: direct reads/writes, tree,
   search, backlinks, recent, trash, history and WebSocket events.
   `none` paths answer 404 (their existence is not revealed).

## users.yaml

Accounts, groups and rules live in a hot-reloadable file
(`auth.usersFile`, default `users.yaml`; in Docker use a writable
volume, e.g. `/data/users.yaml`). It is managed from the admin UI
(`/admin`) or the admin API — no restarts. `config.yaml` accounts keep
working as an emergency fallback.

```yaml
users:
  - username: lena
    passwordHash: "$2a$10$…"   # obsidianweb-cli hash-password
    role: editor
    groups: [hr]

acl:
  - path: "HR/**"              # first matching rule decides
    allow:
      - { group: hr, access: write }
      - { user: sev, access: read }
    default: none              # everyone else: invisible
  - path: "Docs/**"
    allow:
      - { group: docs, access: write }
    default: read              # read-only for the rest
  - path: "Private/*/**"
    special: owner             # Private/<username>/… writable by owner only
```

Rules are evaluated top-down; put specific globs before general ones.
Paths without a matching rule are unrestricted (up to the global role).

## Admin API (`settings:write`)

| Method | Path | Description |
| --- | --- | --- |
| GET/POST | `/api/admin/users` | list / create (password hashed server-side) |
| PUT/DELETE | `/api/admin/users/{name}` | change role/groups/password / delete |
| POST | `/api/admin/users/{name}/revoke` | bump tokenVersion → all sessions & tokens invalid instantly |
| GET/PUT | `/api/admin/acl` | read / replace rules (validated) |
| GET | `/api/admin/check?user=&path=` | effective access — rule debugging |
| POST | `/api/admin/reload` | re-read users.yaml after manual edits |

## Personal API tokens

Any store-managed user can mint tokens for scripts/integrations at
`/tokens` (UI) or `POST /api/tokens {name, ttlDays, permissions}` —
permissions may only narrow the role's set. Tokens are listed and
revoked individually; `GET /api/note/...` etc. accept them like session
tokens. The token value is shown exactly once and never stored.

## SSO groundwork

`auth.IdentityProvider` is the interface an OAuth/OIDC module
implements; provider modules are a future, separate deliverable.
