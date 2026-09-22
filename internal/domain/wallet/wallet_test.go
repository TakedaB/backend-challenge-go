package wallet

import (
	"errors"
	"sync"
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

func newTestWallet(t *testing.T, initial string) *Wallet {
	t.Helper()
	balance := mustMoney(t, initial, "BRL")
	w, err := New("wallet-1", "player-1", balance, time.Now())
	if err != nil {
		t.Fatalf("New(): %v", err)
	}
	return w
}

func TestNew_SetsVersionOne(t *testing.T) {
	w := newTestWallet(t, "100.00")
	if w.Version() != 1 {
		t.Errorf("Version() = %d, want 1", w.Version())
	}
	if w.Balance().String() != "100.00" {
		t.Errorf("Balance() = %s, want 100.00", w.Balance().String())
	}
}

func TestNew_RejectsNegativeInitialBalance(t *testing.T) {
	negative, err := money.NewFromString("-10.00", "BRL", true)
	if err != nil {
		t.Fatalf("unexpected error building fixture: %v", err)
	}
	_, err = New("wallet-1", "player-1", negative, time.Now())
	if !errors.Is(err, ErrNonPositiveAmount) {
		t.Errorf("New() error = %v, want ErrNonPositiveAmount", err)
	}
}

func TestNew_RejectsEmptyIdentifiers(t *testing.T) {
	balance := mustMoney(t, "10.00", "BRL")

	if _, err := New("", "player-1", balance, time.Now()); !errors.Is(err, ErrEmptyWalletID) {
		t.Errorf("New() with empty id error = %v, want ErrEmptyWalletID", err)
	}
	if _, err := New("wallet-1", "", balance, time.Now()); !errors.Is(err, ErrEmptyPlayerID) {
		t.Errorf("New() with empty playerID error = %v, want ErrEmptyPlayerID", err)
	}
}

func TestRehydrate_DoesNotReplayMovements(t *testing.T) {
	balance := mustMoney(t, "50.00", "BRL")
	created := time.Now().Add(-time.Hour)
	updated := time.Now()

	w, err := Rehydrate("wallet-1", "player-1", "BRL", balance, 7, created, updated)
	if err != nil {
		t.Fatalf("Rehydrate(): %v", err)
	}
	if w.Version() != 7 {
		t.Errorf("Version() = %d, want 7 (rehydrate must not reset it)", w.Version())
	}
	if w.Balance().String() != "50.00" {
		t.Errorf("Balance() = %s, want 50.00", w.Balance().String())
	}
}

func TestRehydrate_RejectsInvalidVersion(t *testing.T) {
	balance := mustMoney(t, "50.00", "BRL")
	_, err := Rehydrate("wallet-1", "player-1", "BRL", balance, 0, time.Now(), time.Now())
	if !errors.Is(err, ErrInvalidVersion) {
		t.Errorf("Rehydrate() error = %v, want ErrInvalidVersion", err)
	}
}

func TestDebit_Success(t *testing.T) {
	w := newTestWallet(t, "100.00")
	amount := mustMoney(t, "30.00", "BRL")

	mv, err := w.Debit(amount, time.Now())
	if err != nil {
		t.Fatalf("Debit(): %v", err)
	}

	if w.Balance().String() != "70.00" {
		t.Errorf("Balance() = %s, want 70.00", w.Balance().String())
	}
	if w.Version() != 2 {
		t.Errorf("Version() = %d, want 2", w.Version())
	}
	if mv.Direction != DirectionDebit {
		t.Errorf("Movement.Direction = %s, want DEBIT", mv.Direction)
	}
	if mv.BalanceBefore.String() != "100.00" || mv.BalanceAfter.String() != "70.00" {
		t.Errorf("Movement before/after = %s/%s, want 100.00/70.00", mv.BalanceBefore, mv.BalanceAfter)
	}
}

func TestDebit_RejectsInsufficientBalance(t *testing.T) {
	w := newTestWallet(t, "50.00")
	amount := mustMoney(t, "80.00", "BRL")

	_, err := w.Debit(amount, time.Now())
	if !errors.Is(err, ErrInsufficientBalance) {
		t.Errorf("Debit() error = %v, want ErrInsufficientBalance", err)
	}
	// Balance and version must be untouched by a rejected debit.
	if w.Balance().String() != "50.00" {
		t.Errorf("Balance() = %s, want unchanged 50.00", w.Balance().String())
	}
	if w.Version() != 1 {
		t.Errorf("Version() = %d, want unchanged 1", w.Version())
	}
}

func TestDebit_ExactBalance_AllowsZeroResult(t *testing.T) {
	w := newTestWallet(t, "50.00")
	amount := mustMoney(t, "50.00", "BRL")

	_, err := w.Debit(amount, time.Now())
	if err != nil {
		t.Fatalf("Debit() should allow driving balance to exactly zero: %v", err)
	}
	if !w.Balance().IsZero() {
		t.Errorf("Balance() = %s, want 0.00", w.Balance().String())
	}
}

func TestDebit_RejectsNonPositiveAmount(t *testing.T) {
	w := newTestWallet(t, "50.00")
	zero, _ := money.Zero("BRL")

	if _, err := w.Debit(zero, time.Now()); !errors.Is(err, ErrNonPositiveAmount) {
		t.Errorf("Debit(zero) error = %v, want ErrNonPositiveAmount", err)
	}
}

func TestDebit_RejectsCurrencyMismatch(t *testing.T) {
	w := newTestWallet(t, "50.00")
	usd := mustMoney(t, "10.00", "USD")

	if _, err := w.Debit(usd, time.Now()); !errors.Is(err, ErrCurrencyMismatch) {
		t.Errorf("Debit() error = %v, want ErrCurrencyMismatch", err)
	}
}

func TestCredit_Success(t *testing.T) {
	w := newTestWallet(t, "100.00")
	amount := mustMoney(t, "25.00", "BRL")

	mv, err := w.Credit(amount, time.Now())
	if err != nil {
		t.Fatalf("Credit(): %v", err)
	}
	if w.Balance().String() != "125.00" {
		t.Errorf("Balance() = %s, want 125.00", w.Balance().String())
	}
	if w.Version() != 2 {
		t.Errorf("Version() = %d, want 2", w.Version())
	}
	if mv.Direction != DirectionCredit {
		t.Errorf("Movement.Direction = %s, want CREDIT", mv.Direction)
	}
}

func TestCredit_RejectsNonPositiveAmount(t *testing.T) {
	w := newTestWallet(t, "50.00")
	zero, _ := money.Zero("BRL")

	if _, err := w.Credit(zero, time.Now()); !errors.Is(err, ErrNonPositiveAmount) {
		t.Errorf("Credit(zero) error = %v, want ErrNonPositiveAmount", err)
	}
}

func TestCredit_RejectsCurrencyMismatch(t *testing.T) {
	w := newTestWallet(t, "50.00")
	usd := mustMoney(t, "10.00", "USD")

	if _, err := w.Credit(usd, time.Now()); !errors.Is(err, ErrCurrencyMismatch) {
		t.Errorf("Credit() error = %v, want ErrCurrencyMismatch", err)
	}
}

// TestConcurrentDebits_OnSingleInMemoryWallet is the domain-level
// analogue of the README's mandatory scenario: a wallet with 100.00
// BRL receives two concurrent bets of 80.00 BRL each.
//
// At this layer there is no database and no repository yet, so this
// test protects the in-memory Wallet with an explicit mutex to
// serialize access — that mutex stands in for what will later be a
// real optimistic-lock conditional UPDATE at the repository/
// integration layer. The point of this test is to prove the domain
// invariant itself (Debit never allows the balance to go negative,
// exactly one of the two debits succeeds, the ledger-worthy final
// balance is deterministic) under real concurrent goroutines with
// their own scheduling, which is why it is run with `go test -race`.
func TestConcurrentDebits_OnSingleInMemoryWallet(t *testing.T) {
	w := newTestWallet(t, "100.00")
	amount := mustMoney(t, "80.00", "BRL")

	var mu sync.Mutex
	var wg sync.WaitGroup

	results := make([]error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			mu.Lock()
			defer mu.Unlock()
			_, err := w.Debit(amount, time.Now())
			results[idx] = err
		}(i)
	}
	wg.Wait()

	successCount := 0
	rejectionCount := 0
	for _, err := range results {
		switch {
		case err == nil:
			successCount++
		case errors.Is(err, ErrInsufficientBalance):
			rejectionCount++
		default:
			t.Fatalf("unexpected error: %v", err)
		}
	}

	if successCount != 1 {
		t.Errorf("successCount = %d, want exactly 1", successCount)
	}
	if rejectionCount != 1 {
		t.Errorf("rejectionCount = %d, want exactly 1", rejectionCount)
	}
	if w.Balance().String() != "20.00" {
		t.Errorf("final Balance() = %s, want 20.00", w.Balance().String())
	}
	if w.Balance().IsNegative() {
		t.Errorf("balance must never go negative, got %s", w.Balance().String())
	}
	// Exactly one successful debit means the version advanced by
	// exactly one step from its initial value of 1.
	if w.Version() != 2 {
		t.Errorf("Version() = %d, want 2 (exactly one successful movement)", w.Version())
	}
}

