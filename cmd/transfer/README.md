# Manually Sending+Relaying IBC v2 & CCTP Transfers

## Installation

Install the command with the following while in the root of the relayer repository:

- Note you should have `make` installed.

```bash
make transfer
```

This builds the `transfer` binary and places it at `bin/transfer`. You can
choose to add the `bin` directory to your `$PATH` to make the usage simpler.

To add to `bin` to your path for your current session, use:

```bash
export PATH=$PATH:$(pwd)/bin
```

The following usage will assume that the `transfer` binary is in your `$PATH`.

## Usage

### IBC v2 Transfers

The following flags are required:

- `--source-chain-id`: (REQUIRED) The chain id to send the transfer from (only `1` is currently supported).
- `--dest-chain-id`: (REQUIRED) The chain id to send the transfer to (only `cosmoshub-4` is currently supported).
- `--source-client-id`: (REQUIRED) The client id to send the transfer from.
- `--receiver`: (REQUIRED) The address of the receiver of the transfer on the destination chain.
- `--denom`: (REQUIRED) The denom to send. The wallet with private key set via `--private-key` must have >= `amount` of this `denom`.
- `--amount`: (OPTIONAL defaults to `1`) The amount of `--denom` to send.
- `--private-key`: (REQUIRED) The private key of the wallet to send from.
- `--memo`: (OPTIONAL defaults to "") The memo to attach to the transfer on the source chain.
- `--cfg-path`: (OPTIONAL defaults to `./config/local/config.yml`) The path to the config file to use.
- `--relayer-grpc-url`: (OPTIONAL defaults to `relayer-grpc.dev.skip-internal.money:443`) The URL of the relayer to relay this transfer.

A command following the above flags is required (**the command must be specified after the flags**):

- `ibcv2`: Send the transfer over the `ibcv2` bridge.

#### Examples

1. Send + Relay an IBC v2 Transfer of ATOM From Ethereum to Cosmos Hub

```bash
transfer --source-chain-id 1 --dest-chain-id cosmoshub-4 --source-client-id cosmoshub-0 --denom 0xbf6Bc6782f7EB580312CC09B976e9329f3e027B3 --amount 1 --memo "" --receiver cosmos1vu55xd53m64932dzqffz29wekmrsr7tt77j2vv ibcv2
```
