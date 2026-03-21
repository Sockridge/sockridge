package ratelimit

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// Limiter implements sliding window rate limiting backed by Redis.
type Limiter struct {
	redis *redis.Client
}

func New(redis *redis.Client) *Limiter {
	return &Limiter{redis: redis}
}

// Rule defines a rate limit rule.
type Rule struct {
	Key      string        // e.g. "publish:publisher_id"
	Limit    int           // max requests
	Window   time.Duration // per window
}

// Allow returns true if the request is allowed, false if rate limited.
// Uses Redis sorted sets for sliding window.
func (l *Limiter) Allow(ctx context.Context, rule Rule) (bool, error) {
	if l == nil || l.redis == nil {
		return true, nil // fail open
	}
	now := time.Now()
	windowStart := now.Add(-rule.Window).UnixMilli()
	key := fmt.Sprintf("rl:%s", rule.Key)

	pipe := l.redis.Pipeline()

	// remove old entries outside window
	pipe.ZRemRangeByScore(ctx, key, "0", fmt.Sprintf("%d", windowStart))

	// count current entries in window
	countCmd := pipe.ZCard(ctx, key)

	// add current request
	pipe.ZAdd(ctx, key, redis.Z{
		Score:  float64(now.UnixMilli()),
		Member: fmt.Sprintf("%d", now.UnixNano()),
	})

	// expire key after window
	pipe.Expire(ctx, key, rule.Window*2)

	_, err := pipe.Exec(ctx)
	if err != nil {
		// fail open on Redis errors — don't block requests
		return true, fmt.Errorf("rate limit check failed: %w", err)
	}

	count := countCmd.Val()
	return count < int64(rule.Limit), nil
}

// ── Predefined rules ──────────────────────────────────────────────────────────

func PublishRule(publisherID string) Rule {
	return Rule{
		Key:    fmt.Sprintf("publish:%s", publisherID),
		Limit:  10,
		Window: time.Hour,
	}
}

func SearchRule(ip string) Rule {
	return Rule{
		Key:    fmt.Sprintf("search:%s", ip),
		Limit:  100,
		Window: time.Minute,
	}
}

func ResolveRule(sharedKey string) Rule {
	return Rule{
		Key:    fmt.Sprintf("resolve:%s", sharedKey[:8]), // use first 8 chars only
		Limit:  50,
		Window: time.Minute,
	}
}

func LoginRule(publisherID string) Rule {
	return Rule{
		Key:    fmt.Sprintf("login:%s", publisherID),
		Limit:  10,
		Window: time.Minute,
	}
}