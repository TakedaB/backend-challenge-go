package wallet

// Package wallet implements Wallet as the financial aggregate root.
//
// Wallet owns balance mutation exclusively: every balance change goes
// through Debit or Credit, which enforce the domain invariants (no
// negative balance, matching currency) and advance the optimistic
// concurrency version. The package has no dependency on Fx, HTTP, SQS
// or any persistence library — it is pure domain logic.

import (
	"errors"
	"time"

	"github.com/TakedaB/backend-challenge-go/internal/domain/money"
)

// Sentinel errors classifiable via errors.Is.
var (
	// ErrInsufficientBalance is returned when a debit would drive the
	// wallet balance below zero.
	ErrInsufficientBalance = errors.New("wallet: insufficient balance")
	// ErrCurrencyMismatch is returned when a movement's currency does
	// not match the wallet's own currency.
	ErrCurrencyMismatch = errors.New("wallet: currency mismatch")
	// ErrNonPositiveAmount is returned when a debit or credit is
	// attempted with a zero or negative amount. Movements must be
	// strictly positive; LOSS (which moves nothing) never calls these
	// methods at all.
	ErrNonPositiveAmount = errors.New("wallet: amount must be positive")
	// ErrEmptyPlayerID is returned when constructing a wallet without a
	// player identity.
	ErrEmptyPlayerID = errors.New("wallet: playerId is required")
	// ErrEmptyWalletID is returned when rehydrating a wallet without an
	// identity.
	ErrEmptyWalletID = errors.New("wallet: id is required")
	// ErrInvalidVersion is returned when rehydrating a wallet with a
	// non-positive version. Version starts at 1 and only increases.
	ErrInvalidVersion = errors.New("wallet: version must be >= 1")
	// ErrStaleVersion is returned by repository-level optimistic-lock
	// checks (not by this package directly) when a write targets a
	// version that no longer matches what is stored. It lives here so
	// callers across layers can errors.Is against a single definition.
	ErrStaleVersion = errors.New("wallet: stale version, wallet was modified concurrently")
)

// Direction identifies whether a ledger movement increases or
// decreases the wallet balance.
type Direction string

const (
	DirectionDebit  Direction = "DEBIT"
	DirectionCredit Direction = "CREDIT"
)

// Movement describes a single balance change produced by Debit or
// Credit, ready to be persisted as a WalletLedgerEntry by the caller.
// Wallet itself does not persist anything — it only computes and
// validates the movement and returns the resulting state.
type Movement struct {
	Direction     Direction
	Amount        money.Money
	BalanceBefore money.Money
	BalanceAfter  money.Money
	WalletVersion int64
}

// Wallet is the financial aggregate root. It carries identity, player,
// currency, balance, an optimistic-concurrency version, and creation
// and update timestamps.
//
// Concurrency strategy: optimistic locking via Version. Wallet itself
// never talks to a database or takes a lock — it just increments
// Version every time Debit or Credit successfully changes the
// balance. The repository layer (built later, on top of this) is
// responsible for a conditional update such as
// `UPDATE wallets SET balance = ?, version = ? WHERE id = ? AND version = ?`
// and must translate an unaffected row into ErrStaleVersion so the
// use case can retry against a freshly reloaded Wallet. This keeps
// wallets for different players/currencies free to advance in
// parallel — there is no global lock anywhere in this design.
type Wallet struct {
	id        string
	playerID  string
	currency  string
	balance   money.Money
	version   int64
	createdAt time.Time
	updatedAt time.Time
}

// New creates a brand-new Wallet with the given initial balance. This
// is the "creation" path — it is distinct from Rehydrate so that
// loading an existing wallet from storage never re-runs creation-time
// logic. Version starts at 1, matching the OPENING contract in the
// README (the version is 1 right after the wallet's opening, whether
// the initial balance is zero or positive).
//
// initialBalance must be zero or positive; a negative initial balance
// is rejected by the money package's own external-input parsing before
// it would ever reach here, but Wallet checks again defensively since
// it does not trust callers to have validated upstream.
func New(id, playerID string, initialBalance money.Money, now time.Time) (*Wallet, error) {
	if id == "" {
		return nil, ErrEmptyWalletID
	}
	if playerID == "" {
		return nil, ErrEmptyPlayerID
	}
	if initialBalance.IsNegative() {
		return nil, ErrNonPositiveAmount
	}

	return &Wallet{
		id:        id,
		playerID:  playerID,
		currency:  initialBalance.Currency(),
		balance:   initialBalance,
		version:   1,
		createdAt: now,
		updatedAt: now,
	}, nil
}

