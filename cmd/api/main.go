package main

// Command api is the entrypoint for the backend-challenge-go HTTP
// service. All wiring lives here: Fx reads the fx.Provide list,
// figures out the dependency graph, and constructs everything in the
// right order.

import (
	"net/http"

	"go.uber.org/fx"

	"github.com/TakedaB/backend-challenge-go/internal/httpapi"
)

func main() {
	fx.New(
		fx.Provide(
			httpapi.NewRouter,
			httpapi.NewHTTPServer,
		),
		// fx.Invoke forces Fx to actually build *http.Server (and
		// everything it depends on) even though nothing else in the
		// graph asks for one directly — without this, Fx would never
		// construct it, since Go doesn't eagerly build unused values.
		fx.Invoke(func(*http.Server) {}),
	).Run()
}
