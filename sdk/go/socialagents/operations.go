package socialagents

import (
	"context"
	"crypto/ed25519"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"connectrpc.com/connect"
	registryv1 "github.com/Sockridge/sockridge/server/gen/go/agentregistry/v1"
	"google.golang.org/protobuf/proto"
)

// ── Publish ───────────────────────────────────────────────────────────────────

// Publish signs and publishes an AgentCard to the registry.
func (r *Registry) Publish(ctx context.Context, card *AgentCard) (*AgentCard, error) {
	if r.token == "" {
		return nil, fmt.Errorf("not authenticated — call Login() first")
	}

	protoCard := toProto(card)

	payload, err := proto.Marshal(protoCard)
	if err != nil {
		return nil, fmt.Errorf("serializing agent card: %w", err)
	}

	home, _ := os.UserHomeDir()
	keyPath := filepath.Join(home, ".sockridge", "ed25519.key")
	privKey, err := loadPrivateKey(keyPath)
	if err != nil {
		return nil, fmt.Errorf("loading private key: %w", err)
	}

	sig := ed25519.Sign(privKey, payload)

	resp, err := r.authRegistry().PublishAgent(ctx, connect.NewRequest(&registryv1.PublishAgentRequest{
		Payload: &registryv1.SignedPayload{
			Payload:   payload,
			Signature: sig,
			KeyId:     r.publisherID,
		},
	}))
	if err != nil {
		return nil, fmt.Errorf("publishing agent: %w", err)
	}

	return fromProto(resp.Msg.Agent), nil
}

// ── Discovery ─────────────────────────────────────────────────────────────────

// Search lists agents by tags.
func (r *Registry) Search(ctx context.Context, tags []string, limit uint32) ([]*AgentCard, error) {
	if limit == 0 {
		limit = 20
	}

	stream, err := r.discovery.ListAgents(ctx, connect.NewRequest(&registryv1.ListAgentsRequest{
		Tags:  tags,
		Limit: limit,
	}))
	if err != nil {
		return nil, fmt.Errorf("listing agents: %w", err)
	}

	var agents []*AgentCard
	for stream.Receive() {
		agents = append(agents, fromProto(stream.Msg().Agent))
	}
	if err := stream.Err(); err != nil && err != io.EOF {
		return nil, fmt.Errorf("stream error: %w", err)
	}
	return agents, nil
}

// SemanticSearch finds agents by natural language query.
func (r *Registry) SemanticSearch(ctx context.Context, query string, topK uint32, minScore float32) ([]*SearchResult, error) {
	if topK == 0 {
		topK = 10
	}
	if minScore == 0 {
		minScore = 0.1
	}

	stream, err := r.discovery.SemanticSearch(ctx, connect.NewRequest(&registryv1.SemanticSearchRequest{
		Query:    query,
		TopK:     topK,
		MinScore: minScore,
	}))
	if err != nil {
		return nil, fmt.Errorf("semantic search: %w", err)
	}

	var results []*SearchResult
	for stream.Receive() {
		msg := stream.Msg()
		results = append(results, &SearchResult{
			Agent: fromProto(msg.Agent),
			Score: msg.Score,
		})
	}
	if err := stream.Err(); err != nil && err != io.EOF {
		return nil, fmt.Errorf("stream error: %w", err)
	}
	return results, nil
}

// GetAgent fetches a single agent by ID.
func (r *Registry) GetAgent(ctx context.Context, agentID string) (*AgentCard, error) {
	resp, err := r.discovery.GetAgent(ctx, connect.NewRequest(&registryv1.DiscoveryServiceGetAgentRequest{
		AgentId: agentID,
	}))
	if err != nil {
		return nil, fmt.Errorf("getting agent: %w", err)
	}
	return fromProto(resp.Msg.Agent), nil
}

// ── Access Agreements ─────────────────────────────────────────────────────────

// RequestAccess sends a mutual access request to another publisher.
func (r *Registry) RequestAccess(ctx context.Context, receiverID, message string) (*Agreement, error) {
	resp, err := r.authAccess().RequestAccess(ctx, connect.NewRequest(&registryv1.RequestAccessRequest{
		RequesterId: r.publisherID,
		ReceiverId:  receiverID,
		Message:     message,
	}))
	if err != nil {
		return nil, fmt.Errorf("requesting access: %w", err)
	}
	return fromProtoAgreement(resp.Msg.Agreement), nil
}

// ApproveAccess approves a pending request and returns the shared key.
func (r *Registry) ApproveAccess(ctx context.Context, agreementID string) (string, error) {
	resp, err := r.authAccess().ApproveAccess(ctx, connect.NewRequest(&registryv1.ApproveAccessRequest{
		PublisherId: r.publisherID,
		AgreementId: agreementID,
	}))
	if err != nil {
		return "", fmt.Errorf("approving access: %w", err)
	}
	return resp.Msg.SharedKey, nil
}

// RevokeAccess revokes an active agreement.
func (r *Registry) RevokeAccess(ctx context.Context, agreementID string) error {
	_, err := r.authAccess().RevokeAccess(ctx, connect.NewRequest(&registryv1.RevokeAccessRequest{
		PublisherId: r.publisherID,
		AgreementId: agreementID,
	}))
	return err
}

// ResolveEndpoint reveals an agent's URL using a shared key.
func (r *Registry) ResolveEndpoint(ctx context.Context, agentID, sharedKey string) (*ResolvedEndpoint, error) {
	resp, err := r.access.ResolveEndpoint(ctx, connect.NewRequest(&registryv1.ResolveEndpointRequest{
		AgentId:   agentID,
		SharedKey: sharedKey,
	}))
	if err != nil {
		return nil, fmt.Errorf("resolving endpoint: %w", err)
	}

	return &ResolvedEndpoint{
		URL:       resp.Msg.Url,
		Transport: resp.Msg.Transport,
		Agent:     fromProto(resp.Msg.Agent),
	}, nil
}

// ── helpers ───────────────────────────────────────────────────────────────────

func fromProtoAgreement(a *registryv1.AccessAgreement) *Agreement {
	if a == nil {
		return nil
	}
	return &Agreement{
		ID:          a.Id,
		RequesterID: a.RequesterId,
		ReceiverID:  a.ReceiverId,
		Message:     a.Message,
		Status:      a.Status.String(),
		SharedKey:   a.SharedKey,
	}
}
