-- Wallet aggregate root. balance_minor_units is BIGINT to match
-- Money's internal int64 minor-unit representation exactly — no
-- floating point anywhere in the schema.
CREATE TABLE wallets (
    id                  UUID PRIMARY KEY,
    player_id           TEXT NOT NULL,
    currency            CHAR(3) NOT NULL,
    balance_minor_units BIGINT NOT NULL CHECK (balance_minor_units >= 0),
    version             BIGINT NOT NULL CHECK (version >= 1),
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- One wallet per (player, currency) — matches the domain's identity
-- rule for Wallet.
CREATE UNIQUE INDEX idx_wallets_player_currency ON wallets (player_id, currency);