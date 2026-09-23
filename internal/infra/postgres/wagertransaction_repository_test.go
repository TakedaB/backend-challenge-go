package postgres

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/TakedaB/backend-challenge-go/internal/domain/money"
	"github.com/TakedaB/backend-challenge-go/internal/domain/wagertransaction"
	"github.com/TakedaB/backend-challenge-go/internal/domain/wallet"
)

func TestWagerTransactionRepository_CreateAndFindByID(t *testing.T) {
	pool := connectTestPool(t)
	walletRepo := NewWalletRepository(pool)
	txRepo := NewWagerTransactionRepository(pool)
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

	txID := newTestUUID(t)
	amount, _ := money.NewFromString("25.00", "BRL", false)
	tx, err := wagertransaction.NewExternal(
		txID, "ext-1", "provider-a", "provider-a:ext-1", "hash-abc",
		walletID, w.PlayerID(), "round-1", "game-1",
		wagertransaction.KindBet, amount, "", time.Now(),
	)
	if err != nil {
		t.Fatalf("NewExternal(): %v", err)
	}

	if err := txRepo.Create(ctx, tx); err != nil {
		t.Fatalf("Create(): %v", err)
	}
	t.Cleanup(func() {
		pool.Exec(context.Background(), "DELETE FROM wager_transactions WHERE id = $1", txID)
	})

	loaded, err := txRepo.FindByID(ctx, txID)
	if err != nil {
		t.Fatalf("FindByID(): %v", err)
	}
	if loaded.Kind() != wagertransaction.KindBet {
		t.Errorf("Kind() = %s, want BET", loaded.Kind())
	}
	if loaded.State() != wagertransaction.StatePending {
		t.Errorf("State() = %s, want PENDING", loaded.State())
	}
	if loaded.Amount().String() != "25.00" {
		t.Errorf("Amount() = %s, want 25.00", loaded.Amount().String())
	}
}

func TestWagerTransactionRepository_FindByIdempotencyKey(t *testing.T) {
	pool := connectTestPool(t)
	walletRepo := NewWalletRepository(pool)
	txRepo := NewWagerTransactionRepository(pool)
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

	txID := newTestUUID(t)
	amount, _ := money.NewFromString("25.00", "BRL", false)
	idempotencyKey := "idem-" + txID
	tx, _ := wagertransaction.NewExternal(
		txID, "ext-1", "provider-a", idempotencyKey, "hash-abc",
		walletID, w.PlayerID(), "round-1", "game-1",
		wagertransaction.KindBet, amount, "", time.Now(),
	)
	if err := txRepo.Create(ctx, tx); err != nil {
		t.Fatalf("Create(): %v", err)
	}
	t.Cleanup(func() {
		pool.Exec(context.Background(), "DELETE FROM wager_transactions WHERE id = $1", txID)
	})

	found, err := txRepo.FindByIdempotencyKey(ctx, "provider-a", idempotencyKey)
	if err != nil {
		t.Fatalf("FindByIdempotencyKey(): %v", err)
	}
	if found.ID() != txID {
		t.Errorf("found.ID() = %s, want %s", found.ID(), txID)
	}

	_, err = txRepo.FindByIdempotencyKey(ctx, "provider-a", "does-not-exist")
	if !errors.Is(err, ErrWagerTransactionNotFound) {
		t.Errorf("FindByIdempotencyKey() for unknown key error = %v, want ErrWagerTransactionNotFound", err)
	}
}

func TestWagerTransactionRepository_Update(t *testing.T) {
	pool := connectTestPool(t)
	walletRepo := NewWalletRepository(pool)
	txRepo := NewWagerTransactionRepository(pool)
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

	txID := newTestUUID(t)
	amount, _ := money.NewFromString("25.00", "BRL", false)
	tx, _ := wagertransaction.NewExternal(
		txID, "ext-1", "provider-a", "provider-a:"+txID, "hash-abc",
		walletID, w.PlayerID(), "round-1", "game-1",
		wagertransaction.KindBet, amount, "", time.Now(),
	)
	if err := txRepo.Create(ctx, tx); err != nil {
		t.Fatalf("Create(): %v", err)
	}
	t.Cleanup(func() {
		pool.Exec(context.Background(), "DELETE FROM wager_transactions WHERE id = $1", txID)
	})

	if err := tx.Process(time.Now()); err != nil {
		t.Fatalf("Process(): %v", err)
	}
	if err := txRepo.Update(ctx, tx); err != nil {
		t.Fatalf("Update(): %v", err)
	}

	reloaded, err := txRepo.FindByID(ctx, txID)
	if err != nil {
		t.Fatalf("FindByID() after update: %v", err)
	}
	if reloaded.State() != wagertransaction.StateProcessed {
		t.Errorf("State() after update = %s, want PROCESSED", reloaded.State())
	}
}
