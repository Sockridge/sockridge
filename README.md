<h1 align="center">Sockridge</h1>

<p align="center">
  <strong>Agent discovery infrastructure.</strong> Publish your AI agent once. Any other agent can find it, request access, and connect, without you lifting a finger.
</p>

<p align="center">
  <a href="https://sockridge.com"><img src="https://img.shields.io/badge/Homepage-sockridge.com-39ff14?style=for-the-badge&labelColor=0a0a0a" /></a>
  <a href="https://sockridge.com:9000/healthz"><img src="https://img.shields.io/badge/API-Status-27c93f?style=for-the-badge&labelColor=0a0a0a" /></a>
  <a href="https://sockridge.com:9000/metrics"><img src="https://img.shields.io/badge/Metrics-Prometheus-ffbd2e?style=for-the-badge&labelColor=0a0a0a" /></a>
  <a href="https://github.com/Sockridge/sockridge/releases/latest"><img src="https://img.shields.io/github/v/release/Sockridge/sockridge?style=for-the-badge&labelColor=0a0a0a&color=7c4dff" /></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/License-MIT-blue?style=for-the-badge&labelColor=0a0a0a" /></a>
</p>

<p align="center">
  <img src="https://github.com/user-attachments/assets/b158a13e-0366-41d6-a6d1-d31c9120b9bd" alt="sock" />
</p>

---

## What is this?

Sockridge is a registry where AI agents publish themselves and discover each other. It's pure infrastructure, a phonebook for agents. The registry never sees agent-to-agent traffic. It just tells agents where other agents are.

```
Agent A searches registry → finds Agent B → requests access
Agent B owner approves → shared key issued
Agent A uses key → resolves Agent B's URL → calls Agent B directly
Registry never sees that call
```

## Stack

| Layer      | Tech                                                                  |
| ---------- | --------------------------------------------------------------------- |
| Server     | Go, ConnectRPC, Protobuf                                              |
| Storage    | ScyllaDB (agents), Redis (cache/rate limiting), pgvector (embeddings) |
| Embedder   | Python FastAPI + sentence-transformers (all-MiniLM-L6-v2)             |
| Auth       | Ed25519 challenge-response + JWT                                      |
| Gatekeeper | Anthropic Claude Haiku (Groq fallback)                                |

## Install CLI

**macOS / Linux:**

```bash
curl -fsSL https://sockridge.com/install.sh | sh
```

**Manual download:**

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

## Quick start

```bash
# register
sockridge auth keygen
sockridge auth register --handle yourhandle --server http://sockridge.com:9000
sockridge auth login --server http://sockridge.com:9000

# publish your agent
sockridge publish --file agent.json

# search
sockridge search list
sockridge search semantic "find a lab analyzer"
sockridge search get <agent-id>
sockridge search mine <agent-id>   # your own agent, shows URL
```

Example `agent.json`:

```json
{
  "name": "My Agent",
  "description": "Does something useful for other agents",
  "version": "1.0.0",
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
  "capabilities": { "streaming": true }
}
```

## Access agreements

```bash
# request mutual access with another publisher
sockridge access request --to <publisher-id> --message "building a pipeline"

# list incoming requests
sockridge access pending

# approve — generates shared key
sockridge access approve --id <agreement-id>

# set expiry (optional)
sockridge access set-expiry --id <agreement-id> --days 30

# resolve agent endpoint using shared key
sockridge access resolve --agent <agent-id> --key sk_...

# revoke
sockridge access revoke --id <agreement-id>
```

## Webhooks

```bash
# register a webhook
sockridge webhook register \
  --url https://myserver.com/hooks \
  --event access_request \
  --event agent_active

# list webhooks
sockridge webhook list

# test delivery
sockridge webhook test --id <webhook-id>

# delete
sockridge webhook delete --id <webhook-id>
```

Available events: `access_request`, `access_approved`, `access_denied`, `access_revoked`, `agent_active`, `agent_inactive`, `agent_published`, `agent_rejected`

Webhooks are signed with HMAC-SHA256. Verify with the `X-Sockridge-Signature` header.

## Audit log

```bash
sockridge audit list
sockridge audit list --limit 100
```

## SDKs

| Language   | Install                                        |
| ---------- | ---------------------------------------------- |
| Python     | `pip install sockridge`                        |
| TypeScript | `npm install @sockridge/sdk`                   |
| Go         | `go get github.com/Sockridge/sockridge/sdk/go` |

```python
from sockridge import Registry, AgentCard, Skill

registry = Registry("http://sockridge.com:9000")
registry.login()

published = registry.publish(AgentCard(
    name="My Agent",
    description="Does something useful",
    url="https://my-agent.example.com",
    skills=[Skill(id="do.thing", name="Do Thing", description="Does the thing", tags=["thing"])]
))
```

## How agents get approved

Every published agent goes through the gatekeeper pipeline automatically:

```
publish → PENDING
  → validate fields (name, description, skills required)
  → ping URL (is the agent actually running?)
  → GET /.well-known/agent.json (A2A compliance check)
  → verify name + skills match the submitted card
  → AI scores the card (0.0 - 1.0)
  → score >= 0.4 → ACTIVE
  → score < 0.4  → REJECTED
```

Your agent must expose `/.well-known/agent.json` returning a valid AgentCard JSON.

## Rate limits

| Operation | Limit                 |
| --------- | --------------------- |
| publish   | 10/hour per publisher |
| search    | 100/min per IP        |
| resolve   | 50/min per shared key |
| login     | 10/min per publisher  |

## Metrics

```bash
curl http://sockridge.com:9000/metrics
```

Prometheus-compatible output including agent counts by status, request totals, rate limit hits, and uptime.

## Self-hosting

```bash
git clone https://github.com/Sockridge/sockridge.git
cd sockridge
cp .env.example .env  # edit with your keys
docker compose up -d --build
```

See `RUNBOOK.md` for full production deployment guide.

## License

MIT
