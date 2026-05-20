# Manually Sending and Relaying IBC v2 Transfers

## Installation

From the relayer repo root (`apps/relayer` in this monorepo):

```bash
make transfer
```

This builds `bin/transfer`.

## Usage

Two transfer types are supported, both over IBC v2:

- `ics20` — ERC20-denominated ICS20 transfers (EVM source, EVM or Cosmos dest)
- `ift` — IFT (interchain fungible token) transfers (EVM source, EVM or Cosmos dest)

The transfer type is a positional argument parsed after `flag.Parse()`, so it must come after all flags. Use `transfer [flags] <ics20|ift>`, not `transfer <ics20|ift> [flags]`.

### Common flags

Required:
- `--source-chain-id`
- `--dest-chain-id`
- `--source-client-id`
- `--receiver` — bech32 for Cosmos dest, hex address for EVM dest
- `--private-key`
- `--relayer-grpc-url`

Optional:
- `--amount` defaults to `1`
- `--timeout` packet timeout duration on the destination chain, defaults to `12h`
- `--config` defaults to `./config/local/config.yml`
- `--insecure` dial the relayer over plaintext

### ics20-only flags

- `--ics20-address` — ICS20Transfer contract address on the source chain (required)
- `--denom` — ERC20 contract address on the source chain (required)
- `--memo` — optional memo string

### ift-only flags

- `--ift-address` — IFT token contract address on the source chain (required)

### Examples

ICS20 from eth-sepolia to wfchain:

```bash
./bin/transfer \
  --source-chain-id 11155111 \
  --dest-chain-id wfchain-1 \
  --source-client-id wfchain-2 \
  --ics20-address 0xb143eC94eA375D78773F26F1C3fA8A4354Fa6E13 \
  --denom 0x1c7D4B196Cb0C7B01d743Fbc6116a902379C7238 \
  --receiver wf13q4m2jw293d7t8uvn6hhgsnnta427ychrqh6hx \
  --amount 1 \
  --private-key <hex-private-key> \
  --relayer-grpc-url localhost:9000 \
  --insecure \
  ics20
```

IFT from eth-sepolia to base-sepolia:

```bash
./bin/transfer \
  --source-chain-id 11155111 \
  --dest-chain-id 84532 \
  --source-client-id base-attestations-1 \
  --ift-address 0xA5D1b01b31474C653Ef1A03F258F0607CD938a5d \
  --receiver 0x810587fad19A9EF79AA661C0F7C49c92A3A2eFE4 \
  --amount 1 \
  --private-key <hex-private-key> \
  --relayer-grpc-url localhost:9000 \
  --insecure \
  ift
```
