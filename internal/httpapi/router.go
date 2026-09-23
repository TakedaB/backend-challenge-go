package httpapi

// Package httpapi wires the HTTP transport layer: routes and the
// server itself. It knows about the domain only through the handlers
// it registers (added incrementally as we build out wallet/wager
// endpoints) — for now it only exposes a health check, enough to
// prove the Fx lifecycle wiring works end to end.

import "net/http"

// NewRouter builds the HTTP route table. Fx will call this
// automatically because it is registered with fx.Provide in main.go.
func NewRouter() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", handleHealth)
	return mux
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"ok"}`))
}
