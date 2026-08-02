# sso-oidc — reference SSO plugin for Obsidian Web

A standalone service that signs users in against any **OpenID Connect**
provider (Keycloak, Authentik, Authelia, Google, Azure AD, …) and hands
them back to Obsidian Web. It is the reference implementation of the
plugin contract in [../../docs/sso-plugin.md](../../docs/sso-plugin.md);
copy it as a starting point for other protocols (SAML, LDAP) in any
language — the platform contract is REST only.

The platform core contains no OIDC code: this module is the only place
`go-oidc`/`oauth2` live.

## How it works

```
browser ─ login button ─► /start ─► IdP ─► /callback
/callback ─ service token ─► POST /api/service/users       (upsert)
/callback ─ service token ─► POST /api/service/login-code  {username}
/callback ─ 302 ──────────► <platform>/login?code=<code>
browser ──────────────────► POST /api/auth/code {code} → session JWT
```

The service token can provision users and mint a one-time, 30-second
login code — it can **never** issue a session directly, so a leaked
token is not an impersonation master key. Keep the plugin↔platform link
on an internal network.

## Setup

1. In Obsidian Web, go to **Settings → SSO → Service tokens** and create
   a token with `users:provision` and `session:code` (add
   `login-providers:write` too if you want the plugin to self-register
   its login button). Copy the secret — it is shown once.
2. Register an OIDC client at your IdP with redirect URI
   `<PLUGIN_PUBLIC_URL>/callback`.
3. Run the plugin (see [docker-compose.example.yml](docker-compose.example.yml)).
4. If you did not set `REGISTER_PROVIDER=true`, add a login button under
   **Settings → SSO → Login providers** pointing at
   `<PLUGIN_PUBLIC_URL>/start`.

## Configuration (environment)

| Variable              | Required | Default              | Purpose |
| --------------------- | :------: | -------------------- | ------- |
| `PLATFORM_URL`        | ✅       | —                    | Platform base URL for server-to-server calls (internal network) |
| `PLATFORM_PUBLIC_URL` |          | = `PLATFORM_URL`     | Where the browser is redirected to finish login |
| `PLUGIN_PUBLIC_URL`   | ✅       | —                    | This plugin's public base URL; redirect_uri is `<it>/callback` |
| `SERVICE_TOKEN`       | ✅       | —                    | `svc_<id>_<secret>` from Settings → SSO |
| `OIDC_ISSUER`         | ✅       | —                    | OIDC issuer (discovery URL without `/.well-known/...`) |
| `OIDC_CLIENT_ID`      | ✅       | —                    | OIDC client id |
| `OIDC_CLIENT_SECRET`  |          | —                    | OIDC client secret (public clients may omit) |
| `OIDC_SCOPES`         |          | `openid profile email` | Space/comma-separated scopes |
| `OIDC_GROUPS_CLAIM`   |          | `groups`             | id-token claim carrying group membership |
| `DEFAULT_ROLE`        |          | `viewer`             | Role for provisioned accounts (≤ token `roleCeiling`) |
| `REGISTER_PROVIDER`   |          | `false`              | Self-register the login button on startup |
| `PROVIDER_ID`         |          | `oidc`               | Login-provider id (with `REGISTER_PROVIDER`) |
| `PROVIDER_NAME`       |          | `Sign in with SSO`   | Login-button label |
| `LISTEN_ADDR`         |          | `:8080`              | Bind address |

## Migrating from the built-in SSO

The built-in OIDC support was removed once this plugin reached parity.
To carry an existing `users.yaml` `sso:` block over to plugin
environment variables:

```bash
obsidianweb-cli migrate-sso -users /data/users.yaml
```

It prints the equivalent env vars (issuer, client id/secret, default
role) and reminds you to create a service token. The old `sso:` block in
`users.yaml` is ignored by the platform and can be deleted.

## Build & test

```bash
go build ./...
go test ./...
```
