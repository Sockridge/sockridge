import * as crypto from "crypto";
import * as os from "os";
import * as path from "path";
import { loadCredentials, loadPrivateKey } from "./auth";
import type {
  AgentCard,
  Agreement,
  ResolvedEndpoint,
  SearchResult,
} from "./models";

export class RegistryError extends Error {
  constructor(
    message: string,
    public statusCode?: number,
  ) {
    super(message);
    this.name = "RegistryError";
  }
}

export class Registry {
  private serverURL: string;
  private token = "";
  private publisherId = "";
  private privateKeyBytes: Buffer | null = null;

  constructor(serverURL = "http://localhost:9000") {
    this.serverURL = serverURL.replace(/\/$/, "");
  }

  // ── Auth ────────────────────────────────────────────────────────────────────

  async login(credentialsPath?: string, keyPath?: string): Promise<void> {
    const creds = loadCredentials(credentialsPath);
    this.privateKeyBytes = loadPrivateKey(keyPath);
    this.publisherId = creds.publisher_id;

    // step 1: get challenge nonce
    const chalResp = await this.post(
      "/agentregistry.v1.RegistryService/AuthChallenge",
      { publisherId: creds.publisher_id },
    );

    // step 2: sign nonce
    const nonce = chalResp.nonce as string;
    const sig = this.sign(Buffer.from(nonce));

    // step 3: verify and get token
    const verifyResp = await this.post(
      "/agentregistry.v1.RegistryService/AuthVerify",
      {
        publisherId: creds.publisher_id,
        nonce,
        signature: sig.toString("base64"),
      },
    );

    this.token = verifyResp.sessionToken as string;
  }

  // ── Publish ─────────────────────────────────────────────────────────────────

  async publish(card: AgentCard): Promise<AgentCard> {
    this.requireAuth();

    const payload = Buffer.from(
      JSON.stringify(this.toWireFormat(card)),
      "utf-8",
    );
    const signature = this.sign(payload);

    const resp = await this.postAuth(
      "/agentregistry.v1.RegistryService/PublishAgent",
      {
        payload: {
          payload: payload.toString("base64"),
          signature: signature.toString("base64"),
          keyId: this.publisherId,
        },
      },
    );

    return this.parseAgentCard((resp.agent ?? {}) as Record<string, unknown>);
  }

  // ── Discovery ───────────────────────────────────────────────────────────────

  async search(tags: string[] = [], limit = 20): Promise<AgentCard[]> {
    const resp = await this.post(
      "/agentregistry.v1.DiscoveryService/ListAgents",
      { tags, limit },
    );
    if (!Array.isArray(resp)) return [];
    return resp.map((r: Record<string, unknown>) =>
      this.parseAgentCard((r.agent ?? {}) as Record<string, unknown>),
    );
  }

  async semanticSearch(
    query: string,
    topK = 10,
    minScore = 0.1,
  ): Promise<SearchResult[]> {
    const resp = await this.post(
      "/agentregistry.v1.DiscoveryService/SemanticSearch",
      { query, topK, minScore },
    );
    if (!Array.isArray(resp)) return [];
    return resp.map((r: Record<string, unknown>) => ({
      agent: this.parseAgentCard((r.agent ?? {}) as Record<string, unknown>),
      score: (r.score as number) ?? 0,
    }));
  }

  async getAgent(agentId: string): Promise<AgentCard> {
    const resp = await this.post(
      "/agentregistry.v1.DiscoveryService/GetAgent",
      { agentId },
    );
    return this.parseAgentCard((resp.agent ?? {}) as Record<string, unknown>);
  }

  // ── Access Agreements ───────────────────────────────────────────────────────

  async requestAccess(receiverId: string, message = ""): Promise<Agreement> {
    const resp = await this.postAuth(
      "/agentregistry.v1.AccessAgreementService/RequestAccess",
      { requesterId: this.publisherId, receiverId, message },
    );
    return this.parseAgreement(
      (resp.agreement ?? {}) as Record<string, unknown>,
    );
  }

