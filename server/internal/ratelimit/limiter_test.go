package ratelimit

import (
	"context"
	"testing"
	"time"
)

// mockRedis is a simple in-memory implementation for testing
type mockRedisClient struct {
	data map[string][]int64
}

func newMockRedis() *mockRedisClient {
	return &mockRedisClient{data: make(map[string][]int64)}
}

func TestPublishRule(t *testing.T) {
	rule := PublishRule("publisher-123")

	if rule.Key != "publish:publisher-123" {
		t.Errorf("expected key publish:publisher-123, got %s", rule.Key)
	}
	if rule.Limit != 10 {
		t.Errorf("expected limit 10, got %d", rule.Limit)
	}
	if rule.Window != time.Hour {
		t.Errorf("expected window 1h, got %s", rule.Window)
	}
}

func TestSearchRule(t *testing.T) {
	rule := SearchRule("1.2.3.4")

	if rule.Key != "search:1.2.3.4" {
		t.Errorf("expected key search:1.2.3.4, got %s", rule.Key)
	}
	if rule.Limit != 100 {
		t.Errorf("expected limit 100, got %d", rule.Limit)
	}
	if rule.Window != time.Minute {
		t.Errorf("expected window 1m, got %s", rule.Window)
	}
}

func TestLoginRule(t *testing.T) {
	rule := LoginRule("publisher-456")

	if rule.Key != "login:publisher-456" {
		t.Errorf("expected key login:publisher-456, got %s", rule.Key)
	}
	if rule.Limit != 10 {
		t.Errorf("expected limit 10, got %d", rule.Limit)
	}
	if rule.Window != time.Minute {
		t.Errorf("expected window 1m, got %s", rule.Window)
	}
}

func TestResolveRule(t *testing.T) {
	key := "sk_abc123defgh"
	rule := ResolveRule(key)

	// should use first 8 chars
	if rule.Key != "resolve:sk_abc12" {
		t.Errorf("expected key resolve:sk_abc12, got %s", rule.Key)
	}
	if rule.Limit != 50 {
		t.Errorf("expected limit 50, got %d", rule.Limit)
	}
}

func TestRuleKeyUniqueness(t *testing.T) {
	// different publishers should have different keys
	r1 := PublishRule("pub-a")
	r2 := PublishRule("pub-b")

	if r1.Key == r2.Key {
		t.Error("different publishers should have different rate limit keys")
	}
}

func TestLimiterAllowFailOpen(t *testing.T) {
	// nil limiter should fail open without panicking
	var l *Limiter

	rule := PublishRule("test-publisher")
	allowed, err := l.Allow(context.Background(), rule)

	if !allowed {
		t.Error("nil limiter should fail open (allow=true)")
	}
	if err != nil {
		t.Errorf("nil limiter should return nil error, got: %v", err)
	}
}