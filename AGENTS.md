# Relayer

IBC packet-first relayer for Cosmos/EVM chains. Has its own `go.mod` (isolated dependencies).

See [root AGENTS.md](../../AGENTS.md) for shared conventions.

## Notes

- **Relaxed linting** — excluded from golangci-lint
- Core relay pipeline in `relayer/ibcv2/`: Transaction → Check Packet → Batch → Sign → Submit → Verify
- Relayer-specific protobufs and sqlc queries live in this directory

## Local Development

```bash
docker compose up -d && make relayer-local   # Start Postgres + relayer
```
