package tmrpc

import (
	"context"
	"encoding/hex"
	"net/http"

	"github.com/cometbft/cometbft/rpc/client"
	rpcclienthttp "github.com/cometbft/cometbft/rpc/client/http"
	coretypes "github.com/cometbft/cometbft/rpc/core/types"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	cryptocodec "github.com/cosmos/cosmos-sdk/crypto/codec"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"

	"github.com/cosmos/platform-relayer/shared/config"
	"github.com/cosmos/platform-relayer/shared/utils"
)

var reg codectypes.InterfaceRegistry

func init() {
	reg = codectypes.NewInterfaceRegistry()
	authtypes.RegisterInterfaces(reg)
	cryptocodec.RegisterInterfaces(reg)
	banktypes.RegisterInterfaces(reg)
}

type TendermintRPCClient interface {
	client.Client
}

type TendermintRPCQuerier interface {
	GetTx(ctx context.Context, txHash string) (*coretypes.ResultTx, error)
	GetBlock(ctx context.Context, height int64) (*coretypes.ResultBlock, error)
	Status(ctx context.Context) (*coretypes.ResultStatus, error)
	TxSearch(
		ctx context.Context,
		query string,
		prove bool,
		page, perPage *int,
		orderBy string,
	) (*coretypes.ResultTxSearch, error)
}

type rcpQuerierImpl struct {
	cli TendermintRPCClient
}

func DefaultTendermintRPCQuerier(ctx context.Context, chainID string) (TendermintRPCQuerier, error) {
	rpc, err := config.GetConfigReader(ctx).GetRPCEndpoint(chainID)
	if err != nil {
		return nil, err
	}

	basicAuth, err := config.GetConfigReader(ctx).GetBasicAuth(chainID)
	if err != nil {
		return nil, err
	}

	client, err := rpcclienthttp.NewWithClient(rpc, "/websocket", &http.Client{
		Transport: utils.NewBasicAuthTransport(basicAuth, http.DefaultTransport),
	})
	if err != nil {
		return nil, err
	}

	return NewTendermintRPCQuerier(client), nil
}

func NewTendermintRPCQuerier(cli TendermintRPCClient) TendermintRPCQuerier {
	return &rcpQuerierImpl{cli: cli}
}

func (rq *rcpQuerierImpl) Status(ctx context.Context) (*coretypes.ResultStatus, error) {
	return rq.cli.Status(ctx)
}

func (rq *rcpQuerierImpl) GetTx(ctx context.Context, txHash string) (*coretypes.ResultTx, error) {
	txBytes, err := hex.DecodeString(txHash)
	if err != nil {
		return nil, err
	}

	return rq.cli.Tx(
		ctx,
		txBytes,
		false,
	)
}

func (rq *rcpQuerierImpl) GetBlock(ctx context.Context, height int64) (*coretypes.ResultBlock, error) {
	return rq.cli.Block(
		ctx,
		&height,
	)
}

func (rq *rcpQuerierImpl) TxSearch(
	ctx context.Context,
	query string,
	prove bool,
	page, perPage *int,
	orderBy string,
) (*coretypes.ResultTxSearch, error) {
	return rq.cli.TxSearch(ctx, query, prove, page, perPage, orderBy)
}
