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

// Reserve reserves a given amount of quota for a key.
// Returns true if the reservation was successful, false if the quota is insufficient.
func (s *SecretKey) Reserve(ctx context.Context, key string, amount int) (bool, error) {
	if key != s.secretKey {
		return false, fmt.Errorf("invalid key")
	}
	// SecretKey adapter allows unlimited quota for valid keys
	return true, nil
}

// Refund refunds a given amount of quota for a key.
// Returns true if the refund was successful, false if the quota is insufficient.
func (s *SecretKey) Refund(ctx context.Context, key string, amount int) (bool, error) {
	if key != s.secretKey {
		return false, fmt.Errorf("invalid key")
	}
	// SecretKey adapter always allows refunds for valid keys
	return true, nil
}

// ServiceID returns the service ID this adapter is configured for
func (s *SecretKey) ServiceID() string {
	return s.serviceID
}
