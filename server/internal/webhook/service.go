package webhook

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"

	registryv1 "github.com/Sockridge/sockridge/server/gen/go/agentregistry/v1"
	"github.com/Sockridge/sockridge/server/gen/go/agentregistry/v1/registryv1connect"
)

type Service struct {
	store      *Store
	dispatcher *Dispatcher
}

func NewService(store *Store, dispatcher *Dispatcher) *Service {
	return &Service{store: store, dispatcher: dispatcher}
}

var _ registryv1connect.WebhookServiceHandler = (*Service)(nil)

func (s *Service) RegisterWebhook(
	ctx context.Context,
	req *connect.Request[registryv1.RegisterWebhookRequest],
) (*connect.Response[registryv1.RegisterWebhookResponse], error) {
	if req.Msg.PublisherId == "" || req.Msg.Url == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("publisher_id and url are required"))
	}

	if len(req.Msg.Events) == 0 {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("at least one event is required"))
	}

	// generate signing secret
	secret, err := generateSecret()
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("generating secret: %w", err))
	}

	wh := &registryv1.Webhook{
		Id:          uuid.NewString(),
		PublisherId: req.Msg.PublisherId,
		Url:         req.Msg.Url,
		Events:      req.Msg.Events,
		Secret:      secret,
		Active:      true,
	}

	if err := s.store.Save(ctx, wh); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("saving webhook: %w", err))
	}

	// don't return secret in the webhook object — return it separately (shown once)
	whNoSecret := *wh
	whNoSecret.Secret = ""

	return connect.NewResponse(&registryv1.RegisterWebhookResponse{
		Webhook: &whNoSecret,
		Secret:  secret,
	}), nil
}

func (s *Service) ListWebhooks(
	ctx context.Context,
	req *connect.Request[registryv1.ListWebhooksRequest],
	stream *connect.ServerStream[registryv1.ListWebhooksResponse],
) error {
	webhooks, err := s.store.ListByPublisher(ctx, req.Msg.PublisherId)
	if err != nil {
		return connect.NewError(connect.CodeInternal, fmt.Errorf("listing webhooks: %w", err))
	}

	for _, wh := range webhooks {
		// never expose the secret in list
		whSafe := *wh
		whSafe.Secret = ""
		if err := stream.Send(&registryv1.ListWebhooksResponse{Webhook: &whSafe}); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) DeleteWebhook(
	ctx context.Context,
	req *connect.Request[registryv1.DeleteWebhookRequest],
) (*connect.Response[registryv1.DeleteWebhookResponse], error) {
	wh, err := s.store.Get(ctx, req.Msg.WebhookId)
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("webhook not found"))
	}

	if wh.PublisherId != req.Msg.PublisherId {
		return nil, connect.NewError(connect.CodePermissionDenied, fmt.Errorf("not your webhook"))
	}

	if err := s.store.Delete(ctx, req.Msg.WebhookId); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("deleting webhook: %w", err))
	}

	return connect.NewResponse(&registryv1.DeleteWebhookResponse{Deleted: true}), nil
}

func (s *Service) TestWebhook(
	ctx context.Context,
	req *connect.Request[registryv1.TestWebhookRequest],
) (*connect.Response[registryv1.TestWebhookResponse], error) {
	wh, err := s.store.Get(ctx, req.Msg.WebhookId)
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("webhook not found"))
	}

	if wh.PublisherId != req.Msg.PublisherId {
		return nil, connect.NewError(connect.CodePermissionDenied, fmt.Errorf("not your webhook"))
	}

	// send test ping
	client := &http.Client{Timeout: 10 * time.Second}
	testBody := []byte(`{"event":"test","publisher_id":"` + req.Msg.PublisherId + `","data":{"message":"test delivery from sockridge"}}`)
	sig := sign(testBody, wh.Secret)

	httpReq, _ := http.NewRequestWithContext(ctx, http.MethodPost, wh.Url, nil)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("X-Sockridge-Event", "test")
	httpReq.Header.Set("X-Sockridge-Signature", "sha256="+sig)

	resp, err := client.Do(httpReq)
	if err != nil {
		return connect.NewResponse(&registryv1.TestWebhookResponse{
			Success: false,
			Error:   err.Error(),
		}), nil
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)

	return connect.NewResponse(&registryv1.TestWebhookResponse{
		Success:    resp.StatusCode >= 200 && resp.StatusCode < 300,
		StatusCode: int32(resp.StatusCode),
	}), nil
}

func generateSecret() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return "whsec_" + hex.EncodeToString(b), nil
}