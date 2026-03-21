import base64
import json
import time
import httpx
import struct
from typing import Optional

from .models import AgentCard, AgentStatus, GatekeeperResult, Skill, Capabilities
from .auth import KeyPair, Credentials


class RegistryError(Exception):
    pass


class Registry:
    """
    SockRidge registry client.

    Usage:
        from sockridge import Registry, AgentCard, Skill, Capabilities

        registry = Registry("https://sockridge.com:9000")
        registry.login(credentials_path="~/.sockridge/credentials.json")

        card = AgentCard(
            name="My Agent",
            description="Does something useful",
            url="https://my-agent.example.com",
            skills=[Skill(id="do.thing", name="Do Thing", description="Does the thing", tags=["thing"])],
            capabilities=Capabilities(streaming=True),
        )

        published = registry.publish(card)
        print(published.id)
    """

    def __init__(self, server_url: str = "https://sockridge.com:9000"):
        self.server_url  = server_url.rstrip("/")
        self._token      = ""
        self._keypair: Optional[KeyPair] = None
        self._publisher_id = ""
        self._client = httpx.Client(http2=True, timeout=15.0)

    # ── Auth ──────────────────────────────────────────────────────────────────

    def login(
        self,
        credentials_path: str | None = None,
        key_path: str | None = None,
    ) -> None:
        """Load credentials and perform challenge-response auth."""
        creds = Credentials.load(credentials_path)
        kp    = KeyPair.load(key_path or str(Credentials.DEFAULT_KEY))

        self._keypair      = kp
        self._publisher_id = creds.publisher_id

        # challenge → sign → token
        challenge = self._post("/agentregistry.v1.RegistryService/AuthChallenge", {
            "publisherId": creds.publisher_id,
        })
        nonce = challenge["nonce"]
        sig   = kp.sign(nonce.encode())

        verify = self._post("/agentregistry.v1.RegistryService/AuthVerify", {
            "publisherId": creds.publisher_id,
            "nonce":       nonce,
            "signature":   base64.b64encode(sig).decode(),
        })

        self._token = verify["sessionToken"]

    # ── Publish ───────────────────────────────────────────────────────────────

    def publish(self, card: AgentCard) -> AgentCard:
        """
        Publish an agent to the registry.
        Signs the payload with Ed25519 before sending.
        Returns the AgentCard with server-assigned id and status.
        """
        if not self._keypair:
            raise RegistryError("not authenticated — call registry.login() first")

        # use compact JSON with sorted keys for deterministic signing
        payload_json = json.dumps(card.to_dict(), separators=(",", ":"), sort_keys=True).encode()
        signature    = self._keypair.sign(payload_json)

        resp = self._post_auth("/agentregistry.v1.RegistryService/PublishAgent", {
            "payload": {
                "payload":   base64.b64encode(payload_json).decode(),
                "signature": base64.b64encode(signature).decode(),
                "keyId":     self._publisher_id,
            }
        })

        return self._parse_agent_card(resp.get("agent", {}))

    # ── Discovery ─────────────────────────────────────────────────────────────

    def search(self, tags: list[str] = [], limit: int = 20) -> list[AgentCard]:
        """List agents by tags. URL is not included (use resolve for that)."""
        resp = self._post("/agentregistry.v1.DiscoveryService/ListAgents", {
            "tags":  tags,
            "limit": limit,
        })
        return [self._parse_agent_card(a) for a in resp if a]

    def semantic_search(self, query: str, top_k: int = 10, min_score: float = 0.1) -> list[dict]:
        """Find agents by natural language description."""
        resp = self._post("/agentregistry.v1.DiscoveryService/SemanticSearch", {
            "query":    query,
            "topK":     top_k,
            "minScore": min_score,
        })
        return [{"agent": self._parse_agent_card(r.get("agent", {})), "score": r.get("score", 0)} for r in resp]

    def get_agent(self, agent_id: str) -> AgentCard:
        """Get a single agent by ID."""
        resp = self._post("/agentregistry.v1.DiscoveryService/GetAgent", {
            "agentId": agent_id,
        })
        return self._parse_agent_card(resp.get("agent", {}))

    # ── Access Agreements ─────────────────────────────────────────────────────

    def request_access(self, receiver_id: str, message: str = "") -> dict:
        """Request mutual access with another publisher."""
        resp = self._post_auth("/agentregistry.v1.AccessAgreementService/RequestAccess", {
            "requesterId": self._publisher_id,
            "receiverId":  receiver_id,
            "message":     message,
        })
        return resp.get("agreement", {})

    def approve_access(self, agreement_id: str) -> str:
        """Approve a pending access request. Returns the shared key."""
        resp = self._post_auth("/agentregistry.v1.AccessAgreementService/ApproveAccess", {
            "publisherId": self._publisher_id,
            "agreementId": agreement_id,
        })
        return resp.get("sharedKey", "")

    def resolve_endpoint(self, agent_id: str, shared_key: str) -> dict:
        """
        Resolve an agent's endpoint URL using a shared key.
        Returns { url, transport, agent: AgentCard }
        """
        resp = self._post("/agentregistry.v1.AccessAgreementService/ResolveEndpoint", {
            "agentId":   agent_id,
            "sharedKey": shared_key,
        })
        return {
            "url":       resp.get("url", ""),
            "transport": resp.get("transport", "http"),
            "agent":     self._parse_agent_card(resp.get("agent", {})),
        }

    # ── Self-register helper ──────────────────────────────────────────────────

    def register_and_publish(
        self,
        card: AgentCard,
        credentials_path: str | None = None,
        key_path: str | None = None,
    ) -> AgentCard:
        """
        Convenience method: login then publish in one call.
        Ideal for agent startup scripts.

        Example:
            registry = Registry("https://sockridge.com:9000")
            published = registry.register_and_publish(my_card)
        """
        self.login(credentials_path, key_path)
        return self.publish(card)

    # ── HTTP helpers ──────────────────────────────────────────────────────────

    def _post(self, path: str, body: dict) -> dict | list:
        url  = self.server_url + path
        resp = self._client.post(
            url,
            json=body,
            headers={"Content-Type": "application/json"},
        )
        self._raise_for_status(resp)
        data = resp.json()
        # connect-rpc returns arrays for server-streaming endpoints
        if isinstance(data, list):
            return data
        return data

    def _post_auth(self, path: str, body: dict) -> dict:
        if not self._token:
            raise RegistryError("not authenticated — call registry.login() first")
        url  = self.server_url + path
        resp = self._client.post(
            url,
            json=body,
            headers={
                "Content-Type":  "application/json",
                "Authorization": f"Bearer {self._token}",
            },
        )
        self._raise_for_status(resp)
        return resp.json()

    def _raise_for_status(self, resp: httpx.Response) -> None:
        if resp.status_code >= 400:
            try:
                detail = resp.json().get("message", resp.text)
            except Exception:
                detail = resp.text
            raise RegistryError(f"registry error {resp.status_code}: {detail}")

    def _parse_agent_card(self, data: dict) -> AgentCard:
        if not data:
            return AgentCard(name="", description="", url="")

        skills = [
            Skill(
                id=s.get("id", ""),
                name=s.get("name", ""),
                description=s.get("description", ""),
                tags=s.get("tags", []),
            )
            for s in data.get("skills", [])
        ]

        caps_data = data.get("capabilities", {})
        caps = Capabilities(
            streaming=caps_data.get("streaming", False),
            push_notifications=caps_data.get("pushNotifications", False),
            multi_turn=caps_data.get("multiTurn", False),
            tool_use=caps_data.get("toolUse", False),
        ) if caps_data else None

        gk_data = data.get("gatekeeperResult")
        gk = GatekeeperResult(
            approved=gk_data.get("approved", False),
            confidence_score=gk_data.get("confidenceScore", 0),
            reason=gk_data.get("reason", ""),
            reachable=gk_data.get("reachable", False),
            ping_latency_ms=gk_data.get("pingLatencyMs", 0),
        ) if gk_data else None

        return AgentCard(
            id=data.get("id"),
            name=data.get("name", ""),
            description=data.get("description", ""),
            url=data.get("url", ""),
            version=data.get("version", "0.1.0"),
            protocol_version=data.get("protocolVersion", "0.3.0"),
            skills=skills,
            capabilities=caps,
            publisher_id=data.get("publisherId"),
            status=AgentStatus(data["status"]) if data.get("status") else None,
            gatekeeper_result=gk,
        )