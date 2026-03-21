package audit

import (
	"context"
	"fmt"
	"time"

	"connectrpc.com/connect"
	"github.com/gocql/gocql"
	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/timestamppb"

	registryv1 "github.com/Sockridge/sockridge/server/gen/go/agentregistry/v1"
	"github.com/Sockridge/sockridge/server/gen/go/agentregistry/v1/registryv1connect"
)

const (
	ActionPublish        = "publish"
	ActionUpdate         = "update"
	ActionDeprecate      = "deprecate"
	ActionLogin          = "login"
	ActionAccessRequest  = "access_request"
	ActionAccessApprove  = "access_approve"
	ActionAccessDeny     = "access_deny"
	ActionAccessRevoke   = "access_revoke"
	ActionResolve        = "resolve"
)

// Service stores and queries audit events.
type Service struct {
	session *gocql.Session
}

func New(session *gocql.Session) *Service {
	return &Service{session: session}
}

// CreateSchema creates the audit_events table if it doesn't exist.
func (s *Service) CreateSchema(ctx context.Context) error {
	return s.session.Query(`
		CREATE TABLE IF NOT EXISTS audit_events (
			publisher_id  text,
			occurred_at   timestamp,
			event_id      text,
			action        text,
			agent_id      text,
			target_id     text,
			ip            text,
			detail        text,
			PRIMARY KEY (publisher_id, occurred_at, event_id)
		) WITH CLUSTERING ORDER BY (occurred_at DESC)
	`).WithContext(ctx).Exec()
}

// Log records an audit event.
func (s *Service) Log(ctx context.Context, publisherID, action, agentID, targetID, ip, detail string) {
	// fire and forget — don't block the main request
	go func() {
		bgCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := s.session.Query(`
			INSERT INTO audit_events (publisher_id, occurred_at, event_id, action, agent_id, target_id, ip, detail)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			publisherID,
			time.Now(),
			uuid.NewString(),
			action,
			agentID,
			targetID,
			ip,
			detail,
		).WithContext(bgCtx).Exec(); err != nil {
			fmt.Printf("[WARN] audit log failed: %v\n", err)
		}
	}()
}

// ListEvents returns recent audit events for a publisher.
func (s *Service) ListEvents(
	ctx context.Context,
	req *connect.Request[registryv1.ListAuditEventsRequest],
	stream *connect.ServerStream[registryv1.ListAuditEventsResponse],
) error {
	if req.Msg.PublisherId == "" {
		return connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("publisher_id is required"))
	}

	limit := int(req.Msg.Limit)
	if limit <= 0 || limit > 100 {
		limit = 50
	}

	iter := s.session.Query(`
		SELECT event_id, action, agent_id, target_id, ip, detail, occurred_at
		FROM audit_events
		WHERE publisher_id = ?
		LIMIT ?`,
		req.Msg.PublisherId, limit,
	).WithContext(ctx).Iter()

	var (
		eventID    string
		action     string
		agentID    string
		targetID   string
		ip         string
		detail     string
		occurredAt time.Time
	)

	for iter.Scan(&eventID, &action, &agentID, &targetID, &ip, &detail, &occurredAt) {
		if err := stream.Send(&registryv1.ListAuditEventsResponse{
			Event: &registryv1.AuditEvent{
				Id:          eventID,
				PublisherId: req.Msg.PublisherId,
				Action:      action,
				AgentId:     agentID,
				TargetId:    targetID,
				Ip:          ip,
				Detail:      detail,
				OccurredAt:  timestamppb.New(occurredAt),
			},
		}); err != nil {
			return err
		}
	}

	if err := iter.Close(); err != nil {
		return connect.NewError(connect.CodeInternal, fmt.Errorf("querying audit log: %w", err))
	}

	return nil
}

var _ registryv1connect.AuditServiceHandler = (*Service)(nil)