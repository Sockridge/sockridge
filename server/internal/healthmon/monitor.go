package healthmon

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	registryv1 "github.com/Sockridge/sockridge/server/gen/go/agentregistry/v1"
	"github.com/Sockridge/sockridge/server/internal/store"
)

const (
	pingInterval   = 5 * time.Minute  // how often to check all agents
	pingTimeout    = 5 * time.Second  // per-agent timeout
	maxFailures    = 3                // failures before marking INACTIVE
	recoveryChecks = 2                // successes before marking ACTIVE again
)

// Monitor periodically pings all active agents and updates their status.
type Monitor struct {
	agents    store.AgentStore
	cache     store.CacheStore
	failures  map[string]int // agent_id → consecutive failure count
	recoveries map[string]int // agent_id → consecutive success count
	mu        sync.Mutex
}

func New(agents store.AgentStore, cache store.CacheStore) *Monitor {
	return &Monitor{
		agents:     agents,
		cache:      cache,
		failures:   make(map[string]int),
		recoveries: make(map[string]int),
	}
}

// Start begins the background health monitoring loop.
func (m *Monitor) Start(ctx context.Context) {
	go m.run(ctx)
	fmt.Println("[INFO] health monitor started (interval: 5m, max failures: 3)")
}

func (m *Monitor) run(ctx context.Context) {
	ticker := time.NewTicker(pingInterval)
	defer ticker.Stop()

	// run once immediately on start
	m.checkAll(ctx)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.checkAll(ctx)
		}
	}
}

func (m *Monitor) checkAll(ctx context.Context) {
	agents, err := m.agents.ListAll(ctx)
	if err != nil {
		fmt.Printf("[WARN] health monitor: failed to list agents: %v\n", err)
		return
	}

	// check active and inactive agents (inactive might recover)
	var toCheck []*registryv1.AgentCard
	for _, a := range agents {
		if a.Status == registryv1.AgentStatus_AGENT_STATUS_ACTIVE ||
			a.Status == registryv1.AgentStatus_AGENT_STATUS_INACTIVE {
			toCheck = append(toCheck, a)
		}
	}

	if len(toCheck) == 0 {
		return
	}

	fmt.Printf("[INFO] health monitor: checking %d agents\n", len(toCheck))

	// check concurrently with a worker pool
	sem := make(chan struct{}, 20) // max 20 concurrent pings
	var wg sync.WaitGroup

	for _, agent := range toCheck {
		wg.Add(1)
		go func(a *registryv1.AgentCard) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			m.checkAgent(ctx, a)
		}(agent)
	}

	wg.Wait()
}

func (m *Monitor) checkAgent(ctx context.Context, agent *registryv1.AgentCard) {
	if agent.Url == "" {
		return
	}

	reachable := ping(ctx, agent.Url)

	m.mu.Lock()
	defer m.mu.Unlock()

	if !reachable {
		m.failures[agent.Id]++
		m.recoveries[agent.Id] = 0

		fmt.Printf("[INFO] health monitor: agent %s (%s) unreachable (failures: %d/%d)\n",
			agent.Id, agent.Name, m.failures[agent.Id], maxFailures)

		if m.failures[agent.Id] >= maxFailures &&
			agent.Status == registryv1.AgentStatus_AGENT_STATUS_ACTIVE {
			m.markInactive(ctx, agent)
		}
	} else {
		m.failures[agent.Id] = 0

		if agent.Status == registryv1.AgentStatus_AGENT_STATUS_INACTIVE {
			m.recoveries[agent.Id]++
			fmt.Printf("[INFO] health monitor: agent %s (%s) recovering (successes: %d/%d)\n",
				agent.Id, agent.Name, m.recoveries[agent.Id], recoveryChecks)

			if m.recoveries[agent.Id] >= recoveryChecks {
				m.markActive(ctx, agent)
			}
		}
	}
}

func (m *Monitor) markInactive(ctx context.Context, agent *registryv1.AgentCard) {
	agent.Status = registryv1.AgentStatus_AGENT_STATUS_INACTIVE
	if err := m.agents.Update(ctx, agent); err != nil {
		fmt.Printf("[WARN] health monitor: failed to mark agent %s inactive: %v\n", agent.Id, err)
		return
	}
	_ = m.cache.DeleteAgent(ctx, agent.Id)
	fmt.Printf("[WARN] health monitor: agent %s (%s) marked INACTIVE after %d failures\n",
		agent.Id, agent.Name, maxFailures)
}

func (m *Monitor) markActive(ctx context.Context, agent *registryv1.AgentCard) {
	agent.Status = registryv1.AgentStatus_AGENT_STATUS_ACTIVE
	if err := m.agents.Update(ctx, agent); err != nil {
		fmt.Printf("[WARN] health monitor: failed to mark agent %s active: %v\n", agent.Id, err)
		return
	}
	_ = m.cache.DeleteAgent(ctx, agent.Id)
	m.recoveries[agent.Id] = 0
	fmt.Printf("[INFO] health monitor: agent %s (%s) marked ACTIVE (recovered)\n", agent.Id, agent.Name)
}

func ping(ctx context.Context, agentURL string) bool {
	agentURL = strings.TrimRight(agentURL, "/")
	client := &http.Client{Timeout: pingTimeout}

	for _, endpoint := range []string{
		agentURL + "/healthz",
		agentURL + "/.well-known/agent.json",
		agentURL,
	} {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			continue
		}
		resp, err := client.Do(req)
		if err != nil {
			continue
		}
		resp.Body.Close()
		if resp.StatusCode < 500 {
			return true
		}
	}
	return false
}