package wagertransaction

// Package wagertransaction implements WagerTransaction: the aggregate
// that represents one external or internal wager operation and the
// state machine that governs how it moves from PENDING to a terminal
// state.
//
// The package has no dependency on Fx, HTTP, SQS or any persistence
// library — it is pure domain logic. Persistence, retries and outbox
// publishing are handled by layers built on top of this one.

import (
	"errors"
	"time"

	"github.com/TakedaB/backend-challenge-go/internal/domain/money"
)

// Kind identifies the type of wager operation.
type Kind string

const (
	// KindOpening is reserved for internal wallet opening. It must be
	// rejected whenever it arrives via HTTP or SQS — this package does
	// not enforce that transport-level rule itself (it has no concept
	// of HTTP or SQS), but NewExternal refuses to construct a KindOpening
	// transaction, which is the domain-level half of that guarantee.
	KindOpening Kind = "OPENING"
	// KindBet debits the wallet. Requires a positive amount and
	// sufficient balance (enforced by the wallet, not here).
	KindBet Kind = "BET"
	// KindWin credits the wallet. Requires a positive amount.
	KindWin Kind = "WIN"
	// KindLoss moves nothing. Requires amount == 0.00 and produces no
	// ledger entry and no balance/version change.
	KindLoss Kind = "LOSS"
	// KindRefund credits the wallet, fully refunding a processed BET.
	// Requires a positive amount and a reference to that BET.
	KindRefund Kind = "REFUND"
	// KindRollback reverses a processed BET, WIN or REFUND. Requires a
	// positive amount and a reference to the reversed operation.
	KindRollback Kind = "ROLLBACK"
)

// isValidExternalKind reports whether k is one of the five external
// kinds accepted over HTTP/SQS (i.e. anything but OPENING).
func isValidExternalKind(k Kind) bool {
	switch k {
	case KindBet, KindWin, KindLoss, KindRefund, KindRollback:
		return true
	default:
		return false
	}
}

// State is a WagerTransaction's position in its state machine.
type State string

const (
	// StatePending: accepted, processing not yet concluded.
	StatePending State = "PENDING"
	// StatePendingReference: waiting on a reference
	// (REFUND/ROLLBACK target) that has not arrived yet.
	StatePendingReference State = "PENDING_REFERENCE"
	// StateProcessed is terminal: the operation completed successfully.
	StateProcessed State = "PROCESSED"
	// StateRejected is terminal: the operation was refused by a
	// business rule (e.g. insufficient balance, reference not found).
	StateRejected State = "REJECTED"
	// StateFailed is terminal: a permanent infrastructure failure was
	// recorded for audit.
	StateFailed State = "FAILED"
)

// isTerminal reports whether s is one of the three terminal states, in
// which no further transition is allowed.
func isTerminal(s State) bool {
	return s == StateProcessed || s == StateRejected || s == StateFailed
}

