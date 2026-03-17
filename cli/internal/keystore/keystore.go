package keystore

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

const (
	dirName         = ".agentctl"
	keyFile         = "ed25519.key"
	credentialsFile = "credentials.json"
)

type Credentials struct {
	PublisherID  string `json:"publisher_id"`
	Handle       string `json:"handle"`
	ServerURL    string `json:"server_url"`
	SessionToken string `json:"session_token,omitempty"`
}

type KeyPair struct {
	PublicKey  string // base64
	PrivateKey ed25519.PrivateKey
}

// Generate creates a new Ed25519 keypair and saves the private key to disk.
// Returns the public key (base64) for registration with the server.
func Generate() (*KeyPair, error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generating keypair: %w", err)
	}

	dir, err := ensureDir()
	if err != nil {
		return nil, err
	}

	// save private key — chmod 600, never readable by others
	keyPath := filepath.Join(dir, keyFile)
	encoded := base64.StdEncoding.EncodeToString(priv)
	if err := os.WriteFile(keyPath, []byte(encoded), 0600); err != nil {
		return nil, fmt.Errorf("writing private key: %w", err)
	}

	return &KeyPair{
		PublicKey:  base64.StdEncoding.EncodeToString(pub),
		PrivateKey: priv,
	}, nil
}

// Load reads the private key from disk and derives the public key.
func Load() (*KeyPair, error) {
	dir, err := ensureDir()
	if err != nil {
		return nil, err
	}

	keyPath := filepath.Join(dir, keyFile)
	data, err := os.ReadFile(keyPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("no keypair found — run: agentctl auth keygen")
		}
		return nil, fmt.Errorf("reading private key: %w", err)
	}

	privBytes, err := base64.StdEncoding.DecodeString(string(data))
	if err != nil {
		return nil, fmt.Errorf("decoding private key: %w", err)
	}

	priv := ed25519.PrivateKey(privBytes)
	pub := priv.Public().(ed25519.PublicKey)

	return &KeyPair{
		PublicKey:  base64.StdEncoding.EncodeToString(pub),
		PrivateKey: priv,
	}, nil
}

// Sign signs a message with the loaded private key.
func (k *KeyPair) Sign(message []byte) []byte {
	return ed25519.Sign(k.PrivateKey, message)
}

// SaveCredentials persists publisher credentials after successful registration.
func SaveCredentials(creds *Credentials) error {
	dir, err := ensureDir()
	if err != nil {
		return err
	}

	data, err := json.MarshalIndent(creds, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling credentials: %w", err)
	}

	path := filepath.Join(dir, credentialsFile)
	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("writing credentials: %w", err)
	}

	return nil
}

// LoadCredentials reads saved publisher credentials from disk.
func LoadCredentials() (*Credentials, error) {
	dir, err := ensureDir()
	if err != nil {
		return nil, err
	}

	path := filepath.Join(dir, credentialsFile)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("not registered — run: agentctl auth register")
		}
		return nil, fmt.Errorf("reading credentials: %w", err)
	}

	var creds Credentials
	if err := json.Unmarshal(data, &creds); err != nil {
		return nil, fmt.Errorf("parsing credentials: %w", err)
	}

	return &creds, nil
}

func ensureDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("finding home dir: %w", err)
	}

	dir := filepath.Join(home, dirName)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", fmt.Errorf("creating %s: %w", dir, err)
	}

	return dir, nil
}
