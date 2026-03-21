package store

import (
	"context"
	"fmt"
	"time"

	registryv1 "github.com/Sockridge/sockridge/server/gen/go/agentregistry/v1"
	"github.com/gocql/gocql"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// CreateAgreementSchema creates the agreements tables.
// Called from ScyllaStore.CreateSchema.
func (s *ScyllaStore) CreateAgreementSchema(ctx context.Context) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS agreements (
			agreement_id  text PRIMARY KEY,
			requester_id  text,
			receiver_id   text,
			status        int,
			shared_key    text,
			data          blob,
			requested_at  timestamp
		)`,

		// index for listing by receiver (pending requests)
		`CREATE INDEX IF NOT EXISTS agreements_receiver_idx ON agreements (receiver_id)`,

		// index for listing by requester
		`CREATE INDEX IF NOT EXISTS agreements_requester_idx ON agreements (requester_id)`,

		// index for key-based lookup (resolve endpoint)
		`CREATE INDEX IF NOT EXISTS agreements_key_idx ON agreements (shared_key)`,
	}

	for _, stmt := range stmts {
		if err := s.session.Query(stmt).WithContext(ctx).Exec(); err != nil {
			return fmt.Errorf("creating agreement schema: %w\nstmt: %s", err, stmt)
		}
	}
	return nil
}

func (s *ScyllaStore) SaveAgreement(ctx context.Context, agreement *registryv1.AccessAgreement) error {
	now := time.Now()
	agreement.RequestedAt = timestamppb.New(now)

	data, err := proto.Marshal(agreement)
	if err != nil {
		return fmt.Errorf("marshaling agreement: %w", err)
	}

	if err := s.session.Query(`
		INSERT INTO agreements (agreement_id, requester_id, receiver_id, status, shared_key, data, requested_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		agreement.Id,
		agreement.RequesterId,
		agreement.ReceiverId,
		int(agreement.Status),
		agreement.SharedKey,
		data,
		now,
	).WithContext(ctx).Exec(); err != nil {
		return fmt.Errorf("saving agreement: %w", err)
	}
	return nil
}

func (s *ScyllaStore) GetAgreement(ctx context.Context, agreementID string) (*registryv1.AccessAgreement, error) {
	var data []byte
	if err := s.session.Query(`
		SELECT data FROM agreements WHERE agreement_id = ?`, agreementID,
	).WithContext(ctx).Scan(&data); err != nil {
		if err == gocql.ErrNotFound {
			return nil, fmt.Errorf("agreement %q not found", agreementID)
		}
		return nil, fmt.Errorf("querying agreement: %w", err)
	}

	var agreement registryv1.AccessAgreement
	if err := proto.Unmarshal(data, &agreement); err != nil {
		return nil, fmt.Errorf("unmarshaling agreement: %w", err)
	}
	return &agreement, nil
}

func (s *ScyllaStore) UpdateAgreement(ctx context.Context, agreement *registryv1.AccessAgreement) error {
	data, err := proto.Marshal(agreement)
	if err != nil {
		return fmt.Errorf("marshaling agreement: %w", err)
	}

	if err := s.session.Query(`
		UPDATE agreements SET data = ?, status = ?, shared_key = ?
		WHERE agreement_id = ?`,
		data,
		int(agreement.Status),
		agreement.SharedKey,
		agreement.Id,
	).WithContext(ctx).Exec(); err != nil {
		return fmt.Errorf("updating agreement: %w", err)
	}
	return nil
}

func (s *ScyllaStore) ListPendingForReceiver(ctx context.Context, receiverID string) ([]*registryv1.AccessAgreement, error) {
	return s.listAgreementsByFilter(ctx,
		`SELECT data FROM agreements WHERE receiver_id = ? ALLOW FILTERING`,
		receiverID,
		func(a *registryv1.AccessAgreement) bool {
			return a.Status == registryv1.AgreementStatus_AGREEMENT_STATUS_PENDING
		},
	)
}

func (s *ScyllaStore) ListActiveForPublisher(ctx context.Context, publisherID string) ([]*registryv1.AccessAgreement, error) {
	// fetch agreements where publisher is either requester or receiver
	byRequester, err := s.listAgreementsByFilter(ctx,
		`SELECT data FROM agreements WHERE requester_id = ? ALLOW FILTERING`,
		publisherID,
		func(a *registryv1.AccessAgreement) bool {
			return a.Status == registryv1.AgreementStatus_AGREEMENT_STATUS_ACTIVE
		},
	)
	if err != nil {
		return nil, err
	}

	byReceiver, err := s.listAgreementsByFilter(ctx,
		`SELECT data FROM agreements WHERE receiver_id = ? ALLOW FILTERING`,
		publisherID,
		func(a *registryv1.AccessAgreement) bool {
			return a.Status == registryv1.AgreementStatus_AGREEMENT_STATUS_ACTIVE
		},
	)
	if err != nil {
		return nil, err
	}

	// deduplicate
	seen := make(map[string]struct{})
	var all []*registryv1.AccessAgreement
	for _, a := range append(byRequester, byReceiver...) {
		if _, ok := seen[a.Id]; !ok {
			seen[a.Id] = struct{}{}
			all = append(all, a)
		}
	}
	return all, nil
}

func (s *ScyllaStore) GetAgreementByKey(ctx context.Context, sharedKey string) (*registryv1.AccessAgreement, error) {
	agreements, err := s.listAgreementsByFilter(ctx,
		`SELECT data FROM agreements WHERE shared_key = ? ALLOW FILTERING`,
		sharedKey,
		func(a *registryv1.AccessAgreement) bool {
			return a.Status == registryv1.AgreementStatus_AGREEMENT_STATUS_ACTIVE
		},
	)
	if err != nil {
		return nil, err
	}
	if len(agreements) == 0 {
		return nil, fmt.Errorf("no active agreement found for key")
	}
	return agreements[0], nil
}

func (s *ScyllaStore) listAgreementsByFilter(ctx context.Context, query string, arg string, filter func(*registryv1.AccessAgreement) bool) ([]*registryv1.AccessAgreement, error) {
	iter := s.session.Query(query, arg).WithContext(ctx).Iter()

	var agreements []*registryv1.AccessAgreement
	var data []byte
	for iter.Scan(&data) {
		var a registryv1.AccessAgreement
		if err := proto.Unmarshal(data, &a); err != nil {
			continue
		}
		if filter(&a) {
			agreements = append(agreements, &a)
		}
	}

	if err := iter.Close(); err != nil {
		return nil, fmt.Errorf("listing agreements: %w", err)
	}
	return agreements, nil
}