  async approveAccess(agreementId: string): Promise<string> {
    const resp = await this.postAuth(
      "/agentregistry.v1.AccessAgreementService/ApproveAccess",
      { publisherId: this.publisherId, agreementId },
    );
    return (resp.sharedKey as string) ?? "";
  }

  async revokeAccess(agreementId: string): Promise<void> {
    await this.postAuth(
      "/agentregistry.v1.AccessAgreementService/RevokeAccess",
      { publisherId: this.publisherId, agreementId },
    );
  }

  async resolveEndpoint(
    agentId: string,
    sharedKey: string,
  ): Promise<ResolvedEndpoint> {
    const resp = await this.post(
      "/agentregistry.v1.AccessAgreementService/ResolveEndpoint",
      { agentId, sharedKey },
    );
    return {
      url: (resp.url as string) ?? "",
      transport: (resp.transport as string) ?? "http",
      agent: resp.agent
        ? this.parseAgentCard(resp.agent as Record<string, unknown>)
        : undefined,
    };
  }

  // ── HTTP helpers ─────────────────────────────────────────────────────────────

  private async post(
    path: string,
    body: unknown,
  ): Promise<Record<string, unknown>> {
    const res = await fetch(`${this.serverURL}${path}`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body),
    });

    const data = (await res.json()) as Record<string, unknown>;
    if (!res.ok) {
      throw new RegistryError(
        (data.message as string) ?? `HTTP ${res.status}`,
        res.status,
      );
    }
    return data;
  }

  private async postAuth(
    path: string,
    body: unknown,
  ): Promise<Record<string, unknown>> {
    if (!this.token)
      throw new RegistryError("not authenticated — call login() first");

    const res = await fetch(`${this.serverURL}${path}`, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        Authorization: `Bearer ${this.token}`,
      },
      body: JSON.stringify(body),
    });

    const data = (await res.json()) as Record<string, unknown>;
    if (!res.ok) {
      throw new RegistryError(
        (data.message as string) ?? `HTTP ${res.status}`,
        res.status,
      );
    }
    return data;
  }

  private sign(message: Buffer): Buffer {
    if (!this.privateKeyBytes) throw new RegistryError("no private key loaded");

    // Ed25519 PKCS8 DER header
    const header = Buffer.from("302e020100300506032b657004220420", "hex");
    const seed = this.privateKeyBytes.slice(0, 32);
    const der = Buffer.concat([header, seed]);

    const keyObj = crypto.createPrivateKey({
      key: der,
      format: "der",
      type: "pkcs8",
    });
    return crypto.sign(null, message, keyObj);
  }

  private requireAuth(): void {
    if (!this.token)
      throw new RegistryError("not authenticated — call login() first");
  }

  // ── Parsers ──────────────────────────────────────────────────────────────────

  private toWireFormat(card: AgentCard): Record<string, unknown> {
    return {
      name: card.name,
      description: card.description,
      url: card.url,
      version: card.version ?? "0.1.0",
      protocolVersion: card.protocolVersion ?? "0.3.0",
      skills: card.skills ?? [],
      capabilities: card.capabilities ?? {},
    };
  }

  private parseAgentCard(data: Record<string, unknown>): AgentCard {
    return {
      id: data.id as string,
      name: (data.name as string) ?? "",
      description: (data.description as string) ?? "",
      url: (data.url as string) ?? "",
      version: (data.version as string) ?? "0.1.0",
      protocolVersion: (data.protocolVersion as string) ?? "0.3.0",
      skills: (data.skills as AgentCard["skills"]) ?? [],
      capabilities: data.capabilities as AgentCard["capabilities"],
      publisherId: data.publisherId as string,
      status: data.status as AgentCard["status"],
      gatekeeperResult: data.gatekeeperResult as AgentCard["gatekeeperResult"],
    };
  }

  private parseAgreement(data: Record<string, unknown>): Agreement {
    return {
      id: (data.id as string) ?? "",
      requesterId: (data.requesterId as string) ?? "",
      receiverId: (data.receiverId as string) ?? "",
      message: (data.message as string) ?? "",
      status:
        (data.status as Agreement["status"]) ?? "AGREEMENT_STATUS_PENDING",
      sharedKey: data.sharedKey as string,
    };
  }
}
