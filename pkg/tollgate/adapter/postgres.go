// Package adapter provides different implementations of the Adapter interface
package adapter

import (
	"context"

	"httpcache/pkg/dbsqlc"
	"httpcache/pkg/tollgate"
)

// Postgres implements the tollgate.Adapter interface using PostgreSQL
type Postgres struct {
	queries   *dbsqlc.Queries
	serviceID string
}

// NewPostgres creates a new PostgreSQL adapter for a specific service
func NewPostgres(db dbsqlc.DBTX, serviceID string) tollgate.Adapter {
	return &Postgres{
		queries:   dbsqlc.New(db),
		serviceID: serviceID,
	}
}

// Consume decrements the quota for the given API key and returns remaining balance
func (p *Postgres) Consume(ctx context.Context, key string) (int, error) {
	balance, err := p.queries.ConsumeQuotaByKeyString(ctx, key)
	if err != nil {
		return 0, err
	}
	return int(balance), nil
}

// Balance returns the current quota balance for the given API key
func (p *Postgres) Balance(ctx context.Context, key string) (int, error) {
	balance, err := p.queries.GetBalanceByKeyString(ctx, key)
	if err != nil {
		return 0, err
	}
	return int(balance), nil
}

// ServiceID returns the service ID this adapter is configured for
func (p *Postgres) ServiceID() string {
	return p.serviceID
}
