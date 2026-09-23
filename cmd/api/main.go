package main

import (
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/fx"

	"github.com/TakedaB/backend-challenge-go/internal/app"
	"github.com/TakedaB/backend-challenge-go/internal/httpapi"
	"github.com/TakedaB/backend-challenge-go/internal/infra/postgres"
)

func main() {
	fx.New(
		fx.Provide(
			postgres.NewConfigFromEnv,
			postgres.NewPool,
			postgres.NewWalletRepository,
			postgres.NewLedgerRepository,
			postgres.NewWagerTransactionRepository,
			app.NewWalletService,
			httpapi.NewWalletHandler,
			httpapi.NewWagerTransactionHandler,
			httpapi.NewRouter,
			httpapi.NewHTTPServer,
		),
		fx.Invoke(func(*http.Server) {}),
		fx.Invoke(func(*pgxpool.Pool) {}),
	).Run()
}
