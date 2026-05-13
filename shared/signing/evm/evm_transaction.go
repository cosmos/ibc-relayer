package evm

import "github.com/ethereum/go-ethereum/core/types"

//nolint:revive // EVMTransaction is the canonical name across the codebase
type EVMTransaction struct {
	raw *types.Transaction
}

func (tx *EVMTransaction) Raw() any {
	return tx.raw
}

func (tx *EVMTransaction) Bytes() ([]byte, error) {
	return tx.raw.MarshalBinary()
}

func (tx *EVMTransaction) Marshal() ([]byte, error) {
	return tx.raw.MarshalBinary()
}
