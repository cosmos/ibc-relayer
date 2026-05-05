package ibcv2

import (
	"context"
	"fmt"

	"github.com/cosmos/ibc-relayer/db/gen/db"
)

func insertTransferTxSubmissionMappings(ctx context.Context, q *db.Queries, submissionID int32, transfers []*IBCV2Transfer) error {
	for _, transfer := range transfers {
		if transfer.ProcessingError != nil {
			continue
		}
		if transfer.GetID() == 0 {
			return fmt.Errorf(
				"transfer from source chain %s packet sequence %d and source client %s is missing db id",
				transfer.GetSourceChainID(),
				transfer.GetPacketSequenceNumber(),
				transfer.GetPacketSourceClientID(),
			)
		}

		if err := q.InsertIBCV2TransferTxSubmission(ctx, db.InsertIBCV2TransferTxSubmissionParams{
			TransferID:   transfer.GetID(),
			SubmissionID: submissionID,
		}); err != nil {
			return fmt.Errorf(
				"inserting tx submission mapping for transfer %d and submission %d: %w",
				transfer.GetID(),
				submissionID,
				err,
			)
		}
	}

	return nil
}
