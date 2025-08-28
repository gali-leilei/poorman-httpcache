// Package adapter provides different implementations of the Adapter interface
package adapter

import (
	"context"
	"database/sql"
	"errors"

	"httpcache/pkg/dbsqlc"
	"httpcache/pkg/tollgate"
)

// Postgres implements the tollgate.Adapter interface using PostgreSQL
type Postgres struct {
	queries     *dbsqlc.Queries
	serviceName string
}

// NewPostgres creates a new PostgreSQL adapter for a specific service
func NewPostgres(db dbsqlc.DBTX, serviceName string) tollgate.Adapter {
	return &Postgres{
		queries:     dbsqlc.New(db),
		serviceName: serviceName,
	}
}

// Reserve reserves a given amount of quota for a key.
// Returns true if the reservation was successful, false if the quota is insufficient.
func (p *Postgres) Reserve(ctx context.Context, key string, amount int) (bool, error) {
	// Use the new ReserveQuota query to atomically check and reserve quota
	result, err := p.queries.ReserveQuota(ctx, &dbsqlc.ReserveQuotaParams{
		KeyString:      key,
		Name:           p.serviceName,
		RemainingQuota: int32(amount),
	})

	if err != nil {
		// If no rows were affected, it means insufficient quota
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, err
	}

	// Successfully reserved quota - result contains the remaining quota after reservation
	return result.RemainingQuota >= 0, nil
}

// Refund refunds a given amount of quota for a key.
// Returns true if the refund was successful, false if the quota is insufficient.
func (p *Postgres) Refund(ctx context.Context, key string, amount int) (bool, error) {
	// Use the new RefundQuota query to atomically refund quota
	result, err := p.queries.RefundQuota(ctx, &dbsqlc.RefundQuotaParams{
		KeyString:      key,
		Name:           p.serviceName,
		RemainingQuota: int32(amount),
	})

	if err != nil {
		// If no rows were affected, it means the key/service combination doesn't exist
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, err
	}

	// Successfully refunded quota - result contains the remaining quota after refund
	return result.RemainingQuota <= result.InitialQuota, nil
}
