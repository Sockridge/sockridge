# sockridge — Python SDK

Python SDK for the [Sockridge](https://sockridge.com) agent registry.

## Install

```bash
pip install sockridge
```

Or pin to a specific version:

```bash
pip install sockridge==0.1.0
```

For local development:

```bash
git clone https://github.com/Sockridge/sockridge.git
pip install -e sockridge/sdk/python
```

## Prerequisites

You need a publisher account before using the SDK. Set one up with the CLI:

```bash
# install CLI
go install github.com/Sockridge/sockridge/cli@latest

# register
agentctl auth keygen
agentctl auth register --handle yourhandle --server http://sockridge.com:9000
agentctl auth login --server http://sockridge.com:9000
```

This creates `~/.agentctl/credentials.json` and `~/.agentctl/ed25519.key` which the SDK reads automatically.

---

## Usage

### Connect

```python
from sockridge import Registry

registry = Registry("http://sockridge.com:9000")
registry.login()  # reads ~/.agentctl/credentials.json
```

Custom credentials path:

```python
registry.login(
    credentials_path="/custom/path/credentials.json",
    key_path="/custom/path/ed25519.key",
)
```

---

### Publish an agent

```python
from sockridge import Registry, AgentCard, Skill, Capabilities

registry = Registry("http://sockridge.com:9000")
registry.login()

card = AgentCard(
    name="My FHIR Agent",
    description="Analyzes lab trends from FHIR data using ML models",
    url="https://my-agent.example.com",
    version="1.0.0",
    skills=[
        Skill(
            id="fhir.lab.analyze",
            name="Lab Analyzer",
            description="Detects anomalies in lab result trends",
            tags=["fhir", "labs", "analysis"],
        )
    ],
    capabilities=Capabilities(streaming=True, tool_use=True),
)

published = registry.publish(card)
print(f"id: {published.id}")
print(f"status: {published.status}")  # PENDING → ACTIVE after gatekeeper
```

The agent goes through automatic validation after publish:

- Fields are checked (name, description, skills required)
- URL is pinged to verify the agent is running
- AI scores the card quality (0.0 - 1.0)
- Score >= 0.4 → `AGENT_STATUS_ACTIVE`

---

### Self-register on startup

For agents that register themselves when they start:

```python
registry = Registry("http://sockridge.com:9000")
published = registry.register_and_publish(card)
print(f"registered: {published.id}")
```

---

### Search for agents

```python
# by tag
agents = registry.search(tags=["fhir", "labs"])
for agent in agents:
    print(f"{agent.name} — {agent.id}")

# by natural language
results = registry.semantic_search("find me a lab result analyzer")
for r in results:
    print(f"{r['score']:.2f}  {r['agent'].name}")

# by ID
agent = registry.get_agent("agent-uuid-here")
print(agent.name, agent.skills)
```

---

### Access agreements

Agents can only get each other's endpoint URLs after a mutual access agreement is approved by both publishers.

```python
# request access from another publisher
agreement = registry.request_access(
    receiver_id="other-publisher-uuid",
    message="want to connect our agents for a healthcare pipeline",
)
print(f"agreement id: {agreement['id']}")
print(f"status: {agreement['status']}")  # AGREEMENT_STATUS_PENDING

# once they approve, get the shared key
# (they share it with you out of band, or you retrieve via get_agreement)

# resolve an agent's endpoint URL
result = registry.resolve_endpoint(
    agent_id="agent-uuid",
    shared_key="sk_abc123...",
)
print(f"url: {result['url']}")
print(f"transport: {result['transport']}")
print(f"skills: {[s.name for s in result['agent'].skills]}")
```

---

### Full agreement flow

```python
# publisher A requests access to publisher B
agreement = registry_a.request_access(
    receiver_id=publisher_b_id,
    message="building a medical pipeline",
)

# publisher B approves (on their side)
shared_key = registry_b.approve_access(agreement["id"])
# shared_key = "sk_abc123..."

# both sides can now resolve each other's agents
endpoint_a = registry_b.resolve_endpoint(agent_a_id, shared_key)
endpoint_b = registry_a.resolve_endpoint(agent_b_id, shared_key)

# either side can revoke
registry_a.revoke_access(agreement["id"])
# key is instantly invalid
```

---

## API reference

### `Registry(server_url)`

| Method                                       | Description                                                |
| -------------------------------------------- | ---------------------------------------------------------- |
| `login(credentials_path?, key_path?)`        | Authenticate with Ed25519 challenge-response               |
| `publish(card)`                              | Publish an AgentCard, returns card with server-assigned id |
| `register_and_publish(card, ...)`            | Login + publish in one call                                |
| `search(tags?, limit?)`                      | List agents by tag. URL not included                       |
| `semantic_search(query, top_k?, min_score?)` | Natural language search                                    |
| `get_agent(agent_id)`                        | Get a single agent by ID                                   |
| `request_access(receiver_id, message?)`      | Send mutual access request                                 |
| `approve_access(agreement_id)`               | Approve a pending request, returns shared key              |
| `resolve_endpoint(agent_id, shared_key)`     | Resolve agent URL using shared key                         |

### `AgentCard`

| Field              | Type         | Required            |
| ------------------ | ------------ | ------------------- |
| `name`             | str          | ✓                   |
| `description`      | str          | ✓                   |
| `url`              | str          | ✓                   |
| `version`          | str          | defaults to `0.1.0` |
| `protocol_version` | str          | defaults to `0.3.0` |
| `skills`           | list[Skill]  | ✓ at least one      |
| `capabilities`     | Capabilities | optional            |

### `Skill`

| Field         | Type      | Required                    |
| ------------- | --------- | --------------------------- |
| `id`          | str       | ✓ (e.g. `fhir.lab.analyze`) |
| `name`        | str       | ✓                           |
| `description` | str       | ✓                           |
| `tags`        | list[str] | optional                    |

### `Capabilities`

| Field                | Type | Default |
| -------------------- | ---- | ------- |
| `streaming`          | bool | False   |
| `push_notifications` | bool | False   |
| `multi_turn`         | bool | False   |
| `tool_use`           | bool | False   |

---

## Requirements

- Python 3.11+
- `httpx[http2]` — HTTP/2 client (required, server speaks h2c)
- `cryptography` — Ed25519 signing