// Sentinel errors classifiable via errors.Is.
var (
	// ErrEmptyID is returned when constructing without an internal id.
	ErrEmptyID = errors.New("wagertransaction: id is required")
	// ErrOpeningNotAllowedExternally is returned when NewExternal is
	// called with KindOpening — that kind is reserved for internal use
	// and must never be accepted from HTTP or SQS.
	ErrOpeningNotAllowedExternally = errors.New("wagertransaction: OPENING is not a valid external kind")
	// ErrInvalidKind is returned for a kind outside the six known
	// values.
	ErrInvalidKind = errors.New("wagertransaction: invalid kind")
	// ErrMissingExternalMetadata is returned when an external
	// transaction is missing providerId, externalTransactionId,
	// idempotencyKey or payloadHash.
	ErrMissingExternalMetadata = errors.New("wagertransaction: providerId, externalTransactionId, idempotencyKey and payloadHash are required for external transactions")
	// ErrLossAmountMustBeZero is returned when a LOSS transaction
	// carries a non-zero amount.
	ErrLossAmountMustBeZero = errors.New("wagertransaction: LOSS requires amount == 0.00")
	// ErrAmountMustBePositive is returned when BET, WIN, REFUND or
	// ROLLBACK carries a zero or negative amount.
	ErrAmountMustBePositive = errors.New("wagertransaction: BET, WIN, REFUND and ROLLBACK require a positive amount")
	// ErrReferenceRequired is returned when REFUND or ROLLBACK is
	// missing referenceExternalTransactionId.
	ErrReferenceRequired = errors.New("wagertransaction: REFUND and ROLLBACK require referenceExternalTransactionId")
	// ErrReferenceNotApplicable is returned when a kind other than
	// REFUND/ROLLBACK carries a reference — the README says reference
	// only applies to those two kinds.
	ErrReferenceNotApplicable = errors.New("wagertransaction: referenceExternalTransactionId only applies to REFUND and ROLLBACK")
	// ErrInvalidTransition is returned when a state-machine method is
	// called from a state that does not allow it — most commonly
	// because the transaction is already terminal.
	ErrInvalidTransition = errors.New("wagertransaction: invalid state transition")
	// ErrEmptyFailureCode is returned when Reject or Fail is called
	// without a failureCode. The README requires every rejection to
	// carry a stable, documented failureCode.
	ErrEmptyFailureCode = errors.New("wagertransaction: failureCode is required")
)

// WagerTransaction is the aggregate representing one wager operation,
// external or internal, and its progress through the state machine
// described in the package doc.
type WagerTransaction struct {
	id                             string
	externalTransactionID          string // empty for OPENING
	providerID                     string // empty for OPENING
	idempotencyKey                 string // empty for OPENING
	payloadHash                    string // empty for OPENING
	walletID                       string
	playerID                       string
	roundID                        string // not applicable to OPENING
	gameID                         string // not applicable to OPENING
	kind                           Kind
	amount                         money.Money
	referenceExternalTransactionID string // only for REFUND/ROLLBACK
	resolvedReferenceTransactionID string // internal id, once resolved
	state                          State
	failureCode                    string
	createdAt                      time.Time
	updatedAt                      time.Time
}

// NewExternal constructs a PENDING WagerTransaction for one of the
// five external kinds (BET, WIN, LOSS, REFUND, ROLLBACK), validating
// the per-kind amount rule and the reference requirement up front.
// OPENING is rejected here unconditionally: callers that need to open
// a wallet must use NewOpening instead.
func NewExternal(
	id string,
	externalTransactionID string,
	providerID string,
	idempotencyKey string,
	payloadHash string,
	walletID string,
	playerID string,
	roundID string,
	gameID string,
	kind Kind,
	amount money.Money,
	referenceExternalTransactionID string,
	now time.Time,
) (*WagerTransaction, error) {
	if id == "" {
		return nil, ErrEmptyID
	}
	if kind == KindOpening {
		return nil, ErrOpeningNotAllowedExternally
	}
	if !isValidExternalKind(kind) {
		return nil, ErrInvalidKind
	}
	if externalTransactionID == "" || providerID == "" || idempotencyKey == "" || payloadHash == "" {
		return nil, ErrMissingExternalMetadata
	}

	if err := validateAmountForKind(kind, amount); err != nil {
		return nil, err
	}
	if err := validateReferenceForKind(kind, referenceExternalTransactionID); err != nil {
		return nil, err
	}

	return &WagerTransaction{
		id:                             id,
		externalTransactionID:          externalTransactionID,
		providerID:                     providerID,
		idempotencyKey:                 idempotencyKey,
		payloadHash:                    payloadHash,
		walletID:                       walletID,
		playerID:                       playerID,
		roundID:                        roundID,
		gameID:                         gameID,
		kind:                           kind,
		amount:                         amount,
		referenceExternalTransactionID: referenceExternalTransactionID,
		state:                          StatePending,
		createdAt:                      now,
		updatedAt:                      now,
	}, nil
}

