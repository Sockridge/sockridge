package metrics

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	registryv1 "github.com/Sockridge/sockridge/server/gen/go/agentregistry/v1"
)

func TestInc(t *testing.T) {
	// reset counters
	publishTotal.Store(0)
	searchTotal.Store(0)
	resolveTotal.Store(0)
	loginTotal.Store(0)
	rateLimitTotal.Store(0)
	webhookTotal.Store(0)

	Inc("publish")
	Inc("publish")
	Inc("search")
	Inc("resolve")
	Inc("login")
	Inc("rate_limit")
	Inc("webhook")
	Inc("unknown") // should not panic

	if publishTotal.Load() != 2 {
		t.Errorf("expected publish=2, got %d", publishTotal.Load())
	}
	if searchTotal.Load() != 1 {
		t.Errorf("expected search=1, got %d", searchTotal.Load())
	}
	if resolveTotal.Load() != 1 {
		t.Errorf("expected resolve=1, got %d", resolveTotal.Load())
	}
	if loginTotal.Load() != 1 {
		t.Errorf("expected login=1, got %d", loginTotal.Load())
	}
	if rateLimitTotal.Load() != 1 {
		t.Errorf("expected rate_limit=1, got %d", rateLimitTotal.Load())
	}
	if webhookTotal.Load() != 1 {
		t.Errorf("expected webhook=1, got %d", webhookTotal.Load())
	}
}

func TestHandlerResponseFormat(t *testing.T) {
	publishTotal.Store(5)
	searchTotal.Store(10)
	rateLimitTotal.Store(2)

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	w := httptest.NewRecorder()

	// use stub store that returns empty list
	Handler(&stubAgentStore{})(w, req)

	body := w.Body.String()

	// check prometheus format
	if !strings.Contains(body, "# HELP") {
		t.Error("metrics should contain # HELP lines")
	}
	if !strings.Contains(body, "# TYPE") {
		t.Error("metrics should contain # TYPE lines")
	}
	if !strings.Contains(body, "sockridge_requests_total") {
		t.Error("metrics should contain sockridge_requests_total")
	}
	if !strings.Contains(body, "sockridge_rate_limit_total") {
		t.Error("metrics should contain sockridge_rate_limit_total")
	}
	if !strings.Contains(body, "sockridge_uptime_seconds") {
		t.Error("metrics should contain sockridge_uptime_seconds")
	}

	// check content type
	ct := w.Header().Get("Content-Type")
	if !strings.Contains(ct, "text/plain") {
		t.Errorf("expected text/plain content type, got %s", ct)
	}
}

func TestHandlerStatusCode(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	w := httptest.NewRecorder()

	Handler(&stubAgentStore{})(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestIncConcurrent(t *testing.T) {
	publishTotal.Store(0)

	// fire 100 concurrent increments
	done := make(chan struct{}, 100)
	for i := 0; i < 100; i++ {
		go func() {
			Inc("publish")
			done <- struct{}{}
		}()
	}
	for i := 0; i < 100; i++ {
		<-done
	}

	if publishTotal.Load() != 100 {
		t.Errorf("expected 100 concurrent increments, got %d", publishTotal.Load())
	}
}

// stubAgentStore satisfies store.AgentStore for tests
type stubAgentStore struct{}

func (s *stubAgentStore) ListAll(_ context.Context) ([]*registryv1.AgentCard, error) { return nil, nil }
func (s *stubAgentStore) Save(_ context.Context, _ *registryv1.AgentCard) error { return nil }
func (s *stubAgentStore) Get(_ context.Context, _ string) (*registryv1.AgentCard, error) { return nil, nil }
func (s *stubAgentStore) Update(_ context.Context, _ *registryv1.AgentCard) error { return nil }
func (s *stubAgentStore) SetStatus(_ context.Context, _ string, _ registryv1.AgentStatus) error { return nil }
func (s *stubAgentStore) ListByTags(_ context.Context, _ []string, _ int, _ string) ([]*registryv1.AgentCard, string, error) { return nil, "", nil }