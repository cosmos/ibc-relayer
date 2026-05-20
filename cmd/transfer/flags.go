package main

import (
	"flag"
	"time"
)

// Shared flags consumed by both the `ics20` and `ift` subcommands.

var sourceChainID = flag.String(
	"source-chain-id",
	"",
	"the source chain id to make the transfer from",
)

var destChainID = flag.String(
	"dest-chain-id",
	"",
	"destination chain to send the transfer to",
)

var sourceClientID = flag.String(
	"source-client-id",
	"",
	"client id on the source chain to initiate the transfer from",
)

var receiver = flag.String(
	"receiver",
	"",
	"receiver of the transfer",
)

var amount = flag.Uint64(
	"amount",
	1,
	"amount of denom to transfer",
)

var timeout = flag.Duration(
	"timeout",
	12*time.Hour,
	"duration after which the packet times out on the destination chain",
)

var cfgPath = flag.String(
	"config",
	"./config/local/config.yml",
	"path to the config file to use",
)

var relayerGRPCURL = flag.String(
	"relayer-grpc-url",
	"",
	"url of the grpc endpoint for the relayer to relay this transfer",
)

var insecureDial = flag.Bool(
	"insecure",
	false,
	"dial the relayer gRPC endpoint over plaintext",
)

var privateKey = flag.String(
	"private-key",
	"",
	"private key of the wallet that will send the transfer",
)
