package socialagents

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"connectrpc.com/connect"
	registryv1 "github.com/Sockridge/sockridge/server/gen/go/agentregistry/v1"
)

// Login performs Ed25519 challenge-response auth.
// credentialsPath defaults to ~/.sockridge/credentials.json
// keyPath defaults to ~/.sockridge/ed25519.key
func (r *Registry) Login(credentialsPath, keyPath string) error {
	if credentialsPath == "" {
		home, _ := os.UserHomeDir()
		credentialsPath = filepath.Join(home, ".sockridge", "credentials.json")
	}
	if keyPath == "" {
		home, _ := os.UserHomeDir()
		keyPath = filepath.Join(home, ".sockridge", "ed25519.key")
	}

	priv, err := loadPrivateKey(keyPath)
	if err != nil {
		return fmt.Errorf("loading keypair: %w", err)
	}

	creds, err := loadCredentials(credentialsPath)
	if err != nil {
		return fmt.Errorf("loading credentials: %w", err)
	}

	r.publisherID = creds.PublisherID

	ctx := context.Background()

	chalResp, err := r.registry.AuthChallenge(ctx, connect.NewRequest(&registryv1.AuthChallengeRequest{
		PublisherId: creds.PublisherID,
	}))
	if err != nil {
		return fmt.Errorf("auth challenge: %w", err)
	}

	sig := ed25519.Sign(priv, []byte(chalResp.Msg.Nonce))

	verifyResp, err := r.registry.AuthVerify(ctx, connect.NewRequest(&registryv1.AuthVerifyRequest{
		PublisherId: creds.PublisherID,
		Nonce:       chalResp.Msg.Nonce,
		Signature:   sig,
	}))
	if err != nil {
		return fmt.Errorf("auth verify: %w", err)
	}

	r.token = verifyResp.Msg.SessionToken
	return nil
}

func loadPrivateKey(path string) (ed25519.PrivateKey, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading key file: %w", err)
	}
	privBytes, err := base64.StdEncoding.DecodeString(string(data))
	if err != nil {
		return nil, fmt.Errorf("decoding private key: %w", err)
	}
	return ed25519.PrivateKey(privBytes), nil
}

type credentials struct {
	PublisherID  string `json:"publisher_id"`
	Handle       string `json:"handle"`
	ServerURL    string `json:"server_url"`
	SessionToken string `json:"session_token"`
}

func loadCredentials(path string) (*credentials, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading credentials: %w", err)
	}
	var creds credentials
	if err := json.Unmarshal(data, &creds); err != nil {
		return nil, fmt.Errorf("parsing credentials: %w", err)
	}
	return &creds, nil
}
