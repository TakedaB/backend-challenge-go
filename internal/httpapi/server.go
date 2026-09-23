package httpapi

import (
	"context"
	"net"
	"net/http"

	"go.uber.org/fx"
)

// NewHTTPServer builds an *http.Server wired to Fx's lifecycle: it
// starts listening when the Fx app starts (OnStart) and shuts down
// gracefully when the Fx app stops (OnStop). Fx calls this because it
// is registered with fx.Provide, and the *http.ServeMux parameter is
// satisfied automatically from NewRouter's own fx.Provide — this is
// the essence of Fx's dependency injection: we never call NewRouter
// ourselves, we just declare that NewHTTPServer needs one.
func NewHTTPServer(lc fx.Lifecycle, mux *http.ServeMux) *http.Server {
	srv := &http.Server{
		Addr:    ":8080",
		Handler: mux,
	}

	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			ln, err := net.Listen("tcp", srv.Addr)
			if err != nil {
				return err
			}
			go srv.Serve(ln)
			return nil
		},
		OnStop: func(ctx context.Context) error {
			return srv.Shutdown(ctx)
		},
	})

	return srv
}
