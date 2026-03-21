package registry

import (
	"context"
	"fmt"

	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	registryv1 "github.com/Sockridge/sockridge/server/gen/go/agentregistry/v1"
	"github.com/Sockridge/sockridge/server/gen/go/agentregistry/v1/registryv1connect"
	"github.com/Sockridge/sockridge/server/internal/audit"
	"github.com/Sockridge/sockridge/server/internal/auth"
	"github.com/Sockridge/sockridge/server/internal/embedder"
	"github.com/Sockridge/sockridge/server/internal/gatekeeper"
	"github.com/Sockridge/sockridge/server/internal/ratelimit"
	"github.com/Sockridge/sockridge/server/internal/store"
	"github.com/Sockridge/sockridge/server/middleware"
)

type Service struct {
	agents      store.AgentStore
	publishers  store.PublisherStore
	cache       store.CacheStore
	vectors     store.VectorStore
	auth        *auth.Service
	embedder    *embedder.Client
	gatekeeper  *gatekeeper.Service
	rateLimiter *ratelimit.Limiter
	audit       *audit.Service
}

func New(
	agents store.AgentStore,
	publishers store.PublisherStore,
	cache store.CacheStore,
	vectors store.VectorStore,
	authSvc *auth.Service,
	embedderClient *embedder.Client,
	gateKeeperSvc *gatekeeper.Service,
	rateLimiter *ratelimit.Limiter,
	auditSvc *audit.Service,
) *Service {
	return &Service{
		agents:      agents,
		publishers:  publishers,
		cache:       cache,
		vectors:     vectors,
		auth:        authSvc,
		embedder:    embedderClient,
		gatekeeper:  gateKeeperSvc,
		rateLimiter: rateLimiter,
		audit:       auditSvc,
	}
}

var _ registryv1connect.RegistryServiceHandler = (*Service)(nil)

func (s *Service) RegisterPublisher(
	ctx context.Context,
	req *connect.Request[registryv1.RegisterPublisherRequest],
) (*connect.Response[registryv1.RegisterPublisherResponse], error) {
	if req.Msg.Handle == "" || req.Msg.PublicKey == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("handle and public_key are required"))
	}

	existing, _ := s.publishers.GetPublisherByHandle(ctx, req.Msg.Handle)
	if existing != nil {
		return nil, connect.NewError(connect.CodeAlreadyExists, fmt.Errorf("handle %q is already taken", req.Msg.Handle))
	}

	publisher := &registryv1.PublisherAccount{
		Id:        uuid.NewString(),
		Handle:    req.Msg.Handle,
		PublicKey: req.Msg.PublicKey,
		CreatedAt: timestamppb.New(time.Now()),
	}

	if err := s.publishers.SavePublisher(ctx, publisher); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("saving publisher: %w", err))
	}

	return connect.NewResponse(&registryv1.RegisterPublisherResponse{
		PublisherId: publisher.Id,
		Account:     publisher,
	}), nil
}

func (s *Service) AuthChallenge(
	ctx context.Context,
	req *connect.Request[registryv1.AuthChallengeRequest],
) (*connect.Response[registryv1.AuthChallengeResponse], error) {
	if req.Msg.PublisherId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("publisher_id is required"))
	}

	if _, err := s.publishers.GetPublisherByID(ctx, req.Msg.PublisherId); err != nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("publisher not found"))
	}

	nonce, expiresAt, err := s.auth.Challenge(ctx, req.Msg.PublisherId)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	return connect.NewResponse(&registryv1.AuthChallengeResponse{
		Nonce:     nonce,
		ExpiresAt: timestamppb.New(expiresAt),
	}), nil
}

func (s *Service) AuthVerify(
	ctx context.Context,
	req *connect.Request[registryv1.AuthVerifyRequest],
) (*connect.Response[registryv1.AuthVerifyResponse], error) {
	if req.Msg.PublisherId == "" || req.Msg.Nonce == "" || len(req.Msg.Signature) == 0 {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("publisher_id, nonce, and signature are required"))
	}

	publisher, err := s.publishers.GetPublisherByID(ctx, req.Msg.PublisherId)
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("publisher not found"))
	}

	token, err := s.auth.Verify(ctx, req.Msg.PublisherId, publisher.PublicKey, req.Msg.Signature)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, err)
	}

	return connect.NewResponse(&registryv1.AuthVerifyResponse{
		SessionToken: token,
		ExpiresAt:    timestamppb.New(time.Now().Add(time.Hour)),
	}), nil
}

