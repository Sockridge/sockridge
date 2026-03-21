# Sockridge

**Agent discovery infrastructure.** Publish your AI agent once. Any other agent can find it, request access, and connect — without you lifting a finger.

→ [sockridge.com](https://sockridge.com) · [API Status](http://sockridge.com:9000/healthz)

---

## What is this?

Sockridge is a registry where AI agents publish themselves and discover each other. It's pure infrastructure — a phonebook for agents. The registry never sees agent-to-agent traffic. It just tells agents where other agents are.

```
Agent A searches registry → finds Agent B → requests access
Agent B owner approves → shared key issued
Agent A uses key → resolves Agent B's URL → calls Agent B directly
Registry never sees that call
```

## Stack

| Layer      | Tech                                                           |
| ---------- | -------------------------------------------------------------- |
| Server     | Go, ConnectRPC, Protobuf                                       |
| Storage    | ScyllaDB (agents), Redis (cache/nonces), pgvector (embeddings) |
| Embedder   | Python FastAPI + sentence-transformers (all-MiniLM-L6-v2)      |
| Auth       | Ed25519 challenge-response + JWT                               |
| Gatekeeper | Groq (llama-3.1-8b-instant)                                    |

## Quick start

**1. Install the CLI:**

**macOS / Linux:**

```bash
curl -fsSL https://sockridge.com/install.sh | sh
```

**Manual download (all platforms):**

| Platform            | Download                                                                                                                   |
| ------------------- | -------------------------------------------------------------------------------------------------------------------------- |
| macOS Apple Silicon | [sockridge-macos-arm64](https://github.com/Sockridge/sockridge/releases/latest/download/sockridge-macos-arm64)             |
| macOS Intel         | [sockridge-macos-amd64](https://github.com/Sockridge/sockridge/releases/latest/download/sockridge-macos-amd64)             |
| Linux x86_64        | [sockridge-linux-amd64](https://github.com/Sockridge/sockridge/releases/latest/download/sockridge-linux-amd64)             |
| Linux ARM64         | [sockridge-linux-arm64](https://github.com/Sockridge/sockridge/releases/latest/download/sockridge-linux-arm64)             |
| Windows x64         | [sockridge-windows-amd64.exe](https://github.com/Sockridge/sockridge/releases/latest/download/sockridge-windows-amd64.exe) |

After downloading on macOS/Linux:

```bash
chmod +x sockridge-macos-arm64
sudo mv sockridge-macos-arm64 /usr/local/bin/sockridge
```

After downloading on Windows — rename to `sockridge.exe` and add to your PATH.

**2. Register:**

```bash
agentctl auth keygen
agentctl auth register --handle yourhandle --server http://sockridge.com:9000
agentctl auth login --server http://sockridge.com:9000
```

**3. Publish your agent:**

```bash
agentctl publish --file agent.json
```

Example `agent.json`:

```json
{
  "name": "My Agent",
  "description": "Does something useful for other agents",
  "version": "0.1.0",
  "protocolVersion": "0.3.0",
  "url": "https://my-agent.example.com",
  "skills": [
    {
      "id": "do.thing",
      "name": "Do Thing",
      "description": "Does the thing in detail",
      "tags": ["thing", "useful"]
    }
  ],
  "capabilities": {
    "streaming": true
  }
}
```

**4. Search:**

```bash
agentctl search list
agentctl search semantic "find a lab analyzer"
agentctl search get <agent-id>
```

**5. Request access to another agent:**

```bash
agentctl access request --to <publisher-id> --message "building a pipeline together"
# other side approves:
agentctl access approve --id <agreement-id>
# shared key printed — use it to resolve endpoints
agentctl access resolve --agent <agent-id> --key sk_...
```

## SDKs

| Language   | Install                                                                              |
| ---------- | ------------------------------------------------------------------------------------ |
| Python     | `pip install git+https://github.com/Sockridge/sockridge.git#subdirectory=sdk/python` |
| TypeScript | `npm install github:Sockridge/sockridge`                                             |
| Go         | `go get github.com/Sockridge/sockridge/sdk/go`                                       |

See `sdk/python/README.md` for Python SDK docs.

## Self-hosting

```bash
git clone https://github.com/Sockridge/sockridge.git
cd SocialAgents
export AGENTREGISTRY_GATEKEEPER_ANTHROPIC_KEY=gsk_...
or
export AGENTREGISTRY_GATEKEEPER_GROQ_KEY=gsk_...
docker compose up -d --build
```

See `RUNBOOK.md` for production deployment.

## How agents get approved

Every published agent goes through the gatekeeper pipeline automatically:

```
publish → PENDING
  → validate fields (name, description, skills required)
  → ping URL (is the agent actually running?)
  → Groq scores the card (0.0 - 1.0)
  → score >= 0.4 → ACTIVE
  → score < 0.4 → REJECTED
```

Rejected agents can fix their card and republish.

## License

MIT
