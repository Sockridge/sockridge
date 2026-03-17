package discovery

import (
	"context"
	"fmt"

	"connectrpc.com/connect"
	"github.com/google/uuid"

	registryv1 "github.com/utsav-develops/SocialAgents/server/gen/go/agentregistry/v1"
	"github.com/utsav-develops/SocialAgents/server/gen/go/agentregistry/v1/registryv1connect"
	"github.com/utsav-develops/SocialAgents/server/internal/embedder"
	"github.com/utsav-develops/SocialAgents/server/internal/store"
)

type Service struct {
	agents      store.AgentStore
	cache       store.CacheStore
	vectors     store.VectorStore
	embedder    *embedder.Client
	broadcaster *Broadcaster
}

func New(agents store.AgentStore, cache store.CacheStore, vectors store.VectorStore, embedderClient *embedder.Client) *Service {
	return &Service{
		agents:      agents,
		cache:       cache,
		vectors:     vectors,
		embedder:    embedderClient,
		broadcaster: NewBroadcaster(),
	}
}

var _ registryv1connect.DiscoveryServiceHandler = (*Service)(nil)

func (s *Service) Broadcast(event *registryv1.WatchResponse) {
	s.broadcaster.Publish(event)
}

func (s *Service) GetAgent(
	ctx context.Context,
	req *connect.Request[registryv1.DiscoveryServiceGetAgentRequest],
) (*connect.Response[registryv1.DiscoveryServiceGetAgentResponse], error) {
	if req.Msg.AgentId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("agent_id is required"))
	}

	if cached, err := s.cache.GetAgent(ctx, req.Msg.AgentId); err == nil {
		return connect.NewResponse(&registryv1.DiscoveryServiceGetAgentResponse{Agent: cached}), nil
	}

	agent, err := s.agents.Get(ctx, req.Msg.AgentId)
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("agent not found"))
	}

	_ = s.cache.SetAgent(ctx, agent)
	return connect.NewResponse(&registryv1.DiscoveryServiceGetAgentResponse{Agent: agent}), nil
}

func (s *Service) ListAgents(
	ctx context.Context,
	req *connect.Request[registryv1.ListAgentsRequest],
	stream *connect.ServerStream[registryv1.ListAgentsResponse],
) error {
	limit := int(req.Msg.Limit)
	if limit <= 0 {
		limit = 20
	}

	agents, nextToken, err := s.agents.ListByTags(ctx, req.Msg.Tags, limit, req.Msg.PageToken)
	if err != nil {
		return connect.NewError(connect.CodeInternal, fmt.Errorf("listing agents: %w", err))
	}

	for i, agent := range agents {
		var token string
		if i == len(agents)-1 {
			token = nextToken
		}
		if err := stream.Send(&registryv1.ListAgentsResponse{
			Agent:         agent,
			NextPageToken: token,
		}); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) SemanticSearch(
	ctx context.Context,
	req *connect.Request[registryv1.SemanticSearchRequest],
	stream *connect.ServerStream[registryv1.SemanticSearchResponse],
) error {
	if req.Msg.Query == "" {
		return connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("query is required"))
	}
	if s.embedder == nil {
		return connect.NewError(connect.CodeUnimplemented, fmt.Errorf("embedder not configured"))
	}

	// embed the query text
	queryVec, err := s.embedder.EmbedOne(ctx, req.Msg.Query)
	if err != nil {
		return connect.NewError(connect.CodeInternal, fmt.Errorf("embedding query: %w", err))
	}

	topK := int(req.Msg.TopK)
	if topK <= 0 {
		topK = 10
	}

	minScore := req.Msg.MinScore
	if minScore <= 0 {
		minScore = 0.1
	}

	// ANN search in pgvector
	results, err := s.vectors.SemanticSearch(ctx, queryVec, topK, minScore)
	if err != nil {
		return connect.NewError(connect.CodeInternal, fmt.Errorf("vector search: %w", err))
	}

	// fetch agent for each result and stream back
	seen := make(map[string]struct{})
	for _, r := range results {
		if _, ok := seen[r.AgentID]; ok {
			continue // deduplicate agents with multiple matching skills
		}
		seen[r.AgentID] = struct{}{}

		agent, err := s.getAgentCached(ctx, r.AgentID)
		if err != nil {
			continue
		}

		if err := stream.Send(&registryv1.SemanticSearchResponse{
			Agent: agent,
			Score: r.Score,
		}); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) Watch(
	ctx context.Context,
	req *connect.Request[registryv1.WatchRequest],
	stream *connect.ServerStream[registryv1.WatchResponse],
) error {
	subID := uuid.NewString()
	ch := s.broadcaster.Subscribe(subID)
	defer s.broadcaster.Unsubscribe(subID)

	for {
		select {
		case <-ctx.Done():
			return nil
		case event, ok := <-ch:
			if !ok {
				return nil
			}
			if !s.matchesFilter(event.Agent, req.Msg.Tags, req.Msg.Capabilities) {
				continue
			}
			if err := stream.Send(event); err != nil {
				return err
			}
		}
	}
}

func (s *Service) getAgentCached(ctx context.Context, agentID string) (*registryv1.AgentCard, error) {
	if cached, err := s.cache.GetAgent(ctx, agentID); err == nil {
		return cached, nil
	}
	return s.agents.Get(ctx, agentID)
}

func (s *Service) matchesFilter(agent *registryv1.AgentCard, tags []string, capabilities []string) bool {
	if len(tags) == 0 && len(capabilities) == 0 {
		return true
	}

	agentTags := make(map[string]struct{})
	for _, skill := range agent.Skills {
		for _, tag := range skill.Tags {
			agentTags[tag] = struct{}{}
		}
	}
	for _, t := range tags {
		if _, ok := agentTags[t]; ok {
			return true
		}
	}

	if agent.Capabilities != nil {
		for _, cap := range capabilities {
			switch cap {
			case "streaming":
				if agent.Capabilities.Streaming {
					return true
				}
			case "push_notifications":
				if agent.Capabilities.PushNotifications {
					return true
				}
			case "multi_turn":
				if agent.Capabilities.MultiTurn {
					return true
				}
			case "tool_use":
				if agent.Capabilities.ToolUse {
					return true
				}
			}
		}
	}
	return false
}