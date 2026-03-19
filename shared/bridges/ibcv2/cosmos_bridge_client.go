package ibcv2

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"math/big"
	"strconv"
	"strings"
	"sync"
	"time"

	sdk_math "cosmossdk.io/math"
	abci "github.com/cometbft/cometbft/abci/types"
	comet_bytes "github.com/cometbft/cometbft/libs/bytes"
	tmRPC "github.com/cometbft/cometbft/rpc/client"
	coretypes "github.com/cometbft/cometbft/rpc/core/types"
	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/codec"
	codec_types "github.com/cosmos/cosmos-sdk/codec/types"
	"github.com/cosmos/cosmos-sdk/std"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/bech32"
	"github.com/cosmos/cosmos-sdk/types/msgservice"
	txtypes "github.com/cosmos/cosmos-sdk/types/tx"
	"github.com/cosmos/cosmos-sdk/x/auth/tx"
	auth_types "github.com/cosmos/cosmos-sdk/x/auth/types"
	bank_types "github.com/cosmos/cosmos-sdk/x/bank/types"
	"github.com/cosmos/gogoproto/proto"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/cosmos/ibc-relayer/db/gen/db"
	ibc_channel_v2_types "github.com/cosmos/ibc-relayer/proto/gen/ibc/core/channel/v2"
	ibc_client_v1_types "github.com/cosmos/ibc-relayer/proto/gen/ibc/core/client/v1"
	tendermint_v1_types "github.com/cosmos/ibc-relayer/proto/gen/ibc/lightclients/tendermint/v1"
	wasm_v1_types "github.com/cosmos/ibc-relayer/proto/gen/ibc/lightclients/wasm/v1"
	"github.com/cosmos/ibc-relayer/shared/config"
	"github.com/cosmos/ibc-relayer/shared/lmt"
	"github.com/cosmos/ibc-relayer/shared/signing"
)

const (
	attributeKeyPacketSourceClient     = "packet_source_client"
	attributeKeyPacketDestClient       = "packet_dest_client"
	attributeKeyPacketSequence         = "packet_sequence"
	attributeKeyPacketTimeoutTimestamp = "packet_timeout_timestamp"

	eventTypeRecvPacket    = "recv_packet"
	eventTypeAckPacket     = "acknowledge_packet"
	eventTypeTimeoutPacket = "timeout_packet"
	eventTypeWriteAck      = "write_acknowledgement"
	eventTypeSendPacket    = "send_packet"
)

var reg codec_types.InterfaceRegistry

func init() {
	reg = codec_types.NewInterfaceRegistry()
	std.RegisterInterfaces(reg)
	auth_types.RegisterInterfaces(reg)

	reg.RegisterImplementations(
		(*sdk.Msg)(nil),
		&ibc_channel_v2_types.MsgSendPacket{},
		&ibc_channel_v2_types.MsgRecvPacket{},
		&ibc_channel_v2_types.MsgTimeout{},
		&ibc_channel_v2_types.MsgAcknowledgement{},
		&ibc_client_v1_types.MsgUpdateClient{},
		&tendermint_v1_types.ClientState{},
		&tendermint_v1_types.Misbehaviour{},
		&tendermint_v1_types.ConsensusState{},
		&tendermint_v1_types.Header{},
		&tendermint_v1_types.Fraction{},
		&wasm_v1_types.ClientState{},
		&wasm_v1_types.ConsensusState{},
		&wasm_v1_types.ClientMessage{},
	)
	msgservice.RegisterMsgServiceDesc(reg, &ibc_channel_v2_types.Msg_serviceDesc)
}

type CosmosBridgeClient struct {
	chainID string
	signer  signing.Signer
	prefix  string

	gasPrice  float64
	feeDenom  string
	feeAmount uint64

	conn  grpc.ClientConnInterface
	tmRPC tmRPC.Client

	txConfig client.TxConfig
	cdc      *codec.ProtoCodec

	txOnce             *OnceWithKey[*coretypes.ResultTx]
	txSubmissionLock   *sync.Mutex
	lastSubmissionTime time.Time
	txSubmissionDelay  time.Duration

	latestTimestampOnce *OnceWithKey[time.Time]
}

func NewCosmosBridgeClient(
	chainID string,
	signer signing.Signer,
	prefix string,
	gasPrice float64,
	feeDenom string,
	feeAmount uint64,
	conn grpc.ClientConnInterface,
	tmRPC tmRPC.Client,
) *CosmosBridgeClient {
	cdc := codec.NewProtoCodec(reg)

	return &CosmosBridgeClient{
		chainID:             chainID,
		signer:              signer,
		prefix:              prefix,
		gasPrice:            gasPrice,
		feeDenom:            feeDenom,
		feeAmount:           feeAmount,
		conn:                conn,
		tmRPC:               tmRPC,
		txConfig:            tx.NewTxConfig(cdc, tx.DefaultSignModes),
		cdc:                 cdc,
		txOnce:              NewOnceWithKey[*coretypes.ResultTx](time.Minute, 5*time.Second),
		txSubmissionLock:    new(sync.Mutex),
		txSubmissionDelay:   8 * time.Second,
		latestTimestampOnce: NewOnceWithKey[time.Time](6*time.Second, 200*time.Millisecond),
	}
}

