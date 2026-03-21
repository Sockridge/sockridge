package gatekeeper

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	registryv1 "github.com/Sockridge/sockridge/server/gen/go/agentregistry/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type Service struct {
	anthropicKey string
	groqKey      string
}

func New(anthropicKey, groqKey string) *Service {
	return &Service{anthropicKey: anthropicKey, groqKey: groqKey}
}

func (s *Service) Evaluate(ctx context.Context, agent *registryv1.AgentCard) (*registryv1.GatekeeperResult, error) {
	result := &registryv1.GatekeeperResult{
		EvaluatedAt: timestamppb.New(time.Now()),
	}

	// step 1: basic field validation
	if err := validate(agent); err != nil {
		result.Approved = false
		result.Reason = fmt.Sprintf("validation failed: %s", err.Error())
		result.ConfidenceScore = 0
		return result, nil
	}

	// step 2: ping + A2A compliance check
	if agent.Url != "" {
		skillIDs := make([]string, len(agent.Skills))
		for i, s := range agent.Skills {
			skillIDs[i] = s.Id
		}

		pingResult := Ping(ctx, agent.Url, agent.Name, skillIDs)
		result.Reachable = pingResult.Reachable
		result.PingLatencyMs = pingResult.LatencyMs
		result.A2ACompliant = pingResult.A2ACompliant
		result.A2AMatchesCard = pingResult.A2AMatchesCard

		if !pingResult.Reachable {
			result.Approved = false
			result.Reason = fmt.Sprintf("agent URL unreachable: %s", pingResult.Error)
			result.ConfidenceScore = 0.1
			return result, nil
		}

		// A2A compliance check — card mismatch is a hard reject
		if !pingResult.A2ACompliant {
			// not compliant — warn but let AI scorer decide
			fmt.Printf("[INFO] agent %s is reachable but not A2A compliant: %s\n", agent.Id, pingResult.A2AError)
		} else if !pingResult.A2AMatchesCard {
			// card mismatch — hard reject, don't proceed to AI scoring
			fmt.Printf("[WARN] agent %s A2A card mismatch — rejecting: %s\n", agent.Id, pingResult.A2AError)
			result.Approved = false
			result.ConfidenceScore = 0.0
			result.Reason = fmt.Sprintf("rejected: submitted card does not match /.well-known/agent.json — %s", pingResult.A2AError)
			return result, nil
		} else {
			fmt.Printf("[INFO] agent %s passed A2A compliance check\n", agent.Id)
		}
	}

	// step 3: AI scoring
	if s.anthropicKey != "" || s.groqKey != "" {
		cardJSON, err := agentCardToJSON(agent)
		if err != nil {
			return nil, fmt.Errorf("serializing card for scoring: %w", err)
		}

		scoreResult, err := Score(ctx, cardJSON, s.anthropicKey, s.groqKey)
		if err != nil {
			result.Approved = true
			result.ConfidenceScore = 0.3
			result.Reason = "scoring unavailable — approved with low confidence"
			return result, nil
		}

		result.ConfidenceScore = scoreResult.ConfidenceScore
		result.Reason = scoreResult.Reason

		// approve if score >= 0.4 regardless of model boolean
		result.Approved = scoreResult.Approved || scoreResult.ConfidenceScore >= 0.4
	} else {
		result.Approved = true
		result.ConfidenceScore = 0.5
		result.Reason = "auto-approved: AI scoring not configured"
	}

	return result, nil
}

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