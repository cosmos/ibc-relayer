package signer_service

import (
	"context"
	"fmt"
	"math/big"

	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/tx/signing"
	authSigning "github.com/cosmos/cosmos-sdk/x/auth/signing"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/gagliardetto/solana-go"
	"go.uber.org/zap"
	"google.golang.org/grpc/metadata"

	signerservice "github.com/cosmos/platform-relayer/proto/gen/signer"
	"github.com/cosmos/platform-relayer/shared/config"
	"github.com/cosmos/platform-relayer/shared/lmt"
)

var serviceAccountToken = config.GetServiceAccountToken()

type SignerManager struct {
	sc SignerClient
}

func NewSignerManager(sc SignerClient) SignerManager {
	return SignerManager{
		sc: sc,
	}
}

func (s *SignerManager) SignCosmosTransaction(ctx context.Context, txb client.TxBuilder, accountNumber, sequence uint64,
	chainID string, signerWalletID string, config client.TxConfig,
) (sdk.Tx, error) {
	ctx = metadata.NewOutgoingContext(ctx, metadata.Pairs("authorization", "Bearer "+serviceAccountToken))
	tx := txb.GetTx()

	handler := config.SignModeHandler()
	signDocBz, err := authSigning.GetSignBytesAdapter(ctx, handler, signing.SignMode_SIGN_MODE_DIRECT, authSigning.SignerData{
		ChainID:       chainID,
		AccountNumber: accountNumber,
		Sequence:      sequence,
	}, tx)
	if err != nil {
		return nil, fmt.Errorf("getting bytes to sign: %w", err)
	}

	signResp, err := s.sc.Sign(ctx, &signerservice.SignRequest{
		WalletId: signerWalletID,
		Payload: &signerservice.SignRequest_CosmosTransaction{
			CosmosTransaction: &signerservice.CosmosTransaction{
				SignDocBytes: signDocBz,
			},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to sign transaction. Error: %w", err)
	}

	res, err := s.sc.GetWallet(ctx, &signerservice.GetWalletRequest{
		Id:         signerWalletID,
		PubkeyType: signerservice.PubKeyType_Cosmos,
	})
	if err != nil {
		lmt.Logger(ctx).Error("failed to get wallet from signer to retrieve public key", zap.Error(err))
		return nil, fmt.Errorf("failed to get wallet from signer to retrieve public key. Error: %w", err)
	}

	pubKeyBytes := res.GetWallet().GetPubkey()
	pubKey := &secp256k1.PubKey{
		Key: pubKeyBytes,
	}

	if err = txb.SetSignatures(signing.SignatureV2{
		PubKey: pubKey,
		Data: &signing.SingleSignatureData{
			SignMode:  signing.SignMode_SIGN_MODE_DIRECT,
			Signature: signResp.GetCosmosSignature().GetSignature(),
		},
		Sequence: sequence,
	}); err != nil {
		return nil, err
	}

	return txb.GetTx(), err
}

func (s *SignerManager) SignSolanaTransaction(ctx context.Context, signerWalletID string,
	tx *solana.Transaction,
) (*solana.Transaction, error) {
	ctx = metadata.NewOutgoingContext(ctx, metadata.Pairs("authorization", "Bearer "+serviceAccountToken))

	base64Tx, err := tx.ToBase64()
	if err != nil {
		return nil, fmt.Errorf("failed to convert solana transaction to base64 while signing. Error: %w", err)
	}
	signResp, err := s.sc.Sign(ctx, &signerservice.SignRequest{
		WalletId: signerWalletID,
		Payload: &signerservice.SignRequest_SolanaTransaction{
			SolanaTransaction: &signerservice.SolanaTransaction{
				Transaction: base64Tx,
			},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to sign transaction. Error: %w", err)
	}

	tx.Signatures = append(tx.Signatures, solana.SignatureFromBytes(signResp.GetSolanaSignature().GetSignature()))
	return tx, nil
}

func (s *SignerManager) RemoteSignerToBindSignerFn(ctx context.Context, chainID string, signerWalletID string) bind.SignerFn {
	ctx = metadata.NewOutgoingContext(ctx, metadata.Pairs("authorization", "Bearer "+serviceAccountToken))

	return func(_ common.Address, tx *types.Transaction) (*types.Transaction, error) {
		txBytes, err := tx.MarshalBinary()
		if err != nil {
			return nil, fmt.Errorf("failed to marshal transaction: %w", err)
		}
		signResp, err := s.sc.Sign(ctx, &signerservice.SignRequest{
			WalletId: signerWalletID,
			Payload: &signerservice.SignRequest_EvmTransaction{
				EvmTransaction: &signerservice.EvmTransaction{
					ChainId: chainID,
					TxBytes: txBytes,
				},
			},
		})
		if err != nil {
			return nil, fmt.Errorf("failed to sign transaction. Error: %w", err)
		}

		chainIDInt, _ := new(big.Int).SetString(chainID, 10)
		signer := types.NewCancunSigner(chainIDInt)
		hash := signer.Hash(tx)

		sig := append(signResp.GetEvmSignature().GetR(), append(signResp.GetEvmSignature().GetS(),
			signResp.GetEvmSignature().GetV()...)...)

		_, err = crypto.Ecrecover(hash[:], sig)
		if err != nil {
			return nil, fmt.Errorf("failed to get public key from evm signature. Error: %w", err)
		}

		tx, err = tx.WithSignature(signer, sig)
		if err != nil {
			return nil, fmt.Errorf("failed to set signature on evm tx. Error: %w", err)
		}

		return tx, nil
	}
}

func (s *SignerManager) GetWallet(ctx context.Context, req *signerservice.GetWalletRequest) (*signerservice.GetWalletResponse, error) {
	ctx = metadata.NewOutgoingContext(ctx, metadata.Pairs("authorization", "Bearer "+serviceAccountToken))
	return s.sc.GetWallet(ctx, req)
}
