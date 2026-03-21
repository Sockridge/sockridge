package gatekeeper

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type PingResult struct {
	Reachable      bool
	LatencyMs      int32
	StatusCode     int
	Error          string
	A2ACompliant   bool   // responded to /.well-known/agent.json
	A2AMatchesCard bool   // returned AgentCard fields match submission
	A2AError       string // why A2A check failed
}

// a2aAgentCard is a minimal struct for parsing /.well-known/agent.json
type a2aAgentCard struct {
	Name            string `json:"name"`
	Description     string `json:"description"`
	Version         string `json:"version"`
	ProtocolVersion string `json:"protocolVersion"`
	URL             string `json:"url"`
	Skills          []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"skills"`
}

// Ping checks if the agent URL is reachable and A2A compliant.
func Ping(ctx context.Context, agentURL string, submittedName string, submittedSkillIDs []string) PingResult {
	client := &http.Client{Timeout: 5 * time.Second}
	agentURL = strings.TrimRight(agentURL, "/")

	// ── step 1: basic reachability ────────────────────────────────────────────
	result := checkReachability(ctx, client, agentURL)
	if !result.Reachable {
		return result
	}

	// ── step 2: A2A compliance — check /.well-known/agent.json ───────────────
	wellKnownURL := agentURL + "/.well-known/agent.json"
	start := time.Now()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, wellKnownURL, nil)
	if err != nil {
		result.A2AError = fmt.Sprintf("could not build request: %v", err)
		return result
	}

	resp, err := client.Do(req)
	if err != nil {
		result.A2AError = fmt.Sprintf("/.well-known/agent.json unreachable: %v", err)
		return result
	}
	defer resp.Body.Close()

	_ = start

	if resp.StatusCode != http.StatusOK {
		result.A2AError = fmt.Sprintf("/.well-known/agent.json returned %d (expected 200)", resp.StatusCode)
		return result
	}

	// check content type is JSON
	ct := resp.Header.Get("Content-Type")
	if !strings.Contains(ct, "json") {
		result.A2AError = fmt.Sprintf("/.well-known/agent.json has wrong content-type: %s", ct)
		return result
	}

	// parse the response
	body, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024)) // max 64KB
	if err != nil {
		result.A2AError = fmt.Sprintf("reading /.well-known/agent.json: %v", err)
		return result
	}

	var card a2aAgentCard
	if err := json.Unmarshal(body, &card); err != nil {
		result.A2AError = fmt.Sprintf("/.well-known/agent.json is not valid JSON: %v", err)
		return result
	}

	result.A2ACompliant = true

	// ── step 3: validate card matches submission ───────────────────────────────
	mismatches := []string{}

	// name should match (case-insensitive, trimmed)
	if !strings.EqualFold(strings.TrimSpace(card.Name), strings.TrimSpace(submittedName)) {
		mismatches = append(mismatches,
			fmt.Sprintf("name mismatch: submitted %q but agent reports %q", submittedName, card.Name))
	}

	// protocol version should be present
	if card.ProtocolVersion == "" {
		mismatches = append(mismatches, "protocolVersion missing from /.well-known/agent.json")
	}

	// all submitted skill IDs should be present in the agent's card
	agentSkillIDs := make(map[string]bool)
	for _, s := range card.Skills {
		agentSkillIDs[s.ID] = true
	}
	for _, id := range submittedSkillIDs {
		if !agentSkillIDs[id] {
			mismatches = append(mismatches,
				fmt.Sprintf("skill %q in submission not found in /.well-known/agent.json", id))
		}
	}

	// url in remote card must match the submitted url
	if card.URL != "" && !strings.EqualFold(strings.TrimRight(card.URL, "/"), strings.TrimRight(agentURL, "/")) {
		mismatches = append(mismatches,
			fmt.Sprintf("url in /.well-known/agent.json (%q) does not match submitted url (%q)", card.URL, agentURL))
	}

	if len(mismatches) > 0 {
		result.A2AError = "card mismatch: " + strings.Join(mismatches, "; ")
		return result
	}

	result.A2AMatchesCard = true
	return result
}

// checkReachability tries /healthz, /.well-known/agent.json, then root.
func checkReachability(ctx context.Context, client *http.Client, agentURL string) PingResult {
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
		resp.Body.Close()

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