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

export type AuthScheme =
  | "AUTH_SCHEME_NONE"
  | "AUTH_SCHEME_API_KEY"
  | "AUTH_SCHEME_OAUTH2"
  | "AUTH_SCHEME_OPENID";

export interface Provider {
  name: string;
  url?: string;
  contact?: string;
}

export interface Authentication {
  scheme: AuthScheme;
  description?: string;
  tokenUrl?: string;
  scopes?: string;
}

export interface SkillExample {
  input?: string;
  output?: string;
}

export interface Skill {
  id: string;
  name: string;
  description: string;
  tags?: string[];
  inputModes?: string[];
  outputModes?: string[];
  examples?: SkillExample[];
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
  a2aCompliant?: boolean;
  a2aMatchesCard?: boolean;
}

export interface AgentCard {
  // required
  name: string;
  description: string;
  url: string;

  // optional base
  id?: string;
  version?: string;
  protocolVersion?: string;
  skills?: Skill[];
  capabilities?: Capabilities;

  // A2A spec fields
  provider?: Provider;
  authentication?: Authentication;
  iconUrl?: string;
  documentationUrl?: string;
  protocolVersions?: string[];
  extensions?: string[];

  // set by registry
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
