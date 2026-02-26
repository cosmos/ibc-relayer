CREATE TABLE ibcv2_relay_submissions (
    id SERIAL PRIMARY KEY,
    source_chain_id TEXT NOT NULL,
    source_tx_hash TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(source_chain_id, source_tx_hash)
);

CREATE INDEX idx_ibcv2_relay_submissions_lookup ON ibcv2_relay_submissions(source_chain_id, source_tx_hash);
