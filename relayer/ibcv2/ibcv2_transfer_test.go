package ibcv2_test

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/assert"

	"github.com/cosmos/platform-relayer/db/gen/db"
	"github.com/cosmos/platform-relayer/relayer/ibcv2"
	"github.com/cosmos/platform-relayer/shared/lmt"
)

func TestNewIBCV2Transfer(t *testing.T) {
	sourceChainID := "cosmoshub-4"
	destinationChainID := "1"
	sourceTxHash := "0xdeadbeef"
	sourceTxTime := time.Now()
	packetSequenceNum := 10
	packetSourceClientID := "sourceClient"
	packetDestinationClientID := "destClient"

	t.Run("no optional db fields populated", func(t *testing.T) {
		lmt.ConfigureLogger()
		ctx := context.Background()
		ctx = lmt.LoggerContext(ctx)

		dbRep := db.Ibcv2Transfer{
			ID:                        0,
			CreatedAt:                 pgtype.Timestamp{Valid: false},
			UpdatedAt:                 pgtype.Timestamp{Valid: false},
			Status:                    db.Ibcv2RelayStatusPENDING,
			StatusText:                pgtype.Text{Valid: false},
			DestinationChainID:        destinationChainID,
			SourceChainID:             sourceChainID,
			SourceTxHash:              sourceTxHash,
			SourceTxTime:              pgtype.Timestamp{Valid: false},
			PacketSequenceNumber:      int32(packetSequenceNum),
			PacketSourceClientID:      packetSourceClientID,
			PacketDestinationClientID: packetDestinationClientID,
			PacketTimeoutTimestamp:    pgtype.Timestamp{Valid: false},
			RecvTxHash:                pgtype.Text{Valid: false},
			RecvTxTime:                pgtype.Timestamp{Valid: false},
			WriteAckTxHash:            pgtype.Text{Valid: false},
			WriteAckTxTime:            pgtype.Timestamp{Valid: false},
			AckTxHash:                 pgtype.Text{Valid: false},
			AckTxTime:                 pgtype.Timestamp{Valid: false},
			TimeoutTxHash:             pgtype.Text{Valid: false},
			TimeoutTxTime:             pgtype.Timestamp{Valid: false},
		}
		transfer := ibcv2.NewIBCV2Transfer(ctx, dbRep)

		// expect that the transfer has a non nil logger, but dont make any
		// assertions about the content on it
		assert.NotNil(t, transfer.Logger)
		transfer.Logger = nil

		expected := &ibcv2.IBCV2Transfer{
			State:                     db.Ibcv2RelayStatusPENDING,
			DestinationChainID:        destinationChainID,
			SourceChainID:             sourceChainID,
			SourceTxHash:              sourceTxHash,
			PacketSequenceNumber:      uint32(packetSequenceNum),
			PacketSourceClientID:      packetSourceClientID,
			PacketDestinationClientID: packetDestinationClientID,
		}
		assert.Equal(t, expected, transfer)
	})

	t.Run("all optional fields are populated", func(t *testing.T) {
		lmt.ConfigureLogger()
		ctx := context.Background()
		ctx = lmt.LoggerContext(ctx)

		dbRep := db.Ibcv2Transfer{
			ID:                        100,
			CreatedAt:                 pgtype.Timestamp{Valid: true, Time: sourceTxTime},
			UpdatedAt:                 pgtype.Timestamp{Valid: true, Time: sourceTxTime},
			Status:                    db.Ibcv2RelayStatusGETACKPACKET,
			StatusText:                pgtype.Text{Valid: true, String: "status"},
			DestinationChainID:        destinationChainID,
			SourceChainID:             sourceChainID,
			SourceTxHash:              sourceTxHash,
			SourceTxTime:              pgtype.Timestamp{Valid: true, Time: sourceTxTime},
			PacketSequenceNumber:      int32(packetSequenceNum),
			PacketSourceClientID:      packetSourceClientID,
			PacketDestinationClientID: packetDestinationClientID,
			PacketTimeoutTimestamp:    pgtype.Timestamp{Valid: true, Time: sourceTxTime},
			RecvTxHash:                pgtype.Text{Valid: true, String: "0xrecv"},
			RecvTxTime:                pgtype.Timestamp{Valid: true, Time: sourceTxTime},
			WriteAckTxHash:            pgtype.Text{Valid: true, String: "0xwriteack"},
			WriteAckTxTime:            pgtype.Timestamp{Valid: true, Time: sourceTxTime},
			AckTxHash:                 pgtype.Text{Valid: true, String: "0xack"},
			AckTxTime:                 pgtype.Timestamp{Valid: true, Time: sourceTxTime},
			TimeoutTxHash:             pgtype.Text{Valid: true, String: "0xtimeout"},
			TimeoutTxTime:             pgtype.Timestamp{Valid: true, Time: sourceTxTime},
		}
		transfer := ibcv2.NewIBCV2Transfer(ctx, dbRep)

		// expect that the transfer has a non nil logger, but dont make any
		// assertions about the content on it
		assert.NotNil(t, transfer.Logger)
		transfer.Logger = nil

		expected := &ibcv2.IBCV2Transfer{
			State:                     db.Ibcv2RelayStatusGETACKPACKET,
			DestinationChainID:        destinationChainID,
			SourceChainID:             sourceChainID,
			SourceTxHash:              sourceTxHash,
			SourceTxTime:              sourceTxTime,
			PacketSequenceNumber:      uint32(packetSequenceNum),
			PacketSourceClientID:      packetSourceClientID,
			PacketDestinationClientID: packetDestinationClientID,
			PacketTimeoutTimestamp:    dbRep.PacketTimeoutTimestamp.Time,
			RecvTxHash:                &dbRep.RecvTxHash.String,
			RecvTxTime:                &dbRep.RecvTxTime.Time,
			WriteAckTxHash:            &dbRep.WriteAckTxHash.String,
			WriteAckTxTime:            &dbRep.WriteAckTxTime.Time,
			AckTxHash:                 &dbRep.AckTxHash.String,
			AckTxTime:                 &dbRep.AckTxTime.Time,
			TimeoutTxHash:             &dbRep.TimeoutTxHash.String,
			TimeoutTxTime:             &dbRep.TimeoutTxTime.Time,
		}
		assert.Equal(t, expected, transfer)
	})
}
