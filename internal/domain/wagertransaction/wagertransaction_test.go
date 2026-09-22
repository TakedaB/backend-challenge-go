package wagertransaction

import (
	"errors"
	"testing"
	"time"

	"github.com/TakedaB/backend-challenge-go/internal/domain/money"
)

func mustMoney(t *testing.T, amount, currency string) money.Money {
	t.Helper()
	m, err := money.NewFromString(amount, currency, false)
	if err != nil {
		t.Fatalf("mustMoney(%q, %q): %v", amount, currency, err)
	}
	return m
}

func newExternal(t *testing.T, kind Kind, amount money.Money, reference string) (*WagerTransaction, error) {
	t.Helper()
	return NewExternal(
		"tx-1", "ext-1", "provider-a", "provider-a:ext-1", "hash-abc",
		"wallet-1", "player-1", "round-1", "game-1",
		kind, amount, reference, time.Now(),
	)
}

// --- Per-kind amount and reference rules ---

func TestNewExternal_BET_RequiresPositiveAmount(t *testing.T) {
	zero, _ := money.Zero("BRL")
	if _, err := newExternal(t, KindBet, zero, ""); !errors.Is(err, ErrAmountMustBePositive) {
		t.Errorf("BET with zero amount error = %v, want ErrAmountMustBePositive", err)
	}

	positive := mustMoney(t, "25.00", "BRL")
	tx, err := newExternal(t, KindBet, positive, "")
	if err != nil {
		t.Fatalf("BET with positive amount: %v", err)
	}
	if tx.State() != StatePending {
		t.Errorf("State() = %s, want PENDING", tx.State())
	}
}

func TestNewExternal_WIN_RequiresPositiveAmount(t *testing.T) {
	zero, _ := money.Zero("BRL")
	if _, err := newExternal(t, KindWin, zero, ""); !errors.Is(err, ErrAmountMustBePositive) {
		t.Errorf("WIN with zero amount error = %v, want ErrAmountMustBePositive", err)
	}
}

func TestNewExternal_LOSS_RequiresZeroAmount(t *testing.T) {
	positive := mustMoney(t, "10.00", "BRL")
	if _, err := newExternal(t, KindLoss, positive, ""); !errors.Is(err, ErrLossAmountMustBeZero) {
		t.Errorf("LOSS with positive amount error = %v, want ErrLossAmountMustBeZero", err)
	}

	zero, _ := money.Zero("BRL")
	tx, err := newExternal(t, KindLoss, zero, "")
	if err != nil {
		t.Fatalf("LOSS with zero amount: %v", err)
	}
	if tx.Kind() != KindLoss {
		t.Errorf("Kind() = %s, want LOSS", tx.Kind())
	}
}

func TestNewExternal_REFUND_RequiresReferenceAndPositiveAmount(t *testing.T) {
	positive := mustMoney(t, "25.00", "BRL")

	if _, err := newExternal(t, KindRefund, positive, ""); !errors.Is(err, ErrReferenceRequired) {
		t.Errorf("REFUND without reference error = %v, want ErrReferenceRequired", err)
	}

	zero, _ := money.Zero("BRL")
	if _, err := newExternal(t, KindRefund, zero, "ext-original"); !errors.Is(err, ErrAmountMustBePositive) {
		t.Errorf("REFUND with zero amount error = %v, want ErrAmountMustBePositive", err)
	}

	tx, err := newExternal(t, KindRefund, positive, "ext-original")
	if err != nil {
		t.Fatalf("REFUND with reference and positive amount: %v", err)
	}
	if tx.ReferenceExternalTransactionID() != "ext-original" {
		t.Errorf("ReferenceExternalTransactionID() = %s, want ext-original", tx.ReferenceExternalTransactionID())
	}
}

func TestNewExternal_ROLLBACK_RequiresReferenceAndPositiveAmount(t *testing.T) {
	positive := mustMoney(t, "25.00", "BRL")

	if _, err := newExternal(t, KindRollback, positive, ""); !errors.Is(err, ErrReferenceRequired) {
		t.Errorf("ROLLBACK without reference error = %v, want ErrReferenceRequired", err)
	}

	tx, err := newExternal(t, KindRollback, positive, "ext-original")
	if err != nil {
		t.Fatalf("ROLLBACK with reference and positive amount: %v", err)
	}
	if tx.Kind() != KindRollback {
		t.Errorf("Kind() = %s, want ROLLBACK", tx.Kind())
	}
}

func TestNewExternal_RejectsReferenceOnNonReversalKinds(t *testing.T) {
	positive := mustMoney(t, "25.00", "BRL")

	if _, err := newExternal(t, KindBet, positive, "should-not-be-here"); !errors.Is(err, ErrReferenceNotApplicable) {
		t.Errorf("BET with reference error = %v, want ErrReferenceNotApplicable", err)
	}
	if _, err := newExternal(t, KindWin, positive, "should-not-be-here"); !errors.Is(err, ErrReferenceNotApplicable) {
		t.Errorf("WIN with reference error = %v, want ErrReferenceNotApplicable", err)
	}
}

