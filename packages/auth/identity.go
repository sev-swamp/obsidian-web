package auth

// Identity is what an external provider knows about a user.
type Identity struct {
	// Subject is the provider's immutable user id (OIDC `sub`). Account
	// matching MUST use it: username/email claims are often user-editable
	// at the IdP and matching by them allows impersonating existing
	// accounts.
	Subject  string
	Username string
	Email    string
	Groups   []string
}

// IdentityProvider is the contract for SSO modules (OIDC, OAuth). A
// provider module implements the redirect/exchange flow; after Exchange
// the platform provisions or matches a users.yaml account and issues
// its own session JWT. Concrete providers are separate modules — see
// plans/02-access-control.md §3.6.
type IdentityProvider interface {
	// Name identifies the provider ("google", "oidc", …).
	Name() string
	// AuthURL returns the provider login URL for the given CSRF state.
	AuthURL(state string) string
	// Exchange trades an authorization code for a verified identity.
	Exchange(code string) (Identity, error)
}
