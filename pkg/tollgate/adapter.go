// Package tollgate provides a tollgate middleware for HTTP requests.
package tollgate

import "context"

// Adapter defines the interface for quota management implementations
type Adapter interface {
	// Reserve reserves a given amount of quota for a key.
	// Returns true if the reservation was successful, false if the quota is insufficient.
	Reserve(ctx context.Context, key string, amount int) (bool, error)
	// Refund refunds a given amount of quota for a key.
	// Returns true if the refund was successful, false if the quota is insufficient.
	Refund(ctx context.Context, key string, amount int) (bool, error)
}
