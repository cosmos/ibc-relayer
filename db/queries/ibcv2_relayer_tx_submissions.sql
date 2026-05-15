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
    s.id,
    s.tx_hash,
    s.chain_id,
    COALESCE(MIN(t.source_chain_id), ''::text)::text AS source_chain_id,
    COALESCE(MIN(t.destination_chain_id), ''::text)::text AS destination_chain_id,
    s.tx_type,
    s.relayer_address,
    s.submitted_at,
    s.resolved_at,
    s.gas_cost_amount,
    s.gas_cost_usd,
    s.status,
    s.execution_error
FROM ibcv2_relayer_tx_submissions s
JOIN ibcv2_transfer_tx_submissions tts
  ON tts.submission_id = s.id
JOIN ibcv2_transfers t
  ON t.id = tts.transfer_id
WHERE s.status = 'PENDING'
GROUP BY
    s.id,
    s.tx_hash,
    s.chain_id,
    s.tx_type,
    s.relayer_address,
    s.submitted_at,
    s.resolved_at,
    s.gas_cost_amount,
    s.gas_cost_usd,
    s.status,
    s.execution_error
ORDER BY s.submitted_at ASC
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
