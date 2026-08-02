// The reference SSO plugin is an independent Go module: it keeps
// go-oidc/oauth2 out of the platform core (that is the whole point of
// plan 4). Nothing here imports the obsidianweb packages — the contract
// is REST only, so third-party authors can copy this module as a
// starting point in any language.
module github.com/obsidianweb/sso-oidc

go 1.25.0

require (
	github.com/coreos/go-oidc/v3 v3.20.0
	golang.org/x/oauth2 v0.36.0
)

require github.com/go-jose/go-jose/v4 v4.1.4 // indirect
