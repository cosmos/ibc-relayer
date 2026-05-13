package signing

import "context"

type NopSigner struct{}

var _ Signer = (*NopSigner)(nil)

func NewNopSigner() *NopSigner {
	return &NopSigner{}
}

func (*NopSigner) Sign(ctx context.Context, chainID string, tx Transaction) (Transaction, error) {
	return tx, nil
}

func (*NopSigner) Address(ctx context.Context) []byte {
	return nil
}
