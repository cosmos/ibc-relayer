#!/bin/bash

set -e

mkdir -p ./shared/contracts/usdc
abigen --abi ./shared/abi/usdc.json --pkg usdc --out ./shared/contracts/usdc/usdc.go

mkdir -p ./shared/contracts/token_messenger
abigen --abi ./shared/abi/token_messenger.json --pkg token_messenger --out ./shared/contracts/token_messenger/token_messenger.go

mkdir -p ./shared/contracts/message_transmitter
abigen --abi ./shared/abi/message_transmitter.json --pkg message_transmitter --out ./shared/contracts/message_transmitter/message_transmitter.go

mkdir -p ./shared/contracts/cctp_relayer
abigen --abi ./shared/abi/cctp_relayer.json --pkg cctp_relayer --out ./shared/contracts/cctp_relayer/cctp_relayer.go

mkdir -p ./shared/contracts/gas_price_oracle
abigen --abi ./shared/abi/gas_price_oracle.json --pkg gas_price_oracle --out ./shared/contracts/gas_price_oracle/gas_price_oracle.go

