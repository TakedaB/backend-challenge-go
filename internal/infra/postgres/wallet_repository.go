package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/TakedaB/backend-challenge-go/internal/domain/money"
	"github.com/TakedaB/backend-challenge-go/internal/domain/wallet"
)

// ErrWalletNotFound is returned when no wallet matches the requested
// id.
var ErrWalletNotFound = errors.New("postgres: wallet not found")

// WalletRepository persists and loads Wallet aggregates. It is the
// only place in the codebase that knows the wallets table's schema —
// callers work exclusively with *wallet.Wallet.
type WalletRepository struct {
	pool *pgxpool.Pool
}

// NewWalletRepository constructs a WalletRepository. Fx will call
// this automatically once it is registered with fx.Provide, injecting
// the *pgxpool.Pool it already knows how to build.
func NewWalletRepository(pool *pgxpool.Pool) *WalletRepository {
	return &WalletRepository{pool: pool}
}

// Create inserts a brand-new wallet row. Fails if a wallet for the
// same (playerId, currency) already exists, via the unique index
// idx_wallets_player_currency.
func (r *WalletRepository) Create(ctx context.Context, w *wallet.Wallet) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO wallets (id, player_id, currency, balance_minor_units, version, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, w.ID(), w.PlayerID(), w.Currency(), w.Balance().MinorUnits(), w.Version(), w.CreatedAt(), w.UpdatedAt())
	if err != nil {
		return fmt.Errorf("postgres: inserting wallet: %w", err)
	}
	return nil
}

// FindByID loads a wallet by its id, reconstructing it via
// wallet.Rehydrate so no creation-time validation or transition logic
// re-runs — Rehydrate just restores exactly what was stored.
func (r *WalletRepository) FindByID(ctx context.Context, id string) (*wallet.Wallet, error) {
	var (
		walletID, playerID, currency string
		balanceMinor, version        int64
		createdAt, updatedAt         time.Time
	)

	err := r.pool.QueryRow(ctx, `
		SELECT id, player_id, currency, balance_minor_units, version, created_at, updated_at
		FROM wallets
		WHERE id = $1
	`, id).Scan(&walletID, &playerID, &currency, &balanceMinor, &version, &createdAt, &updatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrWalletNotFound
		}
		return nil, fmt.Errorf("postgres: scanning wallet: %w", err)
	}

	balance, err := money.FromMinorUnits(balanceMinor, currency)
	if err != nil {
		return nil, fmt.Errorf("postgres: rebuilding balance: %w", err)
	}

	return wallet.Rehydrate(walletID, playerID, currency, balance, version, createdAt, updatedAt)
}

// Update persists a wallet's new balance and version with an
// optimistic-lock conditional UPDATE: the WHERE clause matches on
// previousVersion (the version the caller loaded before calling
// Wallet.Debit/Credit, which already advanced w.Version() by one in
// memory). If no row matches — because another request updated the
// same wallet first — zero rows are affected and Update returns
// wallet.ErrStaleVersion, so the caller can reload and retry.
func (r *WalletRepository) Update(ctx context.Context, w *wallet.Wallet, previousVersion int64) error {
	tag, err := r.pool.Exec(ctx, `
		UPDATE wallets
		SET balance_minor_units = $1, version = $2, updated_at = $3
		WHERE id = $4 AND version = $5
	`, w.Balance().MinorUnits(), w.Version(), w.UpdatedAt(), w.ID(), previousVersion)
	if err != nil {
		return fmt.Errorf("postgres: updating wallet: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return wallet.ErrStaleVersion
	}
	return nil
}