// NewOpening constructs the internal OPENING transaction produced
// when a wallet is created. Per the README, OPENING carries none of
// the external metadata (provider, external id, idempotency key,
// payload hash, round, game, reference) — those fields simply do not
// apply to this origin.
//
// A zero initial balance never reaches this constructor in practice
// (the wallet-opening use case skips creating OPENING entirely when
// the initial balance is zero), but NewOpening itself only requires
// amount to be non-negative, since zero is a legitimate initial
// balance per the README.
func NewOpening(id, walletID, playerID string, amount money.Money, now time.Time) (*WagerTransaction, error) {
	if id == "" {
		return nil, ErrEmptyID
	}
	if amount.IsNegative() {
		return nil, ErrAmountMustBePositive
	}

	return &WagerTransaction{
		id:        id,
		walletID:  walletID,
		playerID:  playerID,
		kind:      KindOpening,
		amount:    amount,
		state:     StatePending,
		createdAt: now,
		updatedAt: now,
	}, nil
}

// validateAmountForKind enforces the README's per-kind amount policy:
// LOSS must be exactly zero; BET, WIN, REFUND and ROLLBACK must be
// strictly positive.
func validateAmountForKind(kind Kind, amount money.Money) error {
	if kind == KindLoss {
		if !amount.IsZero() {
			return ErrLossAmountMustBeZero
		}
		return nil
	}
	if !amount.IsPositive() {
		return ErrAmountMustBePositive
	}
	return nil
}

// validateReferenceForKind enforces that only REFUND and ROLLBACK
// carry a referenceExternalTransactionId, and that they always do.
func validateReferenceForKind(kind Kind, reference string) error {
	needsReference := kind == KindRefund || kind == KindRollback
	if needsReference && reference == "" {
		return ErrReferenceRequired
	}
	if !needsReference && reference != "" {
		return ErrReferenceNotApplicable
	}
	return nil
}

// Rehydrate reconstructs a WagerTransaction from persisted state
// without re-running any validation that only applies at creation
// time, and without replaying any transition. Use this whenever
// loading a transaction back from a repository.
func Rehydrate(
	id, externalTransactionID, providerID, idempotencyKey, payloadHash string,
	walletID, playerID, roundID, gameID string,
	kind Kind,
	amount money.Money,
	referenceExternalTransactionID, resolvedReferenceTransactionID string,
	state State,
	failureCode string,
	createdAt, updatedAt time.Time,
) (*WagerTransaction, error) {
	if id == "" {
		return nil, ErrEmptyID
	}
	return &WagerTransaction{
		id:                             id,
		externalTransactionID:          externalTransactionID,
		providerID:                     providerID,
		idempotencyKey:                 idempotencyKey,
		payloadHash:                    payloadHash,
		walletID:                       walletID,
		playerID:                       playerID,
		roundID:                        roundID,
		gameID:                         gameID,
		kind:                           kind,
		amount:                         amount,
		referenceExternalTransactionID: referenceExternalTransactionID,
		resolvedReferenceTransactionID: resolvedReferenceTransactionID,
		state:                          state,
		failureCode:                    failureCode,
		createdAt:                      createdAt,
		updatedAt:                      updatedAt,
	}, nil
}

// --- Getters ---

