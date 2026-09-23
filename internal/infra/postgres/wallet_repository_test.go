package postgres

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/TakedaB/backend-challenge-go/internal/domain/money"
	"github.com/TakedaB/backend-challenge-go/internal/domain/wallet"
)

func TestWalletRepository_CreateAndFindByID(t *testing.T) {
	pool := connectTestPool(t)
	repo := NewWalletRepository(pool)
	ctx := context.Background()

	id := newTestUUID(t)
	balance, err := money.NewFromString("100.00", "BRL", false)
	if err != nil {
		t.Fatalf("fixture: %v", err)
	}
	w, err := wallet.New(id, "player-"+id, balance, time.Now())
	if err != nil {
		t.Fatalf("wallet.New(): %v", err)
	}

	if err := repo.Create(ctx, w); err != nil {
		t.Fatalf("Create(): %v", err)
	}
	t.Cleanup(func() {
		pool.Exec(context.Background(), "DELETE FROM wallets WHERE id = $1", id)
	})

	loaded, err := repo.FindByID(ctx, id)
	if err != nil {
		t.Fatalf("FindByID(): %v", err)
	}
	if loaded.Balance().String() != "100.00" {
		t.Errorf("Balance() = %s, want 100.00", loaded.Balance().String())
	}
	if loaded.Version() != 1 {
		t.Errorf("Version() = %d, want 1", loaded.Version())
	}
	if loaded.PlayerID() != w.PlayerID() {
		t.Errorf("PlayerID() = %s, want %s", loaded.PlayerID(), w.PlayerID())
	}
}

func TestWalletRepository_FindByID_NotFound(t *testing.T) {
	pool := connectTestPool(t)
	repo := NewWalletRepository(pool)

	_, err := repo.FindByID(context.Background(), newTestUUID(t))
	if !errors.Is(err, ErrWalletNotFound) {
		t.Errorf("FindByID() error = %v, want ErrWalletNotFound", err)
	}
}

func TestWalletRepository_Update_Success(t *testing.T) {
	pool := connectTestPool(t)
	repo := NewWalletRepository(pool)
	ctx := context.Background()

	id := newTestUUID(t)
	balance, _ := money.NewFromString("100.00", "BRL", false)
	w, _ := wallet.New(id, "player-"+id, balance, time.Now())
	if err := repo.Create(ctx, w); err != nil {
		t.Fatalf("Create(): %v", err)
	}
	t.Cleanup(func() {
		pool.Exec(context.Background(), "DELETE FROM wallets WHERE id = $1", id)
	})

	previousVersion := w.Version() // 1, before Debit
	amount, _ := money.NewFromString("30.00", "BRL", false)
	if _, err := w.Debit(amount, time.Now()); err != nil {
		t.Fatalf("Debit(): %v", err)
	}

	if err := repo.Update(ctx, w, previousVersion); err != nil {
		t.Fatalf("Update(): %v", err)
	}

	reloaded, err := repo.FindByID(ctx, id)
	if err != nil {
		t.Fatalf("FindByID() after update: %v", err)
	}
	if reloaded.Balance().String() != "70.00" {
		t.Errorf("Balance() after update = %s, want 70.00", reloaded.Balance().String())
	}
	if reloaded.Version() != 2 {
		t.Errorf("Version() after update = %d, want 2", reloaded.Version())
	}
}

// TestWalletRepository_Update_StaleVersion is the integration-level
// proof of the optimistic-lock guarantee: updating with a
// previousVersion that no longer matches what's stored must be
// refused, not silently overwrite the other write.
func TestWalletRepository_Update_StaleVersion(t *testing.T) {
	pool := connectTestPool(t)
	repo := NewWalletRepository(pool)
	ctx := context.Background()

	id := newTestUUID(t)
	balance, _ := money.NewFromString("100.00", "BRL", false)
	w, _ := wallet.New(id, "player-"+id, balance, time.Now())
	if err := repo.Create(ctx, w); err != nil {
		t.Fatalf("Create(): %v", err)
	}
	t.Cleanup(func() {
		pool.Exec(context.Background(), "DELETE FROM wallets WHERE id = $1", id)
	})

	staleVersion := int64(999) // deliberately wrong
	amount, _ := money.NewFromString("30.00", "BRL", false)
	if _, err := w.Debit(amount, time.Now()); err != nil {
		t.Fatalf("Debit(): %v", err)
	}

	err := repo.Update(ctx, w, staleVersion)
	if !errors.Is(err, wallet.ErrStaleVersion) {
		t.Errorf("Update() with stale version error = %v, want ErrStaleVersion", err)
	}

	// The row in the database must be untouched.
	reloaded, err := repo.FindByID(ctx, id)
	if err != nil {
		t.Fatalf("FindByID() after failed update: %v", err)
	}
	if reloaded.Balance().String() != "100.00" {
		t.Errorf("Balance() after failed update = %s, want unchanged 100.00", reloaded.Balance().String())
	}
	if reloaded.Version() != 1 {
		t.Errorf("Version() after failed update = %d, want unchanged 1", reloaded.Version())
	}
}
