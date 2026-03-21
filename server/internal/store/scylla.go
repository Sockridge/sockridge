package store

import (
	"context"
	"fmt"
	"time"

	registryv1 "github.com/Sockridge/sockridge/server/gen/go/agentregistry/v1"
	"github.com/Sockridge/sockridge/server/internal/config"
	"github.com/gocql/gocql"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type ScyllaStore struct {
	session *gocql.Session
}

func NewScyllaStore(cfg config.ScyllaConfig) (*ScyllaStore, error) {
	// step 1: connect WITHOUT keyspace to create it if it doesn't exist
	cluster := gocql.NewCluster(cfg.Hosts...)
	cluster.Consistency = gocql.Quorum
	cluster.Timeout = 10 * time.Second
	cluster.ConnectTimeout = 15 * time.Second

	if cfg.Username != "" {
		cluster.Authenticator = gocql.PasswordAuthenticator{
			Username: cfg.Username,
			Password: cfg.Password,
		}
	}

	initSession, err := cluster.CreateSession()
	if err != nil {
		return nil, fmt.Errorf("connecting to scylla: %w", err)
	}

	// create keyspace if not exists
	err = initSession.Query(fmt.Sprintf(`
		CREATE KEYSPACE IF NOT EXISTS %s
		WITH replication = {'class': 'SimpleStrategy', 'replication_factor': 1}`,
		cfg.Keyspace,
	)).Exec()
	initSession.Close()
	if err != nil {
		return nil, fmt.Errorf("creating keyspace: %w", err)
	}

	// step 2: reconnect WITH keyspace
	cluster.Keyspace = cfg.Keyspace
	session, err := cluster.CreateSession()
	if err != nil {
		return nil, fmt.Errorf("connecting to scylla with keyspace: %w", err)
	}

	return &ScyllaStore{session: session}, nil
}

func (s *ScyllaStore) Session() *gocql.Session {
	return s.session
}

func (s *ScyllaStore) Close() {
	s.session.Close()
}

func (s *ScyllaStore) CreateSchema(ctx context.Context, keyspace string) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS agents (
			agent_id     text PRIMARY KEY,
			publisher_id text,
			status       int,
			data         blob,
			created_at   timestamp,
			updated_at   timestamp
		)`,
		`CREATE INDEX IF NOT EXISTS agents_publisher_idx ON agents (publisher_id)`,
		`CREATE TABLE IF NOT EXISTS publishers (
			publisher_id text PRIMARY KEY,
			handle       text,
			data         blob
		)`,
		`CREATE TABLE IF NOT EXISTS publisher_handles (
			handle       text PRIMARY KEY,
			publisher_id text
		)`,
		`CREATE TABLE IF NOT EXISTS agent_tags (
			tag      text,
			agent_id text,
			PRIMARY KEY (tag, agent_id)
		)`,
	}

	for _, stmt := range stmts {
		if err := s.session.Query(stmt).WithContext(ctx).Exec(); err != nil {
			return fmt.Errorf("executing schema stmt: %w\nstmt: %s", err, stmt)
		}
	}
	return s.CreateAgreementSchema(ctx)
}

// ── AgentStore ────────────────────────────────────────────────────────────────

func (s *ScyllaStore) Save(ctx context.Context, agent *registryv1.AgentCard) error {
	now := time.Now()
	agent.CreatedAt = timestamppb.New(now)
	agent.UpdatedAt = timestamppb.New(now)

	data, err := proto.Marshal(agent)
	if err != nil {
		return fmt.Errorf("marshaling agent: %w", err)
	}

	if err := s.session.Query(`
		INSERT INTO agents (agent_id, publisher_id, status, data, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)`,
		agent.Id, agent.PublisherId, int(agent.Status), data, now, now,
	).WithContext(ctx).Exec(); err != nil {
		return fmt.Errorf("inserting agent: %w", err)
	}

	for _, skill := range agent.Skills {
		for _, tag := range skill.Tags {
			if err := s.session.Query(`
				INSERT INTO agent_tags (tag, agent_id) VALUES (?, ?)`,
				tag, agent.Id,
			).WithContext(ctx).Exec(); err != nil {
				return fmt.Errorf("inserting tag %q: %w", tag, err)
			}
		}
	}
	return nil
}

func (s *ScyllaStore) Get(ctx context.Context, agentID string) (*registryv1.AgentCard, error) {
	var data []byte
	if err := s.session.Query(`
		SELECT data FROM agents WHERE agent_id = ?`, agentID,
	).WithContext(ctx).Scan(&data); err != nil {
		if err == gocql.ErrNotFound {
			return nil, fmt.Errorf("agent %q not found", agentID)
		}
		return nil, fmt.Errorf("querying agent: %w", err)
	}

	var agent registryv1.AgentCard
	if err := proto.Unmarshal(data, &agent); err != nil {
		return nil, fmt.Errorf("unmarshaling agent: %w", err)
	}
	return &agent, nil
}

func (s *ScyllaStore) Update(ctx context.Context, agent *registryv1.AgentCard) error {
	now := time.Now()
	agent.UpdatedAt = timestamppb.New(now)

	data, err := proto.Marshal(agent)
	if err != nil {
		return fmt.Errorf("marshaling agent: %w", err)
	}

	if err := s.session.Query(`
		UPDATE agents SET data = ?, updated_at = ?, status = ?
		WHERE agent_id = ?`,
		data, now, int(agent.Status), agent.Id,
	).WithContext(ctx).Exec(); err != nil {
		return fmt.Errorf("updating agent: %w", err)
	}
	return nil
}

func (s *ScyllaStore) SetStatus(ctx context.Context, agentID string, status registryv1.AgentStatus) error {
	agent, err := s.Get(ctx, agentID)
	if err != nil {
		return err
	}
	agent.Status = status
	return s.Update(ctx, agent)
}

func (s *ScyllaStore) ListByTags(ctx context.Context, tags []string, limit int, pageToken string) ([]*registryv1.AgentCard, string, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	// no tags — scan all agents directly
	if len(tags) == 0 {
		return s.listAll(ctx, limit)
	}

	seen := make(map[string]struct{})
	var agentIDs []string

	for _, tag := range tags {
		iter := s.session.Query(`
			SELECT agent_id FROM agent_tags WHERE tag = ?`, tag,
		).WithContext(ctx).PageSize(limit).Iter()

		var agentID string
		for iter.Scan(&agentID) {
			if _, ok := seen[agentID]; !ok {
				seen[agentID] = struct{}{}
				agentIDs = append(agentIDs, agentID)
			}
		}
		if err := iter.Close(); err != nil {
			return nil, "", fmt.Errorf("listing tags: %w", err)
		}
	}

	var agents []*registryv1.AgentCard
	for _, id := range agentIDs {
		agent, err := s.Get(ctx, id)
		if err != nil {
			continue
		}
		agents = append(agents, agent)
		if len(agents) >= limit {
			break
		}
	}

	var nextPageToken string
	if len(agents) == limit {
		nextPageToken = agents[len(agents)-1].Id
	}
	return agents, nextPageToken, nil
}

func (s *ScyllaStore) listAll(ctx context.Context, limit int) ([]*registryv1.AgentCard, string, error) {
	iter := s.session.Query(`SELECT data FROM agents`).WithContext(ctx).PageSize(limit).Iter()

	var agents []*registryv1.AgentCard
	var data []byte
	for iter.Scan(&data) {
		var agent registryv1.AgentCard
		if err := proto.Unmarshal(data, &agent); err != nil {
			continue
		}
		agents = append(agents, &agent)
		if len(agents) >= limit {
			break
		}
	}

	if err := iter.Close(); err != nil {
		return nil, "", fmt.Errorf("listing all agents: %w", err)
	}

	var nextPageToken string
	if len(agents) == limit {
		nextPageToken = agents[len(agents)-1].Id
	}
	return agents, nextPageToken, nil
}

// ── PublisherStore ────────────────────────────────────────────────────────────

func (s *ScyllaStore) SavePublisher(ctx context.Context, publisher *registryv1.PublisherAccount) error {
	data, err := proto.Marshal(publisher)
	if err != nil {
		return fmt.Errorf("marshaling publisher: %w", err)
	}

	batch := s.session.NewBatch(gocql.LoggedBatch)
	batch.Query(`INSERT INTO publishers (publisher_id, handle, data) VALUES (?, ?, ?)`,
		publisher.Id, publisher.Handle, data)
	batch.Query(`INSERT INTO publisher_handles (handle, publisher_id) VALUES (?, ?)`,
		publisher.Handle, publisher.Id)

	if err := s.session.ExecuteBatch(batch); err != nil {
		return fmt.Errorf("saving publisher: %w", err)
	}
	return nil
}

func (s *ScyllaStore) GetPublisherByID(ctx context.Context, publisherID string) (*registryv1.PublisherAccount, error) {
	var data []byte
	if err := s.session.Query(`
		SELECT data FROM publishers WHERE publisher_id = ?`, publisherID,
	).WithContext(ctx).Scan(&data); err != nil {
		if err == gocql.ErrNotFound {
			return nil, fmt.Errorf("publisher not found")
		}
		return nil, fmt.Errorf("querying publisher: %w", err)
	}

	var pub registryv1.PublisherAccount
	if err := proto.Unmarshal(data, &pub); err != nil {
		return nil, fmt.Errorf("unmarshaling publisher: %w", err)
	}
	return &pub, nil
}

func (s *ScyllaStore) GetPublisherByHandle(ctx context.Context, handle string) (*registryv1.PublisherAccount, error) {
	var publisherID string
	if err := s.session.Query(`
		SELECT publisher_id FROM publisher_handles WHERE handle = ?`, handle,
	).WithContext(ctx).Scan(&publisherID); err != nil {
		if err == gocql.ErrNotFound {
			return nil, fmt.Errorf("handle %q not found", handle)
		}
		return nil, fmt.Errorf("looking up handle: %w", err)
	}
	return s.GetPublisherByID(ctx, publisherID)
}

func (s *ScyllaStore) ListAll(ctx context.Context) ([]*registryv1.AgentCard, error) {
	iter := s.session.Query(`SELECT data FROM agents`).WithContext(ctx).Iter()

	var agents []*registryv1.AgentCard
	var data []byte
	for iter.Scan(&data) {
		var agent registryv1.AgentCard
		if err := proto.Unmarshal(data, &agent); err != nil {
			continue
		}
		agents = append(agents, &agent)
	}

	if err := iter.Close(); err != nil {
		return nil, fmt.Errorf("listing all agents: %w", err)
	}
	return agents, nil
}