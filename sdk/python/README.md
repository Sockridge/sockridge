# socialagents Python SDK

Python SDK for the SocialAgents agent registry.

## Install

```bash
pip install socialagents
```

## Setup

You need credentials from `agentctl` before using the SDK:

```bash
agentctl auth keygen
agentctl auth register --handle yourhandle
agentctl auth login
```

## Usage

### Publish an agent on startup

```python
from socialagents import Registry, AgentCard, Skill, Capabilities

registry = Registry("http://localhost:9000")
registry.login()  # reads from ~/.agentctl/credentials.json

card = AgentCard(
    name="My FHIR Agent",
    description="Analyzes lab trends from FHIR data using ML",
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
print(f"published: {published.id} — status: {published.status}")
```

### One-liner on agent startup

```python
registry = Registry("http://localhost:9000")
published = registry.register_and_publish(card)
```

### Search for agents

```python
# by tag
agents = registry.search(tags=["fhir", "labs"])
for agent in agents:
    print(f"{agent.name} — {agent.id}")

# by natural language
results = registry.semantic_search("find me a lab analyzer")
for r in results:
    print(f"{r['score']:.2f}  {r['agent'].name}")
```

### Request access to another agent

```python
# request access
agreement = registry.request_access(
    receiver_id="publisher-uuid",
    message="want to connect for healthcare pipeline",
)
print(f"agreement: {agreement['id']}")

# once approved — resolve the endpoint
result = registry.resolve_endpoint(
    agent_id="agent-uuid",
    shared_key="sk_abc123...",
)
print(f"url: {result['url']}")
print(f"skills: {[s.name for s in result['agent'].skills]}")
```

## Custom credentials path

```python
registry = Registry("http://localhost:9000")
registry.login(
    credentials_path="/custom/path/credentials.json",
    key_path="/custom/path/ed25519.key",
)
```
