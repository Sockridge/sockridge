export type AgentStatus =
  | "AGENT_STATUS_PENDING"
  | "AGENT_STATUS_ACTIVE"
  | "AGENT_STATUS_REJECTED"
  | "AGENT_STATUS_INACTIVE"
  | "AGENT_STATUS_DEPRECATED";

export type AgreementStatus =
  | "AGREEMENT_STATUS_PENDING"
  | "AGREEMENT_STATUS_ACTIVE"
  | "AGREEMENT_STATUS_DENIED"
  | "AGREEMENT_STATUS_REVOKED";

export interface Skill {
  id: string;
  name: string;
  description: string;
  tags?: string[];
}

export interface Capabilities {
  streaming?: boolean;
  pushNotifications?: boolean;
  multiTurn?: boolean;
  toolUse?: boolean;
}

export interface GatekeeperResult {
  approved: boolean;
  confidenceScore: number;
  reason: string;
  reachable: boolean;
  pingLatencyMs: number;
}

export interface AgentCard {
  id?: string;
  name: string;
  description: string;
  url: string;
  version?: string;
  protocolVersion?: string;
  skills?: Skill[];
  capabilities?: Capabilities;
  publisherId?: string;
  status?: AgentStatus;
  gatekeeperResult?: GatekeeperResult;
}

export interface Agreement {
  id: string;
  requesterId: string;
  receiverId: string;
  message: string;
  status: AgreementStatus;
  sharedKey?: string;
}

export interface ResolvedEndpoint {
  url: string;
  transport: string;
  agent?: AgentCard;
}

export interface SearchResult {
  agent: AgentCard;
  score: number;
}
