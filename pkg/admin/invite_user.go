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

// CreateNewUser creates a new user in the system.
// This function will return an error if the user already exists.
func (as *AdminService) CreateNewUser(ctx context.Context, email string, isServiceKey bool) (int64, error) {
	// Start transaction
	tx, err := as.db.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() {
		if rollbackErr := tx.Rollback(ctx); rollbackErr != nil {
			// Rollback errors are typically expected after successful commits
			_ = rollbackErr // Acknowledge but don't propagate rollback errors
		}
	}()

	// Create queries with transaction context
	qtx := as.queries.WithTx(tx)

	// Check if user already exists
	_, err = qtx.GetUserByEmail(ctx, email)
	if err == nil {
		return 0, fmt.Errorf("user with email %s already exists", email)
	}

	// Create new user
	user, err := qtx.CreateUser(ctx, email)
	if err != nil {
		return 0, fmt.Errorf("failed to create user: %w", err)
	}

	// Commit transaction
	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("failed to commit transaction: %w", err)
	}

	return user.ID, nil
}

// isServiceKey checks if an API key string is a service key based on its prefix
func isServiceKey(apiKey string) bool {
	return len(apiKey) > len(ServiceKeyPrefix) && apiKey[:len(ServiceKeyPrefix)] == ServiceKeyPrefix
}

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
				ApiKeyID:     apiKeyRecord.ID,
				ServiceID:    service.ID,
				InitialQuota: serviceDetails.DefaultQuota,
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

// InviteNewUser assigns an API key to a new user and optionally sets up initial quotas for all services.
// If isServiceKey is true, creates a service key with no quota limitations.
// If isServiceKey is false, creates a normal user key with default quotas for all services.
// This function will return an error if the user already exists.
// This function now uses the split CreateNewUser and AddKeyToUser functions.
func (as *AdminService) InviteNewUser(ctx context.Context, email string, isServiceKey bool) (*InviteNewUserResult, error) {
	// Step 1: Create new user
	userID, err := as.CreateNewUser(ctx, email, isServiceKey)
	if err != nil {
		return nil, err
	}

	// Step 2: Generate API key string
	keyString, err := generateAPIKey()
	if err != nil {
		return nil, fmt.Errorf("failed to generate API key: %w", err)
	}

	// Step 3: Add key to user (this handles quota initialization internally)
	err = as.AddKeyToUser(ctx, userID, keyString)
	if err != nil {
		return nil, err
	}

	// Step 4: Fetch the created data to return the result
	// We need to query the database to get the full user and API key details
	qtx := as.queries

	// Get user by their email (no direct GetUserByID method available)
	user, err := qtx.GetUserByEmail(ctx, email)
	if err != nil {
		return nil, fmt.Errorf("failed to get created user: %w", err)
	}

	apiKeyBasic, err := qtx.GetAPIKeyByKeyString(ctx, keyString)
	if err != nil {
		return nil, fmt.Errorf("failed to get created API key: %w", err)
	}

	// Get full API key details with timestamps
	apiKey, err := qtx.GetAPIKeyWithUser(ctx, apiKeyBasic.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to get API key details: %w", err)
	}

	// Get quotas if not a service key
	var initialQuotas []*ServiceQuota
	if !isServiceKey {
		quotas, err := qtx.GetAPIKeyQuotas(ctx, apiKey.ID)
		if err != nil {
			return nil, fmt.Errorf("failed to get API key quotas: %w", err)
		}

		for _, quota := range quotas {
			// Get service details by name since no GetServiceByID is available
			// We need to get all services and find the matching one
			services, err := qtx.GetAllServices(ctx)
			if err != nil {
				return nil, fmt.Errorf("failed to get services: %w", err)
			}

			var serviceName string
			for _, service := range services {
				if service.ID == quota.ServiceID {
					serviceName = service.Name
					break
				}
			}

			if serviceName == "" {
				return nil, fmt.Errorf("service not found for ID %d", quota.ServiceID)
			}

			initialQuotas = append(initialQuotas, &ServiceQuota{
				ServiceName:    serviceName,
				InitialQuota:   quota.InitialQuota,
				RemainingQuota: quota.RemainingQuota,
			})
		}
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
