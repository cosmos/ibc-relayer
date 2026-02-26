package ibcv2

import (
	"context"
	"fmt"

	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	signerservice "github.com/cosmos/platform-relayer/proto/gen/signer"
	"github.com/cosmos/platform-relayer/shared/signing"
	"github.com/cosmos/platform-relayer/shared/signing/signer_service"
)

var _ signing.Signer = (*RemoteCosmosSigner)(nil)

type RemoteCosmosSigner struct {
	manager  *signer_service.SignerManager
	walletID string
	chainID  string
	txConfig client.TxConfig
}

func NewRemoteCosmosSigner(
	manager *signer_service.SignerManager,
	walletID string,
	chainID string,
	txConfig client.TxConfig,
) *RemoteCosmosSigner {
	return &RemoteCosmosSigner{
		manager:  manager,
		walletID: walletID,
		chainID:  chainID,
		txConfig: txConfig,
	}
}

func (s *RemoteCosmosSigner) Sign(ctx context.Context, chainID string, tx signing.Transaction) (signing.Transaction, error) {
	cosmosTx, ok := tx.(*signing.CosmosTransaction)
	if !ok {
		return nil, fmt.Errorf("expected CosmosTransaction, got %T", tx)
	}

	txBuilder, err := s.txConfig.WrapTxBuilder(cosmosTx.Tx)
	if err != nil {
		return nil, fmt.Errorf("wrapping tx as builder: %w", err)
	}

	signedTx, err := s.manager.SignCosmosTransaction(
		ctx,
		txBuilder,
		cosmosTx.AccountNumber,
		cosmosTx.Sequence,
		chainID,
		s.walletID,
		s.txConfig,
	)
	if err != nil {
		return nil, fmt.Errorf("remote signing failed: %w", err)
	}

	cosmosTx.Tx = signedTx
	return cosmosTx, nil
}

func (s *RemoteCosmosSigner) Address(ctx context.Context) []byte {
	resp, err := s.manager.GetWallet(ctx, &signerservice.GetWalletRequest{
		Id:         s.walletID,
		PubkeyType: signerservice.PubKeyType_Cosmos,
	})
	if err != nil {
		// Return empty on error to avoid initialization panics
		return []byte{}
	}

	pubKey := &secp256k1.PubKey{Key: resp.GetWallet().GetPubkey()}
	return pubKey.Address()
}
