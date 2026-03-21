package auth

import (
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const issuer = "agentregistry"

type claims struct {
	PublisherID string `json:"pid"`
	jwt.RegisteredClaims
}

// IssueJWT creates a signed JWT for an authenticated publisher session.
func (s *Service) IssueJWT(publisherID string) (string, error) {
	expiry := time.Duration(s.cfg.JWTExpiry) * time.Hour

	c := claims{
		PublisherID: publisherID,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    issuer,
			Subject:   publisherID,
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(expiry)),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, c)
	signed, err := token.SignedString([]byte(s.cfg.JWTSecret))
	if err != nil {
		return "", fmt.Errorf("signing jwt: %w", err)
	}

	return signed, nil
}

// ValidateJWT parses and validates a JWT, returning the publisher ID claim.
func (s *Service) ValidateJWT(tokenStr string) (string, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &claims{}, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return []byte(s.cfg.JWTSecret), nil
	}, jwt.WithIssuer(issuer), jwt.WithExpirationRequired())

	if err != nil {
		return "", fmt.Errorf("parsing jwt: %w", err)
	}

	c, ok := token.Claims.(*claims)
	if !ok || !token.Valid {
		return "", fmt.Errorf("invalid token claims")
	}

	return c.PublisherID, nil
}