// TestConcurrentDebits_DifferentWallets_BothProceed documents that the
// concurrency guard above is per-wallet, not global: two independent
// wallets must both be able to debit successfully at the same time,
// since the README prohibits any global lock.
func TestConcurrentDebits_DifferentWallets_BothProceed(t *testing.T) {
	walletA := newTestWallet(t, "100.00")
	walletB, err := New("wallet-2", "player-2", mustMoney(t, "100.00", "BRL"), time.Now())
	if err != nil {
		t.Fatalf("New(): %v", err)
	}
	amount := mustMoney(t, "80.00", "BRL")

	var wg sync.WaitGroup
	errs := make([]error, 2)

	wg.Add(2)
	go func() {
		defer wg.Done()
		_, errs[0] = walletA.Debit(amount, time.Now())
	}()
	go func() {
		defer wg.Done()
		_, errs[1] = walletB.Debit(amount, time.Now())
	}()
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("wallet %d: unexpected error: %v", i, err)
		}
	}
	if walletA.Balance().String() != "20.00" {
		t.Errorf("walletA.Balance() = %s, want 20.00", walletA.Balance().String())
	}
	if walletB.Balance().String() != "20.00" {
		t.Errorf("walletB.Balance() = %s, want 20.00", walletB.Balance().String())
	}
}
