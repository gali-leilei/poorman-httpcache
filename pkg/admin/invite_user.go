// Package admin provides administrative operations for user and API key management.
package admin

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"httpcache/pkg/dbsqlc"

	"github.com/jackc/pgx/v5"
)

// User represents a user in the system
type User struct {
	ID        int64     `json:"id"`
	Email     string    `json:"email"`
	CreatedAt time.Time `json:"created_at"`
}

// APIKey represents an API key assigned to a user
type APIKey struct {
	ID        int64     `json:"id"`
	KeyString string    `json:"key_string"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}

// ServiceQuota represents quota allocation for a service
type ServiceQuota struct {
	ServiceName    string `json:"service_name"`
	InitialQuota   int32  `json:"initial_quota"`
	RemainingQuota int32  `json:"remaining_quota"`
}

// InviteNewUserResult represents the result of inviting a new user
type InviteNewUserResult struct {
	User          *User           `json:"user"`
	APIKey        *APIKey         `json:"api_key"`
	InitialQuotas []*ServiceQuota `json:"initial_quotas"`
}

// AdminService provides administrative operations
type AdminService struct {
	db      *pgx.Conn
	queries *dbsqlc.Queries
}

// NewAdminService creates a new admin service
func NewAdminService(db *pgx.Conn) *AdminService {
	return &AdminService{
		db:      db,
		queries: dbsqlc.New(db),
	}
}

// ServiceKeyPrefix is the prefix for service keys
// format {prefix}{random_string}
const ServiceKeyPrefix = "svc-miro-api01-"

// generateAPIKey creates a secure random API key string
func generateAPIKey() (string, error) {
	bytes := make([]byte, 32) // 64 character hex string
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("failed to generate random bytes: %w", err)
	}
	return ServiceKeyPrefix + hex.EncodeToString(bytes), nil
}

// InviteNewUser assigns an API key to a new user and optionally sets up initial quotas for all services.
// If isServiceKey is true, creates a service key with no quota limitations.
// If isServiceKey is false, creates a normal user key with default quotas for all services.
// This function will return an error if the user already exists.
// This function uses a transaction to ensure atomicity across multiple database operations
func (as *AdminService) InviteNewUser(ctx context.Context, email string, isServiceKey bool) (*InviteNewUserResult, error) {
	// Start transaction
	tx, err := as.db.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx) // Will be a no-op if transaction is committed successfully

	// Create queries with transaction context
	qtx := as.queries.WithTx(tx)

	// Step 1: Check if user already exists
	_, err = qtx.GetUserByEmail(ctx, email)
	if err == nil {
		return nil, fmt.Errorf("user with email %s already exists", email)
	}

	// Step 2: Create new user
	user, err := qtx.CreateUser(ctx, email)
	if err != nil {
		return nil, fmt.Errorf("failed to create user: %w", err)
	}

	// Step 3: Generate API key string
	keyString, err := generateAPIKey()
	if err != nil {
		return nil, fmt.Errorf("failed to generate API key: %w", err)
	}

	// Step 4: Create user API key with quota in "unassigned" status
	apiKey, err := qtx.CreateUserAPIKey(ctx, &dbsqlc.CreateUserAPIKeyParams{
		UserID:    user.ID,
		KeyString: keyString,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create API key: %w", err)
	}

	// Step 5: Update API key status to "assigned"
	apiKey, err = qtx.UpdateAPIKeyStatus(ctx, &dbsqlc.UpdateAPIKeyStatusParams{
		ID:     apiKey.ID,
		Status: "assigned",
	})
	if err != nil {
		return nil, fmt.Errorf("failed to update API key status: %w", err)
	}

	// Step 6: Initialize quotas only for normal user keys (not service keys)
	var initialQuotas []*ServiceQuota
	if !isServiceKey {
		// Get all services to set up quotas
		services, err := qtx.GetAllServices(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to get services: %w", err)
		}

		// Initialize quotas for all services
		for _, service := range services {
			// Get service details to access default quota
			serviceDetails, err := qtx.GetServiceByName(ctx, service.Name)
			if err != nil {
				return nil, fmt.Errorf("failed to get service details for %s: %w", service.Name, err)
			}

			quota, err := qtx.InitializeKeyServiceQuota(ctx, &dbsqlc.InitializeKeyServiceQuotaParams{
				ApiKeyID:     apiKey.ID,
				ServiceID:    service.ID,
				InitialQuota: serviceDetails.DefaultQuota,
			})
			if err != nil {
				return nil, fmt.Errorf("failed to initialize quota for service %s: %w", service.Name, err)
			}

			// Map to domain model
			initialQuotas = append(initialQuotas, &ServiceQuota{
				ServiceName:    service.Name,
				InitialQuota:   quota.InitialQuota,
				RemainingQuota: quota.RemainingQuota,
			})
		}
	}

	// Commit transaction
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	// Map dbsqlc models to domain models
	return &InviteNewUserResult{
		User: &User{
			ID:        user.ID,
			Email:     user.Email,
			CreatedAt: user.CreatedAt.Time,
		},
		APIKey: &APIKey{
			ID:        apiKey.ID,
			KeyString: apiKey.KeyString,
			Status:    apiKey.Status,
			CreatedAt: apiKey.CreatedAt.Time,
		},
		InitialQuotas: initialQuotas,
	}, nil
}
