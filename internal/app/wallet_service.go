package app

// Package app holds the use cases that orchestrate the domain
// packages (money, wallet, ledger, wagertransaction) together with
// the Postgres repositories. This is the only layer that is allowed
// to know about both the domain and the persistence layer at once —
// domain packages never import postgres, and postgres repositories
// never import each other.

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/TakedaB/backend-challenge-go/internal/domain/ledger"
	"github.com/TakedaB/backend-challenge-go/internal/domain/money"
	"github.com/TakedaB/backend-challenge-go/internal/domain/wagertransaction"
	"github.com/TakedaB/backend-challenge-go/internal/domain/wallet"
	"github.com/TakedaB/backend-challenge-go/internal/infra/postgres"
	"github.com/TakedaB/backend-challenge-go/internal/support/idgen"
)

// WalletService is the use case layer for opening wallets and
// processing wager transactions against them.
type WalletService struct {
	wallets *postgres.WalletRepository
	ledger  *postgres.LedgerRepository
	txs     *postgres.WagerTransactionRepository
}

// NewWalletService constructs a WalletService. Fx will call this
// automatically, injecting the three repositories it already knows
// how to build.
func NewWalletService(
	wallets *postgres.WalletRepository,
	ledgerRepo *postgres.LedgerRepository,
	txs *postgres.WagerTransactionRepository,
) *WalletService {
	return &WalletService{wallets: wallets, ledger: ledgerRepo, txs: txs}
}

// OpenWallet creates a new wallet with the given initial balance. If
// the initial balance is positive, an internal OPENING transaction is
// recorded (PROCESSED immediately — opening a wallet cannot fail once
// the wallet row itself is created). A zero initial balance creates
// no OPENING transaction at all, matching the README's rule that
// OPENING only exists to explain a non-zero starting balance.
func (s *WalletService) OpenWallet(ctx context.Context, playerID string, initialBalance money.Money) (*wallet.Wallet, error) {
	w, err := wallet.New(idgen.New(), playerID, initialBalance, time.Now())
	if err != nil {
		return nil, err
	}

	if err := s.wallets.Create(ctx, w); err != nil {
		return nil, fmt.Errorf("app: persisting new wallet: %w", err)
	}

	if !initialBalance.IsZero() {
		opening, err := wagertransaction.NewOpening(idgen.New(), w.ID(), w.PlayerID(), initialBalance, time.Now())
		if err != nil {
			return nil, err
		}
		if err := s.txs.Create(ctx, opening); err != nil {
			return nil, fmt.Errorf("app: persisting OPENING transaction: %w", err)
		}
		if err := opening.Process(time.Now()); err != nil {
			return nil, err
		}
		if err := s.txs.Update(ctx, opening); err != nil {
			return nil, fmt.Errorf("app: finalizing OPENING transaction: %w", err)
		}
	}

	return w, nil
}

// GetWallet loads a wallet by id, returning postgres.ErrWalletNotFound
// unchanged when it does not exist so the HTTP handler can map that
// specifically to a 404.
func (s *WalletService) GetWallet(ctx context.Context, id string) (*wallet.Wallet, error) {
	return s.wallets.FindByID(ctx, id)
}

// ProcessWagerTransactionInput carries everything needed to process
// one external wager operation (BET, WIN, LOSS, REFUND or ROLLBACK).
type ProcessWagerTransactionInput struct {
	ExternalTransactionID          string
	ProviderID                     string
	IdempotencyKey                 string
	PayloadHash                    string
	WalletID                       string
	PlayerID                       string
	RoundID                        string
	GameID                         string
	Kind                           wagertransaction.Kind
	Amount                         money.Money
	ReferenceExternalTransactionID string
}

