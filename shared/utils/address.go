package utils

import (
	"errors"

	"github.com/cosmos/cosmos-sdk/types/bech32"
	"github.com/ethereum/go-ethereum/common/hexutil"
)

func SliceTo32ByteArray(input []byte) ([32]byte, error) {
	var result [32]byte
	if len(input) > 32 {
		return result, errors.New("input slice is too large")
	}
	copy(result[32-len(input):], input)
	return result, nil
}

func HexTo32ByteAddress(input string) ([32]byte, error) {
	bytes, err := hexutil.Decode(input)
	if err != nil {
		return [32]byte{}, err
	}
	return SliceTo32ByteArray(bytes)
}

func MustBech32ToRawAddress(address string) string {
	_, rawAddress, err := bech32.DecodeAndConvert(address)
	if err != nil {
		panic(err)
	}
	return hexutil.Encode(rawAddress)
}
