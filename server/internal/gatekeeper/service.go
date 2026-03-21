package gatekeeper

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	registryv1 "github.com/Sockridge/sockridge/server/gen/go/agentregistry/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Service runs the full validation pipeline on a published AgentCard.
// Pipeline: validate → ping → score → update agent status
type Service struct {
	groqKey string
}

func New(groqKey string) *Service {
	return &Service{groqKey: groqKey}
}

// Evaluate runs the full gatekeeper pipeline and returns the result.
// This is called asynchronously after PublishAgent so it doesn't block the publisher.
func (s *Service) Evaluate(ctx context.Context, agent *registryv1.AgentCard) (*registryv1.GatekeeperResult, error) {
	result := &registryv1.GatekeeperResult{
		EvaluatedAt: timestamppb.New(time.Now()),
	}

	// step 1: basic validation
	if err := validate(agent); err != nil {
		result.Approved = false
		result.Reason = fmt.Sprintf("validation failed: %s", err.Error())
		result.ConfidenceScore = 0
		return result, nil
	}

	// step 2: ping the agent URL
	if agent.Url != "" {
		pingResult := Ping(ctx, agent.Url)
		result.Reachable = pingResult.Reachable
		result.PingLatencyMs = pingResult.LatencyMs

		if !pingResult.Reachable {
			result.Approved = false
			result.Reason = fmt.Sprintf("agent URL unreachable: %s", pingResult.Error)
			result.ConfidenceScore = 0.1
			return result, nil
		}
	}

	// step 3: AI scoring
	if s.groqKey != "" {
		cardJSON, err := agentCardToJSON(agent)
		if err != nil {
			return nil, fmt.Errorf("serializing card for scoring: %w", err)
		}

		scoreResult, err := Score(ctx, cardJSON, s.groqKey)
		if err != nil {
			// scoring failed — approve with low confidence rather than blocking
			result.Approved = true
			result.ConfidenceScore = 0.3
			result.Reason = "scoring unavailable — approved with low confidence"
			return result, nil
		}

		result.ConfidenceScore = scoreResult.ConfidenceScore
		result.Reason = scoreResult.Reason
		// approve if score >= 0.4 regardless of model boolean
		// the model often rejects valid agents for minor issues like localhost URLs
		result.Approved = scoreResult.Approved || scoreResult.ConfidenceScore >= 0.4
	} else {
		// no API key configured — auto-approve if ping passed
		result.Approved = true
		result.ConfidenceScore = 0.5
		result.Reason = "auto-approved: AI scoring not configured"
	}

	return result, nil
}

// validate checks required fields are present and well-formed.
func validate(agent *registryv1.AgentCard) error {
	if agent.Name == "" {
		return fmt.Errorf("name is required")
	}
	if len(agent.Name) < 3 {
		return fmt.Errorf("name too short (min 3 chars)")
	}
	if agent.Description == "" {
		return fmt.Errorf("description is required")
	}
	if len(agent.Description) < 10 {
		return fmt.Errorf("description too short (min 10 chars)")
	}
	if len(agent.Skills) == 0 {
		return fmt.Errorf("at least one skill is required")
	}
	for _, skill := range agent.Skills {
		if skill.Id == "" {
			return fmt.Errorf("skill id is required")
		}
		if skill.Name == "" {
			return fmt.Errorf("skill name is required")
		}
	}
	return nil
}

// agentCardToJSON converts an AgentCard to a readable JSON string for the scorer.
func agentCardToJSON(agent *registryv1.AgentCard) (string, error) {
	m := map[string]interface{}{
		"name":        agent.Name,
		"description": agent.Description,
		"version":     agent.Version,
		"url":         agent.Url,
		"skills":      agent.Skills,
	}
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return "", err
	}
	return string(b), nil
}