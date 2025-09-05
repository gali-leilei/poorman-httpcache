package admin

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
)

// InviteNewUserResult represents the result of inviting a new user
type InviteNewUserResult struct {
	User          *User      `json:"user"`
	APIKey        *APIKey    `json:"api_key"`
	InitialQuotas []*Service `json:"initial_quotas"`
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
	_, err = as.AddKeyToUser(ctx, userID, keyString)
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

	apiKeyBasic, err := qtx.GetAPIKeyByName(ctx, keyString)
	if err != nil {
		return nil, fmt.Errorf("failed to get created API key: %w", err)
	}

	// Get full API key details with timestamps
	apiKey, err := qtx.GetAPIKeyWithUser(ctx, apiKeyBasic.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to get API key details: %w", err)
	}

	// Get quotas if not a service key
	var initialQuotas []*Service
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

			initialQuotas = append(initialQuotas, &Service{
				ID:             quota.ServiceID,
				Name:           serviceName,
				ServiceName:    serviceName,
				InitialQuota:   quota.Available + quota.Consumed, // Total quota
				RemainingQuota: quota.Available,                  // Available quota
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
