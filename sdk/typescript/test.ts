import { Registry } from "./src/registry";

async function main() {
  const registry = new Registry("http://localhost:9000");

  await registry.login();
  console.log("logged in");

  // search
  const agents = await registry.search(["nlp"]);
  const agentsSem = await registry.semanticSearch("medical guidance");

  console.log(`found ${agents.length} agents`);
  for (const a of agents) {
    console.log(`  ${a.name} — ${a.id}`);
  }

  // publish
  async function publish() {
    const published = await registry.publish({
      name: "TypeScript SDK Agent",
      description: "Published via the TypeScript SDK for testing",
      url: "http://host.docker.internal:4000",
      version: "0.1.0",
      skills: [
        {
          id: "ts.test",
          name: "TS Test",
          description: "Tests the TypeScript SDK",
          tags: ["typescript", "sdk", "test"],
        },
      ],
      capabilities: { streaming: true },
    });

    console.log(`published: ${published.id} — ${published.status}`);
  }
}

main().catch(console.error);
