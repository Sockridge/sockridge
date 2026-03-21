package webhook

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	registryv1 "github.com/Sockridge/sockridge/server/gen/go/agentregistry/v1"
)

// mockStore satisfies enough of Store for dispatcher tests
type mockWebhookStore struct {
	webhooks []*registryv1.Webhook
}

func (m *mockWebhookStore) ListActiveByEvent(_ interface{}, publisherID, event string) ([]*registryv1.Webhook, error) {
	var result []*registryv1.Webhook
	for _, wh := range m.webhooks {
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

func TestDispatcher_Sign(t *testing.T) {
	body := []byte(`{"event":"test"}`)
	secret := "mysecret"

	sig1 := sign(body, secret)
	sig2 := sign(body, secret)

	if sig1 != sig2 {
		t.Error("signing should be deterministic")
	}
	if sig1 == "" {
		t.Error("signature should not be empty")
	}
}

func TestDispatcher_SignDifferentSecrets(t *testing.T) {
	body := []byte(`{"event":"test"}`)
	sig1 := sign(body, "secret1")
	sig2 := sign(body, "secret2")

	if sig1 == sig2 {
		t.Error("different secrets should produce different signatures")
	}
}

func TestDispatcher_Deliver(t *testing.T) {
	var received []byte
	var receivedSig string

	// mock HTTP server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received, _ = io.ReadAll(r.Body)
		receivedSig = r.Header.Get("X-Sockridge-Signature")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	d := &Dispatcher{
		client: &http.Client{Timeout: 5 * time.Second},
	}

	wh := &registryv1.Webhook{
		Id:     "wh-1",
		Url:    server.URL,
		Events: []string{EventAgentActive},
		Secret: "test-secret",
		Active: true,
	}

	body := []byte(`{"event":"agent_active","publisher_id":"pub-1"}`)
	d.deliver(wh, body)

	// give goroutine time to deliver
	time.Sleep(100 * time.Millisecond)

	if string(received) != string(body) {
		t.Errorf("expected body %s, got %s", body, received)
	}

	expectedSig := "sha256=" + sign(body, "test-secret")
	if receivedSig != expectedSig {
		t.Errorf("expected sig %s, got %s", expectedSig, receivedSig)
	}
}

func TestDispatcher_DeliverUnreachable(t *testing.T) {
	d := &Dispatcher{
		client: &http.Client{Timeout: 100 * time.Millisecond},
	}

	wh := &registryv1.Webhook{
		Id:     "wh-bad",
		Url:    "http://localhost:19999", // nothing listening here
		Events: []string{EventAgentActive},
		Secret: "secret",
		Active: true,
	}

	// should not panic
	d.deliver(wh, []byte(`{}`))
}

func TestPayload_JSON(t *testing.T) {
	payload := Payload{
		Event:       EventAccessRequest,
		PublisherID: "pub-123",
		OccurredAt:  time.Now().UTC().Format(time.RFC3339),
		Data: map[string]interface{}{
			"agreement_id": "agr-456",
			"from":         "bob",
		},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var decoded Payload
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if decoded.Event != EventAccessRequest {
		t.Errorf("expected event %s, got %s", EventAccessRequest, decoded.Event)
	}
	if decoded.PublisherID != "pub-123" {
		t.Errorf("expected publisher_id pub-123, got %s", decoded.PublisherID)
	}
}

func TestEventConstants(t *testing.T) {
	events := []string{
		EventAccessRequest,
		EventAccessApproved,
		EventAccessDenied,
		EventAccessRevoked,
		EventAgentActive,
		EventAgentInactive,
		EventAgentPublished,
		EventAgentRejected,
	}

	seen := make(map[string]bool)
	for _, e := range events {
		if e == "" {
			t.Error("event constant should not be empty")
		}
		if seen[e] {
			t.Errorf("duplicate event constant: %s", e)
		}
		seen[e] = true
	}
}
