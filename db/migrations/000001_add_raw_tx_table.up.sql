 CREATE TABLE IF NOT EXISTS raw_txs (
     id         SERIAL PRIMARY KEY,
     created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
     updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,

     cctp_message_id INT NOT NULL,
     tx_hash         TEXT NOT NULL,
     raw_tx          TEXT NOT NULL
);