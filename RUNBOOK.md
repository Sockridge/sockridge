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

### 2. Install buf plugins

```bash
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
go install connectrpc.com/connect/cmd/protoc-gen-connect-go@latest
export PATH="$PATH:$(go env GOPATH)/bin"
```

### 3. Generate proto code

```bash
buf dep update
buf generate

# expected output:
# gen/go/agentregistry/v1/agent.pb.go
# gen/go/agentregistry/v1/registry.pb.go
# gen/go/agentregistry/v1/registryv1connect/registry.connect.go
```

### 4. Tidy modules

```bash
cd server && go mod tidy && cd ..
cd cli    && go mod tidy && cd ..
cd sdk/go && go mod tidy && cd ../..
```

### 5. Run tests

```bash
cd server && go test ./... -v && cd ..

# expected: 42 tests passing across:
# auth, gatekeeper, healthmon, metrics, ratelimit, webhook
```

### 6. Spin up stack

```bash
cp .env.example .env
# edit .env with your keys

docker compose up --build
```

### 7. Build CLI

```bash
cd cli && go build -o ../bin/sockridge . && cd ..
export PATH="$PATH:$(pwd)/bin"
```

### 8. Smoke test

```bash
sockridge auth keygen
sockridge auth register --handle utsav
sockridge auth login
sockridge publish --file test_agents/fhir.json
sockridge search list
sockridge search semantic "lab analyzer"
sockridge audit list
curl http://localhost:9000/metrics
```

### Known issues

| Issue                                    | Fix                                                                                                                                |
| ---------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------- |
| ScyllaDB takes ~30s to start             | wait — healthcheck handles it                                                                                                      |
| `buf generate` fails "plugin not found"  | re-run `go install` for each plugin, check `$GOPATH/bin` in PATH                                                                   |
| `go mod tidy` fails "cannot find module" | run `buf generate` first                                                                                                           |
| "http2: frame too large"                 | use `http://` (h2c) not `https://` for local                                                                                       |
| Semantic search returns nothing          | drop ivfflat index: `docker compose exec postgres psql -U agentregistry -c "DROP INDEX IF EXISTS skill_embeddings_embedding_idx;"` |
| Rate limit test — all publishes succeed  | check `.env` has Redis connected, restart docker                                                                                   |

---

## Production Deploy

### First time

```bash
git clone https://github.com/Sockridge/sockridge.git
cd sockridge

cat > .env << 'EOF'
AGENTREGISTRY_GATEKEEPER_ANTHROPIC_KEY=sk-ant-...
AGENTREGISTRY_GATEKEEPER_GROQ_KEY=gsk_...
AGENTREGISTRY_AUTH_JWT_SECRET=your-strong-random-secret
EOF

buf generate
docker compose up -d --build

docker compose ps
curl http://localhost:9000/healthz
curl http://localhost:9000/metrics
```

### Update

```bash
cd /var/www/sockridge
git pull
buf generate          # only if proto changed
docker compose up -d --build
```

### Rollback

```bash
git log --oneline -10
git checkout <commit-hash>
docker compose up -d --build
```

---

## Environment Variables

| Variable                                 | Required | Description                                                      |
| ---------------------------------------- | -------- | ---------------------------------------------------------------- |
| `AGENTREGISTRY_GATEKEEPER_ANTHROPIC_KEY` | No       | Anthropic key — first priority for AI scoring                    |
| `AGENTREGISTRY_GATEKEEPER_GROQ_KEY`      | No       | Groq key — fallback. Without either, auto-approve with score 0.5 |
| `AGENTREGISTRY_AUTH_JWT_SECRET`          | No       | Defaults to `dev-secret` — **change in production**              |
| `AGENTREGISTRY_SCYLLA_HOSTS`             | No       | Defaults to `scylla:9042`                                        |
| `AGENTREGISTRY_SCYLLA_KEYSPACE`          | No       | Defaults to `agentregistry`                                      |
| `AGENTREGISTRY_REDIS_ADDR`               | No       | Defaults to `redis:6379`                                         |
| `AGENTREGISTRY_POSTGRES_DSN`             | No       | Defaults to docker compose postgres                              |
| `AGENTREGISTRY_EMBEDDER_URL`             | No       | Defaults to `http://embedder:8000`                               |

Generate a strong JWT secret:

```bash
export AGENTREGISTRY_AUTH_JWT_SECRET=$(openssl rand -hex 32)
```

---

## Monitoring

### Containers