func (client *CosmosBridgeClient) IsPacketReceived(ctx context.Context, destinationClientID string, sequence uint64) (bool, error) {
	in := &ibc_channel_v2_types.QueryPacketReceiptRequest{
		ClientId: destinationClientID,
		Sequence: sequence,
	}
	resp, err := ibc_channel_v2_types.NewQueryClient(client.conn).PacketReceipt(ctx, in)
	if err != nil {
		return false, fmt.Errorf("querying for packet receipt with sequence number %d to destination client %s: %w", sequence, destinationClientID, err)
	}

	return resp.Received, nil
}

func (client *CosmosBridgeClient) IsPacketCommitted(ctx context.Context, sourceClientID string, sequence uint64) (bool, error) {
	in := &ibc_channel_v2_types.QueryPacketCommitmentRequest{
		ClientId: sourceClientID,
		Sequence: sequence,
	}
	_, err := ibc_channel_v2_types.NewQueryClient(client.conn).PacketCommitment(ctx, in)
	if err != nil {
		if status, ok := status.FromError(err); ok {
			if status.Code() == codes.NotFound {
				return false, nil
			}
		}
		return false, fmt.Errorf("querying for packet commitment with sequence number %d to source client %s: %w", sequence, sourceClientID, err)
	}

	return true, nil
}

func (client *CosmosBridgeClient) FindRecvTx(
	ctx context.Context,
	sourceClientID string,
	destClientID string,
	sequence uint64,
	timeoutTimestamp time.Time,
) (*BridgeTx, error) {
	query := strings.Join([]string{
		fmt.Sprintf("%s.%s='%s'", eventTypeRecvPacket, attributeKeyPacketSourceClient, sourceClientID),
		fmt.Sprintf("%s.%s='%s'", eventTypeRecvPacket, attributeKeyPacketDestClient, destClientID),
		fmt.Sprintf("%s.%s=%d", eventTypeRecvPacket, attributeKeyPacketSequence, sequence),
		fmt.Sprintf("%s.%s=%d", eventTypeRecvPacket, attributeKeyPacketTimeoutTimestamp, timeoutTimestamp.Unix()),
	}, " AND ")

	searchResult, err := client.tmRPC.TxSearch(ctx, query, false, nil, nil, "desc")
	if err != nil {
		return nil, fmt.Errorf(
			"searching for recv tx from source client %s to dest client %s with sequence %d and timeout tx %d: %w",
			sourceClientID,
			destClientID,
			sequence,
			timeoutTimestamp.UnixNano(),
			err,
		)
	}
	if searchResult.TotalCount == 0 {
		return nil, ErrTxNotFound
	}
	if searchResult.TotalCount != 1 {
		return nil, fmt.Errorf("expected only 1 tx, but got %d", searchResult.TotalCount)
	}

	recvTx := searchResult.Txs[0]

	// fetch header at block height to get timestamp
	headerResult, err := client.tmRPC.Header(ctx, &recvTx.Height)
	if err != nil {
		return nil, fmt.Errorf("fetching block header at recv tx height %d: %w", recvTx.Height, err)
	}
	if headerResult.Header == nil {
		return nil, fmt.Errorf("fetched block header at recv tx height %d without err but header was nil", recvTx.Height)
	}

	signer, err := client.txSigner(recvTx)
	if err != nil {
		// the above could fail for many reasons (some other relayer included
		// some crazy message types in the recv we are unable to decode, weird
		// signature type, etc), however we should not return an error to the
		// client since, we have most of the relevant info that is required,
		// besides the signer address which is not critical
		lmt.Logger(ctx).Error("found recv tx but could not determine the tx's signer, ignoring error and continuing", zap.Error(err), zap.String("tx_hash", recvTx.Hash.String()))
	}

	return &BridgeTx{
		Hash:           recvTx.Hash.String(),
		Timestamp:      headerResult.Header.Time,
		RelayerAddress: signer,
	}, nil
}

