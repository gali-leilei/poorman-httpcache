package admin

import (
	"context"
	"fmt"
	"httpcache/pkg/dbsqlc"
)

// OpenServiceForKey opens a service for a given key by initializing quota for the key-service pair
func (as *AdminService) OpenServiceForKey(ctx context.Context, key *APIKey, service *Service) error {
	// Start transaction
	tx, err := as.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() {
		if rollbackErr := tx.Rollback(ctx); rollbackErr != nil {
			// Rollback errors are typically expected after successful commits
			_ = rollbackErr // Acknowledge but don't propagate rollback errors
		}
	}()

	// Create queries with transaction context
	qtx := as.queries.WithTx(tx)

	// Get service details by name
	serviceDetails, err := qtx.GetServiceByName(ctx, service.ServiceName)
	if err != nil {
		return fmt.Errorf("failed to get service '%s': %w", service.ServiceName, err)
	}

	// Initialize quota for the key-service pair
	_, err = qtx.InitializeKeyServiceQuota(ctx, &dbsqlc.InitializeKeyServiceQuotaParams{
		ApiKeyID:  key.ID,
		ServiceID: serviceDetails.ID,
		Available: serviceDetails.DefaultQuota,
	})
	if err != nil {
		return fmt.Errorf("failed to initialize quota for key %d and service '%s': %w", key.ID, service.ServiceName, err)
	}

	// Commit transaction
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}
