# Bootstrap runbook
# Run these from the agentregistry/ root unless noted otherwise.

# ── 1. Install tools ──────────────────────────────────────────────────────────

brew install bufbuild/buf/buf
brew install go          # need 1.23+
brew install docker      # or Docker Desktop

# ── 2. Install buf plugins for Go + TypeScript codegen ───────────────────────

go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
go install connectrpc.com/connect/cmd/protoc-gen-connect-go@latest

# make sure $GOPATH/bin is in your PATH
export PATH="$PATH:$(go env GOPATH)/bin"

# ── 3. Generate Go code from proto ───────────────────────────────────────────

buf dep update        # resolves googleapis dependency in buf.yaml
buf generate          # writes gen/go/v1/*.go and gen/ts/v1/*.ts

# you should now see:
#   gen/go/v1/agent.pb.go
#   gen/go/v1/registry.pb.go
#   gen/go/v1/registryv1connect/registry.connect.go

# ── 4. Tidy Go modules ───────────────────────────────────────────────────────

# from agentregistry/ root:
cd server && go mod tidy && cd ..
cd cli    && go mod tidy && cd ..
# OR equivalently from root:
# go work sync

# ── 5. Verify it compiles ─────────────────────────────────────────────────────

cd server && go build ./... && cd ..
cd cli    && go build ./... && cd ..

# ── 6. Run auth tests ─────────────────────────────────────────────────────────

cd server && go test ./internal/auth/... -v && cd ..

# expected output:
#   --- PASS: TestChallengeVerify_HappyPath
#   --- PASS: TestVerify_WrongSignature
#   --- PASS: TestVerify_NonceIsOneTimeUse

# ── 7. Spin up the full stack ─────────────────────────────────────────────────

docker compose up --build

# waits for:
#   scylla  → healthy (takes ~30s on first boot)
#   redis   → healthy
#   postgres → healthy
#   server  → starts on :9000

# ── 8. Smoke test end-to-end ─────────────────────────────────────────────────

# build the CLI first
cd cli && go build -o ../bin/agentctl . && cd ..
export PATH="$PATH:$(pwd)/bin"

# generate keypair
agentctl auth keygen

# register publisher
agentctl auth register --handle utsav

# login (challenge → sign → JWT)
agentctl auth login

# create a test agent file
cat > /tmp/test-agent.json << 'JSON'
{
  "name": "Test FHIR Agent",
  "description": "Analyzes lab trends from FHIR",
  "version": "0.1.0",
  "protocol_version": "0.3.0",
  "skills": [
    {
      "id": "lab.analyze",
      "name": "Lab Analyzer",
      "description": "Detects anomalies in lab result trends",
      "tags": ["fhir", "labs", "analysis"]
    }
  ],
  "capabilities": {
    "streaming": true,
    "tool_use": true
  }
}
JSON

# publish it
agentctl publish --file /tmp/test-agent.json

# list agents
agentctl search list

# watch for new agents in another terminal
agentctl search watch --tag fhir

# ── 9. Known issues to watch for ─────────────────────────────────────────────

# Issue: ScyllaDB takes ~30s to be ready on first boot
# Fix:   docker compose up already has healthcheck retries — just wait

# Issue: buf generate fails with "plugin not found"
# Fix:   make sure $GOPATH/bin is in PATH, re-run: go install ... for each plugin

# Issue: go mod tidy fails on "cannot find module"
# Fix:   run buf generate first — gen/go/v1 must exist before tidy

# Issue: "transport: http2: frame too large" on gRPC calls
# Fix:   make sure you're hitting port 9000, not 8080
