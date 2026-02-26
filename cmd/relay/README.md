# Manually Relaying Existing IBC v2 & CCTP Transfers

## Installation

Install the command with the following while in the root of the relayer repository:

- Note you should have `make` installed.

```bash
make relay
```

This builds the `relay` binary and places it at `bin/relay`. You can
choose to add the `bin` directory to your `$PATH` to make the usage simpler.

To add to `bin` to your path for your current session, use:

```bash
export PATH=$PATH:$(pwd)/bin
```

The following usage will assume that the `relay` binary is in your `$PATH`.

## Usage

### IBC v2 Transfers

The following flags are required:

- `--source-chain-id`: (REQUIRED) The chain id to send the transfer from (only `1` is currently supported).
- `--dest-chain-id`: (REQUIRED) The chain id to send the transfer to (only `cosmoshub-4` is currently supported).
- `--bridge`: (REQUIRED) The bridge type to send the tx over (only `ibcv2` is supported).
- `--tx-hash`: (REQUIRED) The transaction hash to relay.
- `--cfg-path`: (OPTIONAL defaults to `./config/local/config.yml`) The path to the config file to use.
- `--relayer-grpc-url`: (OPTIONAL defaults to `relayer-grpc.dev.skip-internal.money:443`) The URL of the relayer to relay this transfer.

#### Examples

1. Relay and existing transaction over `ibcv2`.

```bash
relay --source-chain-id 1 --dest-chain-id cosmoshub-4 --tx-hash 0xdeadbeef --bridge ibcv2
```