func (client *CosmosBridgeClient) FindAckTx(
	ctx context.Context,
	sourceClientID string,
	destClientID string,
	sequence uint64,
) (*BridgeTx, error) {
	query := strings.Join([]string{
		fmt.Sprintf("%s.%s='%s'", eventTypeAckPacket, attributeKeyPacketSourceClient, sourceClientID),
		fmt.Sprintf("%s.%s='%s'", eventTypeAckPacket, attributeKeyPacketDestClient, destClientID),
		fmt.Sprintf("%s.%s=%d", eventTypeAckPacket, attributeKeyPacketSequence, sequence),
	}, " AND ")

	searchResult, err := client.tmRPC.TxSearch(ctx, query, false, nil, nil, "desc")
	if err != nil {
		return nil, fmt.Errorf(
			"searching for ack tx from source client %s to dest client %s with sequence %d: %w",
			sourceClientID,
			destClientID,
			sequence,
			err,
		)
	}
	if searchResult.TotalCount == 0 {
		return nil, ErrTxNotFound
	}
	if searchResult.TotalCount != 1 {
		return nil, fmt.Errorf("expected only 1 tx, but got %d", searchResult.TotalCount)
	}

	ackTx := searchResult.Txs[0]

	// fetch header at block height to get timestamp
	headerResult, err := client.tmRPC.Header(ctx, &ackTx.Height)
	if err != nil {
		return nil, fmt.Errorf("fetching block header at ack tx height %d: %w", ackTx.Height, err)
	}
	if headerResult.Header == nil {
		return nil, fmt.Errorf("fetched block header at ack tx height %d without err but header was nil", ackTx.Height)
	}

	signer, err := client.txSigner(ackTx)
	if err != nil {
		// the above could fail for many reasons (some other relayer included
		// some crazy message types in the ack we are unable to decode, weird
		// signature type, etc), however we should not return an error to the
		// client since, we have most of the relevant info that is required,
		// besides the signer address which is not critical
		lmt.Logger(ctx).Error("found ack tx but could not determine the tx's signer, ignoring error and continuing", zap.Error(err), zap.String("tx_hash", ackTx.Hash.String()))
	}

	return &BridgeTx{
		Hash:           ackTx.Hash.String(),
		Timestamp:      headerResult.Header.Time,
		RelayerAddress: signer,
	}, nil
}

func (client *CosmosBridgeClient) FindTimeoutTx(
	ctx context.Context,
	sourceClientID string,
	destClientID string,
	sequence uint64,
) (*BridgeTx, error) {
	query := strings.Join([]string{
		fmt.Sprintf("%s.%s='%s'", eventTypeTimeoutPacket, attributeKeyPacketSourceClient, sourceClientID),
		fmt.Sprintf("%s.%s='%s'", eventTypeTimeoutPacket, attributeKeyPacketDestClient, destClientID),
		fmt.Sprintf("%s.%s=%d", eventTypeTimeoutPacket, attributeKeyPacketSequence, sequence),
	}, " AND ")

	searchResult, err := client.tmRPC.TxSearch(ctx, query, false, nil, nil, "desc")
	if err != nil {
		return nil, fmt.Errorf(
			"searching for timeout tx from source client %s to dest client %s with sequence %d: %w",
			sourceClientID,
			destClientID,
			sequence,
			err,
		)
	}
	if searchResult.TotalCount == 0 {
		return nil, ErrTxNotFound
	}
	if searchResult.TotalCount != 1 {
		return nil, fmt.Errorf("expected only 1 tx, but got %d", searchResult.TotalCount)
	}

	timeoutTx := searchResult.Txs[0]

	// fetch header at block height to get timestamp
	headerResult, err := client.tmRPC.Header(ctx, &timeoutTx.Height)
	if err != nil {
		return nil, fmt.Errorf("fetching block header at timeout tx height %d: %w", timeoutTx.Height, err)
	}
	if headerResult.Header == nil {
		return nil, fmt.Errorf("fetched block header at timeout tx height %d without err but header was nil", timeoutTx.Height)
	}

	signer, err := client.txSigner(timeoutTx)
	if err != nil {
		// the above could fail for many reasons (some other relayer included
		// some crazy message types in the timeout we are unable to decode, weird
		// signature type, etc), however we should not return an error to the
		// client since, we have most of the relevant info that is required,
		// besides the signer address which is not critical
		lmt.Logger(ctx).Error("found timeout tx but could not determine the tx's signer, ignoring error and continuing", zap.Error(err), zap.String("tx_hash", timeoutTx.Hash.String()))
	}

	return &BridgeTx{
		Hash:           timeoutTx.Hash.String(),
		Timestamp:      headerResult.Header.Time,
		RelayerAddress: signer,
	}, nil
}

