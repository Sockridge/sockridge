package store

import (
	"context"

	registryv1 "github.com/Sockridge/sockridge/server/gen/go/agentregistry/v1"
)

// AgentStore — backed by ScyllaDB
type AgentStore interface {
	Save(ctx context.Context, agent *registryv1.AgentCard) error
	Get(ctx context.Context, agentID string) (*registryv1.AgentCard, error)
	Update(ctx context.Context, agent *registryv1.AgentCard) error
	SetStatus(ctx context.Context, agentID string, status registryv1.AgentStatus) error
	ListByTags(ctx context.Context, tags []string, limit int, pageToken string) ([]*registryv1.AgentCard, string, error)
}

// CacheStore — backed by Redis
type CacheStore interface {
	GetAgent(ctx context.Context, agentID string) (*registryv1.AgentCard, error)
	SetAgent(ctx context.Context, agent *registryv1.AgentCard) error
	DeleteAgent(ctx context.Context, agentID string) error

	// nonce store for auth challenge-response
	SetNonce(ctx context.Context, publisherID string, nonce string, ttlSecs int) error
	GetNonce(ctx context.Context, publisherID string) (string, error)
	DeleteNonce(ctx context.Context, publisherID string) error
}

// VectorStore — backed by pgvector
type VectorStore interface {
	UpsertEmbeddings(ctx context.Context, agentID string, skillID string, embedding []float32) error
	SemanticSearch(ctx context.Context, query []float32, topK int, minScore float32) ([]*SemanticResult, error)
}

type SemanticResult struct {
	AgentID string
	SkillID string
	Score   float32
}

// PublisherStore — backed by ScyllaDB
type PublisherStore interface {
	SavePublisher(ctx context.Context, publisher *registryv1.PublisherAccount) error
	GetPublisherByID(ctx context.Context, publisherID string) (*registryv1.PublisherAccount, error)
	GetPublisherByHandle(ctx context.Context, handle string) (*registryv1.PublisherAccount, error)
}

// AgreementStore — backed by ScyllaDB
type AgreementStore interface {
	SaveAgreement(ctx context.Context, agreement *registryv1.AccessAgreement) error
	GetAgreement(ctx context.Context, agreementID string) (*registryv1.AccessAgreement, error)
	UpdateAgreement(ctx context.Context, agreement *registryv1.AccessAgreement) error
	ListPendingForReceiver(ctx context.Context, receiverID string) ([]*registryv1.AccessAgreement, error)
	ListActiveForPublisher(ctx context.Context, publisherID string) ([]*registryv1.AccessAgreement, error)
	GetAgreementByKey(ctx context.Context, sharedKey string) (*registryv1.AccessAgreement, error)
}
