package auth_test

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"testing"
	"time"

	registryv1 "github.com/utsav-develops/SocialAgents/server/gen/go/agentregistry/v1"
	"github.com/utsav-develops/SocialAgents/server/internal/auth"
	"github.com/utsav-develops/SocialAgents/server/internal/config"
)

type mockCache struct {
	nonces map[string]string
}

func newMockCache() *mockCache {
	return &mockCache{nonces: make(map[string]string)}
}

func (m *mockCache) SetNonce(_ context.Context, publisherID, nonce string, _ int) error {
	m.nonces[publisherID] = nonce
	return nil
}

func (m *mockCache) GetNonce(_ context.Context, publisherID string) (string, error) {
	n, ok := m.nonces[publisherID]
	if !ok {
		return "", fmt.Errorf("not found")
	}
	return n, nil
}

func (m *mockCache) DeleteNonce(_ context.Context, publisherID string) error {
	delete(m.nonces, publisherID)
	return nil
}

func (m *mockCache) GetAgent(_ context.Context, _ string) (*registryv1.AgentCard, error) {
	return nil, nil
}
func (m *mockCache) SetAgent(_ context.Context, _ *registryv1.AgentCard) error { return nil }
func (m *mockCache) DeleteAgent(_ context.Context, _ string) error              { return nil }

func TestChallengeVerify_HappyPath(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("keygen: %v", err)
	}
	pubB64 := base64.StdEncoding.EncodeToString(pub)
	cfg := &config.AuthConfig{JWTSecret: "test-secret", JWTExpiry: 1, NonceTTLSecs: 30}
	svc := auth.New(cfg, newMockCache())

	nonce, expiresAt, err := svc.Challenge(context.Background(), "publisher-1")
	if err != nil {
		t.Fatalf("Challenge: %v", err)
	}
	if time.Until(expiresAt) <= 0 {
		t.Fatal("nonce already expired")
	}

	sig := ed25519.Sign(priv, []byte(nonce))
	token, err := svc.Verify(context.Background(), "publisher-1", pubB64, sig)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if token == "" {
		t.Fatal("expected non-empty token")
	}

	publisherID, err := svc.ValidateJWT(token)
	if err != nil {
		t.Fatalf("ValidateJWT: %v", err)
	}
	if publisherID != "publisher-1" {
		t.Fatalf("wrong publisher id: got %q", publisherID)
	}
}

func TestVerify_WrongSignature(t *testing.T) {
	pub, _, _ := ed25519.GenerateKey(rand.Reader)
	_, wrongPriv, _ := ed25519.GenerateKey(rand.Reader)
	pubB64 := base64.StdEncoding.EncodeToString(pub)
	cfg := &config.AuthConfig{JWTSecret: "test-secret", JWTExpiry: 1, NonceTTLSecs: 30}
	svc := auth.New(cfg, newMockCache())

	nonce, _, _ := svc.Challenge(context.Background(), "publisher-1")
	badSig := ed25519.Sign(wrongPriv, []byte(nonce))

	_, err := svc.Verify(context.Background(), "publisher-1", pubB64, badSig)
	if err == nil {
		t.Fatal("expected error for wrong signature, got nil")
	}
}

func TestVerify_NonceIsOneTimeUse(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	pubB64 := base64.StdEncoding.EncodeToString(pub)
	cfg := &config.AuthConfig{JWTSecret: "test-secret", JWTExpiry: 1, NonceTTLSecs: 30}
	svc := auth.New(cfg, newMockCache())

	nonce, _, _ := svc.Challenge(context.Background(), "publisher-1")
	sig := ed25519.Sign(priv, []byte(nonce))

	_, err := svc.Verify(context.Background(), "publisher-1", pubB64, sig)
	if err != nil {
		t.Fatalf("first Verify: %v", err)
	}
	_, err = svc.Verify(context.Background(), "publisher-1", pubB64, sig)
	if err == nil {
		t.Fatal("expected error on second verify (nonce reuse), got nil")
	}
}