// Rehydrate reconstructs a Wallet from persisted state without
// replaying any balance movement, transition, or event emission. Use
// this whenever loading a wallet back from a repository.
func Rehydrate(id, playerID, currency string, balance money.Money, version int64, createdAt, updatedAt time.Time) (*Wallet, error) {
	if id == "" {
		return nil, ErrEmptyWalletID
	}
	if playerID == "" {
		return nil, ErrEmptyPlayerID
	}
	if version < 1 {
		return nil, ErrInvalidVersion
	}
	if balance.Currency() != currency {
		return nil, ErrCurrencyMismatch
	}

	return &Wallet{
		id:        id,
		playerID:  playerID,
		currency:  currency,
		balance:   balance,
		version:   version,
		createdAt: createdAt,
		updatedAt: updatedAt,
	}, nil
}

// ID returns the wallet's identity.
func (w *Wallet) ID() string { return w.id }

// PlayerID returns the owning player's identity.
func (w *Wallet) PlayerID() string { return w.playerID }

// Currency returns the wallet's ISO 4217 currency code.
func (w *Wallet) Currency() string { return w.currency }

// Balance returns the current balance.
func (w *Wallet) Balance() money.Money { return w.balance }

// Version returns the current optimistic-concurrency version.
func (w *Wallet) Version() int64 { return w.version }

// CreatedAt returns the wallet's creation instant.
func (w *Wallet) CreatedAt() time.Time { return w.createdAt }

// UpdatedAt returns the instant of the wallet's last balance change.
func (w *Wallet) UpdatedAt() time.Time { return w.updatedAt }

// Debit decreases the balance by amount, enforcing that the resulting
// balance is never negative. amount must be strictly positive and in
// the wallet's currency. On success, Version is incremented and
// UpdatedAt is set to now; the caller receives a Movement describing
// exactly what changed, which it uses to build the corresponding
// WalletLedgerEntry and persist both atomically.
//
// This method holds no lock itself — see the Wallet doc comment for
// the concurrency strategy. Two goroutines calling Debit concurrently
// on separate in-memory Wallet instances (as tests do to simulate
// concurrent requests before persistence exists) are not safe by
// themselves; real safety comes from the repository's conditional
// update keyed on Version, tested at the integration layer.
func (w *Wallet) Debit(amount money.Money, now time.Time) (Movement, error) {
	if amount.Currency() != w.currency {
		return Movement{}, ErrCurrencyMismatch
	}
	if !amount.IsPositive() {
		return Movement{}, ErrNonPositiveAmount
	}

	before := w.balance
	after, err := before.Sub(amount)
	if err != nil {
		return Movement{}, err
	}
	if after.IsNegative() {
		return Movement{}, ErrInsufficientBalance
	}

	w.balance = after
	w.version++
	w.updatedAt = now

	return Movement{
		Direction:     DirectionDebit,
		Amount:        amount,
		BalanceBefore: before,
		BalanceAfter:  after,
		WalletVersion: w.version,
	}, nil
}

// Credit increases the balance by amount. amount must be strictly
// positive and in the wallet's currency. On success, Version is
// incremented and UpdatedAt is set to now, mirroring Debit.
func (w *Wallet) Credit(amount money.Money, now time.Time) (Movement, error) {
	if amount.Currency() != w.currency {
		return Movement{}, ErrCurrencyMismatch
	}
	if !amount.IsPositive() {
		return Movement{}, ErrNonPositiveAmount
	}

	before := w.balance
	after, err := before.Add(amount)
	if err != nil {
		return Movement{}, err
	}

	w.balance = after
	w.version++
	w.updatedAt = now

	return Movement{
		Direction:     DirectionCredit,
		Amount:        amount,
		BalanceBefore: before,
		BalanceAfter:  after,
		WalletVersion: w.version,
	}, nil
}
