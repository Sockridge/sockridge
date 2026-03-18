package access

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/timestamppb"

	registryv1 "github.com/utsav-develops/SocialAgents/server/gen/go/agentregistry/v1"
	"github.com/utsav-develops/SocialAgents/server/gen/go/agentregistry/v1/registryv1connect"
	"github.com/utsav-develops/SocialAgents/server/internal/store"
)

type Service struct {
	agreements store.AgreementStore
	agents     store.AgentStore
	publishers store.PublisherStore
}

func New(agreements store.AgreementStore, agents store.AgentStore, publishers store.PublisherStore) *Service {
	return &Service{
		agreements: agreements,
		agents:     agents,
		publishers: publishers,
	}
}

var _ registryv1connect.AccessAgreementServiceHandler = (*Service)(nil)

func (s *Service) RequestAccess(
	ctx context.Context,
	req *connect.Request[registryv1.RequestAccessRequest],
) (*connect.Response[registryv1.RequestAccessResponse], error) {
	if req.Msg.RequesterId == "" || req.Msg.ReceiverId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("requester_id and receiver_id are required"))
	}
	if req.Msg.RequesterId == req.Msg.ReceiverId {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("cannot request access to yourself"))
	}

	// verify both publishers exist
	if _, err := s.publishers.GetPublisherByID(ctx, req.Msg.RequesterId); err != nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("requester not found"))
	}
	if _, err := s.publishers.GetPublisherByID(ctx, req.Msg.ReceiverId); err != nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("receiver not found"))
	}

	agreement := &registryv1.AccessAgreement{
		Id:          uuid.NewString(),
		RequesterId: req.Msg.RequesterId,
		ReceiverId:  req.Msg.ReceiverId,
		Message:     req.Msg.Message,
		Status:      registryv1.AgreementStatus_AGREEMENT_STATUS_PENDING,
	}

	if err := s.agreements.SaveAgreement(ctx, agreement); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("saving agreement: %w", err))
	}

	return connect.NewResponse(&registryv1.RequestAccessResponse{
		Agreement: agreement,
	}), nil
}

func (s *Service) ListPending(
	ctx context.Context,
	req *connect.Request[registryv1.ListPendingRequest],
	stream *connect.ServerStream[registryv1.ListPendingResponse],
) error {
	if req.Msg.PublisherId == "" {
		return connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("publisher_id is required"))
	}

	agreements, err := s.agreements.ListPendingForReceiver(ctx, req.Msg.PublisherId)
	if err != nil {
		return connect.NewError(connect.CodeInternal, fmt.Errorf("listing pending: %w", err))
	}

	for _, agreement := range agreements {
		requester, err := s.publishers.GetPublisherByID(ctx, agreement.RequesterId)
		handle := ""
		if err == nil {
			handle = requester.Handle
		}

		if err := stream.Send(&registryv1.ListPendingResponse{
			Agreement:        agreement,
			RequesterHandle:  handle,
		}); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) ApproveAccess(
	ctx context.Context,
	req *connect.Request[registryv1.ApproveAccessRequest],
) (*connect.Response[registryv1.ApproveAccessResponse], error) {
	if req.Msg.PublisherId == "" || req.Msg.AgreementId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("publisher_id and agreement_id are required"))
	}

	agreement, err := s.agreements.GetAgreement(ctx, req.Msg.AgreementId)
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("agreement not found"))
	}

	if agreement.ReceiverId != req.Msg.PublisherId {
		return nil, connect.NewError(connect.CodePermissionDenied, fmt.Errorf("only the receiver can approve"))
	}

	if agreement.Status != registryv1.AgreementStatus_AGREEMENT_STATUS_PENDING {
		return nil, connect.NewError(connect.CodeFailedPrecondition, fmt.Errorf("agreement is not pending"))
	}

	// generate shared key — 32 random bytes as hex
	sharedKey, err := generateSharedKey()
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("generating key: %w", err))
	}

	agreement.Status = registryv1.AgreementStatus_AGREEMENT_STATUS_ACTIVE
	agreement.SharedKey = sharedKey
	agreement.ResolvedAt = timestamppb.New(time.Now())

	if err := s.agreements.UpdateAgreement(ctx, agreement); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("updating agreement: %w", err))
	}

	return connect.NewResponse(&registryv1.ApproveAccessResponse{
		Agreement: agreement,
		SharedKey: sharedKey,
	}), nil
}

func (s *Service) DenyAccess(
	ctx context.Context,
	req *connect.Request[registryv1.DenyAccessRequest],
) (*connect.Response[registryv1.DenyAccessResponse], error) {
	agreement, err := s.agreements.GetAgreement(ctx, req.Msg.AgreementId)
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("agreement not found"))
	}
	if agreement.ReceiverId != req.Msg.PublisherId {
		return nil, connect.NewError(connect.CodePermissionDenied, fmt.Errorf("only the receiver can deny"))
	}

	agreement.Status = registryv1.AgreementStatus_AGREEMENT_STATUS_DENIED
	agreement.ResolvedAt = timestamppb.New(time.Now())

	if err := s.agreements.UpdateAgreement(ctx, agreement); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("updating agreement: %w", err))
	}

	return connect.NewResponse(&registryv1.DenyAccessResponse{Agreement: agreement}), nil
}

