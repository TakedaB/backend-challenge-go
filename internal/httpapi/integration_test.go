package httpapi

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/TakedaB/backend-challenge-go/internal/app"
	"github.com/TakedaB/backend-challenge-go/internal/infra/postgres"
)

// connectTestPool mirrors the same helper in the postgres package —
// duplicated here (rather than exported and imported) because it's a
// handful of lines and keeps this package's tests self-contained.
func connectTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	cfg := postgres.NewConfigFromEnv()
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

func newTestUUID(t *testing.T) string {
	t.Helper()
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		t.Fatalf("generating test uuid: %v", err)
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// newTestServer wires the real handlers and router against the real
// test Postgres pool — the same construction Fx does in cmd/api/main.go,
// just assembled by hand so the test doesn't need the Fx container.
func newTestServer(t *testing.T) (*httptest.Server, *pgxpool.Pool) {
	t.Helper()
	pool := connectTestPool(t)

	walletRepo := postgres.NewWalletRepository(pool)
	ledgerRepo := postgres.NewLedgerRepository(pool)
	txRepo := postgres.NewWagerTransactionRepository(pool)
	service := app.NewWalletService(walletRepo, ledgerRepo, txRepo)
	walletHandler := NewWalletHandler(service)
	wagerHandler := NewWagerTransactionHandler(service)
	mux := NewRouter(walletHandler, wagerHandler)

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, pool
}

func postJSON(t *testing.T, url string, body any) (*http.Response, map[string]any) {
	t.Helper()
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshaling request body: %v", err)
	}
	resp, err := http.Post(url, "application/json", bytes.NewReader(b))
	if err != nil {
		t.Fatalf("POST %s: %v", url, err)
	}
	defer resp.Body.Close()

	var parsed map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		t.Fatalf("decoding response body: %v", err)
	}
	return resp, parsed
}

func getJSON(t *testing.T, url string) (*http.Response, map[string]any) {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer resp.Body.Close()

	var parsed map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		t.Fatalf("decoding response body: %v", err)
	}
	return resp, parsed
}

// TestIntegration_FullFlow exercises the exact manual flow already
// verified by hand: open a wallet, process a BET against it, confirm
// the balance moved — all through real HTTP requests, real handlers,
// real Postgres.
func TestIntegration_FullFlow(t *testing.T) {
	srv, pool := newTestServer(t)

	_, openResp := postJSON(t, srv.URL+"/wallets", map[string]any{
		"playerId":       "player-" + newTestUUID(t),
		"currency":       "BRL",
		"initialBalance": "100.00",
	})
	walletID := openResp["id"].(string)
	t.Cleanup(func() {
		pool.Exec(context.Background(), "DELETE FROM wager_transactions WHERE wallet_id = $1", walletID)
		pool.Exec(context.Background(), "DELETE FROM wallet_ledger_entries WHERE wallet_id = $1", walletID)
		pool.Exec(context.Background(), "DELETE FROM wallets WHERE id = $1", walletID)
	})

	if openResp["balance"] != "100.00" {
		t.Errorf("open wallet balance = %v, want 100.00", openResp["balance"])
	}

	betResp, betBody := postJSON(t, srv.URL+"/wagering/transactions", map[string]any{
		"externalTransactionId": "ext-" + newTestUUID(t),
		"providerId":            "provider-a",
		"idempotencyKey":        "idem-" + newTestUUID(t),
		"walletId":              walletID,
		"playerId":              "player-1",
		"roundId":               "round-1",
		"gameId":                "game-1",
		"kind":                  "BET",
		"amount":                "30.00",
		"currency":              "BRL",
	})
	if betResp.StatusCode != http.StatusOK {
		t.Fatalf("BET status = %d, want 200, body = %v", betResp.StatusCode, betBody)
	}
	if betBody["state"] != "PROCESSED" {
		t.Errorf("BET state = %v, want PROCESSED", betBody["state"])
	}

	_, walletAfter := getJSON(t, srv.URL+"/wallets/"+walletID)
	if walletAfter["balance"] != "70.00" {
		t.Errorf("balance after BET = %v, want 70.00", walletAfter["balance"])
	}
}

