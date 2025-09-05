package admin

import (
	"context"
	"fmt"
	"httpcache/pkg/dbsqlc"
)

// AddKeyToUser adds an API key to an existing user and sets up quotas if it's not a service key.
func (as *AdminService) AddKeyToUser(ctx context.Context, userID int64, apiKey string) error {
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

	// Create user API key with quota in "unassigned" status
	apiKeyRecord, err := qtx.CreateUserAPIKey(ctx, &dbsqlc.CreateUserAPIKeyParams{
		UserID:    userID,
		KeyString: apiKey,
	})
	if err != nil {
		return fmt.Errorf("failed to create API key: %w", err)
	}

	// Update API key status to "assigned"
	apiKeyRecord, err = qtx.UpdateAPIKeyStatus(ctx, &dbsqlc.UpdateAPIKeyStatusParams{
		ID:     apiKeyRecord.ID,
		Status: "assigned",
	})
	if err != nil {
		return fmt.Errorf("failed to update API key status: %w", err)
	}

	// Initialize quotas only for normal user keys (not service keys)
	if !isServiceKey(apiKey) {
		// Get all services to set up quotas
		services, err := qtx.GetAllServices(ctx)
		if err != nil {
			return fmt.Errorf("failed to get services: %w", err)
		}

		// Initialize quotas for all services
		for _, service := range services {
			// Get service details to access default quota
			serviceDetails, err := qtx.GetServiceByName(ctx, service.Name)
			if err != nil {
				return fmt.Errorf("failed to get service details for %s: %w", service.Name, err)
			}

			_, err = qtx.InitializeKeyServiceQuota(ctx, &dbsqlc.InitializeKeyServiceQuotaParams{
				ApiKeyID:  apiKeyRecord.ID,
				ServiceID: service.ID,
				Available: serviceDetails.DefaultQuota,
			})
			if err != nil {
				return fmt.Errorf("failed to initialize quota for service %s: %w", service.Name, err)
			}
		}
	}

	// Commit transaction
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}
