from dataclasses import dataclass, field
from typing import Optional
from enum import Enum


class AgentStatus(str, Enum):
    PENDING    = "AGENT_STATUS_PENDING"
    ACTIVE     = "AGENT_STATUS_ACTIVE"
    REJECTED   = "AGENT_STATUS_REJECTED"
    INACTIVE   = "AGENT_STATUS_INACTIVE"
    DEPRECATED = "AGENT_STATUS_DEPRECATED"


@dataclass
class Skill:
    id: str
    name: str
    description: str
    tags: list[str] = field(default_factory=list)

    def to_dict(self) -> dict:
        return {
            "id": self.id,
            "name": self.name,
            "description": self.description,
            "tags": self.tags,
        }


@dataclass
class Capabilities:
    streaming: bool          = False
    push_notifications: bool = False
    multi_turn: bool         = False
    tool_use: bool           = False

    def to_dict(self) -> dict:
        return {
            "streaming":          self.streaming,
            "pushNotifications":  self.push_notifications,
            "multiTurn":          self.multi_turn,
            "toolUse":            self.tool_use,
        }


@dataclass
class GatekeeperResult:
    approved: bool
    confidence_score: float
    reason: str
    reachable: bool
    ping_latency_ms: int


@dataclass
class AgentCard:
    name: str
    description: str
    url: str
    version: str                        = "0.1.0"
    protocol_version: str               = "0.3.0"
    skills: list[Skill]                 = field(default_factory=list)
    capabilities: Optional[Capabilities] = None

    # set by registry after publish — do not set manually
    id: Optional[str]                          = None
    publisher_id: Optional[str]                = None
    status: Optional[AgentStatus]              = None
    gatekeeper_result: Optional[GatekeeperResult] = None

    def to_dict(self) -> dict:
        d = {
            "name":            self.name,
            "description":     self.description,
            "url":             self.url,
            "version":         self.version,
            "protocolVersion": self.protocol_version,
            "skills":          [s.to_dict() for s in self.skills],
        }
        if self.capabilities:
            d["capabilities"] = self.capabilities.to_dict()
        return d
