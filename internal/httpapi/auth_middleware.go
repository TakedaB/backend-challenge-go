package httpapi

import (
	"net/http"
	"strings"
)

// AuthMiddleware requires every request to carry
// "Authorization: Bearer <token>" matching cfg.Token. It wraps only
// the wallet/wagering routes — /health stays open so infrastructure
// checks (load balancers, Docker healthchecks) don't need a token.
func AuthMiddleware(cfg AuthConfig, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header := r.Header.Get("Authorization")
		token, found := strings.CutPrefix(header, "Bearer ")
		if !found || token != cfg.Token {
			writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "missing or invalid bearer token"})
			return
		}
		next.ServeHTTP(w, r)
	})
}