func (tx *WagerTransaction) ID() string                    { return tx.id }
func (tx *WagerTransaction) ExternalTransactionID() string { return tx.externalTransactionID }
func (tx *WagerTransaction) ProviderID() string            { return tx.providerID }
func (tx *WagerTransaction) IdempotencyKey() string        { return tx.idempotencyKey }
func (tx *WagerTransaction) PayloadHash() string           { return tx.payloadHash }
func (tx *WagerTransaction) WalletID() string              { return tx.walletID }
func (tx *WagerTransaction) PlayerID() string              { return tx.playerID }
func (tx *WagerTransaction) RoundID() string               { return tx.roundID }
func (tx *WagerTransaction) GameID() string                { return tx.gameID }
func (tx *WagerTransaction) Kind() Kind                    { return tx.kind }
func (tx *WagerTransaction) Amount() money.Money           { return tx.amount }
func (tx *WagerTransaction) ReferenceExternalTransactionID() string {
	return tx.referenceExternalTransactionID
}
func (tx *WagerTransaction) ResolvedReferenceTransactionID() string {
	return tx.resolvedReferenceTransactionID
}
func (tx *WagerTransaction) State() State         { return tx.state }
func (tx *WagerTransaction) FailureCode() string  { return tx.failureCode }
func (tx *WagerTransaction) CreatedAt() time.Time { return tx.createdAt }
func (tx *WagerTransaction) UpdatedAt() time.Time { return tx.updatedAt }

// IsTerminal reports whether the transaction is in PROCESSED,
// REJECTED or FAILED — i.e. it can never transition again.
func (tx *WagerTransaction) IsTerminal() bool { return isTerminal(tx.state) }

// --- State transitions ---
//
// Every transition below refuses to act once the transaction is
// terminal (ErrInvalidTransition), which is the domain-level
// enforcement of "uma transação terminal não deve sofrer novas
// transições" from the README.

// MarkPendingReference transitions PENDING -> PENDING_REFERENCE, used
// when a REFUND/ROLLBACK's reference has not arrived yet. Only valid
// from PENDING.
func (tx *WagerTransaction) MarkPendingReference(now time.Time) error {
	if tx.state != StatePending {
		return ErrInvalidTransition
	}
	tx.state = StatePendingReference
	tx.updatedAt = now
	return nil
}

// ResolveReference records the internal transaction id that a
// REFUND/ROLLBACK's external reference resolved to. It does not
// change state by itself — the caller still calls Process or Reject
// depending on the outcome of validating the resolved reference.
func (tx *WagerTransaction) ResolveReference(resolvedReferenceTransactionID string, now time.Time) error {
	if tx.IsTerminal() {
		return ErrInvalidTransition
	}
	tx.resolvedReferenceTransactionID = resolvedReferenceTransactionID
	tx.updatedAt = now
	return nil
}

// Process transitions PENDING or PENDING_REFERENCE -> PROCESSED. This
// is the terminal success path.
func (tx *WagerTransaction) Process(now time.Time) error {
	if tx.state != StatePending && tx.state != StatePendingReference {
		return ErrInvalidTransition
	}
	tx.state = StateProcessed
	tx.updatedAt = now
	return nil
}

// Reject transitions PENDING or PENDING_REFERENCE -> REJECTED, the
// terminal path for a definitive business-rule refusal (e.g.
// insufficient balance, reference not found after retries exhausted).
// failureCode is required and must be a stable, documented code per
// the README.
func (tx *WagerTransaction) Reject(failureCode string, now time.Time) error {
	if tx.state != StatePending && tx.state != StatePendingReference {
		return ErrInvalidTransition
	}
	if failureCode == "" {
		return ErrEmptyFailureCode
	}
	tx.state = StateRejected
	tx.failureCode = failureCode
	tx.updatedAt = now
	return nil
}

// Fail transitions PENDING or PENDING_REFERENCE -> FAILED, the
// terminal path for a permanent infrastructure failure recorded for
// audit (as opposed to a business-rule rejection).
func (tx *WagerTransaction) Fail(failureCode string, now time.Time) error {
	if tx.state != StatePending && tx.state != StatePendingReference {
		return ErrInvalidTransition
	}
	if failureCode == "" {
		return ErrEmptyFailureCode
	}
	tx.state = StateFailed
	tx.failureCode = failureCode
	tx.updatedAt = now
	return nil
}
