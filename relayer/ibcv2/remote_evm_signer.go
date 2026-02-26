package ibcv2

import (
	"context"
	"fmt"

	signerservice "github.com/cosmos/platform-relayer/proto/gen/signer"
	"github.com/cosmos/platform-relayer/shared/signing"
	"github.com/cosmos/platform-relayer/shared/signing/signer_service"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
)

type RemoteEVMSigner struct {
	manager  *signer_service.SignerManager
	walletID string
	chainID  string
}

func NewRemoteEVMSigner(
	manager *signer_service.SignerManager,
	walletID string,
	chainID string,
) *RemoteEVMSigner {
	return &RemoteEVMSigner{
		manager:  manager,
		walletID: walletID,
		chainID:  chainID,
	}
}

func (s *RemoteEVMSigner) Sign(ctx context.Context, chainID string, tx signing.Transaction) (signing.Transaction, error) {
	evmTx, ok := tx.(*types.Transaction)
	if !ok {
		return nil, fmt.Errorf("expected *types.Transaction, got %T", tx)
	}

	// Get bind.SignerFn from SignerManager
	signerFn := s.manager.RemoteSignerToBindSignerFn(ctx, chainID, s.walletID)

	// Sign the transaction (address param is unused by remote signer)
	signedTx, err := signerFn(common.Address{}, evmTx)
	if err != nil {
		return nil, fmt.Errorf("remote EVM signing failed: %w", err)
	}

	return signedTx, nil
}

func (s *RemoteEVMSigner) Address(ctx context.Context) []byte {
	resp, err := s.manager.GetWallet(ctx, &signerservice.GetWalletRequest{
		Id:         s.walletID,
		PubkeyType: signerservice.PubKeyType_Ethereum,
	})
	if err != nil {
		return []byte{}
	}

	// Derive Ethereum address from public key
	pubKeyBytes := resp.GetWallet().GetPubkey()
	pubKey, err := crypto.UnmarshalPubkey(pubKeyBytes)
	if err != nil {
		return []byte{}
	}

	address := crypto.PubkeyToAddress(*pubKey)
	return address.Bytes()
}
