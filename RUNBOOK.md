# Runbook

Operations guide for running Sockridge in production.

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

## Deploy

### First time

```bash
# clone
git clone https://github.com/Sockridge/sockridge.git
cd sockridge

# set env vars
export AGENTREGISTRY_GATEKEEPER_GROQ_KEY=gsk_...

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
git log --oneline -10       # find previous commit
git checkout <commit-hash>
docker compose up -d --build
```

---

## Environment variables

| Variable                            | Required | Description                                                                 |
| ----------------------------------- | -------- | --------------------------------------------------------------------------- |
| `AGENTREGISTRY_GATEKEEPER_GROQ_KEY` | No       | Groq API key for AI scoring. Without it, agents auto-approve with score 0.5 |
| `AGENTREGISTRY_SCYLLA_HOSTS`        | No       | Defaults to `scylla:9042`                                                   |
| `AGENTREGISTRY_SCYLLA_KEYSPACE`     | No       | Defaults to `agentregistry`                                                 |
| `AGENTREGISTRY_REDIS_ADDR`          | No       | Defaults to `redis:6379`                                                    |
| `AGENTREGISTRY_POSTGRES_DSN`        | No       | Defaults to docker compose postgres                                         |
| `AGENTREGISTRY_EMBEDDER_URL`        | No       | Defaults to `http://embedder:8000`                                          |
| `AGENTREGISTRY_AUTH_JWT_SECRET`     | No       | Defaults to `dev-secret` — **change in production**                         |

Set a strong JWT secret in production:

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
```

### Check gatekeeper is running

```bash
docker compose logs server | grep gatekeeper
# should show: "gatekeeper configured with AI scoring"
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

## Disk usage

```bash
df -h
docker system df
du -sh /var/lib/docker
```

### Clean up unused Docker resources

```bash
# safe — removes stopped containers, dangling images, unused networks
docker system prune -f

# aggressive — removes all unused images (will re-download on next deploy)
docker system prune -a -f
```

---

## Common issues

### Embedder fails healthcheck

```bash
docker compose logs embedder
# if "curl not found":
# add RUN apt-get install -y curl to embedder/Dockerfile
```

### ScyllaDB stuck in starting

ScyllaDB takes 30-60s to start. Wait and check:

```bash
docker compose logs scylla | tail -20
```

### Server can't connect to ScyllaDB

```bash
# check ScyllaDB is healthy
docker compose ps scylla

# check keyspace was created
docker compose exec scylla cqlsh -e "DESCRIBE KEYSPACES;"
```

### Semantic search returns no results

```bash
# check if ivfflat index exists — drop it if you have < 1000 agents
docker compose exec postgres psql -U agentregistry -c \
  "DROP INDEX IF EXISTS skill_embeddings_embedding_idx;"

# check embeddings exist
docker compose exec postgres psql -U agentregistry -c \
  "SELECT COUNT(*) FROM skill_embeddings;"
```

### JWT expired / auth failing

Sessions expire after 1 hour. Re-login:

```bash
sockridge auth login
```

---

## Firewall

```bash
# check current rules
sudo ufw status

# required open ports
sudo ufw allow 22    # SSH
sudo ufw allow 80    # website
sudo ufw allow 443   # website HTTPS
sudo ufw allow 9000  # registry API

sudo ufw enable
```

---

## Nginx (website only)

```bash
# test config
sudo nginx -t

# reload
sudo systemctl reload nginx

# check logs
sudo tail -f /var/log/nginx/access.log
sudo tail -f /var/log/nginx/error.log
```

### SSL renewal

Certbot auto-renews. Test renewal:

```bash
sudo certbot renew --dry-run
```

---

## Scale notes

Current setup handles ~100 concurrent agents comfortably on a 4GB VPS. When you need more:

- ScyllaDB: increase `--memory` flag and `--smp` (CPU cores) in docker-compose.yml
- Embedder: add GPU support by switching to `pytorch/pytorch` base image
- Postgres: add ivfflat index once you have >1000 agents in `skill_embeddings`
- Server: stateless — run multiple replicas behind a load balancer
