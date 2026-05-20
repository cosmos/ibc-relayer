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
- `--relayer-grpc-url` required, the gRPC endpoint of a running relayer
- `--insecure` optional, dial over plaintext

Example:

```bash
./bin/relay --source-chain-id 1 --tx-hash 0xdeadbeef --relayer-grpc-url relayer.example.com:443
```
