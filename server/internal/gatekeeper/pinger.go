package gatekeeper

import (
	"context"
	"fmt"
	"net/http"
	"time"
)

type PingResult struct {
	Reachable  bool
	LatencyMs  int32
	StatusCode int
	Error      string
}

// Ping attempts to reach the agent's URL.
// It tries /healthz first, then falls back to the root URL.
func Ping(ctx context.Context, agentURL string) PingResult {
	client := &http.Client{Timeout: 5 * time.Second}

	endpoints := []string{
		agentURL + "/healthz",
		agentURL + "/.well-known/agent.json",
		agentURL,
	}

	for _, endpoint := range endpoints {
		start := time.Now()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			continue
		}

		resp, err := client.Do(req)
		latencyMs := int32(time.Since(start).Milliseconds())

		if err != nil {
			continue
		}
		defer resp.Body.Close()

		if resp.StatusCode < 500 {
			return PingResult{
				Reachable:  true,
				LatencyMs:  latencyMs,
				StatusCode: resp.StatusCode,
			}
		}
	}

	return PingResult{
		Reachable: false,
		Error:     fmt.Sprintf("agent at %s is unreachable", agentURL),
	}
}
