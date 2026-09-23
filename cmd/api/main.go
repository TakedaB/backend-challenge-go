package main

import (
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/fx"

	"github.com/TakedaB/backend-challenge-go/internal/httpapi"
	"github.com/TakedaB/backend-challenge-go/internal/infra/postgres"
)

func main() {
	fx.New(
		fx.Provide(
			postgres.NewConfigFromEnv,
			postgres.NewPool,
			httpapi.NewRouter,
			httpapi.NewHTTPServer,
		),
		// Both invokes exist purely to force Fx to build things nothing
		// else in the graph asks for: *http.Server (the HTTP server
		// itself) and *pgxpool.Pool (so the Postgres Ping in OnStart
		// actually runs, proving the connection works at startup).
		fx.Invoke(func(*http.Server) {}),
		fx.Invoke(func(*pgxpool.Pool) {}),
	).Run()
}
