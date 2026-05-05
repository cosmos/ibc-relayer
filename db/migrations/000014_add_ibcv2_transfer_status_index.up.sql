CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_ibcv2_transfers_chain_ids_status
ON ibcv2_transfers (source_chain_id, destination_chain_id, status);
