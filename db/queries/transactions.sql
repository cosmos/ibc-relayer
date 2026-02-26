-- name: InsertRawTx :one
INSERT INTO raw_txs (cctp_message_id, tx_hash, raw_tx, chain_id, nonce) VALUES ($1, $2, $3, $4, $5) RETURNING *;
