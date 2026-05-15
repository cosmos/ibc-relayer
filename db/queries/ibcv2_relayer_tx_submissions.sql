-- name: InsertIBCV2RelayerTxSubmission :one
INSERT INTO ibcv2_relayer_tx_submissions (
    tx_hash,
    chain_id,
    tx_type,
    relayer_address,
    submitted_at,
    status
) VALUES ($1, $2, $3, $4, $5, $6)
RETURNING id;

-- name: GetUnresolvedIBCV2RelayerTxSubmissions :many
SELECT
    id,
    tx_hash,
    chain_id,
    tx_type,
    relayer_address,
    submitted_at,
    resolved_at,
    gas_cost_amount,
    gas_cost_usd,
    status,
    execution_error
FROM ibcv2_relayer_tx_submissions
WHERE status = 'PENDING'
ORDER BY submitted_at ASC
LIMIT $1;

-- name: UpdateIBCV2RelayerTxSubmissionTracking :exec
UPDATE ibcv2_relayer_tx_submissions
SET
    resolved_at = NOW(),
    gas_cost_amount = $1,
    gas_cost_usd = $2,
    status = $3,
    execution_error = $4
WHERE id = $5;

-- name: ExpirePendingIBCV2RelayerTxSubmission :exec
UPDATE ibcv2_relayer_tx_submissions
SET
    resolved_at = NOW(),
    status = 'EXPIRED',
    execution_error = NULL
WHERE id = $1
  AND status = 'PENDING';
