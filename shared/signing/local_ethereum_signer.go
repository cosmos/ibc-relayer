package signing

import (
	"context"
	"crypto/ecdsa"
	"errors"
	"math/big"

	ethereum "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
)

type LocalEthereumSigner struct {
	privateKey *ecdsa.PrivateKey
}

var _ Signer = (*LocalEthereumSigner)(nil)

func NewLocalEthereumSigner(privateKey *ecdsa.PrivateKey) *LocalEthereumSigner {
	return &LocalEthereumSigner{
		privateKey: privateKey,
	}
}

func (s *LocalEthereumSigner) Sign(ctx context.Context, chainID string, tx Transaction) (Transaction, error) {
	ethereumTx, ok := tx.(*ethereum.Transaction)
	if !ok {
		return nil, errors.New("unsupported transaction type")
	}

	intChainID, ok := new(big.Int).SetString(chainID, 10)
	if !ok {
		return nil, errors.New("invalid chain ID")
	}

	signedTx, err := ethereum.SignTx(ethereumTx, ethereum.NewCancunSigner(intChainID), s.privateKey)
	if err != nil {
		return nil, err
	}

	return signedTx, nil
}

func (s *LocalEthereumSigner) Address(ctx context.Context) []byte {
	return crypto.PubkeyToAddress(s.privateKey.PublicKey).Bytes()
}
