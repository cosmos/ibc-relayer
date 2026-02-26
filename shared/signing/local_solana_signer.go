package signing

import (
	"context"
	"fmt"

	"github.com/gagliardetto/solana-go"
)

type LocalSolanaSigner struct {
	privateKey solana.PrivateKey
}

var _ Signer = (*LocalSolanaSigner)(nil)

func NewLocalSolanaSigner(privateKeyBase58 string) (*LocalSolanaSigner, error) {
	privateKey, err := solana.PrivateKeyFromBase58(privateKeyBase58)
	if err != nil {
		return nil, fmt.Errorf("failed to parse private key: %w", err)
	}

	return &LocalSolanaSigner{
		privateKey: privateKey,
	}, nil
}

// Not used for Solana signing
func (s *LocalSolanaSigner) Sign(ctx context.Context, chainID string, tx Transaction) (Transaction, error) {
	return nil, nil
}

func (s *LocalSolanaSigner) GetPrivateKey() *solana.PrivateKey {
	return &s.privateKey
}

func (s *LocalSolanaSigner) Address(ctx context.Context) []byte {
	return s.privateKey.PublicKey().Bytes()
}
