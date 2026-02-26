package evmrpc

import (
	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"golang.org/x/net/context"
)

const (
	transferSignature = "0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef"
)

type EVMChainRPC interface {
	GetERC20Transfers(ctx context.Context, txHash string) ([]ERC20Transfer, error)
	SendTx(ctx context.Context, txBytes []byte) (string, error)
	GetTxReceipt(ctx context.Context, txHash string) (*types.Receipt, error)
	GetTxByHash(ctx context.Context, txHash string) (*types.Transaction, bool, error)
	GetLogs(ctx context.Context, topics [][]common.Hash, addresses []common.Address) ([]types.Log, error)
	BlockByHash(ctx context.Context, hash common.Hash) (*types.Block, error)
	HeaderByHash(ctx context.Context, hash common.Hash) (*types.Header, error)
	LatestHeight(ctx context.Context) (uint64, error)
	Client() *ethclient.Client
}

type ERC20Transfer struct {
	Source string
	Dest   string
}

type chainRPCImpl struct {
	cc *ethclient.Client
}

func NewEVMChainRPC(cc *ethclient.Client) EVMChainRPC {
	return &chainRPCImpl{cc: cc}
}

func (cr *chainRPCImpl) GetERC20Transfers(ctx context.Context, txHash string) ([]ERC20Transfer, error) {
	var transfers []ERC20Transfer

	receipt, err := cr.cc.TransactionReceipt(context.Background(), common.HexToHash(txHash))
	if err != nil {
		return nil, err
	}
	for _, log := range receipt.Logs {
		logSignature := log.Topics[0].Hex()
		// check if there is a token transfer and store the recipient address if so
		if logSignature == transferSignature {
			sourceAddress := log.Topics[1].Hex()
			destAddress := log.Topics[2].Hex()
			transfers = append(transfers, ERC20Transfer{
				Source: "0x" + sourceAddress[len(sourceAddress)-40:],
				Dest:   "0x" + destAddress[len(destAddress)-40:],
			})
		}
	}
	return transfers, nil
}

func (cr *chainRPCImpl) SendTx(ctx context.Context, txBytes []byte) (string, error) {
	tx := &types.Transaction{}
	err := tx.UnmarshalBinary(txBytes)
	if err != nil {
		return "", err
	}
	err = cr.cc.SendTransaction(ctx, tx)
	if err != nil {
		return "", err
	}
	return tx.Hash().Hex(), nil
}

func (cr *chainRPCImpl) GetTxReceipt(ctx context.Context, txHash string) (*types.Receipt, error) {
	receipt, err := cr.cc.TransactionReceipt(ctx, common.HexToHash(txHash))
	if err != nil {
		return nil, err
	}
	return receipt, nil
}

func (cr *chainRPCImpl) GetTxByHash(ctx context.Context, txHash string) (*types.Transaction, bool, error) {
	tx, isPending, err := cr.cc.TransactionByHash(ctx, common.HexToHash(txHash))
	if err != nil {
		return nil, false, err
	}
	return tx, isPending, nil
}

func (cr *chainRPCImpl) GetLogs(ctx context.Context, topics [][]common.Hash, addresses []common.Address) ([]types.Log, error) {
	logsTopic, err := cr.cc.FilterLogs(ctx, ethereum.FilterQuery{Topics: topics, Addresses: addresses})
	if err != nil {
		return nil, err
	}
	return logsTopic, nil
}

func (cr *chainRPCImpl) BlockByHash(ctx context.Context, hash common.Hash) (*types.Block, error) {
	return cr.cc.BlockByHash(ctx, hash)
}

func (cr *chainRPCImpl) HeaderByHash(ctx context.Context, hash common.Hash) (*types.Header, error) {
	return cr.cc.HeaderByHash(ctx, hash)
}

func (cr *chainRPCImpl) LatestHeight(ctx context.Context) (uint64, error) {
	return cr.cc.BlockNumber(ctx)
}

func (cr *chainRPCImpl) Client() *ethclient.Client {
	return cr.cc
}
