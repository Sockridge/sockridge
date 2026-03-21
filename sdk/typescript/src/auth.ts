import * as crypto from "crypto";
import * as fs from "fs";
import * as path from "path";
import * as os from "os";

export interface Credentials {
  publisher_id: string;
  handle: string;
  server_url: string;
  session_token?: string;
}

export function loadCredentials(credentialsPath?: string): Credentials {
  const p =
    credentialsPath ??
    path.join(os.homedir(), ".sockridge", "credentials.json");
  const data = fs.readFileSync(p, "utf-8");
  return JSON.parse(data) as Credentials;
}

export function loadPrivateKey(keyPath?: string): Buffer {
  const p = keyPath ?? path.join(os.homedir(), ".sockridge", "ed25519.key");
  const b64 = fs.readFileSync(p, "utf-8").trim();
  return Buffer.from(b64, "base64");
}

export function signPayload(privateKeyBytes: Buffer, payload: Buffer): Buffer {
  // Node crypto expects 32-byte seed for Ed25519
  // Go stores 64-byte key (private + public) — take first 32
  const seed = privateKeyBytes.slice(0, 32);
  const keyObject = crypto.createPrivateKey({
    key: seed,
    format: "der",
    type: "pkcs8",
  });
  return crypto.sign(null, payload, keyObject);
}

// Alternative using raw seed directly (more reliable across Node versions)
export function signWithSeed(seed: Buffer, message: Buffer): Buffer {
  const { privateKey } = crypto.generateKeyPairSync("ed25519", {
    privateKeyEncoding: { format: "der", type: "pkcs8" },
    publicKeyEncoding: { format: "der", type: "spki" },
  });
  // use the seed to derive keypair
  const keyObj = crypto.createPrivateKey({
    key: Buffer.concat([
      Buffer.from("302e020100300506032b657004220420", "hex"), // PKCS8 header for Ed25519
      seed.slice(0, 32),
    ]),
    format: "der",
    type: "pkcs8",
  });
  return crypto.sign(null, message, keyObj);
}