func (client *CosmosBridgeClient) DeliverTx(ctx context.Context, bz []byte, _ string) (*BridgeTx, error) {
	var err error
	client.txSubmissionLock.Lock()
	defer func() {
		if err == nil {
			client.lastSubmissionTime = time.Now()
		}
		client.txSubmissionLock.Unlock()
	}()
	select {
	case <-time.After(time.Until(client.lastSubmissionTime.Add(client.txSubmissionDelay))):
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	bech32Address, err := bech32.ConvertAndEncode(client.prefix, client.signer.Address(ctx))
	if err != nil {
		return nil, fmt.Errorf("converting and encoding signer address %s with prefix %s to bech32: %w", client.signer.Address(ctx), client.prefix, err)
	}

	hash, err := client.signAndSubmit(ctx, bz)
	if err != nil {
		return nil, fmt.Errorf("signing and submitting recv tx: %w", err)
	}

	return &BridgeTx{
		Hash:           hash.String(),
		Timestamp:      time.Now().UTC(),
		RelayerAddress: bech32Address,
	}, nil
}

func (client *CosmosBridgeClient) PacketWriteAckStatus(
	ctx context.Context,
	hash string,
	sequence uint64,
	sourceClientID string,
	destClientID string,
) (db.Ibcv2WriteAckStatus, error) {
	hashBytes, err := hex.DecodeString(hash)
	if err != nil {
		return db.Ibcv2WriteAckStatusUNKNOWN, fmt.Errorf("hex decoding tx hash string %s: %w", hash, err)
	}

	// we use the once wrapper so that this function is only called once for a
	// particular value of hash bytes even for many invocations
	txResult, err := client.txOnce.Do(hash, func() (*coretypes.ResultTx, error) {
		return client.tmRPC.Tx(ctx, hashBytes, false)
	})
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return db.Ibcv2WriteAckStatusUNKNOWN, ErrTxNotFound
		}
		return db.Ibcv2WriteAckStatusUNKNOWN, fmt.Errorf("getting tx %s's result: %w", hash, err)
	}

	for _, event := range txResult.TxResult.GetEvents() {
		if event.GetType() != eventTypeWriteAck {
			continue
		}

		var sequenceMatch, sourceClientMatch, destClientMatch bool
		for _, attribute := range event.GetAttributes() {
			switch attribute.GetKey() {
			case attributeKeyPacketSequence:
				if attribute.GetValue() == strconv.Itoa(int(sequence)) {
					sequenceMatch = true
				}
			case attributeKeyPacketSourceClient:
				if attribute.GetValue() == sourceClientID {
					sourceClientMatch = true
				}
			case attributeKeyPacketDestClient:
				if attribute.GetValue() == destClientID {
					destClientMatch = true
				}
			default:
				continue
			}
		}
		if sequenceMatch && sourceClientMatch && destClientMatch {
			// note that we dont want to parse the write ack until we know that
			// the write ack is for the packet that we are interested in, i.e.
			// outside of the loop iterating over the events. Since we do not
			// want a write ack for a different packet to possibly fail parsing
			// and return an error.
			return parseWriteAckStatusForPacket(event)
		}
	}

	return db.Ibcv2WriteAckStatusUNKNOWN, ErrWriteAckNotFoundForPacket
}

func parseWriteAckStatusForPacket(writeAckEvent abci.Event) (db.Ibcv2WriteAckStatus, error) {
	for _, attribute := range writeAckEvent.GetAttributes() {
		if attribute.GetKey() != "encoded_acknowledgement_hex" {
			continue
		}
		protoAckBytes, err := hex.DecodeString(attribute.GetValue())
		if err != nil {
			return db.Ibcv2WriteAckStatusUNKNOWN, fmt.Errorf("%w:%w", ErrWriteAckDecoding, err)
		}

		var ack ibc_channel_v2_types.Acknowledgement
		if err = proto.Unmarshal(protoAckBytes, &ack); err != nil {
			return db.Ibcv2WriteAckStatusUNKNOWN, fmt.Errorf("%w:%w", ErrWriteAckDecoding, err)
		}

		if len(ack.GetAppAcknowledgements()) == 1 {
			// if the ack is an error, there will only be a single app
			// acknowledgement that is the universal error ack
			if bytes.Equal(ack.GetAppAcknowledgements()[0], ErrorAcknowledgement[:]) {
				return db.Ibcv2WriteAckStatusERROR, nil
			}
		}
		return db.Ibcv2WriteAckStatusSUCCESS, nil
	}
	return db.Ibcv2WriteAckStatusUNKNOWN, nil
}

