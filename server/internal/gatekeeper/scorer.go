package gatekeeper

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const scorerPrompt = `You are an AI agent registry validator. Your job is to evaluate an AgentCard submission and determine:
1. Whether it is valid and well-formed
2. A confidence score (0.0 to 1.0) reflecting quality and trustworthiness
3. A brief reason for your decision

Evaluate based on:
- Name: is it descriptive and professional? (not "test", "agent1", "asdf")
- Description: does it clearly explain what the agent does?
- Skills: are they specific and meaningful? do they have proper IDs and descriptions?
- Version: is it a valid semver string?
- URL: is it a real-looking endpoint (not localhost, not example.com)?

Respond ONLY with valid JSON in this exact format:
{
  "approved": true or false,
  "confidence_score": 0.0 to 1.0,
  "reason": "brief explanation"
}

Do not include any other text, markdown, or explanation outside the JSON.`

type groqRequest struct {
	Model    string      `json:"model"`
	Messages []groqMsg   `json:"messages"`
}

type groqMsg struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type groqResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

type ScoreResult struct {
	Approved        bool    `json:"approved"`
	ConfidenceScore float32 `json:"confidence_score"`
	Reason          string  `json:"reason"`
}

// Score calls Groq to evaluate the AgentCard quality.
func Score(ctx context.Context, card string, apiKey string) (ScoreResult, error) {
	payload := groqRequest{
		Model: "llama-3.1-8b-instant",
		Messages: []groqMsg{
			{Role: "system", Content: scorerPrompt},
			{Role: "user", Content: fmt.Sprintf("Evaluate this AgentCard:\n\n%s", card)},
		},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return ScoreResult{}, fmt.Errorf("marshaling request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://api.groq.com/openai/v1/chat/completions",
		bytes.NewReader(body),
	)
	if err != nil {
		return ScoreResult{}, fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return ScoreResult{}, fmt.Errorf("calling groq: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return ScoreResult{}, fmt.Errorf("groq returned status %d", resp.StatusCode)
	}

	var groqResp groqResponse
	if err := json.NewDecoder(resp.Body).Decode(&groqResp); err != nil {
		return ScoreResult{}, fmt.Errorf("decoding response: %w", err)
	}

	if len(groqResp.Choices) == 0 {
		return ScoreResult{}, fmt.Errorf("empty response from groq")
	}

	text := strings.TrimSpace(groqResp.Choices[0].Message.Content)

	// strip markdown code fences if model wraps in ```json
	text = strings.TrimPrefix(text, "```json")
	text = strings.TrimPrefix(text, "```")
	text = strings.TrimSuffix(text, "```")
	text = strings.TrimSpace(text)

	var result ScoreResult
	if err := json.Unmarshal([]byte(text), &result); err != nil {
		return ScoreResult{}, fmt.Errorf("parsing score result: %w\nraw: %s", err, text)
	}

	return result, nil
}