func (s *Service) RevokeAccess(
	ctx context.Context,
	req *connect.Request[registryv1.RevokeAccessRequest],
) (*connect.Response[registryv1.RevokeAccessResponse], error) {
	agreement, err := s.agreements.GetAgreement(ctx, req.Msg.AgreementId)
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("agreement not found"))
	}

	// either side can revoke
	if agreement.RequesterId != req.Msg.PublisherId && agreement.ReceiverId != req.Msg.PublisherId {
		return nil, connect.NewError(connect.CodePermissionDenied, fmt.Errorf("not a party to this agreement"))
	}

	agreement.Status = registryv1.AgreementStatus_AGREEMENT_STATUS_REVOKED
	agreement.ResolvedAt = timestamppb.New(time.Now())
	agreement.SharedKey = "" // invalidate key immediately

	if err := s.agreements.UpdateAgreement(ctx, agreement); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("revoking agreement: %w", err))
	}

	return connect.NewResponse(&registryv1.RevokeAccessResponse{Revoked: true}), nil
}

func (s *Service) ListAgreements(
	ctx context.Context,
	req *connect.Request[registryv1.ListAgreementsRequest],
	stream *connect.ServerStream[registryv1.ListAgreementsResponse],
) error {
	agreements, err := s.agreements.ListActiveForPublisher(ctx, req.Msg.PublisherId)
	if err != nil {
		return connect.NewError(connect.CodeInternal, fmt.Errorf("listing agreements: %w", err))
	}

	for _, agreement := range agreements {
		// find the other publisher's handle
		otherID := agreement.RequesterId
		if otherID == req.Msg.PublisherId {
			otherID = agreement.ReceiverId
		}

		other, err := s.publishers.GetPublisherByID(ctx, otherID)
		handle := otherID
		if err == nil {
			handle = other.Handle
		}

		if err := stream.Send(&registryv1.ListAgreementsResponse{
			Agreement:   agreement,
			OtherHandle: handle,
		}); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) ResolveEndpoint(
	ctx context.Context,
	req *connect.Request[registryv1.ResolveEndpointRequest],
) (*connect.Response[registryv1.ResolveEndpointResponse], error) {
	if req.Msg.AgentId == "" || req.Msg.SharedKey == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("agent_id and shared_key are required"))
	}

	// validate the key maps to an active agreement
	agreement, err := s.agreements.GetAgreementByKey(ctx, req.Msg.SharedKey)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, fmt.Errorf("invalid or expired key"))
	}

	// fetch the agent
	agent, err := s.agents.Get(ctx, req.Msg.AgentId)
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("agent not found"))
	}

	// verify the agent belongs to one of the two publishers in the agreement
	if agent.PublisherId != agreement.RequesterId && agent.PublisherId != agreement.ReceiverId {
		return nil, connect.NewError(connect.CodePermissionDenied, fmt.Errorf("agent not covered by this agreement"))
	}

	// only reveal URL for active agents
	if agent.Status != registryv1.AgentStatus_AGENT_STATUS_ACTIVE {
		return nil, connect.NewError(connect.CodeFailedPrecondition, fmt.Errorf("agent is not active"))
	}

	transport := "http"
	for _, iface := range agent.Interfaces {
		if iface.Primary {
			if iface.Transport == registryv1.TransportType_TRANSPORT_TYPE_GRPC {
				transport = "grpc"
			}
			break
		}
	}

	return connect.NewResponse(&registryv1.ResolveEndpointResponse{
		AgentId:   agent.Id,
		Url:       agent.Url,
		Transport: transport,
		AgentName: agent.Name,
		Agent:     agent, // full card including skills and capabilities
	}), nil
}

func generateSharedKey() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return "sk_" + hex.EncodeToString(b), nil
}

func (s *Service) GetAgreement(
	ctx context.Context,
	req *connect.Request[registryv1.GetAgreementRequest],
) (*connect.Response[registryv1.GetAgreementResponse], error) {
	if req.Msg.PublisherId == "" || req.Msg.AgreementId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("publisher_id and agreement_id are required"))
	}

	agreement, err := s.agreements.GetAgreement(ctx, req.Msg.AgreementId)
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("agreement not found"))
	}

	// only parties to the agreement can see the key
	if agreement.RequesterId != req.Msg.PublisherId && agreement.ReceiverId != req.Msg.PublisherId {
		return nil, connect.NewError(connect.CodePermissionDenied, fmt.Errorf("not a party to this agreement"))
	}

	return connect.NewResponse(&registryv1.GetAgreementResponse{
		Agreement: agreement,
	}), nil
}