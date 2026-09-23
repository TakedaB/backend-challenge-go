package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/TakedaB/backend-challenge-go/internal/domain/ledger"
)

// LedgerRepository persists WalletLedgerEntry records. It exposes
// only Create — there is intentionally no Update or Delete, matching
// the append-only invariant of the domain type itself.
type LedgerRepository struct {
	pool *pgxpool.Pool
}

// NewLedgerRepository constructs a LedgerRepository.
func NewLedgerRepository(pool *pgxpool.Pool) *LedgerRepository {
	return &LedgerRepository{pool: pool}
}

// Create inserts one ledger entry. The unique index
// idx_ledger_wallet_transaction (wallet_id, transaction_id) means a
// second attempt to insert for the same (wallet, transaction) pair
// fails at the database level — that constraint, not any Go code, is
// what makes "a transaction can never post two movements to the same
// wallet" airtight even under concurrent processing.
func (r *LedgerRepository) Create(ctx context.Context, e *ledger.WalletLedgerEntry) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO wallet_ledger_entries
			(id, wallet_id, transaction_id, direction, amount_minor_units, balance_before_minor, balance_after_minor, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`,
		e.ID(), e.WalletID(), e.TransactionID(), string(e.Direction()),
		e.Amount().MinorUnits(), e.BalanceBefore().MinorUnits(), e.BalanceAfter().MinorUnits(), e.CreatedAt(),
	)
	if err != nil {
		return fmt.Errorf("postgres: inserting ledger entry: %w", err)
	}
	return nil
}
