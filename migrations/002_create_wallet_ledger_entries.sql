-- Append-only ledger. There is intentionally no UPDATE or DELETE
-- path for this table anywhere in the application code — correctness
-- comes from never touching a row after INSERT.
CREATE TABLE wallet_ledger_entries (
    id                    UUID PRIMARY KEY,
    wallet_id             UUID NOT NULL REFERENCES wallets (id),
    transaction_id        UUID NOT NULL,
    direction             TEXT NOT NULL CHECK (direction IN ('DEBIT', 'CREDIT')),
    amount_minor_units    BIGINT NOT NULL CHECK (amount_minor_units > 0),
    balance_before_minor  BIGINT NOT NULL CHECK (balance_before_minor >= 0),
    balance_after_minor   BIGINT NOT NULL CHECK (balance_after_minor >= 0),
    created_at            TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- A single transaction can never post two movements to the same
-- wallet — this is the DB-level half of the WalletLedgerEntry
-- invariant that the Go domain layer can't enforce on its own.
CREATE UNIQUE INDEX idx_ledger_wallet_transaction ON wallet_ledger_entries (wallet_id, transaction_id);
CREATE INDEX idx_ledger_wallet_id ON wallet_ledger_entries (wallet_id);