# Manually Relaying Existing IBC v2 Transfers

## Installation

From the relayer repo root (`apps/relayer` in this monorepo):

```bash
make relay
```

This builds `bin/relay`.

## Usage

Supported flags:
- `--source-chain-id` required
- `--tx-hash` required
- `--relayer-grpc-url` optional, defaults to `relayer-grpc.dev.skip-internal.money:443`

Example:

```bash
./bin/relay --source-chain-id 1 --tx-hash 0xdeadbeef
```
