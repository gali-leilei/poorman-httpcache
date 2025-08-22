// Package tollgate provides a tollgate middleware for HTTP requests.
package tollgate

import (
	"context"

	"httpcache/pkg/dbsqlc"
)

type Adapter interface {
	Consume(ctx context.Context, ticket string) (int, error)
	Balance(ctx context.Context, ticket string) (int, error)
	// Topup(ctx context.Context, ticket string, amount int) error
}

type PostgresAdapter struct {
	queries *dbsqlc.Queries
}

func NewPostgresAdapter(db dbsqlc.DBTX) Adapter {
	return &PostgresAdapter{
		queries: dbsqlc.New(db),
	}
}

func (a *PostgresAdapter) Consume(ctx context.Context, ticket string) (int, error) {
	balance, err := a.queries.ConsumeQuotaByKeyString(ctx, ticket)
	if err != nil {
		return 0, err
	}
	return int(balance), nil
}

func (a *PostgresAdapter) Balance(ctx context.Context, ticket string) (int, error) {
	balance, err := a.queries.GetBalanceByKeyString(ctx, ticket)
	if err != nil {
		return 0, err
	}
	return int(balance), nil
}

// func (a *PostgresAdapter) Topup(ctx context.Context, ticket string, amount int) error {
// 	return nil
// }
