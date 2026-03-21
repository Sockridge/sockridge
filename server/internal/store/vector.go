package store

import (
	"context"
	"fmt"

	"github.com/Sockridge/sockridge/server/internal/config"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pgvector/pgvector-go"
)

type VectorStoreImpl struct {
	pool *pgxpool.Pool
}

func NewVectorStore(cfg config.PostgresConfig) (*VectorStoreImpl, error) {
	pool, err := pgxpool.New(context.Background(), cfg.DSN)
	if err != nil {
		return nil, fmt.Errorf("connecting to postgres: %w", err)
	}

	if err := pool.Ping(context.Background()); err != nil {
		return nil, fmt.Errorf("pinging postgres: %w", err)
	}

	return &VectorStoreImpl{pool: pool}, nil
}

func (v *VectorStoreImpl) Close() {
	v.pool.Close()
}

func (v *VectorStoreImpl) UpsertEmbeddings(ctx context.Context, agentID string, skillID string, embedding []float32) error {
	if len(embedding) != 384 {
		return fmt.Errorf("expected 384-dim embedding, got %d", len(embedding))
	}

	_, err := v.pool.Exec(ctx, `
		INSERT INTO skill_embeddings (agent_id, skill_id, embedding)
		VALUES ($1, $2, $3)
		ON CONFLICT (agent_id, skill_id)
		DO UPDATE SET embedding = EXCLUDED.embedding, created_at = now()`,
		agentID, skillID, pgvector.NewVector(embedding),
	)
	if err != nil {
		return fmt.Errorf("upserting embedding for skill %q: %w", skillID, err)
	}
	return nil
}

func (v *VectorStoreImpl) SemanticSearch(ctx context.Context, query []float32, topK int, minScore float32) ([]*SemanticResult, error) {
	if len(query) != 384 {
		return nil, fmt.Errorf("expected 384-dim query vector, got %d", len(query))
	}
	if topK <= 0 {
		topK = 10
	}

	// acquire a connection so we can SET for this session
	conn, err := v.pool.Acquire(ctx)
	if err != nil {
		return nil, fmt.Errorf("acquiring connection: %w", err)
	}
	defer conn.Release()

	// disable index scan — forces exact cosine search, correct for small datasets
	// remove when you have >1000 agents and re-add the ivfflat index
	if _, err := conn.Exec(ctx, "SET enable_indexscan = off"); err != nil {
		return nil, fmt.Errorf("disabling index scan: %w", err)
	}

	rows, err := conn.Query(ctx, `
		SELECT
			agent_id,
			skill_id,
			1 - (embedding <=> $1) AS score
		FROM skill_embeddings
		WHERE 1 - (embedding <=> $1) >= $2
		ORDER BY embedding <=> $1
		LIMIT $3`,
		pgvector.NewVector(query),
		minScore,
		topK,
	)
	if err != nil {
		return nil, fmt.Errorf("semantic search query: %w", err)
	}
	defer rows.Close()

	var results []*SemanticResult
	for rows.Next() {
		r := &SemanticResult{}
		if err := rows.Scan(&r.AgentID, &r.SkillID, &r.Score); err != nil {
			return nil, fmt.Errorf("scanning result: %w", err)
		}
		results = append(results, r)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating results: %w", err)
	}

	return results, nil
}

func (v *VectorStoreImpl) DeleteEmbeddings(ctx context.Context, agentID string) error {
	_, err := v.pool.Exec(ctx,
		`DELETE FROM skill_embeddings WHERE agent_id = $1`, agentID,
	)
	if err != nil {
		return fmt.Errorf("deleting embeddings for agent %q: %w", agentID, err)
	}
	return nil
}