func (s *Service) PublishAgent(
	ctx context.Context,
	req *connect.Request[registryv1.PublishAgentRequest],
) (*connect.Response[registryv1.PublishAgentResponse], error) {
	publisherID, ok := middleware.PublisherIDFromCtx(ctx)
	if !ok {
		return nil, connect.NewError(connect.CodeUnauthenticated, fmt.Errorf("missing publisher identity"))
	}

	// rate limit — 10 publishes per hour per publisher
	if s.rateLimiter != nil {
		allowed, rlErr := s.rateLimiter.Allow(ctx, ratelimit.PublishRule(publisherID))
		if rlErr == nil && !allowed {
			return nil, connect.NewError(connect.CodeResourceExhausted,
				fmt.Errorf("publish rate limit exceeded (max 10/hour)"))
		}
	}

	agent, err := s.unpackSignedAgent(ctx, publisherID, req.Msg.Payload)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	agent.Id = uuid.NewString()
	agent.PublisherId = publisherID
	// start as PENDING — gatekeeper will update to ACTIVE or REJECTED
	agent.Status = registryv1.AgentStatus_AGENT_STATUS_PENDING

	if err := s.agents.Save(ctx, agent); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("saving agent: %w", err))
	}

	_ = s.cache.SetAgent(ctx, agent)
	if err := s.embedSkills(ctx, agent); err != nil {
		fmt.Printf("[WARN] embedSkills failed for agent %s: %v\n", agent.Id, err)
	}

	// run gatekeeper async — updates status to ACTIVE or REJECTED
	go s.runGatekeeper(agent)

	// audit log
	if s.audit != nil {
		ip := req.Header().Get("X-Real-IP")
		if ip == "" {
			ip = req.Header().Get("X-Forwarded-For")
		}
		s.audit.Log(ctx, agent.PublisherId, audit.ActionPublish, agent.Id, "", ip,
			fmt.Sprintf("published agent %q v%s", agent.Name, agent.Version))
	}

	return connect.NewResponse(&registryv1.PublishAgentResponse{
		AgentId: agent.Id,
		Agent:   agent,
	}), nil
}

func (s *Service) UpdateAgent(
	ctx context.Context,
	req *connect.Request[registryv1.UpdateAgentRequest],
) (*connect.Response[registryv1.UpdateAgentResponse], error) {
	publisherID, ok := middleware.PublisherIDFromCtx(ctx)
	if !ok {
		return nil, connect.NewError(connect.CodeUnauthenticated, fmt.Errorf("missing publisher identity"))
	}

	agent, err := s.unpackSignedAgent(ctx, publisherID, req.Msg.Payload)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	existing, err := s.agents.Get(ctx, agent.Id)
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("agent not found"))
	}
	if existing.PublisherId != publisherID {
		return nil, connect.NewError(connect.CodePermissionDenied, fmt.Errorf("agent belongs to a different publisher"))
	}

	agent.PublisherId = publisherID
	if err := s.agents.Update(ctx, agent); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("updating agent: %w", err))
	}

	_ = s.cache.DeleteAgent(ctx, agent.Id)
	_ = s.embedSkills(ctx, agent)

	return connect.NewResponse(&registryv1.UpdateAgentResponse{Agent: agent}), nil
}

func (s *Service) DeprecateAgent(
	ctx context.Context,
	req *connect.Request[registryv1.DeprecateAgentRequest],
) (*connect.Response[registryv1.DeprecateAgentResponse], error) {
	publisherID, ok := middleware.PublisherIDFromCtx(ctx)
	if !ok {
		return nil, connect.NewError(connect.CodeUnauthenticated, fmt.Errorf("missing publisher identity"))
	}

	existing, err := s.agents.Get(ctx, req.Msg.AgentId)
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("agent not found"))
	}
	if existing.PublisherId != publisherID {
		return nil, connect.NewError(connect.CodePermissionDenied, fmt.Errorf("agent belongs to a different publisher"))
	}

	if err := s.agents.SetStatus(ctx, req.Msg.AgentId, registryv1.AgentStatus_AGENT_STATUS_DEPRECATED); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	_ = s.cache.DeleteAgent(ctx, req.Msg.AgentId)
	existing.Status = registryv1.AgentStatus_AGENT_STATUS_DEPRECATED
	return connect.NewResponse(&registryv1.DeprecateAgentResponse{Agent: existing}), nil
}