func (client *CosmosBridgeClient) SendPacketsFromTx(ctx context.Context, sourceChainID string, txHash string) ([]*PacketInfo, error) {
	hashBytes, err := hex.DecodeString(txHash)
	if err != nil {
		return nil, fmt.Errorf("hex decoding tx hash: %w", err)
	}

	txResult, err := client.tmRPC.Tx(ctx, hashBytes, false)
	if err != nil {
		return nil, fmt.Errorf("getting tx result: %w", err)
	}

	var packets []*PacketInfo
	for _, event := range txResult.TxResult.GetEvents() {
		if event.GetType() != eventTypeSendPacket {
			continue
		}
		var packet PacketInfo
		for _, attribute := range event.GetAttributes() {
			switch attribute.GetKey() {
			case attributeKeyPacketSequence:
				sequence, err := strconv.Atoi(attribute.GetValue())
				if err != nil {
					return nil, fmt.Errorf("converting packet sequence %s to int", attribute.GetValue())
				}
				packet.Sequence = uint64(sequence)
			case attributeKeyPacketSourceClient:
				packet.SourceClient = attribute.GetValue()
			case attributeKeyPacketDestClient:
				packet.DestinationClient = attribute.GetValue()
			case attributeKeyPacketTimeoutTimestamp:
				unixts, err := strconv.Atoi(attribute.GetValue())
				if err != nil {
					return nil, fmt.Errorf("converting attribute timeout ts %s to int", attribute.GetValue())
				}
				packet.TimeoutTimestamp = time.Unix(int64(unixts), 0)
			default:
				continue
			}
		}
		if packet.SourceClient == "" || packet.DestinationClient == "" {
			// v1 send packets do not contain this info, we do not care about
			// them
			continue
		}
		packets = append(packets, &packet)
	}
	if len(packets) == 0 {
		return nil, nil
	}

	// only query for the header if we have actually found packets within the
	// tx, doing this after the loop just saves us a call if there are not send
	// packets found in the tx
	header, err := client.tmRPC.Header(ctx, &txResult.Height)
	if err != nil {
		return nil, fmt.Errorf("getting header for height %d: %w", txResult.Height, err)
	}
	if header.Header == nil {
		return nil, fmt.Errorf("got nil header at height %d", txResult.Height)
	}

	for _, packet := range packets {
		packet.Timestamp = header.Header.Time
	}

	return packets, nil
}

func (client *CosmosBridgeClient) ShouldRetryTx(ctx context.Context, txHash string, expiry time.Duration, sentTs time.Time) (bool, error) {
	txHashBytes, err := hex.DecodeString(txHash)
	if err != nil {
		return false, fmt.Errorf("decoding hex tx hash %s to bytes: %w", txHash, err)
	}

	resp, err := client.txOnce.Do(txHash, func() (*coretypes.ResultTx, error) { return client.tmRPC.Tx(ctx, txHashBytes, false) })
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			// tx not found on chain, check if we have waited longer than the
			// expiry of on chain time, and if we have, the tx should be
			// retried

			latestHeaderResult, err := client.tmRPC.Header(ctx, nil)
			if err != nil {
				return false, fmt.Errorf("getting latest block header: %w", err)
			}
			if latestHeaderResult.Header == nil {
				return false, fmt.Errorf("fetched block header at latest height without err but header was nil")
			}

			expiresAt := sentTs.UTC().Add(expiry)
			isExpired := expiresAt.Before(latestHeaderResult.Header.Time.UTC())
			if isExpired {
				// we have waited longer than the expiry and the tx is still
				// not found, retry it
				return true, nil
			}

			// tx not found but we have not timed out waiting for the tx yet,
			// return that the tx should not be retried yet, but return a
			// sentinel error to inform the user of the situation
			return false, ErrTxNotFound
		}
		// error fetching tx on chain, node may be down or some other issue so
		// do not retry yet
		return false, fmt.Errorf("fetching tx %s on chain: %w", txHash, err)
	}
	if resp.TxResult.GetCode() != 0 {
		// tx was unsuccessful, needs to be retried
		return true, nil
	}

	// tx exists and was successful, no need to retry
	return false, nil
}

func (client *CosmosBridgeClient) WaitForChain(ctx context.Context) error {
	const initialTick = time.Millisecond
	const tick = time.Second

	ticker := time.NewTicker(initialTick)

	now := time.Now()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			ticker.Stop()

			latest, err := client.tmRPC.Header(ctx, nil)
			if err != nil {
				return fmt.Errorf("getting latest block header: %w", err)
			}
			if latest.Header == nil {
				return fmt.Errorf("latest header was nil")
			}

			if latest.Header.Time.After(now) {
				return nil
			}

			if time.Since(now) > time.Second*30 {
				// if the chain hasnt reached the current height in 30s, something
				// may be wrong and we should start logging warnings
				lmt.Logger(ctx).Warn(
					"latest header time is behind current time, waiting... (already been waiting for >30s)",
					zap.Time("chain_time", latest.Header.Time),
					zap.Time("wait_until", now),
				)
			} else {
				lmt.Logger(ctx).Debug(
					"latest header time is behind current time, waiting...",
					zap.Time("chain_time", latest.Header.Time),
					zap.Time("wait_until", now),
				)
			}

			ticker.Reset(tick)
		}
	}
}

func (client *CosmosBridgeClient) IsTxFinalized(ctx context.Context, txHash string, offset *uint64) (bool, error) {
	return true, nil
}

func (client *CosmosBridgeClient) IsTimestampFinalized(ctx context.Context, timestamp time.Time, offset *uint64) (bool, error) {
	// Cosmos chains have instant finality, so we just need to check if the latest
	// block timestamp has passed the target timestamp.
	// Note: We don't use LatestOnChainTimestamp here because it caches the result.
	header, err := client.tmRPC.Header(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("getting latest header: %w", err)
	}
	if header.Header == nil {
		return false, fmt.Errorf("got nil header when fetching latest header")
	}
	return header.Header.Time.After(timestamp), nil
}

