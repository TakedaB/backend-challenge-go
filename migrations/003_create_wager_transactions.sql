-- WagerTransaction aggregate. external_transaction_id, provider_id,
-- idempotency_key and payload_hash are all nullable because OPENING
-- (internal) never carries them — only BET/WIN/LOSS/REFUND/ROLLBACK do.
CREATE TABLE wager_transactions (
    id                                  UUID PRIMARY KEY,
    external_transaction_id             TEXT,
    provider_id                         TEXT,
    idempotency_key                     TEXT,
    payload_hash                        TEXT,
    wallet_id                           UUID NOT NULL REFERENCES wallets (id),
    player_id                           TEXT NOT NULL,
    round_id                            TEXT,
    game_id                             TEXT,
    kind                                TEXT NOT NULL CHECK (kind IN ('OPENING', 'BET', 'WIN', 'LOSS', 'REFUND', 'ROLLBACK')),
    amount_minor_units                  BIGINT NOT NULL CHECK (amount_minor_units >= 0),
    currency                            CHAR(3) NOT NULL,
    reference_external_transaction_id   TEXT,
    resolved_reference_transaction_id   UUID,
    state                               TEXT NOT NULL CHECK (state IN ('PENDING', 'PENDING_REFERENCE', 'PROCESSED', 'REJECTED', 'FAILED')),
    failure_code                        TEXT,
    created_at                          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at                          TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Idempotency: the same (provider, idempotencyKey) must never create
-- two transactions. Partial index because OPENING rows have a NULL
-- idempotency_key and must not collide with each other under this
-- constraint.
CREATE UNIQUE INDEX idx_wager_tx_idempotency ON wager_transactions (provider_id, idempotency_key)
    WHERE idempotency_key IS NOT NULL;

CREATE INDEX idx_wager_tx_wallet_id ON wager_transactions (wallet_id);
CREATE INDEX idx_wager_tx_state ON wager_transactions (state) WHERE state IN ('PENDING', 'PENDING_REFERENCE');