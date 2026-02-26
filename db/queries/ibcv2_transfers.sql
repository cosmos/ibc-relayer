-- name: UpdateTransferState :exec
UPDATE ibcv2_transfers
SET updated_at=NOW(), status=$1
WHERE source_chain_id=$2 AND packet_source_client_id=$3 AND packet_sequence_number=$4;

-- name: UpdateTransferRecvTx :exec
UPDATE ibcv2_transfers
SET updated_at=NOW(), recv_tx_hash=$1, recv_tx_time=$2, recv_tx_relayer_address=$3
WHERE source_chain_id=$4 AND packet_source_client_id=$5 AND packet_sequence_number=$6;

-- name: UpdateTransferRecvTxGasCostUSD :exec
UPDATE ibcv2_transfers
SET updated_at=NOW(), recv_tx_gas_cost_usd=$1
WHERE source_chain_id=$2 AND packet_source_client_id=$3 AND packet_sequence_number=$4;

-- name: UpdateTransferWriteAckTx :exec
UPDATE ibcv2_transfers
SET updated_at=NOW(), write_ack_tx_hash=$1, write_ack_tx_time=$2, write_ack_status=$3
WHERE source_chain_id=$4 AND packet_source_client_id=$5 AND packet_sequence_number=$6;

-- name: UpdateTransferAckTx :exec
UPDATE ibcv2_transfers
SET updated_at=NOW(), ack_tx_hash=$1, ack_tx_time=$2, ack_tx_relayer_address=$3
WHERE source_chain_id=$4 AND packet_source_client_id=$5 AND packet_sequence_number=$6;

-- name: UpdateTransferAckTxGasCostUSD :exec
UPDATE ibcv2_transfers
SET updated_at=NOW(), ack_tx_gas_cost_usd=$1
WHERE source_chain_id=$2 AND packet_source_client_id=$3 AND packet_sequence_number=$4;

-- name: UpdateTransferTimeoutTx :exec
UPDATE ibcv2_transfers
SET updated_at=NOW(), timeout_tx_hash=$1, timeout_tx_time=$2, timeout_tx_relayer_address=$3
WHERE source_chain_id=$4 AND packet_source_client_id=$5 AND packet_sequence_number=$6;

-- name: UpdateTransferTimeoutTxGasCostUSD :exec
UPDATE ibcv2_transfers
SET updated_at=NOW(), timeout_tx_gas_cost_usd=$1
WHERE source_chain_id=$2 AND packet_source_client_id=$3 AND packet_sequence_number=$4;

-- name: UpdateTransferSourceTxFinalizedTime :exec
UPDATE ibcv2_transfers
SET updated_at=NOW(), source_tx_finalized_time=$1
WHERE source_chain_id=$2 AND packet_source_client_id=$3 AND packet_sequence_number=$4;

-- name: UpdateTransferWriteAckTxFinalizedTime :exec
UPDATE ibcv2_transfers
SET updated_at=NOW(), write_ack_tx_finalized_time=$1
WHERE source_chain_id=$2 AND packet_source_client_id=$3 AND packet_sequence_number=$4;

-- name: GetUnfinishedIBCV2Transfers :many
SELECT * FROM ibcv2_transfers
WHERE status!='COMPLETE_WITH_ACK' AND status!='COMPLETE_WITH_TIMEOUT' AND status!='FAILED' and status!='COMPLETE_WITH_WRITE_ACK_SUCCESS' and status!='COMPLETE_WITH_WRITE_ACK_ERROR';

-- name: InsertIBCV2Transfer :exec
INSERT INTO ibcv2_transfers (
    source_chain_id,
    destination_chain_id,
    source_tx_hash,
    source_tx_time,
    packet_sequence_number,
    packet_source_client_id,
    packet_destination_client_id,
    packet_timeout_timestamp
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8);

-- name: ClearRecvTx :exec
UPDATE ibcv2_transfers 
set recv_tx_hash=null, recv_tx_time=null
WHERE source_chain_id=$1 AND packet_source_client_id=$2 AND packet_sequence_number=$3;

-- name: ClearAckTx :exec
UPDATE ibcv2_transfers 
set ack_tx_hash=null, ack_tx_time=null
WHERE source_chain_id=$1 AND packet_source_client_id=$2 AND packet_sequence_number=$3;

-- name: ClearTimeoutTx :exec
UPDATE ibcv2_transfers 
set timeout_tx_hash=null, timeout_tx_time=null
WHERE source_chain_id=$1 AND packet_source_client_id=$2 AND packet_sequence_number=$3;

-- name: GetTransfersBySourceTx :many
SELECT * FROM ibcv2_transfers
WHERE source_chain_id = $1 AND source_tx_hash = $2;

-- name: InsertRelaySubmission :exec
INSERT INTO ibcv2_relay_submissions (source_chain_id, source_tx_hash)
VALUES ($1, $2)
ON CONFLICT (source_chain_id, source_tx_hash) DO NOTHING;

-- name: GetRelaySubmission :one
SELECT * FROM ibcv2_relay_submissions
WHERE source_chain_id = $1 AND source_tx_hash = $2;
