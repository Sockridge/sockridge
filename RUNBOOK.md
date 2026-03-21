# Runbook

Complete guide for local development and production operations.

---

## Infrastructure

| Service           | Port    | Notes             |
| ----------------- | ------- | ----------------- |
| Registry server   | 9000    | gRPC + ConnectRPC |
| ScyllaDB          | 9042    | internal only     |
| Redis             | 6379    | internal only     |
| Postgres/pgvector | 5432    | internal only     |
| Embedder sidecar  | 8000    | internal only     |
| Nginx             | 80, 443 | website only      |

Only port 9000 and 80/443 are publicly exposed.

---

## Local Development

### 1. Install tools

```bash
brew install bufbuild/buf/buf
brew install go          # need 1.23+
brew install docker      # or Docker Desktop
```

### 2. Install buf plugins for Go codegen

```bash
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
go install connectrpc.com/connect/cmd/protoc-gen-connect-go@latest

# make sure $GOPATH/bin is in your PATH
export PATH="$PATH:$(go env GOPATH)/bin"
```

### 3. Generate Go code from proto

```bash
buf dep update   # resolves googleapis dependency in buf.yaml
buf generate     # writes gen/go/agentregistry/v1/*.go

# you should now see:
# gen/go/agentregistry/v1/agent.pb.go
# gen/go/agentregistry/v1/registry.pb.go
# gen/go/agentregistry/v1/registryv1connect/registry.connect.go
```

### 4. Tidy Go modules

```bash
cd server && go mod tidy && cd ..
cd cli    && go mod tidy && cd ..
cd sdk/go && go mod tidy && cd ../..
# or from root: go work sync
```

### 5. Verify it compiles

```bash
cd server && go build ./... && cd ..
cd cli    && go build ./... && cd ..
```

### 6. Run auth tests

```bash
cd server && go test ./internal/auth/... -v && cd ..

# expected output:
# --- PASS: TestChallengeVerify_HappyPath
# --- PASS: TestVerify_WrongSignature
# --- PASS: TestVerify_NonceIsOneTimeUse
```

### 7. Spin up the full stack

```bash
export AGENTREGISTRY_GATEKEEPER_ANTHROPIC_KEY=sk-ant-...   # optional
export AGENTREGISTRY_GATEKEEPER_GROQ_KEY=gsk_...           # optional fallback

docker compose up --build

# waits for:
# scylla    → healthy (~30s on first boot)
# redis     → healthy
# postgres  → healthy
# embedder  → healthy
# server    → starts on :9000
```

### 8. Build the CLI

```bash
cd cli && go build -o ../bin/sockridge . && cd ..
export PATH="$PATH:$(pwd)/bin"
```

### 9. Smoke test end-to-end

```bash
# register
sockridge auth keygen
sockridge auth register --handle utsav
sockridge auth login

# publish a test agent
cat > /tmp/test-agent.json << 'JSON'
{
  "name": "Test FHIR Agent",
  "description": "Analyzes lab trends from FHIR",
  "version": "0.1.0",
  "protocolVersion": "0.3.0",
  "url": "https://your-agent.example.com",
  "skills": [
    {
      "id": "lab.analyze",
      "name": "Lab Analyzer",
      "description": "Detects anomalies in lab result trends",
      "tags": ["fhir", "labs", "analysis"]
    }
  ],
  "capabilities": { "streaming": true, "toolUse": true }
}
JSON

sockridge publish --file /tmp/test-agent.json
sockridge search list
sockridge search semantic "lab trend analyzer"

# watch for new agents in another terminal
sockridge search watch --tag fhir
```

### Known local issues

| Issue                                        | Fix                                                                                                                                |
| -------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------- |
| ScyllaDB takes ~30s to be ready              | docker compose already has healthcheck retries — just wait                                                                         |
| `buf generate` fails with "plugin not found" | make sure `$GOPATH/bin` is in PATH, re-run `go install` for each plugin                                                            |
| `go mod tidy` fails on "cannot find module"  | run `buf generate` first — `gen/go/` must exist before tidy                                                                        |
| "http2: frame too large" on gRPC calls       | make sure you're hitting port 9000 with `http://` (h2c) not `https://`                                                             |
| Semantic search returns nothing              | drop ivfflat index: `docker compose exec postgres psql -U agentregistry -c "DROP INDEX IF EXISTS skill_embeddings_embedding_idx;"` |

---

## Production Deploy

### First time

```bash
# clone
git clone https://github.com/Sockridge/sockridge.git
cd sockridge

# create .env file with your keys
cat > .env << 'EOF'
AGENTREGISTRY_GATEKEEPER_ANTHROPIC_KEY=sk-ant-...
AGENTREGISTRY_GATEKEEPER_GROQ_KEY=gsk_...
AGENTREGISTRY_AUTH_JWT_SECRET=your-strong-random-secret
EOF

# generate proto (requires buf + Go)
buf generate

# start everything
docker compose up -d --build

# verify
docker compose ps
curl http://localhost:9000/healthz
```

### Update (redeploy)

```bash
cd /var/www/sockridge
git pull
docker compose up -d --build
```

Only changed images are rebuilt. ScyllaDB and Postgres data are preserved in Docker volumes.

### Rollback

```bash
git log --oneline -10          # find previous commit
git checkout <commit-hash>
docker compose up -d --build
```

