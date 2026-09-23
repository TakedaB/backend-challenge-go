package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/fx"
)

// NewPool opens a pgxpool.Pool and wires it to Fx's lifecycle: the
// connection is verified with a Ping on OnStart (so the application
// fails fast if Postgres is unreachable, instead of only failing on
// the first request), and the pool is closed cleanly on OnStop.
func NewPool(lc fx.Lifecycle, cfg Config) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(context.Background(), cfg.DSN())
	if err != nil {
		return nil, fmt.Errorf("postgres: creating pool: %w", err)
	}

	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			if err := pool.Ping(ctx); err != nil {
				return fmt.Errorf("postgres: ping failed: %w", err)
			}
			return nil
		},
		OnStop: func(ctx context.Context) error {
			pool.Close()
			return nil
		},
	})

	return pool, nil
}