func (client *CosmosBridgeClient) ChainType() config.ChainType {
	return config.ChainType_COSMOS
}

func (client *CosmosBridgeClient) signAndSubmit(ctx context.Context, bz []byte) (comet_bytes.HexBytes, error) {
	var txBody txtypes.TxBody
	if err := proto.Unmarshal(bz, &txBody); err != nil {
		return nil, fmt.Errorf("unmarshaling tx bytes: %w", err)
	}
	if len(txBody.Messages) == 0 {
		return nil, fmt.Errorf("no messages to relay")
	}

	var numClientUpdates int
	updateClientTypeURL := codec_types.MsgTypeURL(&ibc_client_v1_types.MsgUpdateClient{})

	var msgs []sdk.Msg
	for _, msg := range txBody.Messages {
		var sdkMsg sdk.Msg
		if err := reg.UnpackAny(msg, &sdkMsg); err != nil {
			return nil, fmt.Errorf("unpacking msg into sdk.Msg: %w", err)
		}
		if codec_types.MsgTypeURL(sdkMsg) == updateClientTypeURL {
			numClientUpdates++
		}
		msgs = append(msgs, sdkMsg)
	}

	if numClientUpdates > 1 {
		// if the number of update client messages is > 1, then this tx is going to
		// be too large to submit (http limitation, code 413). to avoid this,
		// we will submit each update client one by one until there are none
		// left. then we will submit the non update client messages
		lmt.Logger(ctx).Warn(
			fmt.Sprintf("found %d update client messages tx, submitting them on chain individually", numClientUpdates),
			zap.Int("num_client_updates", numClientUpdates),
		)
		var numSubmitted int
		var nonClientUpdateMsgs []sdk.Msg
		for _, msg := range msgs {
			if codec_types.MsgTypeURL(msg) != updateClientTypeURL {
				nonClientUpdateMsgs = append(nonClientUpdateMsgs, msg)
				continue
			}

			hash, err := client.signAndSubmitMessages(ctx, msg)
			if err != nil {
				return nil, fmt.Errorf("submitting individual update client message number %d since there were %d included in tx: %w", numSubmitted, numClientUpdates, err)
			}
			numSubmitted++

			lmt.Logger(ctx).Info(
				fmt.Sprintf("submitted individual client update, sleeping for %s to submit next tx", client.txSubmissionDelay),
				zap.String("tx_hash", hash.String()),
			)
			time.Sleep(client.txSubmissionDelay)
		}

		// we have submitted all client update msgs individually, now only
		// submit non update client messages
		lmt.Logger(ctx).Info("submitted all individual client updates", zap.Int("num_client_update", numClientUpdates))
		msgs = nonClientUpdateMsgs
	}

	return client.signAndSubmitMessages(ctx, msgs...)
}

func (client *CosmosBridgeClient) signAndSubmitMessages(ctx context.Context, msgs ...sdk.Msg) (comet_bytes.HexBytes, error) {
	bech32Address, err := bech32.ConvertAndEncode(client.prefix, client.signer.Address(ctx))
	if err != nil {
		return nil, fmt.Errorf("converting and encoding signer address %s with prefix %s to bech32: %w", client.signer.Address(ctx), client.prefix, err)
	}

	accountClient := auth_types.NewQueryClient(client.conn)
	account, err := accountClient.AccountInfo(ctx, &auth_types.QueryAccountInfoRequest{Address: bech32Address})
	if err != nil {
		return nil, fmt.Errorf("getting account info for cosmos signer %s on chain %s: %w", bech32Address, client.chainID, err)
	}
	accountInfo := account.GetInfo()
	if accountInfo == nil {
		return nil, fmt.Errorf("got no info for cosmos signer account %s on chain %s: %w", bech32Address, client.chainID, err)
	}

	txBuilder := client.txConfig.NewTxBuilder()
	if err = txBuilder.SetMsgs(msgs...); err != nil {
		return nil, fmt.Errorf("setting message in tx: %w", err)
	}

	gasLimit, err := client.getEstimatedGasLimit(ctx, txBuilder, accountInfo)
	if err != nil {
		return nil, fmt.Errorf("getting estimated gas limit for tx: %w", err)
	}
	txBuilder.SetGasLimit(gasLimit)

	if client.feeDenom != "" {
		fee, err := client.getTxFee(gasLimit)
		if err != nil {
			return nil, fmt.Errorf("getting tx fee: %w", err)
		}
		txBuilder.SetFeeAmount(sdk.NewCoins(fee))
	}

	unsigedTx := signing.NewCosmosTransaction(txBuilder.GetTx(), accountInfo.GetAccountNumber(), accountInfo.Sequence, client.txConfig)
	signedTx, err := client.signer.Sign(ctx, client.chainID, unsigedTx)
	if err != nil {
		return nil, fmt.Errorf("signing tx: %w", err)
	}

	signedTxBytes, err := client.txConfig.TxEncoder()(signedTx.(*signing.CosmosTransaction).Tx)
	if err != nil {
		return nil, fmt.Errorf("encoding signed tx: %w", err)
	}

	result, err := client.tmRPC.BroadcastTxSync(ctx, signedTxBytes)
	if err != nil {
		return nil, fmt.Errorf("broadcasting tx synchronously from signer %s: %w", bech32Address, err)
	}
	if result.Code != 0 {
		return nil, fmt.Errorf("broadcasting cosmos tx from signer %s returned error code %d: %s", bech32Address, result.Code, result.Log)
	}

	return result.Hash, nil
}

