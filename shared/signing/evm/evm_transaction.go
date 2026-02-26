package evm

import "github.com/ethereum/go-ethereum/core/types"

type EVMTransaction struct {
	raw *types.Transaction
}

func (tx *EVMTransaction) Raw() interface{} {
	return tx.raw
}

func (tx *EVMTransaction) Bytes() ([]byte, error) {
	return tx.raw.MarshalBinary()
}

func (tx *EVMTransaction) Marshal() ([]byte, error) {
	return tx.raw.MarshalBinary()
}
