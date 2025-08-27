// Package tollgate provides a tollgate middleware for HTTP requests.
package tollgate

import "context"

// Adapter defines the interface for quota management implementations
type Adapter interface {
	Consume(ctx context.Context, key string) (int, error)
	Balance(ctx context.Context, key string) (int, error)
	// Topup(ctx context.Context, key string, amount int) error
}
