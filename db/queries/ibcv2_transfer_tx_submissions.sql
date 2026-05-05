-- name: InsertIBCV2TransferTxSubmission :exec
INSERT INTO ibcv2_transfer_tx_submissions (
    transfer_id,
    submission_id
) VALUES ($1, $2)
ON CONFLICT (transfer_id, submission_id) DO NOTHING;

-- name: GetIBCV2RelayerTxSubmissionsByTransferID :many
SELECT s.*
FROM ibcv2_relayer_tx_submissions s
JOIN ibcv2_transfer_tx_submissions tts
  ON tts.submission_id = s.id
WHERE tts.transfer_id = $1
ORDER BY s.submitted_at ASC, s.id ASC;
