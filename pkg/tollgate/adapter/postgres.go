// Package adapter provides different implementations of the Adapter interface
package adapter

import (
	"context"

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
	// For now, we consume the quota upfront (actual reservation logic needs implementation)
	balance, err := p.queries.ConsumeQuotaByKeyString(ctx, key)
	if err != nil {
		return false, err
	}
	// Return true if we have sufficient balance (simplified logic)
	return int(balance) >= amount, nil
}

// Refund refunds a given amount of quota for a key.
// Returns true if the refund was successful, false if the quota is insufficient.
func (p *Postgres) Refund(ctx context.Context, key string, amount int) (bool, error) {
	// TODO: Implement proper quota refunding mechanism in database
	// For now, always return true as refunding mechanism needs to be implemented
	_, err := p.queries.GetBalanceByName(ctx, &dbsqlc.GetBalanceByNameParams{
		KeyString: key,
		Name:      p.serviceName,
	})
	if err != nil {
		return false, err
	}
	return true, nil
}

// ServiceID returns the service ID this adapter is configured for
func (p *Postgres) ServiceID() string {
	return p.serviceName
}
