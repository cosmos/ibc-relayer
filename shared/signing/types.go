package signing

import (
	"context"

	"github.com/cosmos/cosmos-sdk/client"
	sdk "github.com/cosmos/cosmos-sdk/types"
)

type Signer interface {
	Sign(ctx context.Context, chainID string, tx Transaction) (Transaction, error)
	Address(ctx context.Context) []byte
}

type Transaction interface {
	MarshalBinary() ([]byte, error)
	UnmarshalBinary([]byte) error

	MarshalJSON() ([]byte, error)
	UnmarshalJSON([]byte) error
}

type CosmosTransaction struct {
	Tx            sdk.Tx
	AccountNumber uint64
	Sequence      uint64
	TxConfig      client.TxConfig
}

func NewCosmosTransaction(tx sdk.Tx, accountNumber, sequence uint64, txConfig client.TxConfig) *CosmosTransaction {
	return &CosmosTransaction{
		Tx:            tx,
		AccountNumber: accountNumber,
		Sequence:      sequence,
		TxConfig:      txConfig,
	}
}

func (t *CosmosTransaction) MarshalBinary() ([]byte, error) {
	return t.TxConfig.TxEncoder()(t.Tx)
}

func (t *CosmosTransaction) UnmarshalBinary(data []byte) error {
	var err error
	t.Tx, err = t.TxConfig.TxDecoder()(data)
	return err
}

func (t *CosmosTransaction) MarshalJSON() ([]byte, error) {
	return t.TxConfig.TxJSONEncoder()(t.Tx)
}

func (t *CosmosTransaction) UnmarshalJSON(data []byte) error {
	var err error
	t.Tx, err = t.TxConfig.TxJSONDecoder()(data)
	return err
}
