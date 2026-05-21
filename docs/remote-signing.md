# Remote signing

For production deployments, the relayer delegates signing to an external gRPC service rather than holding key material on the host. This keeps private keys off the relayer process — typically in an HSM, KMS, or dedicated signing daemon — and lets the operator manage keys without redeploying the relayer.

The relayer is configured for remote signing by setting `signing.grpc_address` in its YAML config; see the [README's Signing section](../README.md#signing) for the quick-start.

## Configuration

| Field                 | Required             | Description                                                                                                     |
|-----------------------|----------------------|-----------------------------------------------------------------------------------------------------------------|
| `grpc_address`        | yes (remote mode)    | gRPC URL of the remote signer service. Setting this enables remote signing; ignores `keys_path`.                |
| `grpc_tls_enabled`    | no                   | Use TLS 1.3 on the signer connection.                                                                           |
| `cosmos_wallet_key`   | only if any chain has `type: cosmos` | Wallet ID on the signer service to use for Cosmos chain signing. Sent in every `SignRequest` for Cosmos chains. |
| `evm_wallet_key`      | only if any chain has `type: evm`    | Wallet ID on the signer service to use for EVM chain signing.                                                   |

## Authentication

The relayer reads the `SERVICE_ACCOUNT_TOKEN` environment variable **once at process startup** (`shared/config/config.go:568-570`) and attaches it as `authorization: Bearer <token>` on every outgoing gRPC request.

## Startup health check

When configured for remote signing, the relayer probes the signer at startup before serving traffic (`relayer/ibcv2/client_manager.go`):

- For every chain type configured (`cosmos`, `evm`), the relayer calls `GetWallet` against the matching wallet ID with the matching `PubKeyType` (`PubKeyType_Cosmos`, `PubKeyType_Ethereum`).
- Any failure (network error, missing wallet, signer down) aborts startup with `remote signer health check failed for wallet <id>: <error>`.

## Signer service interface

The proto lives at [`proto/signer/signerservice.proto`](../proto/signer/signerservice.proto). A conforming service must implement:

```proto
service SignerService {
    rpc GetWallet (GetWalletRequest) returns (GetWalletResponse) {}
    rpc Sign      (SignRequest)      returns (SignResponse) {}
}
```

### What the relayer calls

The relayer invokes **`Sign` and `GetWallet`** only:

- `Sign` is called for every transaction the relayer submits to a chain.
- `GetWallet`.

### Per-chain signing contracts

The `SignRequest.payload` is a `oneof` over five payload types; the relayer uses three:

#### EVM (`EvmTransaction`)

The relayer:

1. Builds the typed Ethereum transaction (e.g. dynamic-fee EIP-1559, Cancun signer).
2. RLP-marshals it via `tx.MarshalBinary()` to `tx_bytes`.
3. Sends `SignRequest { wallet_id, evm_transaction: EvmTransaction { chain_id, tx_bytes } }` (`shared/signing/signer_service/signer_manager.go:127-135`).

The signer is expected to:

- Decode the RLP-encoded transaction.
- Compute the canonical Ethereum sign hash (the **Cancun signer** scheme, identical to London/Paris: `keccak256` of the RLP-encoded transaction with the chain ID).
- Produce an ECDSA secp256k1 signature over that 32-byte hash.
- Return `EvmTransactionSignature { r: 32 bytes BE, s: 32 bytes BE, v: 1 byte }`.

The relayer concatenates `r || s || v` (65 bytes) and runs `crypto.Ecrecover(hash, sig)` to verify the signature is cryptographically well-formed (`signer_manager.go:144-150`).

#### Cosmos (`CosmosTransaction`)

The relayer:

1. Builds the Cosmos SDK transaction.
2. Generates the canonical `SignDoc` bytes via `authsigning.GetSignBytesAdapter` with `SIGN_MODE_DIRECT`.
3. Sends `SignRequest { wallet_id, cosmos_transaction: CosmosTransaction { sign_doc_bytes } }` (`signer_manager.go:54-61`).

The signer is expected to:

- SHA-256 hash the `sign_doc_bytes` (the standard Cosmos `SIGN_MODE_DIRECT` hash).
- Produce an ECDSA secp256k1 signature over that 32-byte hash.
- Return `CosmosTransactionSignature { signature: 64 bytes, raw r || s, no recovery byte }`.

The relayer also calls `GetWallet(wallet_id, PubKeyType_Cosmos)` to fetch the pubkey and embeds it (alongside the 64-byte signature) in a `SignatureV2` on the transaction. The pubkey must match the key that produced the signature, or the Cosmos chain will reject the tx.

### `GetWallet` contract

For `GetWallet(GetWalletRequest { id, pubkey_type })`, the relayer reads only the `pubkey` field of the response. Field expectations:

| Field    | Per `pubkey_type = Cosmos`           | Per `pubkey_type = Ethereum`         |
|----------|--------------------------------------|--------------------------------------|
| `id`     | Must match the request `id`          | Must match the request `id`          |
| `pubkey` | secp256k1 compressed (33 bytes)      | secp256k1 uncompressed (65 bytes)    |


**Invariant:** the address derivable from `pubkey` must match the address that signatures from `Sign` for the same `wallet_id` recover to. If `GetWallet` returns pubkey A and `Sign` uses key B, on-chain verification will fail — see Common pitfalls.
