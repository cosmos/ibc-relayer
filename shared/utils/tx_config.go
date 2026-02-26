package utils

import (
	evidencetypes "cosmossdk.io/x/evidence/types"
	feegrant "cosmossdk.io/x/feegrant"
	upgradetypes "cosmossdk.io/x/upgrade/types"
	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/codec"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	secp256r1 "github.com/cosmos/cosmos-sdk/crypto/keys/secp256r1"
	std "github.com/cosmos/cosmos-sdk/std"
	authtx "github.com/cosmos/cosmos-sdk/x/auth/tx"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	vestingtypes "github.com/cosmos/cosmos-sdk/x/auth/vesting/types"
	authz "github.com/cosmos/cosmos-sdk/x/authz"
	// banklegacyv040 "github.com/cosmos/cosmos-sdk/x/bank/legacy/v040"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	distributiontypes "github.com/cosmos/cosmos-sdk/x/distribution/types"
	proposal "github.com/cosmos/cosmos-sdk/x/params/types/proposal"
	slashingtypes "github.com/cosmos/cosmos-sdk/x/slashing/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
)

func RegisterAllInterfaces(registry codectypes.InterfaceRegistry) {
	// Register interfaces for all Cosmos SDK types
	std.RegisterInterfaces(registry)
	secp256r1.RegisterInterfaces(registry)

	// Register interfaces for all Cosmos SDK module types
	authtypes.RegisterInterfaces(registry)
	vestingtypes.RegisterInterfaces(registry)
	authz.RegisterInterfaces(registry)
	// banklegacyv040.RegisterInterfaces(registry)
	banktypes.RegisterInterfaces(registry)
	distributiontypes.RegisterInterfaces(registry)
	evidencetypes.RegisterInterfaces(registry)
	feegrant.RegisterInterfaces(registry)
	proposal.RegisterInterfaces(registry)
	slashingtypes.RegisterInterfaces(registry)
	stakingtypes.RegisterInterfaces(registry)
	upgradetypes.RegisterInterfaces(registry)
}

func DefaultCodec() *codec.ProtoCodec {
	registry := codectypes.NewInterfaceRegistry()
	RegisterAllInterfaces(registry)
	return codec.NewProtoCodec(registry)
}

func DefaultTxConfig() client.TxConfig {
	cdc := DefaultCodec()
	return authtx.NewTxConfig(cdc, authtx.DefaultSignModes)
}