func TestNewExternal_RejectsOpeningKind(t *testing.T) {
	amount := mustMoney(t, "10.00", "BRL")
	if _, err := newExternal(t, KindOpening, amount, ""); !errors.Is(err, ErrOpeningNotAllowedExternally) {
		t.Errorf("NewExternal(OPENING) error = %v, want ErrOpeningNotAllowedExternally", err)
	}
}

func TestNewExternal_RequiresExternalMetadata(t *testing.T) {
	amount := mustMoney(t, "10.00", "BRL")
	_, err := NewExternal("tx-1", "", "provider-a", "key", "hash", "wallet-1", "player-1", "round-1", "game-1", KindBet, amount, "", time.Now())
	if !errors.Is(err, ErrMissingExternalMetadata) {
		t.Errorf("NewExternal without externalTransactionID error = %v, want ErrMissingExternalMetadata", err)
	}
}

// --- OPENING ---

func TestNewOpening_AllowsZeroAmount(t *testing.T) {
	zero, _ := money.Zero("BRL")
	tx, err := NewOpening("tx-opening-1", "wallet-1", "player-1", zero, time.Now())
	if err != nil {
		t.Fatalf("NewOpening() with zero balance: %v", err)
	}
	if tx.Kind() != KindOpening {
		t.Errorf("Kind() = %s, want OPENING", tx.Kind())
	}
	if tx.ExternalTransactionID() != "" || tx.ProviderID() != "" {
		t.Errorf("OPENING must carry no external metadata, got externalTransactionID=%q providerID=%q",
			tx.ExternalTransactionID(), tx.ProviderID())
	}
}

func TestNewOpening_AllowsPositiveAmount(t *testing.T) {
	positive := mustMoney(t, "1000.00", "BRL")
	tx, err := NewOpening("tx-opening-1", "wallet-1", "player-1", positive, time.Now())
	if err != nil {
		t.Fatalf("NewOpening() with positive balance: %v", err)
	}
	if tx.Amount().String() != "1000.00" {
		t.Errorf("Amount() = %s, want 1000.00", tx.Amount().String())
	}
}

func TestNewOpening_RejectsNegativeAmount(t *testing.T) {
	negative, err := money.NewFromString("-1.00", "BRL", true)
	if err != nil {
		t.Fatalf("fixture error: %v", err)
	}
	if _, err := NewOpening("tx-opening-1", "wallet-1", "player-1", negative, time.Now()); !errors.Is(err, ErrAmountMustBePositive) {
		t.Errorf("NewOpening() with negative amount error = %v, want ErrAmountMustBePositive", err)
	}
}

// --- Rehydrate ---

func TestRehydrate_DoesNotValidateOrTransition(t *testing.T) {
	amount := mustMoney(t, "25.00", "BRL")
	tx, err := Rehydrate(
		"tx-1", "ext-1", "provider-a", "provider-a:ext-1", "hash-abc",
		"wallet-1", "player-1", "round-1", "game-1",
		KindBet, amount, "", "",
		StateProcessed, "", time.Now(), time.Now(),
	)
	if err != nil {
		t.Fatalf("Rehydrate(): %v", err)
	}
	if tx.State() != StateProcessed {
		t.Errorf("State() = %s, want PROCESSED (rehydrate must preserve stored state)", tx.State())
	}
	if !tx.IsTerminal() {
		t.Errorf("expected rehydrated PROCESSED transaction to report IsTerminal() == true")
	}
}

// --- State machine ---

func TestStateMachine_HappyPath_PendingToProcessed(t *testing.T) {
	amount := mustMoney(t, "25.00", "BRL")
	tx, err := newExternal(t, KindBet, amount, "")
	if err != nil {
		t.Fatalf("newExternal(): %v", err)
	}

	if err := tx.Process(time.Now()); err != nil {
		t.Fatalf("Process(): %v", err)
	}
	if tx.State() != StateProcessed {
		t.Errorf("State() = %s, want PROCESSED", tx.State())
	}
}

func TestStateMachine_PendingToPendingReferenceToProcessed(t *testing.T) {
	amount := mustMoney(t, "25.00", "BRL")
	tx, err := newExternal(t, KindRefund, amount, "ext-original")
	if err != nil {
		t.Fatalf("newExternal(): %v", err)
	}

	if err := tx.MarkPendingReference(time.Now()); err != nil {
		t.Fatalf("MarkPendingReference(): %v", err)
	}
	if tx.State() != StatePendingReference {
		t.Errorf("State() = %s, want PENDING_REFERENCE", tx.State())
	}

	if err := tx.ResolveReference("tx-original-internal-id", time.Now()); err != nil {
		t.Fatalf("ResolveReference(): %v", err)
	}
	if tx.ResolvedReferenceTransactionID() != "tx-original-internal-id" {
		t.Errorf("ResolvedReferenceTransactionID() = %s, want tx-original-internal-id", tx.ResolvedReferenceTransactionID())
	}

	if err := tx.Process(time.Now()); err != nil {
		t.Fatalf("Process() from PENDING_REFERENCE: %v", err)
	}
	if tx.State() != StateProcessed {
		t.Errorf("State() = %s, want PROCESSED", tx.State())
	}
}