func (client *CosmosBridgeClient) getTxFee(gasLimit uint64) (sdk.Coin, error) {
	if client.feeDenom == "" {
		return sdk.Coin{}, fmt.Errorf("fee denom not set for chain %s", client.chainID)
	}
	if client.gasPrice > 0 && client.feeAmount > 0 {
		return sdk.Coin{}, fmt.Errorf("gas price and fee amount cannot both be set for chain %s", client.chainID)
	}

	if client.gasPrice > 0 {
		feeAmount, _ := new(big.Float).Mul(big.NewFloat(client.gasPrice), new(big.Float).SetInt64(int64(gasLimit))).Int(nil)
		feeAmount.Add(feeAmount, big.NewInt(1))
		return sdk.NewCoin(client.feeDenom, sdk_math.NewIntFromBigInt(feeAmount)), nil
	}
	if client.feeAmount > 0 {
		return sdk.NewCoin(client.feeDenom, sdk_math.NewIntFromUint64(client.feeAmount)), nil
	}

	return sdk.NewCoin(client.feeDenom, sdk_math.NewIntFromUint64(0)), nil
}

func (client *CosmosBridgeClient) getEstimatedGasLimit(ctx context.Context, txBuilder client.TxBuilder, accountInfo *auth_types.BaseAccount) (uint64, error) {
	unsigedTx := signing.NewCosmosTransaction(txBuilder.GetTx(), accountInfo.GetAccountNumber(), accountInfo.GetSequence(), client.txConfig)
	signedTx, err := client.signer.Sign(ctx, client.chainID, unsigedTx)
	if err != nil {
		return 0, fmt.Errorf("signing tx: %w", err)
	}

	signedTxBytes, err := client.txConfig.TxEncoder()(signedTx.(*signing.CosmosTransaction).Tx)
	if err != nil {
		return 0, fmt.Errorf("encoding signed tx: %w", err)
	}

	txService := txtypes.NewServiceClient(client.conn)
	res, err := txService.Simulate(ctx, &txtypes.SimulateRequest{TxBytes: signedTxBytes})
	if err != nil {
		return 0, fmt.Errorf("simulating tx to get estimated gas usage: %w", err)
	}
	if res.GetGasInfo() == nil {
		return 0, fmt.Errorf("got nil gas info after simulating tx")
	}

	// multiply gas used by a multiplier to give us a buffer on the gas limit
	const gasMultiplier float64 = 1.5
	return uint64(math.Ceil(float64(res.GetGasInfo().GetGasUsed()) * gasMultiplier)), nil
}

func (client *CosmosBridgeClient) LatestOnChainTimestamp(ctx context.Context) (time.Time, error) {
	return client.latestTimestampOnce.Do("key", func() (time.Time, error) {
		header, err := client.tmRPC.Header(ctx, nil)
		if err != nil {
			return time.Time{}, fmt.Errorf("getting latest header: %w", err)
		}
		if header.Header == nil {
			return time.Time{}, fmt.Errorf("got nil header when fetching latest header")
		}
		return header.Header.Time, nil
	})
}

func (client *CosmosBridgeClient) SignerGasTokenBalance(ctx context.Context) (*big.Int, error) {
	bech32Address, err := bech32.ConvertAndEncode(client.prefix, client.signer.Address(ctx))
	if err != nil {
		return nil, fmt.Errorf("converting and encoding signer address %s with prefix %s to bech32: %w", client.signer.Address(ctx), client.prefix, err)
	}

	req := &bank_types.QueryBalanceRequest{Address: bech32Address, Denom: client.feeDenom}
	resp, err := bank_types.NewQueryClient(client.conn).Balance(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("getting %s balance for signer on chain %s: %w", client.feeDenom, client.chainID, err)
	}
	if resp.GetBalance() == nil {
		return nil, fmt.Errorf("got nil %s balance for signer on chain %s: %w", client.feeDenom, client.chainID, err)
	}
	return resp.GetBalance().Amount.BigInt(), nil
}

