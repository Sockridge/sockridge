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

type ScoreResult struct {
	Approved        bool    `json:"approved"`
	ConfidenceScore float32 `json:"confidence_score"`
	Reason          string  `json:"reason"`
}

// Score evaluates an AgentCard using Anthropic first, Groq as fallback.
func Score(ctx context.Context, card string, anthropicKey string, groqKey string) (ScoreResult, error) {
	if anthropicKey != "" {
		result, err := scoreWithAnthropic(ctx, card, anthropicKey)
		if err == nil {
			return result, nil
		}
		fmt.Printf("[WARN] Anthropic scoring failed, falling back to Groq: %v\n", err)
	}

	if groqKey != "" {
		return scoreWithGroq(ctx, card, groqKey)
	}

	return ScoreResult{}, fmt.Errorf("no scoring API key configured")
}

// ── Anthropic ─────────────────────────────────────────────────────────────────

type anthropicRequest struct {
	Model     string         `json:"model"`
	MaxTokens int            `json:"max_tokens"`
	System    string         `json:"system"`
	Messages  []anthropicMsg `json:"messages"`
}

type anthropicMsg struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type anthropicResponse struct {
	Content []struct {
		Text string `json:"text"`
	} `json:"content"`
}

func scoreWithAnthropic(ctx context.Context, card string, apiKey string) (ScoreResult, error) {
	payload := anthropicRequest{
		Model:     "claude-haiku-4-5-20251001",
		MaxTokens: 256,
		System:    scorerPrompt,
		Messages: []anthropicMsg{
			{Role: "user", Content: fmt.Sprintf("Evaluate this AgentCard:\n\n%s", card)},
		},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return ScoreResult{}, fmt.Errorf("marshaling request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://api.anthropic.com/v1/messages",
		bytes.NewReader(body),
	)
	if err != nil {
		return ScoreResult{}, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return ScoreResult{}, fmt.Errorf("calling anthropic: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return ScoreResult{}, fmt.Errorf("anthropic returned status %d", resp.StatusCode)
	}

	var ar anthropicResponse
	if err := json.NewDecoder(resp.Body).Decode(&ar); err != nil {
		return ScoreResult{}, fmt.Errorf("decoding response: %w", err)
	}

	if len(ar.Content) == 0 {
		return ScoreResult{}, fmt.Errorf("empty response from anthropic")
	}

	return parseScoreResult(ar.Content[0].Text)
}

// ── Groq ──────────────────────────────────────────────────────────────────────

type groqRequest struct {
	Model    string    `json:"model"`
	Messages []groqMsg `json:"messages"`
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

func scoreWithGroq(ctx context.Context, card string, apiKey string) (ScoreResult, error) {
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
		return ScoreResult{}, err
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

	var gr groqResponse
	if err := json.NewDecoder(resp.Body).Decode(&gr); err != nil {
		return ScoreResult{}, fmt.Errorf("decoding response: %w", err)
	}

	if len(gr.Choices) == 0 {
		return ScoreResult{}, fmt.Errorf("empty response from groq")
	}

	return parseScoreResult(gr.Choices[0].Message.Content)
}

// ── shared parser ─────────────────────────────────────────────────────────────

func parseScoreResult(text string) (ScoreResult, error) {
	text = strings.TrimSpace(text)
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