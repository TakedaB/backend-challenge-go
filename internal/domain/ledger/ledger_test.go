package ledger

import (
	"errors"
	"testing"
	"time"

	"github.com/TakedaB/backend-challenge-go/internal/domain/money"
	"github.com/TakedaB/backend-challenge-go/internal/domain/wallet"
)

func mustMoney(t *testing.T, amount, currency string) money.Money {
	t.Helper()
	m, err := money.NewFromString(amount, currency, false)
	if err != nil {
		t.Fatalf("mustMoney(%q, %q): %v", amount, currency, err)
	}
	return m
}

func TestNew_DebitConsistent(t *testing.T) {
	before := mustMoney(t, "100.00", "BRL")
	amount := mustMoney(t, "30.00", "BRL")
	after := mustMoney(t, "70.00", "BRL")

	entry, err := New("entry-1", "wallet-1", "tx-1", wallet.DirectionDebit, amount, before, after, time.Now())
	if err != nil {
		t.Fatalf("New(): %v", err)
	}
	if entry.Direction() != wallet.DirectionDebit {
		t.Errorf("Direction() = %s, want DEBIT", entry.Direction())
	}
	if entry.BalanceAfter().String() != "70.00" {
		t.Errorf("BalanceAfter() = %s, want 70.00", entry.BalanceAfter().String())
	}
}

func TestNew_CreditConsistent(t *testing.T) {
	before := mustMoney(t, "100.00", "BRL")
	amount := mustMoney(t, "30.00", "BRL")
	after := mustMoney(t, "130.00", "BRL")

	entry, err := New("entry-1", "wallet-1", "tx-1", wallet.DirectionCredit, amount, before, after, time.Now())
	if err != nil {
		t.Fatalf("New(): %v", err)
	}
	if entry.BalanceAfter().String() != "130.00" {
		t.Errorf("BalanceAfter() = %s, want 130.00", entry.BalanceAfter().String())
	}
}

func TestNew_RejectsInconsistentDebit(t *testing.T) {
	before := mustMoney(t, "100.00", "BRL")
	amount := mustMoney(t, "30.00", "BRL")
	wrongAfter := mustMoney(t, "80.00", "BRL") // should be 70.00

	_, err := New("entry-1", "wallet-1", "tx-1", wallet.DirectionDebit, amount, before, wrongAfter, time.Now())
	if !errors.Is(err, ErrInconsistentBalance) {
		t.Errorf("New() error = %v, want ErrInconsistentBalance", err)
	}
}

func TestNew_RejectsInconsistentCredit(t *testing.T) {
	before := mustMoney(t, "100.00", "BRL")
	amount := mustMoney(t, "30.00", "BRL")
	wrongAfter := mustMoney(t, "125.00", "BRL") // should be 130.00

	_, err := New("entry-1", "wallet-1", "tx-1", wallet.DirectionCredit, amount, before, wrongAfter, time.Now())
	if !errors.Is(err, ErrInconsistentBalance) {
		t.Errorf("New() error = %v, want ErrInconsistentBalance", err)
	}
}

func TestNew_RejectsNonPositiveAmount(t *testing.T) {
	before := mustMoney(t, "100.00", "BRL")
	zero, _ := money.Zero("BRL")

	_, err := New("entry-1", "wallet-1", "tx-1", wallet.DirectionDebit, zero, before, before, time.Now())
	if !errors.Is(err, ErrNonPositiveAmount) {
		t.Errorf("New() error = %v, want ErrNonPositiveAmount", err)
	}
}

func TestNew_RejectsInvalidDirection(t *testing.T) {
	before := mustMoney(t, "100.00", "BRL")
	amount := mustMoney(t, "30.00", "BRL")
	after := mustMoney(t, "70.00", "BRL")

	_, err := New("entry-1", "wallet-1", "tx-1", wallet.Direction("WHATEVER"), amount, before, after, time.Now())
	if !errors.Is(err, ErrInvalidDirection) {
		t.Errorf("New() error = %v, want ErrInvalidDirection", err)
	}
}

func TestNew_RejectsCurrencyMismatch(t *testing.T) {
	before := mustMoney(t, "100.00", "BRL")
	amount := mustMoney(t, "30.00", "USD") // different currency
	after := mustMoney(t, "70.00", "BRL")

	_, err := New("entry-1", "wallet-1", "tx-1", wallet.DirectionDebit, amount, before, after, time.Now())
	if !errors.Is(err, ErrCurrencyMismatch) {
		t.Errorf("New() error = %v, want ErrCurrencyMismatch", err)
	}
}

func TestNew_RejectsEmptyIdentifiers(t *testing.T) {
	before := mustMoney(t, "100.00", "BRL")
	amount := mustMoney(t, "30.00", "BRL")
	after := mustMoney(t, "70.00", "BRL")

	if _, err := New("", "wallet-1", "tx-1", wallet.DirectionDebit, amount, before, after, time.Now()); !errors.Is(err, ErrEmptyID) {
		t.Errorf("New() with empty id error = %v, want ErrEmptyID", err)
	}
	if _, err := New("entry-1", "", "tx-1", wallet.DirectionDebit, amount, before, after, time.Now()); !errors.Is(err, ErrEmptyWalletID) {
		t.Errorf("New() with empty walletID error = %v, want ErrEmptyWalletID", err)
	}
	if _, err := New("entry-1", "wallet-1", "", wallet.DirectionDebit, amount, before, after, time.Now()); !errors.Is(err, ErrEmptyTransactionID) {
		t.Errorf("New() with empty transactionID error = %v, want ErrEmptyTransactionID", err)
	}
}

// TestFromMovement_MatchesWalletDebit proves the ledger entry built
// from a real Wallet.Debit call is always internally consistent —
// this is the intended call pattern in the use-case layer, so it is
// worth an explicit end-to-end check across both packages.
func TestFromMovement_MatchesWalletDebit(t *testing.T) {
	balance := mustMoney(t, "100.00", "BRL")
	w, err := wallet.New("wallet-1", "player-1", balance, time.Now())
	if err != nil {
		t.Fatalf("wallet.New(): %v", err)
	}

	amount := mustMoney(t, "80.00", "BRL")
	mv, err := w.Debit(amount, time.Now())
	if err != nil {
		t.Fatalf("Debit(): %v", err)
	}

	entry, err := FromMovement("entry-1", w.ID(), "tx-1", mv, time.Now())
	if err != nil {
		t.Fatalf("FromMovement(): %v", err)
	}
	if entry.BalanceAfter().String() != w.Balance().String() {
		t.Errorf("entry.BalanceAfter() = %s, want %s (wallet's own balance)", entry.BalanceAfter(), w.Balance())
	}
}
