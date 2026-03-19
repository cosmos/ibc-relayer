package evm

import (
	"context"
	"errors"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"

	"github.com/cosmos/ibc-relayer/shared/signing"
)

func EthereumSignerToBindSignerFn(ctx context.Context, signer signing.Signer, chainID string) bind.SignerFn {
	return func(_ common.Address, tx *types.Transaction) (*types.Transaction, error) {
		signedTx, err := signer.Sign(ctx, chainID, tx)
		if err != nil {
			return nil, err
		}

		rawTx, ok := signedTx.(*types.Transaction)
		if !ok {
			return nil, errors.New("invalid transaction type")
		}

		return rawTx, nil
	}
}
