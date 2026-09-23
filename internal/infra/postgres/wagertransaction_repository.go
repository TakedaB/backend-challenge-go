package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/TakedaB/backend-challenge-go/internal/domain/money"
	"github.com/TakedaB/backend-challenge-go/internal/domain/wagertransaction"
)

// ErrWagerTransactionNotFound is returned when no transaction matches
// the requested lookup.
var ErrWagerTransactionNotFound = errors.New("postgres: wager transaction not found")

// WagerTransactionRepository persists and loads WagerTransaction
// aggregates.
type WagerTransactionRepository struct {
	pool *pgxpool.Pool
}

// NewWagerTransactionRepository constructs a WagerTransactionRepository.
func NewWagerTransactionRepository(pool *pgxpool.Pool) *WagerTransactionRepository {
	return &WagerTransactionRepository{pool: pool}
}

// nullableString converts an empty string to nil so it is stored as
// SQL NULL — used for the columns that only apply to external
// transactions (OPENING leaves them empty).
func nullableString(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

// Create inserts a brand-new PENDING transaction row.
func (r *WagerTransactionRepository) Create(ctx context.Context, tx *wagertransaction.WagerTransaction) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO wager_transactions (
			id, external_transaction_id, provider_id, idempotency_key, payload_hash,
			wallet_id, player_id, round_id, game_id,
			kind, amount_minor_units, currency,
			reference_external_transaction_id, resolved_reference_transaction_id,
			state, failure_code, created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5,
			$6, $7, $8, $9,
			$10, $11, $12,
			$13, $14,
			$15, $16, $17, $18
		)
	`,
		tx.ID(), nullableString(tx.ExternalTransactionID()), nullableString(tx.ProviderID()),
		nullableString(tx.IdempotencyKey()), nullableString(tx.PayloadHash()),
		tx.WalletID(), tx.PlayerID(), nullableString(tx.RoundID()), nullableString(tx.GameID()),
		string(tx.Kind()), tx.Amount().MinorUnits(), tx.Amount().Currency(),
		nullableString(tx.ReferenceExternalTransactionID()), nullableString(tx.ResolvedReferenceTransactionID()),
		string(tx.State()), nullableString(tx.FailureCode()), tx.CreatedAt(), tx.UpdatedAt(),
	)
	if err != nil {
		return fmt.Errorf("postgres: inserting wager transaction: %w", err)
	}
	return nil
}

// scanRow is shared by FindByID and FindByIdempotencyKey: both select
// the same columns in the same order and rebuild the aggregate via
// wagertransaction.Rehydrate the same way.
func scanWagerTransactionRow(row pgx.Row) (*wagertransaction.WagerTransaction, error) {
	var (
		id                                                             string
		externalTransactionID, providerID, idempotencyKey, payloadHash *string
		walletID, playerID                                             string
		roundID, gameID                                                *string
		kind                                                           string
		amountMinor                                                    int64
		currency                                                       string
		referenceExternalTransactionID, resolvedReferenceTransactionID *string
		state                                                          string
		failureCode                                                    *string
		createdAt, updatedAt                                           time.Time
	)

	err := row.Scan(
		&id, &externalTransactionID, &providerID, &idempotencyKey, &payloadHash,
		&walletID, &playerID, &roundID, &gameID,
		&kind, &amountMinor, &currency,
		&referenceExternalTransactionID, &resolvedReferenceTransactionID,
		&state, &failureCode, &createdAt, &updatedAt,
	)
	if err != nil {
		return nil, err
	}

	amount, err := money.FromMinorUnits(amountMinor, currency)
	if err != nil {
		return nil, fmt.Errorf("postgres: rebuilding amount: %w", err)
	}

	return wagertransaction.Rehydrate(
		id, deref(externalTransactionID), deref(providerID), deref(idempotencyKey), deref(payloadHash),
		walletID, playerID, deref(roundID), deref(gameID),
		wagertransaction.Kind(kind), amount,
		deref(referenceExternalTransactionID), deref(resolvedReferenceTransactionID),
		wagertransaction.State(state), deref(failureCode),
		createdAt, updatedAt,
	)
}

// deref safely converts a possibly-nil *string (from a nullable SQL
// column) back to the domain's plain string representation, where
// NULL means "not applicable" and is represented as "".
func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

const wagerTransactionColumns = `
	id, external_transaction_id, provider_id, idempotency_key, payload_hash,
	wallet_id, player_id, round_id, game_id,
	kind, amount_minor_units, currency,
	reference_external_transaction_id, resolved_reference_transaction_id,
	state, failure_code, created_at, updated_at
`

// FindByID loads a transaction by its internal id.
func (r *WagerTransactionRepository) FindByID(ctx context.Context, id string) (*wagertransaction.WagerTransaction, error) {
	row := r.pool.QueryRow(ctx, "SELECT "+wagerTransactionColumns+" FROM wager_transactions WHERE id = $1", id)
	tx, err := scanWagerTransactionRow(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrWagerTransactionNotFound
		}
		return nil, fmt.Errorf("postgres: scanning wager transaction: %w", err)
	}
	return tx, nil
}

// FindByIdempotencyKey loads a transaction by (providerId,
// idempotencyKey) — the lookup the HTTP handler uses to detect a
// retried request before doing any processing. Returns
// ErrWagerTransactionNotFound when no such transaction exists yet,
// which the caller treats as "safe to proceed as a new request".
func (r *WagerTransactionRepository) FindByIdempotencyKey(ctx context.Context, providerID, idempotencyKey string) (*wagertransaction.WagerTransaction, error) {
	row := r.pool.QueryRow(ctx,
		"SELECT "+wagerTransactionColumns+" FROM wager_transactions WHERE provider_id = $1 AND idempotency_key = $2",
		providerID, idempotencyKey,
	)
	tx, err := scanWagerTransactionRow(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrWagerTransactionNotFound
		}
		return nil, fmt.Errorf("postgres: scanning wager transaction: %w", err)
	}
	return tx, nil
}

// Update persists a transaction's current state, failureCode and
// resolvedReferenceTransactionId after a state-machine transition
// (Process, Reject, Fail, ResolveReference, ...).
func (r *WagerTransactionRepository) Update(ctx context.Context, tx *wagertransaction.WagerTransaction) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE wager_transactions
		SET state = $1, failure_code = $2, resolved_reference_transaction_id = $3, updated_at = $4
		WHERE id = $5
	`, string(tx.State()), nullableString(tx.FailureCode()), nullableString(tx.ResolvedReferenceTransactionID()), tx.UpdatedAt(), tx.ID())
	if err != nil {
		return fmt.Errorf("postgres: updating wager transaction: %w", err)
	}
	return nil
}