---

## Environment Variables

| Variable                                 | Required | Description                                                                            |
| ---------------------------------------- | -------- | -------------------------------------------------------------------------------------- |
| `AGENTREGISTRY_GATEKEEPER_ANTHROPIC_KEY` | No       | Anthropic key for AI scoring (first priority)                                          |
| `AGENTREGISTRY_GATEKEEPER_GROQ_KEY`      | No       | Groq key for AI scoring (fallback). Without either, agents auto-approve with score 0.5 |
| `AGENTREGISTRY_SCYLLA_HOSTS`             | No       | Defaults to `scylla:9042`                                                              |
| `AGENTREGISTRY_SCYLLA_KEYSPACE`          | No       | Defaults to `agentregistry`                                                            |
| `AGENTREGISTRY_REDIS_ADDR`               | No       | Defaults to `redis:6379`                                                               |
| `AGENTREGISTRY_POSTGRES_DSN`             | No       | Defaults to docker compose postgres                                                    |
| `AGENTREGISTRY_EMBEDDER_URL`             | No       | Defaults to `http://embedder:8000`                                                     |
| `AGENTREGISTRY_AUTH_JWT_SECRET`          | No       | Defaults to `dev-secret` — **change in production**                                    |

Set a strong JWT secret:

```bash
export AGENTREGISTRY_AUTH_JWT_SECRET=$(openssl rand -hex 32)
```

---

## Monitoring

### Check all containers

```bash
docker compose ps
docker stats --no-stream
```

### Check logs

```bash
# all services
docker compose logs -f

# specific service
docker compose logs -f server
docker compose logs -f embedder
docker compose logs -f scylla

# live gatekeeper activity
docker compose logs -f server | grep -i "gatekeeper\|score\|approved\|rejected\|WARN\|ERROR"
```

### Check gatekeeper scoring mode

```bash
docker compose logs server | grep gatekeeper
# "gatekeeper configured with Anthropic scoring (Groq as fallback)"
# "gatekeeper configured with Groq scoring"
# "gatekeeper running without AI scoring ..."
```

### Check pgvector embeddings

```bash
docker compose exec postgres psql -U agentregistry -c \
  "SELECT COUNT(*) FROM skill_embeddings;"
```

### Check ScyllaDB agents

```bash
docker compose exec scylla cqlsh -e \
  "SELECT agent_id, publisher_id FROM agentregistry.agents;"
```

---

## Backup

### ScyllaDB

```bash
# snapshot
docker compose exec scylla nodetool snapshot agentregistry

# copy snapshot out
docker cp sockridge-scylla-1:/var/lib/scylla/data/agentregistry \
  ./backups/scylla-$(date +%Y%m%d)
```

### Postgres (embeddings)

```bash
docker compose exec postgres pg_dump -U agentregistry agentregistry \
  > ./backups/postgres-$(date +%Y%m%d).sql
```

### Redis

Redis is cache only — no backup needed. Data rebuilds on next request.

---

## Disk & Memory

```bash
df -h
docker system df
sudo du -sh /var/lib/docker
free -h
```

### Clean up unused Docker resources

```bash
# safe — removes stopped containers, dangling images, unused networks
docker system prune -f

# aggressive — removes all unused images (will re-download on next deploy)
docker system prune -a -f
```

---

## Common Issues

### Embedder fails healthcheck

```bash
docker compose logs embedder
# if "curl not found": add RUN apt-get install -y curl to embedder/Dockerfile
```

### ScyllaDB stuck in starting

ScyllaDB takes 30-60s. Wait and check:

```bash
docker compose logs scylla | tail -20
```

### Server can't connect to ScyllaDB

```bash
docker compose ps scylla
docker compose exec scylla cqlsh -e "DESCRIBE KEYSPACES;"
```

### JWT expired / auth failing

Sessions expire after 1 hour:

```bash
sockridge auth login
```

### "no space left on device" during Docker build

The embedder image is large (~2GB). Clean up first:

```bash
docker system prune -a --volumes -f
docker compose up -d --build
```

### Gatekeeper not using AI scoring

Check `.env` file has the keys and Docker picked them up:

```bash
docker compose exec server env | grep GATEKEEPER
```

If empty, keys aren't in `.env` or Docker wasn't restarted after adding them.

---

## Firewall

```bash
sudo ufw allow 22     # SSH
sudo ufw allow 80     # website
sudo ufw allow 443    # website HTTPS
sudo ufw allow 9000   # registry API
sudo ufw enable
sudo ufw status
```

---

## Nginx

```bash
sudo nginx -t                           # test config
sudo systemctl reload nginx             # reload
sudo tail -f /var/log/nginx/access.log  # access logs
sudo tail -f /var/log/nginx/error.log   # error logs
sudo certbot renew --dry-run            # test SSL renewal
```

---

## Scale Notes

Current setup handles ~100 concurrent agents on a 4GB VPS. When you need more:

- **ScyllaDB** — increase `--memory` flag and `--smp` (CPU cores) in docker-compose.yml
- **Embedder** — switch to `pytorch/pytorch` base image for GPU support
- **Postgres** — add ivfflat index once you have >1000 agents: `CREATE INDEX ON skill_embeddings USING ivfflat (embedding vector_cosine_ops) WITH (lists=100);`
- **Server** — stateless, run multiple replicas behind a load balancer
