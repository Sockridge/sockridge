package store

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	registryv1 "github.com/utsav-develops/SocialAgents/server/gen/go/agentregistry/v1"
	"github.com/utsav-develops/SocialAgents/server/internal/config"
	"google.golang.org/protobuf/proto"
)

const (
	agentKeyPrefix = "agent:"
	nonceKeyPrefix = "nonce:"
	agentCacheTTL  = 5 * time.Minute
)

// RedisStore implements CacheStore.
// Agents are cached as serialized proto bytes — same wire format as ScyllaDB.
// Nonces are stored as plain strings with a short TTL.
type RedisStore struct {
	client *redis.Client
}

func NewRedisStore(cfg config.RedisConfig) (*RedisStore, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     cfg.Addr,
		Password: cfg.Password,
		DB:       cfg.DB,
	})

	// verify connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("connecting to redis: %w", err)
	}

	return &RedisStore{client: client}, nil
}

func (r *RedisStore) Close() error {
	return r.client.Close()
}

// ── Agent cache ───────────────────────────────────────────────────────────────

func (r *RedisStore) GetAgent(ctx context.Context, agentID string) (*registryv1.AgentCard, error) {
	data, err := r.client.Get(ctx, agentKey(agentID)).Bytes()
	if err != nil {
		if err == redis.Nil {
			return nil, fmt.Errorf("cache miss: agent %q", agentID)
		}
		return nil, fmt.Errorf("redis get agent: %w", err)
	}

	var agent registryv1.AgentCard
	if err := proto.Unmarshal(data, &agent); err != nil {
		return nil, fmt.Errorf("unmarshaling cached agent: %w", err)
	}

	return &agent, nil
}

func (r *RedisStore) SetAgent(ctx context.Context, agent *registryv1.AgentCard) error {
	data, err := proto.Marshal(agent)
	if err != nil {
		return fmt.Errorf("marshaling agent for cache: %w", err)
	}

	if err := r.client.Set(ctx, agentKey(agent.Id), data, agentCacheTTL).Err(); err != nil {
		return fmt.Errorf("redis set agent: %w", err)
	}

	return nil
}

func (r *RedisStore) DeleteAgent(ctx context.Context, agentID string) error {
	if err := r.client.Del(ctx, agentKey(agentID)).Err(); err != nil {
		return fmt.Errorf("redis del agent: %w", err)
	}
	return nil
}

// ── Nonce store ───────────────────────────────────────────────────────────────

func (r *RedisStore) SetNonce(ctx context.Context, publisherID string, nonce string, ttlSecs int) error {
	if err := r.client.Set(ctx, nonceKey(publisherID), nonce,
		time.Duration(ttlSecs)*time.Second,
	).Err(); err != nil {
		return fmt.Errorf("redis set nonce: %w", err)
	}
	return nil
}

func (r *RedisStore) GetNonce(ctx context.Context, publisherID string) (string, error) {
	nonce, err := r.client.Get(ctx, nonceKey(publisherID)).Result()
	if err != nil {
		if err == redis.Nil {
			return "", fmt.Errorf("nonce not found or expired for publisher %q", publisherID)
		}
		return "", fmt.Errorf("redis get nonce: %w", err)
	}
	return nonce, nil
}

func (r *RedisStore) DeleteNonce(ctx context.Context, publisherID string) error {
	if err := r.client.Del(ctx, nonceKey(publisherID)).Err(); err != nil {
		return fmt.Errorf("redis del nonce: %w", err)
	}
	return nil
}

// ── helpers ───────────────────────────────────────────────────────────────────

func agentKey(agentID string) string  { return agentKeyPrefix + agentID }
func nonceKey(publisherID string) string { return nonceKeyPrefix + publisherID }