```bash
docker compose ps
docker stats --no-stream
```

### Logs

```bash
docker compose logs -f server
docker compose logs -f embedder
docker compose logs -f server | grep -i "gatekeeper\|score\|approved\|rejected\|WARN\|ERROR"
```

### Gatekeeper mode

```bash
docker compose logs server | grep gatekeeper
# "gatekeeper configured with Anthropic scoring (Groq as fallback)"
# "gatekeeper configured with Groq scoring"
# "gatekeeper running without AI scoring ..."
```

### Health monitor

```bash
docker compose logs -f server | grep -i "health monitor"
# "health monitor started (interval: 5m, max failures: 3)"
# "health monitor: checking N agents"
# "agent X marked INACTIVE after 3 failures"
# "agent X marked ACTIVE (recovered)"
```

### Metrics

```bash
curl http://localhost:9000/metrics

# key metrics:
# sockridge_agents_total{status="active"}
# sockridge_requests_total{operation="publish"}
# sockridge_rate_limit_total
# sockridge_uptime_seconds
```

### Database checks

```bash
# agents in ScyllaDB
docker compose exec scylla cqlsh -e \
  "SELECT agent_id, publisher_id FROM agentregistry.agents;"

# embeddings in Postgres
docker compose exec postgres psql -U agentregistry -c \
  "SELECT COUNT(*) FROM skill_embeddings;"

# webhooks
docker compose exec scylla cqlsh -e \
  "SELECT webhook_id, publisher_id, url FROM agentregistry.webhooks;"

# audit log
docker compose exec scylla cqlsh -e \
  "SELECT publisher_id, action, occurred_at FROM agentregistry.audit_events LIMIT 20;"
```

---

## Rate Limits

| Operation | Limit | Window                    |
| --------- | ----- | ------------------------- |
| publish   | 10    | per hour per publisher    |
| search    | 100   | per minute per IP         |
| resolve   | 50    | per minute per shared key |
| login     | 10    | per minute per publisher  |

---

## Backup

### ScyllaDB

```bash
docker compose exec scylla nodetool snapshot agentregistry
docker cp sockridge-scylla-1:/var/lib/scylla/data/agentregistry \
  ./backups/scylla-$(date +%Y%m%d)
```

### Postgres

```bash
docker compose exec postgres pg_dump -U agentregistry agentregistry \
  > ./backups/postgres-$(date +%Y%m%d).sql
```

### Redis

Cache only — no backup needed.

---

## Disk & Memory

```bash
df -h
free -h
docker system df
sudo du -sh /var/lib/docker
```

### Cleanup

```bash
docker system prune -f          # safe
docker system prune -a -f       # aggressive — re-downloads on next build
```

---

## Common Issues

### Embedder fails healthcheck

```bash
docker compose logs embedder
# add curl: RUN apt-get install -y curl to embedder/Dockerfile
```

### ScyllaDB stuck starting

```bash
docker compose logs scylla | tail -20
# takes 30-60s — just wait
```

### Gatekeeper not using AI scoring

```bash
docker compose exec server env | grep GATEKEEPER
# if empty: keys aren't in .env or docker wasn't restarted
```

### Webhook not delivering

```bash
docker compose logs -f server | grep webhook
# check URL is reachable from VPS
# test: sockridge webhook test --id <id>
```

### JWT expired

```bash
sockridge auth login
```

### "no space left on device" during build

```bash
docker system prune -a --volumes -f
docker compose up -d --build
```

---

## Firewall

```bash
sudo ufw allow 22     # SSH
sudo ufw allow 80     # website
sudo ufw allow 443    # HTTPS
sudo ufw allow 9000   # registry API
sudo ufw enable
sudo ufw status
```

---

## Nginx

```bash
sudo nginx -t
sudo systemctl reload nginx
sudo tail -f /var/log/nginx/error.log
sudo certbot renew --dry-run
```

---

## Scale Notes

Current setup handles ~100 concurrent agents on a 4GB VPS.

- **ScyllaDB** — increase `--memory` and `--smp` in docker-compose.yml
- **Embedder** — switch to `pytorch/pytorch` base image for GPU
- **Postgres** — add ivfflat index when >1000 agents: `CREATE INDEX ON skill_embeddings USING ivfflat (embedding vector_cosine_ops) WITH (lists=100);`
- **Server** — stateless, run multiple replicas behind a load balancer
- **Metrics** — add Prometheus + Grafana for dashboarding at scale