// TestIntegration_Idempotency proves the same guarantee we confirmed
// manually: replaying the same (providerId, idempotencyKey) returns
// the original transaction and never debits twice.
func TestIntegration_Idempotency(t *testing.T) {
	srv, pool := newTestServer(t)

	_, openResp := postJSON(t, srv.URL+"/wallets", map[string]any{
		"playerId":       "player-" + newTestUUID(t),
		"currency":       "BRL",
		"initialBalance": "100.00",
	})
	walletID := openResp["id"].(string)
	t.Cleanup(func() {
		pool.Exec(context.Background(), "DELETE FROM wager_transactions WHERE wallet_id = $1", walletID)
		pool.Exec(context.Background(), "DELETE FROM wallet_ledger_entries WHERE wallet_id = $1", walletID)
		pool.Exec(context.Background(), "DELETE FROM wallets WHERE id = $1", walletID)
	})

	idempotencyKey := "idem-" + newTestUUID(t)
	betRequest := map[string]any{
		"externalTransactionId": "ext-" + newTestUUID(t),
		"providerId":            "provider-a",
		"idempotencyKey":        idempotencyKey,
		"walletId":              walletID,
		"playerId":              "player-1",
		"roundId":               "round-1",
		"gameId":                "game-1",
		"kind":                  "BET",
		"amount":                "30.00",
		"currency":              "BRL",
	}

	_, first := postJSON(t, srv.URL+"/wagering/transactions", betRequest)
	_, second := postJSON(t, srv.URL+"/wagering/transactions", betRequest)

	if first["id"] != second["id"] {
		t.Errorf("replayed request got a different transaction id: first=%v second=%v", first["id"], second["id"])
	}

	_, walletAfter := getJSON(t, srv.URL+"/wallets/"+walletID)
	if walletAfter["balance"] != "70.00" {
		t.Errorf("balance after replayed BET = %v, want 70.00 (must not double-debit)", walletAfter["balance"])
	}
}

// TestIntegration_ConcurrentBets_OnlyOneSucceeds is the README's
// mandatory scenario at the level it actually matters: two real,
// concurrent HTTP requests against the same wallet, going through the
// full stack (handler -> service -> repository -> Postgres). This is
// what proves the optimistic-lock version check works under genuine
// concurrent load, not just sequential calls.
func TestIntegration_ConcurrentBets_OnlyOneSucceeds(t *testing.T) {
	srv, pool := newTestServer(t)

	_, openResp := postJSON(t, srv.URL+"/wallets", map[string]any{
		"playerId":       "player-" + newTestUUID(t),
		"currency":       "BRL",
		"initialBalance": "100.00",
	})
	walletID := openResp["id"].(string)
	t.Cleanup(func() {
		pool.Exec(context.Background(), "DELETE FROM wager_transactions WHERE wallet_id = $1", walletID)
		pool.Exec(context.Background(), "DELETE FROM wallet_ledger_entries WHERE wallet_id = $1", walletID)
		pool.Exec(context.Background(), "DELETE FROM wallets WHERE id = $1", walletID)
	})

	var wg sync.WaitGroup
	var processedCount, rejectedOrFailedCount int64

	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			_, body := postJSON(t, srv.URL+"/wagering/transactions", map[string]any{
				"externalTransactionId": fmt.Sprintf("ext-concurrent-%d-%s", idx, newTestUUID(t)),
				"providerId":            "provider-a",
				"idempotencyKey":        fmt.Sprintf("idem-concurrent-%d-%s", idx, newTestUUID(t)),
				"walletId":              walletID,
				"playerId":              "player-1",
				"roundId":               "round-1",
				"gameId":                "game-1",
				"kind":                  "BET",
				"amount":                "80.00",
				"currency":              "BRL",
			})
			if body["state"] == "PROCESSED" {
				atomic.AddInt64(&processedCount, 1)
			} else {
				atomic.AddInt64(&rejectedOrFailedCount, 1)
			}
		}(i)
	}
	wg.Wait()

	if processedCount != 1 {
		t.Errorf("processedCount = %d, want exactly 1", processedCount)
	}
	if rejectedOrFailedCount != 1 {
		t.Errorf("rejectedOrFailedCount = %d, want exactly 1", rejectedOrFailedCount)
	}

	_, walletAfter := getJSON(t, srv.URL+"/wallets/"+walletID)
	if walletAfter["balance"] != "20.00" {
		t.Errorf("final balance = %v, want 20.00 (exactly one 80.00 debit from 100.00)", walletAfter["balance"])
	}
}
