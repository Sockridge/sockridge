package auth

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"fmt"
)

// VerifySignature verifies an Ed25519 signature.
// publicKeyB64: base64-encoded Ed25519 public key (32 bytes)
// message:      the raw bytes that were signed
// signature:    the raw signature bytes (64 bytes)
func VerifySignature(publicKeyB64 string, message []byte, signature []byte) error {
	pubKeyBytes, err := base64.StdEncoding.DecodeString(publicKeyB64)
	if err != nil {
		return fmt.Errorf("decoding public key: %w", err)
	}

	if len(pubKeyBytes) != ed25519.PublicKeySize {
		return fmt.Errorf("invalid public key length: got %d, want %d", len(pubKeyBytes), ed25519.PublicKeySize)
	}

	if len(signature) != ed25519.SignatureSize {
		return fmt.Errorf("invalid signature length: got %d, want %d", len(signature), ed25519.SignatureSize)
	}

	pub := ed25519.PublicKey(pubKeyBytes)
	if !ed25519.Verify(pub, message, signature) {
		return fmt.Errorf("signature verification failed")
	}

	return nil
}

// GenerateNonce creates a cryptographically random 32-byte nonce, base64-encoded.
func GenerateNonce() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generating nonce: %w", err)
	}
	return base64.StdEncoding.EncodeToString(buf), nil
}
