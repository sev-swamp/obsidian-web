# Writing an SSO plugin

An SSO plugin is a **separate service** (any language, its own
container) that signs users in against an external identity provider
(OIDC, SAML, LDAP, …) and hands them back to Obsidian Web. The platform
knows nothing about the protocol — the contract is three REST calls and
a browser redirect. See plans/04-sso-plugin.md for the design.

## Security model

- The plugin authenticates to the platform with a **service token**
  (`Authorization: Bearer svc_<id>_<secret>`), created by an admin in
  Settings → SSO → Service tokens. The secret is shown once and stored
  bcrypt-hashed in users.yaml; revocation applies on the next request.
- A service token can **never issue a session directly**. It only mints
  a one-time login code (30 s TTL, single use); the browser exchanges
  the code for a JWT itself. The token is not an impersonation
  master key.
- `roleCeiling` caps every role the token may assign. A ceiling of
  `editor` can neither create admins nor touch existing admins.
- Run the plugin on an internal network (same docker network as the
  platform); its start route is the only thing the browser needs to
  reach.

## Contract

The token needs these permissions (assigned at creation):

| Permission              | Endpoint                           | Purpose |
| ----------------------- | ---------------------------------- | ------- |
| `users:provision`       | `POST /api/service/users`          | Upsert `{username, role, groups}` — SSO-only account, no password |
| `session:code`          | `POST /api/service/login-code`     | `{username}` → `{code, expiresAt}` |
| `login-providers:write` | `PUT /api/service/login-providers` | Register the login button `{id, name, url}` (optional — an admin can add it manually) |

## Flow

```
browser ── login page button ──► plugin /start
plugin  ── protocol dance ─────► identity provider ──► plugin /callback
plugin  ── svc token ──────────► POST /api/service/users        (upsert)
plugin  ── svc token ──────────► POST /api/service/login-code   {username}
plugin  ── 302 ────────────────► https://<platform>/login?code=<code>
browser ───────────────────────► POST /api/auth/code {code} → session JWT
```

1. The login page lists providers from `GET /api/auth/providers`; each
   button links to the plugin's start route (`url`).
2. The plugin runs its protocol (state/CSRF handling is entirely its
   responsibility) and derives a stable username from the provider's
   immutable subject — never from a user-editable claim alone.
3. It upserts the account, requests a login code and redirects the
   browser to `/login?code=…` on the platform.
4. The frontend exchanges the code for a JWT (`POST /api/auth/code`).
   The code burns on first use and after 30 seconds; user existence and
   tokenVersion are checked at exchange time, so revocation always wins.

## Error handling

| Situation                          | Platform's answer            | Plugin should |
| ---------------------------------- | ---------------------------- | ------------- |
| Role above the token ceiling       | 403                          | fix its role mapping / ask the admin for a higher ceiling |
| User already outranks the ceiling  | 403                          | redirect with an error message |
| Statically configured username     | 403                          | treat as reserved |
| users.yaml edited concurrently     | 409                          | retry after `POST /api/admin/reload` by an admin |
| Token revoked                      | 401                          | stop; a new token is required |

To show an error to the user, redirect to
`/login?sso_error=<urlencoded message>`.

## Deployment (compose sketch)

```yaml
services:
  obsidianweb:
    image: obsidianweb
    networks: [internal, web]
  sso-plugin:
    image: my-sso-plugin
    environment:
      PLATFORM_URL: http://obsidianweb:8787
      SERVICE_TOKEN: svc_sso-keycloak_…   # from Settings → SSO
    networks: [internal, web]             # /start must be browser-reachable
networks:
  internal: {internal: true}
  web: {}
```

The platform does not need to know the plugin exists — the only link is
the `loginProviders` entry pointing at it.
