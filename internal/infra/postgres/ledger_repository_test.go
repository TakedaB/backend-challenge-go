package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/TakedaB/backend-challenge-go/internal/domain/ledger"
	"github.com/TakedaB/backend-challenge-go/internal/domain/money"
	"github.com/TakedaB/backend-challenge-go/internal/domain/wallet"
)

func TestLedgerRepository_Create(t *testing.T) {
	pool := connectTestPool(t)
	walletRepo := NewWalletRepository(pool)
	ledgerRepo := NewLedgerRepository(pool)
	ctx := context.Background()

	walletID := newTestUUID(t)
	balance, _ := money.NewFromString("100.00", "BRL", false)
	w, _ := wallet.New(walletID, "player-"+walletID, balance, time.Now())
	if err := walletRepo.Create(ctx, w); err != nil {
		t.Fatalf("wallet Create(): %v", err)
	}
	t.Cleanup(func() {
		pool.Exec(context.Background(), "DELETE FROM wallets WHERE id = $1", walletID)
	})

	amount, _ := money.NewFromString("30.00", "BRL", false)
	before, _ := money.NewFromString("100.00", "BRL", false)
	after, _ := money.NewFromString("70.00", "BRL", false)

	entryID := newTestUUID(t)
	txID := newTestUUID(t)
	entry, err := ledger.New(entryID, walletID, txID, wallet.DirectionDebit, amount, before, after, time.Now())
	if err != nil {
		t.Fatalf("ledger.New(): %v", err)
	}

	if err := ledgerRepo.Create(ctx, entry); err != nil {
		t.Fatalf("Create(): %v", err)
	}
	t.Cleanup(func() {
		pool.Exec(context.Background(), "DELETE FROM wallet_ledger_entries WHERE id = $1", entryID)
	})
}

// TestLedgerRepository_Create_RejectsDuplicateWalletTransactionPair
// proves the database-level half of "a transaction can never post
// two movements to the same wallet" — the unique index on
// (wallet_id, transaction_id), not any Go code, is what makes this
// airtight under real concurrency.
func TestLedgerRepository_Create_RejectsDuplicateWalletTransactionPair(t *testing.T) {
	pool := connectTestPool(t)
	walletRepo := NewWalletRepository(pool)
	ledgerRepo := NewLedgerRepository(pool)
	ctx := context.Background()

	walletID := newTestUUID(t)
	balance, _ := money.NewFromString("100.00", "BRL", false)
	w, _ := wallet.New(walletID, "player-"+walletID, balance, time.Now())
	if err := walletRepo.Create(ctx, w); err != nil {
		t.Fatalf("wallet Create(): %v", err)
	}
	t.Cleanup(func() {
		pool.Exec(context.Background(), "DELETE FROM wallets WHERE id = $1", walletID)
	})

	amount, _ := money.NewFromString("30.00", "BRL", false)
	before, _ := money.NewFromString("100.00", "BRL", false)
	after, _ := money.NewFromString("70.00", "BRL", false)
	txID := newTestUUID(t)

	entry1ID := newTestUUID(t)
	entry1, _ := ledger.New(entry1ID, walletID, txID, wallet.DirectionDebit, amount, before, after, time.Now())
	if err := ledgerRepo.Create(ctx, entry1); err != nil {
		t.Fatalf("first Create(): %v", err)
	}
	t.Cleanup(func() {
		pool.Exec(context.Background(), "DELETE FROM wallet_ledger_entries WHERE id = $1", entry1ID)
	})

	entry2ID := newTestUUID(t)
	entry2, _ := ledger.New(entry2ID, walletID, txID, wallet.DirectionDebit, amount, before, after, time.Now())
	err := ledgerRepo.Create(ctx, entry2)
	if err == nil {
		t.Errorf("second Create() with same (walletID, transactionID) should have failed on the unique index, got nil error")
	}
}
