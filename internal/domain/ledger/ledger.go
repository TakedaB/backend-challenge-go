package ledger

// Package ledger implements WalletLedgerEntry: the immutable,
// append-only record of a single balance movement on a wallet.
//
// A ledger entry is never edited or deleted after creation — domain
// correctness comes entirely from validating the arithmetic at
// construction time. Uniqueness of (walletId, transactionId) and
// protection against edit/delete are enforced by the database schema
// once persistence exists; this package only guarantees that an entry
// that does get constructed is internally consistent.

import (
	"errors"
	"time"

	"github.com/TakedaB/backend-challenge-go/internal/domain/money"
	"github.com/TakedaB/backend-challenge-go/internal/domain/wallet"
)

// Sentinel errors classifiable via errors.Is.
var (
	// ErrEmptyID is returned when constructing an entry without an
	// identity.
	ErrEmptyID = errors.New("ledger: id is required")
	// ErrEmptyWalletID is returned when constructing an entry without a
	// wallet reference.
	ErrEmptyWalletID = errors.New("ledger: walletId is required")
	// ErrEmptyTransactionID is returned when constructing an entry
	// without a transaction reference.
	ErrEmptyTransactionID = errors.New("ledger: transactionId is required")
	// ErrInvalidDirection is returned when direction is neither DEBIT
	// nor CREDIT.
	ErrInvalidDirection = errors.New("ledger: direction must be DEBIT or CREDIT")
	// ErrNonPositiveAmount is returned when amount is zero or negative.
	// A ledger entry always records a strictly positive movement; its
	// Direction says which way it moved the balance. This mirrors the
	// README rule that LOSS and rejected operations never produce a
	// ledger entry at all — this type simply cannot represent "no
	// movement".
	ErrNonPositiveAmount = errors.New("ledger: amount must be positive")
	// ErrCurrencyMismatch is returned when amount, balanceBefore and
	// balanceAfter do not all share the same currency.
	ErrCurrencyMismatch = errors.New("ledger: currency mismatch among amount/balanceBefore/balanceAfter")
	// ErrInconsistentBalance is returned when balanceAfter does not
	// equal balanceBefore ± amount for the given direction. This is the
	// core invariant the README requires: "balanceAfter = balanceBefore
	// ± money, conforme a direção".
	ErrInconsistentBalance = errors.New("ledger: balanceAfter does not match balanceBefore and amount for this direction")
)

// WalletLedgerEntry is an immutable record of one balance movement.
// Once constructed via New, none of its fields can be changed — there
// are only getters. Building a new entry (rather than mutating one)
// is the only way to represent a correction, matching the append-only
// ledger requirement.
type WalletLedgerEntry struct {
	id            string
	walletID      string
	transactionID string
	direction     wallet.Direction
	amount        money.Money
	balanceBefore money.Money
	balanceAfter  money.Money
	createdAt     time.Time
}

// New constructs a WalletLedgerEntry, validating that:
//   - amount is strictly positive,
//   - amount, balanceBefore and balanceAfter all share one currency,
//   - direction is DEBIT or CREDIT,
//   - balanceAfter equals balanceBefore - amount (DEBIT) or
//     balanceBefore + amount (CREDIT) — exactly, with no tolerance.
//
// This is normally called with the exact same Movement.Amount,
// Movement.BalanceBefore, Movement.BalanceAfter that Wallet.Debit or
// Wallet.Credit just returned, so in practice validation always
// passes; it still runs the check explicitly so a bug elsewhere in
// the codebase cannot silently produce an inconsistent ledger row.
func New(
	id string,
	walletID string,
	transactionID string,
	direction wallet.Direction,
	amount money.Money,
	balanceBefore money.Money,
	balanceAfter money.Money,
	createdAt time.Time,
) (*WalletLedgerEntry, error) {
	if id == "" {
		return nil, ErrEmptyID
	}
	if walletID == "" {
		return nil, ErrEmptyWalletID
	}
	if transactionID == "" {
		return nil, ErrEmptyTransactionID
	}
	if direction != wallet.DirectionDebit && direction != wallet.DirectionCredit {
		return nil, ErrInvalidDirection
	}
	if !amount.IsPositive() {
		return nil, ErrNonPositiveAmount
	}
	if amount.Currency() != balanceBefore.Currency() || amount.Currency() != balanceAfter.Currency() {
		return nil, ErrCurrencyMismatch
	}

	var expectedAfter money.Money
	var err error
	switch direction {
	case wallet.DirectionDebit:
		expectedAfter, err = balanceBefore.Sub(amount)
	case wallet.DirectionCredit:
		expectedAfter, err = balanceBefore.Add(amount)
	}
	if err != nil {
		return nil, err
	}
	if !expectedAfter.Equal(balanceAfter) {
		return nil, ErrInconsistentBalance
	}

	return &WalletLedgerEntry{
		id:            id,
		walletID:      walletID,
		transactionID: transactionID,
		direction:     direction,
		amount:        amount,
		balanceBefore: balanceBefore,
		balanceAfter:  balanceAfter,
		createdAt:     createdAt,
	}, nil
}

// ID returns the entry's identity.
func (e *WalletLedgerEntry) ID() string { return e.id }

// WalletID returns the wallet this entry belongs to.
func (e *WalletLedgerEntry) WalletID() string { return e.walletID }

// TransactionID returns the WagerTransaction that produced this entry.
// The database enforces uniqueness of (WalletID, TransactionID) so a
// single transaction can never post two movements to the same wallet.
func (e *WalletLedgerEntry) TransactionID() string { return e.transactionID }

// Direction returns whether this entry is a DEBIT or a CREDIT.
func (e *WalletLedgerEntry) Direction() wallet.Direction { return e.direction }

// Amount returns the strictly positive movement amount.
func (e *WalletLedgerEntry) Amount() money.Money { return e.amount }

// BalanceBefore returns the wallet balance immediately before this
// movement.
func (e *WalletLedgerEntry) BalanceBefore() money.Money { return e.balanceBefore }

// BalanceAfter returns the wallet balance immediately after this
// movement.
func (e *WalletLedgerEntry) BalanceAfter() money.Money { return e.balanceAfter }

// CreatedAt returns when this entry was recorded. Entries are
// append-only, so there is no UpdatedAt.
func (e *WalletLedgerEntry) CreatedAt() time.Time { return e.createdAt }

// FromMovement is a convenience constructor that builds a
// WalletLedgerEntry directly from the Movement a Wallet.Debit or
// Wallet.Credit call just returned, avoiding repetition of its four
// money/direction fields at every call site.
func FromMovement(id, walletID, transactionID string, mv wallet.Movement, createdAt time.Time) (*WalletLedgerEntry, error) {
	return New(id, walletID, transactionID, mv.Direction, mv.Amount, mv.BalanceBefore, mv.BalanceAfter, createdAt)
}
