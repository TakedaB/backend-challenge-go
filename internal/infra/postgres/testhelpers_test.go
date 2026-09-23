package postgres

import (
	"context"
	"crypto/rand"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// connectTestPool opens a pool against the same Postgres the
// docker-compose.yml starts, using NewConfigFromEnv's defaults (which
// match the compose file's environment values). The pool is closed
// automatically when the test finishes.
func connectTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	cfg := NewConfigFromEnv()
	pool, err := pgxpool.New(context.Background(), cfg.DSN())
	if err != nil {
		t.Fatalf("connecting to test database: %v", err)
	}
	if err := pool.Ping(context.Background()); err != nil {
		t.Fatalf("pinging test database (is `docker compose up -d` running?): %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

// newTestUUID generates a random UUID v4 string, since the wallets,
// wallet_ledger_entries and wager_transactions tables all use UUID
// primary/foreign keys and every test needs fresh, non-colliding ids.
func newTestUUID(t *testing.T) string {
	t.Helper()
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		t.Fatalf("generating test uuid: %v", err)
	}
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant 10
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
