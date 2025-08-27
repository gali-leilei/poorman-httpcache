package adapter

import (
	"context"
	"fmt"

	"httpcache/pkg/tollgate"
)

// SecretKey is an adapter that validates against a secret key
type SecretKey struct {
	secretKey string
	serviceID string
}

// NewSecretKey creates a new secret key adapter for a specific service
func NewSecretKey(secretKey, serviceID string) tollgate.Adapter {
	return &SecretKey{
		secretKey: secretKey,
		serviceID: serviceID,
	}
}

// Consume validates the key and returns 1 if valid
func (s *SecretKey) Consume(ctx context.Context, key string) (int, error) {
	if key != s.secretKey {
		return 0, fmt.Errorf("invalid key")
	}
	return 1, nil
}

// Balance validates the key and returns 1 if valid
func (s *SecretKey) Balance(ctx context.Context, key string) (int, error) {
	if key != s.secretKey {
		return 0, fmt.Errorf("invalid key")
	}
	return 1, nil
}

// ServiceID returns the service ID this adapter is configured for
func (s *SecretKey) ServiceID() string {
	return s.serviceID
}
