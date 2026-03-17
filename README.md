# agentregistry — proto schema

The `.proto` files in this directory are the **single source of truth** for the entire registry.
All server code, CLI clients, and SDKs are generated from these.

## Files

| File | Purpose |
|---|---|
| `v1/agent.proto` | Core data models: `AgentCard`, `Skill`, `PublisherAccount`, `SignedPayload` |
| `v1/registry.proto` | RPC service definitions: `RegistryService` + `DiscoveryService` |

## Key design decisions

### Why Ed25519 + challenge-response auth?
Agents are machines. Password-based auth is wrong for this use case.
- Each publisher generates an Ed25519 keypair locally (`agentctl auth keygen`)
- Public key is registered once with the server
- Every mutating RPC (`PublishAgent`, `UpdateAgent`) wraps the payload in a `SignedPayload`
- The server verifies the Ed25519 signature before writing anything
- Session tokens (JWT) are short-lived (1h) — obtained via challenge-response to prevent replay attacks

### Why `SignedPayload` wrapper?
Instead of signing at the transport layer (mTLS), we sign at the payload level.
This means the signature is stored alongside the agent record — you can verify
who published an agent even after the fact, from the DB record alone.

### Why two services?
`RegistryService` — write path, requires auth session token in gRPC metadata.
`DiscoveryService` — read path, intentionally unauthenticated for basic discovery.
Agents should be able to find each other without needing credentials.
`SemanticSearch` and `Watch` are the power features — no auth needed to consume.

### The `Watch` RPC
Long-lived server-streaming RPC. An agent connects once and receives a stream
of `WatchEvent` messages as other agents are published/updated/deprecated.
This is the key advantage of gRPC over REST polling — zero overhead discovery.

### `embedding` field on `Skill`
Populated server-side, never by the publisher.
When an agent is published, the server embeds each skill description using
a small embedding model and stores it in pgvector.
`SemanticSearch` then does ANN (approximate nearest neighbor) lookup.

## Generating code

```bash
# Install buf
brew install bufbuild/buf/buf

# Generate Go + TypeScript from proto
buf generate

# Lint proto files
buf lint

# Check for breaking changes against last commit
buf breaking --against '.git#branch=main'
```

## Auth flow (CLI)

```
agentctl auth keygen
  → generates ~/.agentctl/keys/ed25519.key (private, chmod 600)
  → generates ~/.agentctl/keys/ed25519.pub (public)

agentctl auth register --handle utsav
  → calls RegisterPublisher(handle, public_key)
  → saves publisher_id to ~/.agentctl/credentials

agentctl publish ./my-agent.proto
  → calls AuthChallenge(publisher_id) → gets nonce
  → signs nonce with private key
  → calls AuthVerify(publisher_id, nonce, sig) → gets session_token
  → wraps AgentCard bytes in SignedPayload
  → calls PublishAgent(signed_payload) with session_token in gRPC metadata
```
