package utils

import (
	"crypto/ecdsa"
	"encoding/hex"
	"io"
	"os"

	"github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	"github.com/ethereum/go-ethereum/crypto"
)

func LoadECDSAPrivateKey(privateKeyPath string) (*ecdsa.PrivateKey, error) {
	f, err := os.Open(privateKeyPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	privateKeyHexBytes, err := io.ReadAll(f)
	if err != nil {
		return nil, err
	}

	privateKey, err := crypto.HexToECDSA(string(privateKeyHexBytes))
	if err != nil {
		return nil, err
	}

	return privateKey, nil
}

func LoadSecp256k1PrivateKey(privateKeyPath string) (*secp256k1.PrivKey, error) {
	f, err := os.Open(privateKeyPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	privateKeyHexBytes, err := io.ReadAll(f)
	if err != nil {
		return nil, err
	}

	privateKeyBytes, err := hex.DecodeString(string(privateKeyHexBytes))
	if err != nil {
		return nil, err
	}

	privateKey := &secp256k1.PrivKey{}
	if err := privateKey.UnmarshalAmino(privateKeyBytes); err != nil {
		return nil, err
	}

	return privateKey, nil
}