func (client *CosmosBridgeClient) TxFee(ctx context.Context, txHash string) (*big.Int, error) {
	hashBytes, err := hex.DecodeString(txHash)
	if err != nil {
		return nil, fmt.Errorf("hex decoding tx hash string %s: %w", txHash, err)
	}

	result, err := client.txOnce.Do(txHash, func() (*coretypes.ResultTx, error) {
		return client.tmRPC.Tx(ctx, hashBytes, false)
	})
	if err != nil {
		return nil, fmt.Errorf("getting results for ts hash %s: %w", txHash, err)
	}

	tx, err := client.txConfig.TxDecoder()(result.Tx)
	if err != nil {
		return nil, fmt.Errorf("decoding tx result: %w", err)
	}

	feeTx, ok := tx.(sdk.FeeTx)
	if !ok {
		return nil, fmt.Errorf("could not convert decoded tx to sdk.FeeTx")
	}

	fee := feeTx.GetFee().AmountOf(client.feeDenom)
	if fee == sdk_math.ZeroInt() {
		return nil, fmt.Errorf("zero fee amount found for denom %s in tx result auth info", client.feeDenom)
	}

	return fee.BigInt(), nil
}

func (client *CosmosBridgeClient) txSigner(result *coretypes.ResultTx) (string, error) {
	decoded, err := client.txConfig.TxDecoder()(result.Tx)
	if err != nil {
		return "", fmt.Errorf("decoding tx result: %w", err)
	}

	for _, msg := range decoded.GetMsgs() {
		switch t := msg.(type) {
		case *ibc_channel_v2_types.MsgRecvPacket:
			return t.Signer, nil
		case *ibc_channel_v2_types.MsgAcknowledgement:
			return t.Signer, nil
		case *ibc_channel_v2_types.MsgTimeout:
			return t.Signer, nil
		default:
			continue
		}
	}
	return "", fmt.Errorf("expected recv packet, ack packet, or timeout packet msg but found none")
}

func (client *CosmosBridgeClient) SendTransfer(
	ctx context.Context,
	clientID string,
	denom string,
	receiver string,
	amount *big.Int,
	memo string,
) (string, error) {
	panic("unimplemented")
}

func (client *CosmosBridgeClient) TimestampAtHeight(ctx context.Context, height uint64) (time.Time, error) {
	heightI := int64(height)
	header, err := client.tmRPC.Header(ctx, &heightI)
	if err != nil {
		return time.Time{}, fmt.Errorf("fetching block header at height %d: %w", height, err)
	}
	if header.Header == nil {
		return time.Time{}, fmt.Errorf("fetched block header at height %d without err but header was nil", height)
	}
	return header.Header.Time.UTC(), nil
}

func (client *CosmosBridgeClient) ClientState(ctx context.Context, clientID string) (ClientState, error) {
	req := &ibc_client_v1_types.QueryClientStateRequest{ClientId: clientID}
	state, err := ibc_client_v1_types.NewQueryClient(client.conn).ClientState(ctx, req)
	if err != nil {
		return ClientState{}, fmt.Errorf("querying for state of client: %w", err)
	}
	if state.GetClientState() == nil {
		return ClientState{}, fmt.Errorf("got nil client state")
	}
	if state.GetClientState().GetValue() == nil {
		return ClientState{}, fmt.Errorf("got nil client state value")
	}

	switch state.GetClientState().GetTypeUrl() {
	case "/ibc.lightclients.wasm.v1.ClientState":
		var fullstate wasm_v1_types.ClientState
		if err = proto.Unmarshal(state.GetClientState().GetValue(), &fullstate); err != nil {
			return ClientState{}, fmt.Errorf("proto decoding full wasm client state (%s): %w", string(state.GetClientState().GetValue()), err)
		}

		var wasmClientState WasmClientState
		if err = json.Unmarshal(fullstate.Data, &wasmClientState); err != nil {
			return ClientState{}, fmt.Errorf("json decoding wasm client state data (%s): %w", string(string(fullstate.Data)), err)
		}

		return ClientState{
			WasmClientState: &wasmClientState,
		}, nil
	default:
		return ClientState{}, fmt.Errorf("unsupported client state type url %s", state.GetClientState().GetTypeUrl())
	}
}

func (client *CosmosBridgeClient) WaitForTx(ctx context.Context, hash string) error {
	hashBytes, err := hex.DecodeString(hash)
	if err != nil {
		return fmt.Errorf("hex decoding tx hash string %s: %w", hash, err)
	}

	const tick = time.Second * 3
	ticker := time.NewTicker(tick)

	start := time.Now()
	timeout := time.Minute
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			ticker.Stop()

			result, err := client.tmRPC.Tx(ctx, hashBytes, false)
			if err != nil {
				if strings.Contains(err.Error(), "not found") {
					ticker.Reset(tick)
					continue
				}
				return fmt.Errorf("getting tx result: %w", err)
			}
			if result != nil {
				return nil
			}

			if time.Since(start) > timeout {
				return fmt.Errorf("timeout after waiting %s for transaction result", timeout.String())
			}

			ticker.Reset(tick)
		}
	}
}

func (client *CosmosBridgeClient) GetTransactionSender(ctx context.Context, hash string) (string, error) {
	return "", fmt.Errorf("GetTransactionSender not implemented")
}
