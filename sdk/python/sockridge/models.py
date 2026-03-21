from dataclasses import dataclass, field
from typing import Optional
from enum import Enum


class AgentStatus(str, Enum):
    PENDING    = "AGENT_STATUS_PENDING"
    ACTIVE     = "AGENT_STATUS_ACTIVE"
    REJECTED   = "AGENT_STATUS_REJECTED"
    INACTIVE   = "AGENT_STATUS_INACTIVE"
    DEPRECATED = "AGENT_STATUS_DEPRECATED"


class AuthScheme(str, Enum):
    NONE    = "AUTH_SCHEME_NONE"
    API_KEY = "AUTH_SCHEME_API_KEY"
    OAUTH2  = "AUTH_SCHEME_OAUTH2"
    OPENID  = "AUTH_SCHEME_OPENID"


@dataclass
class Provider:
    name:    str
    url:     str = ""
    contact: str = ""

    def to_dict(self) -> dict:
        return {"name": self.name, "url": self.url, "contact": self.contact}


@dataclass
class Authentication:
    scheme:      AuthScheme = AuthScheme.NONE
    description: str        = ""
    token_url:   str        = ""
    scopes:      str        = ""

    def to_dict(self) -> dict:
        return {
            "scheme":      self.scheme.value,
            "description": self.description,
            "tokenUrl":    self.token_url,
            "scopes":      self.scopes,
        }


@dataclass
class SkillExample:
    input:  str = ""
    output: str = ""

    def to_dict(self) -> dict:
        return {"input": self.input, "output": self.output}


@dataclass
class Skill:
    id:           str
    name:         str
    description:  str
    tags:         list[str]          = field(default_factory=list)
    input_modes:  list[str]          = field(default_factory=list)
    output_modes: list[str]          = field(default_factory=list)
    examples:     list[SkillExample] = field(default_factory=list)

    def to_dict(self) -> dict:
        d = {
            "id":          self.id,
            "name":        self.name,
            "description": self.description,
            "tags":        self.tags,
        }
        if self.input_modes:
            d["inputModes"] = self.input_modes
        if self.output_modes:
            d["outputModes"] = self.output_modes
        if self.examples:
            d["examples"] = [e.to_dict() for e in self.examples]
        return d


@dataclass
class Capabilities:
    streaming:          bool = False
    push_notifications: bool = False
    multi_turn:         bool = False
    tool_use:           bool = False

    def to_dict(self) -> dict:
        return {
            "streaming":         self.streaming,
            "pushNotifications": self.push_notifications,
            "multiTurn":         self.multi_turn,
            "toolUse":           self.tool_use,
        }


@dataclass
class GatekeeperResult:
    approved:         bool
    confidence_score: float
    reason:           str
    reachable:        bool
    ping_latency_ms:  int
    a2a_compliant:    bool = False
    a2a_matches_card: bool = False


@dataclass
class AgentCard:
    name:        str
    description: str
    url:         str

    version:          str = "0.1.0"
    protocol_version: str = "0.3.0"

    skills:       list[Skill]    = field(default_factory=list)
    capabilities: Optional[Capabilities] = None

    # A2A spec fields
    provider:          Optional[Provider]       = None
    authentication:    Optional[Authentication] = None
    icon_url:          str                      = ""
    documentation_url: str                      = ""
    protocol_versions: list[str]                = field(default_factory=list)
    extensions:        list[str]                = field(default_factory=list)

    # set by registry after publish
    id:               Optional[str]              = None
    publisher_id:     Optional[str]              = None
    status:           Optional[AgentStatus]      = None
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
        if self.provider:
            d["provider"] = self.provider.to_dict()
        if self.authentication:
            d["authentication"] = self.authentication.to_dict()
        if self.icon_url:
            d["iconUrl"] = self.icon_url
        if self.documentation_url:
            d["documentationUrl"] = self.documentation_url
        if self.protocol_versions:
            d["protocolVersions"] = self.protocol_versions
        if self.extensions:
            d["extensions"] = self.extensions
        return d