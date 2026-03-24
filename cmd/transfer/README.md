# Manually Sending and Relaying IBC v2 Transfers

## Installation

From the relayer repo root (`apps/relayer` in this monorepo):

```bash
make transfer
```

This builds `bin/transfer`.

## Usage

The `ibcv2` subcommand sends a transfer and then submits the resulting tx hash to the relayer API.

The bridge type is a positional argument parsed after `flag.Parse()`, so it must come after all flags. Use `transfer [flags] ibcv2`, not `transfer ibcv2 [flags]`.

Required flags:
- `--source-chain-id`
- `--dest-chain-id`
- `--source-client-id`
- `--receiver`
- `--denom`
- `--private-key`

Optional flags:
- `--amount` defaults to `1`
- `--memo` defaults to empty string
- `--config` defaults to `./config/local/config.yml`
- `--relayer-grpc-url` defaults to `relayer-grpc.dev.skip-internal.money:443`

Example:

```bash
./bin/transfer \
  --source-chain-id 1 \
  --dest-chain-id cosmoshub-4 \
  --source-client-id cosmoshub-0 \
  --receiver cosmos1vu55xd53m64932dzqffz29wekmrsr7tt77j2vv \
  --denom 0xbf6Bc6782f7EB580312CC09B976e9329f3e027B3 \
  --amount 1 \
  --private-key <hex-private-key> \
  ibcv2
```
