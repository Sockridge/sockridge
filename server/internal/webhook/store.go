package webhook

import (
	"context"
	"fmt"
	"time"

	registryv1 "github.com/Sockridge/sockridge/server/gen/go/agentregistry/v1"
	"github.com/gocql/gocql"
	"github.com/google/uuid"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type Store struct {
	session *gocql.Session
}

func NewStore(session *gocql.Session) *Store {
	return &Store{session: session}
}

func (s *Store) CreateSchema(ctx context.Context) error {
	return s.session.Query(`
		CREATE TABLE IF NOT EXISTS webhooks (
			webhook_id   text PRIMARY KEY,
			publisher_id text,
			url          text,
			active       boolean,
			data         blob,
			created_at   timestamp
		)
	`).WithContext(ctx).Exec()
}

func (s *Store) Save(ctx context.Context, wh *registryv1.Webhook) error {
	wh.CreatedAt = timestamppb.New(time.Now())
	data, err := proto.Marshal(wh)
	if err != nil {
		return fmt.Errorf("marshaling webhook: %w", err)
	}
	return s.session.Query(`
		INSERT INTO webhooks (webhook_id, publisher_id, url, active, data, created_at)
		VALUES (?, ?, ?, ?, ?, ?)`,
		wh.Id, wh.PublisherId, wh.Url, wh.Active, data, time.Now(),
	).WithContext(ctx).Exec()
}

func (s *Store) Get(ctx context.Context, webhookID string) (*registryv1.Webhook, error) {
	var data []byte
	if err := s.session.Query(`SELECT data FROM webhooks WHERE webhook_id = ?`, webhookID).
		WithContext(ctx).Scan(&data); err != nil {
		if err == gocql.ErrNotFound {
			return nil, fmt.Errorf("webhook %q not found", webhookID)
		}
		return nil, fmt.Errorf("querying webhook: %w", err)
	}
	var wh registryv1.Webhook
	if err := proto.Unmarshal(data, &wh); err != nil {
		return nil, fmt.Errorf("unmarshaling webhook: %w", err)
	}
	return &wh, nil
}

func (s *Store) ListByPublisher(ctx context.Context, publisherID string) ([]*registryv1.Webhook, error) {
	iter := s.session.Query(`SELECT data FROM webhooks WHERE publisher_id = ? ALLOW FILTERING`, publisherID).
		WithContext(ctx).Iter()

	var webhooks []*registryv1.Webhook
	var data []byte
	for iter.Scan(&data) {
		var wh registryv1.Webhook
		if err := proto.Unmarshal(data, &wh); err != nil {
			continue
		}
		webhooks = append(webhooks, &wh)
	}
	if err := iter.Close(); err != nil {
		return nil, fmt.Errorf("listing webhooks: %w", err)
	}
	return webhooks, nil
}

func (s *Store) Delete(ctx context.Context, webhookID string) error {
	return s.session.Query(`DELETE FROM webhooks WHERE webhook_id = ?`, webhookID).
		WithContext(ctx).Exec()
}

func (s *Store) ListActiveByEvent(ctx context.Context, publisherID, event string) ([]*registryv1.Webhook, error) {
	all, err := s.ListByPublisher(ctx, publisherID)
	if err != nil {
		return nil, err
	}
	var result []*registryv1.Webhook
	for _, wh := range all {
		if !wh.Active {
			continue
		}
		for _, e := range wh.Events {
			if e == event || e == "*" {
				result = append(result, wh)
				break
			}
		}
	}
	return result, nil
}

// GenerateID generates a new webhook ID.
func GenerateID() string {
	return uuid.NewString()
}