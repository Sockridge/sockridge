package auth

import (
	"context"
	"fmt"
	"time"

	"github.com/Sockridge/sockridge/server/internal/config"
	"github.com/Sockridge/sockridge/server/internal/store"
)

// Service orchestrates challenge-response auth using Ed25519 + JWT.
//
// Flow:
//  1. client calls Challenge(publisherID) → server generates nonce, stores in Redis with TTL
//  2. client signs nonce with Ed25519 private key
//  3. client calls Verify(publisherID, nonce, signature) → server verifies sig, issues JWT
//  4. client attaches JWT as Bearer token on all protected RPCs
type Service struct {
	cfg   *config.AuthConfig
	cache store.CacheStore
}

func New(cfg *config.AuthConfig, cache store.CacheStore) *Service {
	return &Service{cfg: cfg, cache: cache}
}

// Challenge generates and stores a nonce for the publisher.
// Returns the nonce and its expiry time.
func (s *Service) Challenge(ctx context.Context, publisherID string) (nonce string, expiresAt time.Time, err error) {
	nonce, err = GenerateNonce()
	if err != nil {
		return "", time.Time{}, fmt.Errorf("generating nonce: %w", err)
	}

	if err := s.cache.SetNonce(ctx, publisherID, nonce, s.cfg.NonceTTLSecs); err != nil {
		return "", time.Time{}, fmt.Errorf("storing nonce: %w", err)
	}

	expiresAt = time.Now().Add(time.Duration(s.cfg.NonceTTLSecs) * time.Second)
	return nonce, expiresAt, nil
}

// Verify checks the Ed25519 signature against the stored nonce.
// On success: deletes the nonce (one-time use) and returns a JWT.
// On failure: returns an error without deleting the nonce.
func (s *Service) Verify(ctx context.Context, publisherID string, publicKey string, signature []byte) (string, error) {
	nonce, err := s.cache.GetNonce(ctx, publisherID)
	if err != nil {
		return "", fmt.Errorf("nonce not found or expired — request a new challenge")
	}

	if err := VerifySignature(publicKey, []byte(nonce), signature); err != nil {
		return "", fmt.Errorf("invalid signature: %w", err)
	}

	// nonce is single-use — delete immediately after successful verify
	if err := s.cache.DeleteNonce(ctx, publisherID); err != nil {
		// non-fatal: nonce will expire naturally via Redis TTL
		_ = err
	}

	token, err := s.IssueJWT(publisherID)
	if err != nil {
		return "", fmt.Errorf("issuing token: %w", err)
	}

	return token, nil
}
