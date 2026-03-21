import * as crypto from "crypto";
import * as os from "os";
import * as path from "path";
import { fetch, Agent } from "undici";
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
  private agent: Agent;

  constructor(serverURL = "https://sockridge.com:9000") {
    this.serverURL = serverURL.replace(/\/$/, "");
    // undici Agent with HTTP/2 support
    this.agent = new Agent({ allowH2: true });
  }

  // ── Auth ────────────────────────────────────────────────────────────────────

  async login(credentialsPath?: string, keyPath?: string): Promise<void> {
    const creds = loadCredentials(credentialsPath);
    this.privateKeyBytes = loadPrivateKey(keyPath);
    this.publisherId = creds.publisher_id;

    const chalResp = await this.post(
      "/agentregistry.v1.RegistryService/AuthChallenge",
      { publisherId: creds.publisher_id },
    );
    const nonce = (chalResp as Record<string, unknown>).nonce as string;
    const sig = this.sign(Buffer.from(nonce));

    const verifyResp = await this.post(
      "/agentregistry.v1.RegistryService/AuthVerify",
      {
        publisherId: creds.publisher_id,
        nonce,
        signature: sig.toString("base64"),
      },
    );
    this.token = (verifyResp as Record<string, unknown>).sessionToken as string;
  }

  // ── Publish ─────────────────────────────────────────────────────────────────

  async publish(card: AgentCard): Promise<AgentCard> {
    this.requireAuth();
    const payload = Buffer.from(
      JSON.stringify(this.toWireFormat(card)),
      "utf-8",
    );
    const signature = this.sign(payload);

    const resp = (await this.postAuth(
      "/agentregistry.v1.RegistryService/PublishAgent",
      {
        payload: {
          payload: payload.toString("base64"),
          signature: signature.toString("base64"),
          keyId: this.publisherId,
        },
      },
    )) as Record<string, unknown>;

    return this.parseAgentCard((resp.agent ?? {}) as Record<string, unknown>);
  }

  // ── Discovery ───────────────────────────────────────────────────────────────

  async search(tags: string[] = [], limit = 20): Promise<AgentCard[]> {
    const rows = await this.stream(
      "/agentregistry.v1.DiscoveryService/ListAgents",
      { tags, limit },
    );
    return rows
      .filter((r) => "agent" in r)
      .map((r) => this.parseAgentCard(r.agent as Record<string, unknown>));
  }

  async semanticSearch(
    query: string,
    topK = 10,
    minScore = 0.1,
  ): Promise<SearchResult[]> {
    const rows = await this.stream(
      "/agentregistry.v1.DiscoveryService/SemanticSearch",
      { query, topK, minScore },
    );
    return rows
      .filter((r) => "agent" in r)
      .map((r) => ({
        agent: this.parseAgentCard(r.agent as Record<string, unknown>),
        score: (r.score as number) ?? 0,
      }));
  }

  async getAgent(agentId: string): Promise<AgentCard> {
    const resp = (await this.post(
      "/agentregistry.v1.DiscoveryService/GetAgent",
      { agentId },
    )) as Record<string, unknown>;
    return this.parseAgentCard((resp.agent ?? {}) as Record<string, unknown>);
  }

  // ── Access Agreements ───────────────────────────────────────────────────────

  async requestAccess(receiverId: string, message = ""): Promise<Agreement> {
    const resp = (await this.postAuth(
      "/agentregistry.v1.AccessAgreementService/RequestAccess",
      { requesterId: this.publisherId, receiverId, message },
    )) as Record<string, unknown>;
    return this.parseAgreement(
      (resp.agreement ?? {}) as Record<string, unknown>,
    );
  }

  async approveAccess(agreementId: string): Promise<string> {
    const resp = (await this.postAuth(
      "/agentregistry.v1.AccessAgreementService/ApproveAccess",
      { publisherId: this.publisherId, agreementId },
    )) as Record<string, unknown>;
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
    const resp = (await this.post(
      "/agentregistry.v1.AccessAgreementService/ResolveEndpoint",
      { agentId, sharedKey },
    )) as Record<string, unknown>;
    return {
      url: (resp.url as string) ?? "",
      transport: (resp.transport as string) ?? "http",
      agent: resp.agent
        ? this.parseAgentCard(resp.agent as Record<string, unknown>)
        : undefined,
    };
  }

  // ── Identity ──────────────────────────────────────────────────────────────

  whoami(): { publisherId: string } {
    if (!this.publisherId)
      throw new RegistryError("not authenticated — call login() first");
    return { publisherId: this.publisherId };
  }

  isLoggedIn(): boolean {
    return this.token !== "" && this.publisherId !== "";
  }

  // ── My agents ─────────────────────────────────────────────────────────────

  async getMyAgent(agentId: string): Promise<AgentCard> {
    const resp = (await this.postAuth(
      "/agentregistry.v1.RegistryService/GetAgent",
      { agentId },
    )) as Record<string, unknown>;
    return this.parseAgentCard((resp.agent ?? {}) as Record<string, unknown>);
  }

  // ── Agreement utilities ───────────────────────────────────────────────────

  async listAgreements(): Promise<Agreement[]> {
    const rows = await this.stream(
      "/agentregistry.v1.AccessAgreementService/ListAgreements",
      { publisherId: this.publisherId },
    );
    return rows
      .filter((r) => "agreement" in r)
      .map((r) => this.parseAgreement(r.agreement as Record<string, unknown>));
  }

  async listPending(): Promise<Array<Agreement & { requesterHandle: string }>> {
    const rows = await this.stream(
      "/agentregistry.v1.AccessAgreementService/ListPending",
      { publisherId: this.publisherId },
    );
    return rows
      .filter((r) => "agreement" in r)
      .map((r) => ({
        ...this.parseAgreement(r.agreement as Record<string, unknown>),
        requesterHandle: (r.requesterHandle as string) ?? "",
      }));
  }

  async getAgreement(agreementId: string): Promise<Agreement> {
    const resp = (await this.postAuth(
      "/agentregistry.v1.AccessAgreementService/GetAgreement",
      { publisherId: this.publisherId, agreementId },
    )) as Record<string, unknown>;
    return this.parseAgreement(
      (resp.agreement ?? {}) as Record<string, unknown>,
    );
  }

  async denyAccess(agreementId: string): Promise<void> {
    await this.postAuth("/agentregistry.v1.AccessAgreementService/DenyAccess", {
      publisherId: this.publisherId,
      agreementId,
    });
  }

  // check if a shared key grants access to a specific agent
  // returns the resolved endpoint if valid, null if not
  async hasAccess(
    agentId: string,
    sharedKey: string,
  ): Promise<ResolvedEndpoint | null> {
    try {
      return await this.resolveEndpoint(agentId, sharedKey);
    } catch {
      return null;
    }
  }

  // publish if new, update if agent.id is set and already exists
  async publishOrUpdate(card: AgentCard): Promise<AgentCard> {
    if (card.id) {
      try {
        return await this.updateAgent(card);
      } catch {
        // if update fails (not found etc), fall through to publish
      }
    }
    return await this.publish(card);
  }

  async updateAgent(card: AgentCard): Promise<AgentCard> {
    this.requireAuth();
    if (!card.id) throw new RegistryError("agent id is required for update");

    const payload = Buffer.from(
      JSON.stringify({ ...this.toWireFormat(card), id: card.id }),
      "utf-8",
    );
    const signature = this.sign(payload);

    const resp = (await this.postAuth(
      "/agentregistry.v1.RegistryService/UpdateAgent",
      {
        payload: {
          payload: payload.toString("base64"),
          signature: signature.toString("base64"),
          keyId: this.publisherId,
        },
      },
    )) as Record<string, unknown>;

    return this.parseAgentCard((resp.agent ?? {}) as Record<string, unknown>);
  }

  // ── HTTP ────────────────────────────────────────────────────────────────────

  private async post(path: string, body: unknown): Promise<unknown> {
    const res = await fetch(`${this.serverURL}${path}`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body),
      // @ts-ignore — undici dispatcher
      dispatcher: this.agent,
    });
    const text = await res.text();
    if (!res.ok) {
      let msg = `HTTP ${res.status}`;
      try {
        msg =
          ((JSON.parse(text) as Record<string, unknown>).message as string) ??
          msg;
      } catch {}
      throw new RegistryError(msg, res.status);
    }
    if (!text.trim()) return {};
    return JSON.parse(text);
  }

  private async postAuth(path: string, body: unknown): Promise<unknown> {
    if (!this.token)
      throw new RegistryError("not authenticated — call login() first");
    const res = await fetch(`${this.serverURL}${path}`, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        Authorization: `Bearer ${this.token}`,
      },
      body: JSON.stringify(body),
      // @ts-ignore
      dispatcher: this.agent,
    });
    const text = await res.text();
    if (!res.ok) {
      let msg = `HTTP ${res.status}`;
      try {
        msg =
          ((JSON.parse(text) as Record<string, unknown>).message as string) ??
          msg;
      } catch {}
      throw new RegistryError(msg, res.status);
    }
    if (!text.trim()) return {};
    return JSON.parse(text);
  }

  private async stream(
    path: string,
    body: unknown,
  ): Promise<Record<string, unknown>[]> {
    // gRPC+JSON requires a 5-byte length-prefix frame:
    // byte 0: compression flag (0 = no compression)
    // bytes 1-4: big-endian message length
    const msgBytes = Buffer.from(JSON.stringify(body), "utf-8");
    const frame = Buffer.alloc(5 + msgBytes.length);
    frame.writeUInt8(0, 0);
    frame.writeUInt32BE(msgBytes.length, 1);
    msgBytes.copy(frame, 5);

    const res = await fetch(`${this.serverURL}${path}`, {
      method: "POST",
      headers: {
        "Content-Type": "application/grpc+json",
        TE: "trailers",
      },
      body: frame,
      // @ts-ignore
      dispatcher: this.agent,
    });

    if (!res.ok) {
      const text = await res.text();
      let msg = `HTTP ${res.status}`;
      try {
        msg =
          ((JSON.parse(text) as Record<string, unknown>).message as string) ??
          msg;
      } catch {}
      throw new RegistryError(msg, res.status);
    }

    // parse gRPC response frames
    const buf = Buffer.from(await res.arrayBuffer());
    const results: Record<string, unknown>[] = [];
    let offset = 0;

    while (offset + 5 <= buf.length) {
      const compressed = buf.readUInt8(offset);
      const msgLen = buf.readUInt32BE(offset + 1);
      offset += 5;

      if (compressed === 0x80) break; // trailers frame

      if (offset + msgLen > buf.length) break;
      const msgBuf = buf.slice(offset, offset + msgLen);
      offset += msgLen;

      try {
        const parsed = JSON.parse(msgBuf.toString("utf-8")) as Record<
          string,
          unknown
        >;
        results.push(parsed);
      } catch {
        /* skip malformed frames */
      }
    }

    return results;
  }

  // ── helpers ──────────────────────────────────────────────────────────────────

  private sign(message: Buffer): Buffer {
    if (!this.privateKeyBytes) throw new RegistryError("no private key loaded");
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