func (s *Service) GetAgent(
	ctx context.Context,
	req *connect.Request[registryv1.RegistryServiceGetAgentRequest],
) (*connect.Response[registryv1.RegistryServiceGetAgentResponse], error) {
	agent, err := s.getAgentCached(ctx, req.Msg.AgentId)
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, err)
	}
	// only the owning publisher sees their own URL
	publisherID, _ := middleware.PublisherIDFromCtx(ctx)
	if agent.PublisherId != publisherID {
		agent = stripURL(agent)
	}
	return connect.NewResponse(&registryv1.RegistryServiceGetAgentResponse{Agent: agent}), nil
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func (s *Service) unpackSignedAgent(ctx context.Context, publisherID string, signed *registryv1.SignedPayload) (*registryv1.AgentCard, error) {
	if signed == nil || len(signed.Payload) == 0 {
		return nil, fmt.Errorf("empty payload")
	}

	publisher, err := s.publishers.GetPublisherByID(ctx, publisherID)
	if err != nil {
		return nil, fmt.Errorf("publisher not found")
	}

	if err := auth.VerifySignature(publisher.PublicKey, signed.Payload, signed.Signature); err != nil {
		return nil, fmt.Errorf("invalid payload signature: %w", err)
	}

	var agent registryv1.AgentCard

	// try proto binary first, fall back to JSON (used by Python/JS SDKs)
	if err := proto.Unmarshal(signed.Payload, &agent); err != nil {
		if jsonErr := protojson.Unmarshal(signed.Payload, &agent); jsonErr != nil {
			return nil, fmt.Errorf("unmarshaling agent payload: proto: %v, json: %v", err, jsonErr)
		}
	}
	return &agent, nil
}

func (s *Service) getAgentCached(ctx context.Context, agentID string) (*registryv1.AgentCard, error) {
	if cached, err := s.cache.GetAgent(ctx, agentID); err == nil {
		return cached, nil
	}
	agent, err := s.agents.Get(ctx, agentID)
	if err != nil {
		return nil, err
	}
	_ = s.cache.SetAgent(ctx, agent)
	return agent, nil
}

// embedSkills calls the embedder sidecar to generate vectors for each skill
// and stores them in pgvector for semantic search.
func (s *Service) embedSkills(ctx context.Context, agent *registryv1.AgentCard) error {
	if s.embedder == nil {
		return nil
	}

	for _, skill := range agent.Skills {
		text := skill.Name + " — " + skill.Description
		vec, err := s.embedder.EmbedOne(ctx, text)
		if err != nil {
			return fmt.Errorf("embedding skill %q: %w", skill.Id, err)
		}
		if err := s.vectors.UpsertEmbeddings(ctx, agent.Id, skill.Id, vec); err != nil {
			return fmt.Errorf("storing embedding for skill %q: %w", skill.Id, err)
		}
	}
	return nil
}

// stripURL removes the endpoint URL before returning to non-agreement callers.
func stripURL(agent *registryv1.AgentCard) *registryv1.AgentCard {
	if agent == nil {
		return nil
	}
	copy := *agent
	copy.Url = ""
	copy.Interfaces = nil
	return &copy
}

// runGatekeeper runs the full validation pipeline in a background goroutine.
// Updates the agent status to ACTIVE or REJECTED based on the result.
func (s *Service) runGatekeeper(agent *registryv1.AgentCard) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if s.gatekeeper == nil {
		// no gatekeeper configured — auto-approve
		agent.Status = registryv1.AgentStatus_AGENT_STATUS_ACTIVE
		_ = s.agents.Update(ctx, agent)
		_ = s.cache.DeleteAgent(ctx, agent.Id)
		return
	}

	result, err := s.gatekeeper.Evaluate(ctx, agent)
	if err != nil {
		fmt.Printf("[WARN] gatekeeper evaluation failed for agent %s: %v\n", agent.Id, err)
		// fail open — approve rather than leave stuck in PENDING
		agent.Status = registryv1.AgentStatus_AGENT_STATUS_ACTIVE
	} else {
		agent.GatekeeperResult = result
		if result.Approved {
			agent.Status = registryv1.AgentStatus_AGENT_STATUS_ACTIVE
		} else {
			agent.Status = registryv1.AgentStatus_AGENT_STATUS_REJECTED
		}
	}

	_ = s.agents.Update(ctx, agent)
	_ = s.cache.DeleteAgent(ctx, agent.Id)

	fmt.Printf("[INFO] gatekeeper result for agent %s: approved=%v score=%.2f reason=%q\n",
		agent.Id, agent.Status == registryv1.AgentStatus_AGENT_STATUS_ACTIVE,
		func() float32 {
			if agent.GatekeeperResult != nil {
				return agent.GatekeeperResult.ConfidenceScore
			}
			return 0
		}(),
		func() string {
			if agent.GatekeeperResult != nil {
				return agent.GatekeeperResult.Reason
			}
			return "no gatekeeper"
		}(),
	)
}