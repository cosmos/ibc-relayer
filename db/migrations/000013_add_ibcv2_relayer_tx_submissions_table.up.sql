CREATE TYPE ibcv2_relayer_tx_submission_type AS ENUM (
    'RECV_PACKET',
    'ACK_PACKET',
    'TIMEOUT_PACKET'
);

CREATE TYPE ibcv2_relayer_tx_submission_status AS ENUM (
    'PENDING',
    'SUCCESS',
    'FAILED',
    'EXPIRED'
);

CREATE TABLE ibcv2_relayer_tx_submissions (
    id SERIAL PRIMARY KEY,
    tx_hash TEXT NOT NULL,
    chain_id TEXT NOT NULL,
    tx_type ibcv2_relayer_tx_submission_type NOT NULL,
    relayer_address TEXT NOT NULL,
    submitted_at TIMESTAMPTZ NOT NULL,
    resolved_at TIMESTAMPTZ,
    gas_cost_amount NUMERIC,
    gas_cost_usd NUMERIC,
    status ibcv2_relayer_tx_submission_status NOT NULL,
    execution_error TEXT,
    CONSTRAINT ibcv2_relayer_tx_submissions_chain_id_tx_hash_tx_type_key
        UNIQUE (chain_id, tx_hash, tx_type)
);

CREATE INDEX idx_ibcv2_relayer_tx_submissions_unresolved
    ON ibcv2_relayer_tx_submissions (submitted_at)
    WHERE status = 'PENDING';

CREATE TABLE ibcv2_transfer_tx_submissions (
    transfer_id INTEGER NOT NULL REFERENCES ibcv2_transfers(id),
    submission_id INTEGER NOT NULL REFERENCES ibcv2_relayer_tx_submissions(id),
    PRIMARY KEY (transfer_id, submission_id)
);

CREATE INDEX idx_ibcv2_transfer_tx_submissions_submission_id
    ON ibcv2_transfer_tx_submissions (submission_id);
