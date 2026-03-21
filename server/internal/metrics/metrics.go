package metrics

import (
	"context"
	"fmt"
	"net/http"
	"sync/atomic"
	"time"

	registryv1 "github.com/Sockridge/sockridge/server/gen/go/agentregistry/v1"
	"github.com/Sockridge/sockridge/server/internal/store"
)

// Counters — atomic for thread safety
var (
	publishTotal   atomic.Int64
	searchTotal    atomic.Int64
	resolveTotal   atomic.Int64
	loginTotal     atomic.Int64
	rateLimitTotal atomic.Int64
	webhookTotal   atomic.Int64
	startTime      = time.Now()
)

// Inc increments a named counter.
func Inc(name string) {
	switch name {
	case "publish":
		publishTotal.Add(1)
	case "search":
		searchTotal.Add(1)
	case "resolve":
		resolveTotal.Add(1)
	case "login":
		loginTotal.Add(1)
	case "rate_limit":
		rateLimitTotal.Add(1)
	case "webhook":
		webhookTotal.Add(1)
	}
}

// Handler returns an HTTP handler for /metrics.
// Output is Prometheus text format.
func Handler(agents store.AgentStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		// fetch live counts from store
		allAgents, _ := agents.ListAll(ctx)

		var activeCount, pendingCount, rejectedCount, inactiveCount int64
		for _, a := range allAgents {
			switch a.Status {
			case registryv1.AgentStatus_AGENT_STATUS_ACTIVE:
				activeCount++
			case registryv1.AgentStatus_AGENT_STATUS_PENDING:
				pendingCount++
			case registryv1.AgentStatus_AGENT_STATUS_REJECTED:
				rejectedCount++
			case registryv1.AgentStatus_AGENT_STATUS_INACTIVE:
				inactiveCount++
			}
		}

		uptimeSeconds := int64(time.Since(startTime).Seconds())

		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		fmt.Fprintf(w, `# HELP sockridge_agents_total Total agents by status
# TYPE sockridge_agents_total gauge
sockridge_agents_total{status="active"} %d
sockridge_agents_total{status="pending"} %d
sockridge_agents_total{status="rejected"} %d
sockridge_agents_total{status="inactive"} %d

# HELP sockridge_requests_total Total requests by operation
# TYPE sockridge_requests_total counter
sockridge_requests_total{operation="publish"} %d
sockridge_requests_total{operation="search"} %d
sockridge_requests_total{operation="resolve"} %d
sockridge_requests_total{operation="login"} %d
sockridge_requests_total{operation="webhook_fired"} %d

# HELP sockridge_rate_limit_total Total rate limit rejections
# TYPE sockridge_rate_limit_total counter
sockridge_rate_limit_total %d

# HELP sockridge_uptime_seconds Server uptime in seconds
# TYPE sockridge_uptime_seconds gauge
sockridge_uptime_seconds %d
`,
			activeCount, pendingCount, rejectedCount, inactiveCount,
			publishTotal.Load(), searchTotal.Load(), resolveTotal.Load(),
			loginTotal.Load(), webhookTotal.Load(),
			rateLimitTotal.Load(),
			uptimeSeconds,
		)
	}
}