func TestStateMachine_Reject_RequiresFailureCode(t *testing.T) {
	amount := mustMoney(t, "25.00", "BRL")
	tx, err := newExternal(t, KindBet, amount, "")
	if err != nil {
		t.Fatalf("newExternal(): %v", err)
	}

	if err := tx.Reject("", time.Now()); !errors.Is(err, ErrEmptyFailureCode) {
		t.Errorf("Reject('') error = %v, want ErrEmptyFailureCode", err)
	}

	if err := tx.Reject("INSUFFICIENT_BALANCE", time.Now()); err != nil {
		t.Fatalf("Reject(): %v", err)
	}
	if tx.State() != StateRejected {
		t.Errorf("State() = %s, want REJECTED", tx.State())
	}
	if tx.FailureCode() != "INSUFFICIENT_BALANCE" {
		t.Errorf("FailureCode() = %s, want INSUFFICIENT_BALANCE", tx.FailureCode())
	}
}

func TestStateMachine_Fail_RecordsFailureCode(t *testing.T) {
	amount := mustMoney(t, "25.00", "BRL")
	tx, err := newExternal(t, KindBet, amount, "")
	if err != nil {
		t.Fatalf("newExternal(): %v", err)
	}

	if err := tx.Fail("DATABASE_UNAVAILABLE", time.Now()); err != nil {
		t.Fatalf("Fail(): %v", err)
	}
	if tx.State() != StateFailed {
		t.Errorf("State() = %s, want FAILED", tx.State())
	}
}

// TestStateMachine_TerminalStatesRejectFurtherTransitions is the
// direct check of the README's "uma transação terminal não deve
// sofrer novas transições" rule, across all three terminal states and
// all four transition methods.
func TestStateMachine_TerminalStatesRejectFurtherTransitions(t *testing.T) {
	buildTerminal := func(t *testing.T, state State) *WagerTransaction {
		t.Helper()
		amount := mustMoney(t, "25.00", "BRL")
		tx, err := newExternal(t, KindBet, amount, "")
		if err != nil {
			t.Fatalf("newExternal(): %v", err)
		}
		switch state {
		case StateProcessed:
			if err := tx.Process(time.Now()); err != nil {
				t.Fatalf("Process(): %v", err)
			}
		case StateRejected:
			if err := tx.Reject("SOME_CODE", time.Now()); err != nil {
				t.Fatalf("Reject(): %v", err)
			}
		case StateFailed:
			if err := tx.Fail("SOME_CODE", time.Now()); err != nil {
				t.Fatalf("Fail(): %v", err)
			}
		}
		return tx
	}

	for _, state := range []State{StateProcessed, StateRejected, StateFailed} {
		t.Run(string(state), func(t *testing.T) {
			tx := buildTerminal(t, state)

			if err := tx.Process(time.Now()); !errors.Is(err, ErrInvalidTransition) {
				t.Errorf("Process() from terminal %s error = %v, want ErrInvalidTransition", state, err)
			}
			if err := tx.Reject("CODE", time.Now()); !errors.Is(err, ErrInvalidTransition) {
				t.Errorf("Reject() from terminal %s error = %v, want ErrInvalidTransition", state, err)
			}
			if err := tx.Fail("CODE", time.Now()); !errors.Is(err, ErrInvalidTransition) {
				t.Errorf("Fail() from terminal %s error = %v, want ErrInvalidTransition", state, err)
			}
			if err := tx.MarkPendingReference(time.Now()); !errors.Is(err, ErrInvalidTransition) {
				t.Errorf("MarkPendingReference() from terminal %s error = %v, want ErrInvalidTransition", state, err)
			}
			if err := tx.ResolveReference("whatever", time.Now()); !errors.Is(err, ErrInvalidTransition) {
				t.Errorf("ResolveReference() from terminal %s error = %v, want ErrInvalidTransition", state, err)
			}
		})
	}
}

func TestStateMachine_MarkPendingReference_OnlyFromPending(t *testing.T) {
	amount := mustMoney(t, "25.00", "BRL")
	tx, err := newExternal(t, KindRefund, amount, "ext-original")
	if err != nil {
		t.Fatalf("newExternal(): %v", err)
	}
	if err := tx.MarkPendingReference(time.Now()); err != nil {
		t.Fatalf("first MarkPendingReference(): %v", err)
	}
	// Calling it again from PENDING_REFERENCE must fail: it is only a
	// PENDING -> PENDING_REFERENCE transition, not idempotent re-entry.
	if err := tx.MarkPendingReference(time.Now()); !errors.Is(err, ErrInvalidTransition) {
		t.Errorf("second MarkPendingReference() error = %v, want ErrInvalidTransition", err)
	}
}
