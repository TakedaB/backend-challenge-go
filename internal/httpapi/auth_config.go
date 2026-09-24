package httpapi

import "os"

// AuthConfig holds the shared-secret token used by AuthMiddleware.
//
// This is a deliberately simplified stand-in for the real OIDC/
// Keycloak integration described in the challenge's original README
// (documented as a scope cut in ARCHITECTURE.md): a single static
// token from the environment, checked against the Authorization
// header, rather than per-provider JWT validation.
type AuthConfig struct {
	Token string
}

// NewAuthConfigFromEnv reads API_AUTH_TOKEN from the environment,
// falling back to a fixed development token so the API is usable
// out of the box with the .env.example defaults.
func NewAuthConfigFromEnv() AuthConfig {
	token := os.Getenv("API_AUTH_TOKEN")
	if token == "" {
		token = "dev-secret-token"
	}
	return AuthConfig{Token: token}
}