// ProcessWagerTransaction is idempotent: if a transaction already
// exists for (ProviderID, IdempotencyKey), it is returned as-is
// without reprocessing anything. Otherwise a new PENDING transaction
// is created and immediately driven to a terminal state:
//   - LOSS never touches the wallet; it goes straight to PROCESSED.
//   - BET debits the wallet; insufficient balance rejects the
//     transaction (state REJECTED, failureCode INSUFFICIENT_BALANCE)
//     rather than returning an error — that is a business outcome,
//     not a system failure.
//   - WIN, REFUND and ROLLBACK credit the wallet.
//
// Known scope limitation (documented in ARCHITECTURE.md): REFUND and
// ROLLBACK do not yet look up and validate the referenced original
// transaction (matching amount, correct prior state, blocking a
// second reversal of the same reference) — they credit the wallet
// unconditionally once the domain-level reference-format check in
// wagertransaction.NewExternal passes. Full reference resolution
// (the PENDING_REFERENCE retry flow) is left as a next step.
func (s *WalletService) ProcessWagerTransaction(ctx context.Context, in ProcessWagerTransactionInput) (*wagertransaction.WagerTransaction, error) {
	existing, err := s.txs.FindByIdempotencyKey(ctx, in.ProviderID, in.IdempotencyKey)
	if err == nil {
		return existing, nil
	}
	if !errors.Is(err, postgres.ErrWagerTransactionNotFound) {
		return nil, fmt.Errorf("app: checking idempotency: %w", err)
	}

	tx, err := wagertransaction.NewExternal(
		idgen.New(), in.ExternalTransactionID, in.ProviderID, in.IdempotencyKey, in.PayloadHash,
		in.WalletID, in.PlayerID, in.RoundID, in.GameID,
		in.Kind, in.Amount, in.ReferenceExternalTransactionID, time.Now(),
	)
	if err != nil {
		return nil, err // validation error — handler maps this to 400
	}
	if err := s.txs.Create(ctx, tx); err != nil {
		return nil, fmt.Errorf("app: persisting transaction: %w", err)
	}

	if in.Kind == wagertransaction.KindLoss {
		return s.finalize(ctx, tx, func() error { return tx.Process(time.Now()) })
	}

	w, err := s.wallets.FindByID(ctx, in.WalletID)
	if err != nil {
		if errors.Is(err, postgres.ErrWalletNotFound) {
			return s.finalize(ctx, tx, func() error { return tx.Reject("WALLET_NOT_FOUND", time.Now()) })
		}
		return nil, fmt.Errorf("app: loading wallet: %w", err)
	}
	previousVersion := w.Version()

	var movement wallet.Movement
	switch in.Kind {
	case wagertransaction.KindBet:
		movement, err = w.Debit(in.Amount, time.Now())
	case wagertransaction.KindWin, wagertransaction.KindRefund, wagertransaction.KindRollback:
		movement, err = w.Credit(in.Amount, time.Now())
	}

	if errors.Is(err, wallet.ErrInsufficientBalance) {
		return s.finalize(ctx, tx, func() error { return tx.Reject("INSUFFICIENT_BALANCE", time.Now()) })
	}
	if err != nil {
		return nil, fmt.Errorf("app: applying movement: %w", err)
	}

	entry, err := ledger.FromMovement(idgen.New(), w.ID(), tx.ID(), movement, time.Now())
	if err != nil {
		return nil, err
	}
	if err := s.ledger.Create(ctx, entry); err != nil {
		return nil, fmt.Errorf("app: persisting ledger entry: %w", err)
	}

	if err := s.wallets.Update(ctx, w, previousVersion); err != nil {
		if errors.Is(err, wallet.ErrStaleVersion) {
			// Another request updated this wallet between our
			// FindByID and Update. A full implementation would retry
			// from FindByID; for this scope we record it as a
			// permanent failure for audit, matching the FAILED state's
			// purpose in the README.
			return s.finalize(ctx, tx, func() error { return tx.Fail("CONCURRENT_UPDATE_CONFLICT", time.Now()) })
		}
		return nil, fmt.Errorf("app: persisting wallet update: %w", err)
	}

	return s.finalize(ctx, tx, func() error { return tx.Process(time.Now()) })
}

// finalize applies a state transition to tx and persists the result,
// returning tx either way — the transaction is always something the
// caller can inspect (State(), FailureCode()), never just an error.
func (s *WalletService) finalize(ctx context.Context, tx *wagertransaction.WagerTransaction, transition func() error) (*wagertransaction.WagerTransaction, error) {
	if err := transition(); err != nil {
		return nil, err
	}
	if err := s.txs.Update(ctx, tx); err != nil {
		return nil, fmt.Errorf("app: persisting final transaction state: %w", err)
	}
	return tx, nil
}
