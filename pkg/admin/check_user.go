package admin

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// UserInfo represents complete user information including API key and quotas
type UserInfo struct {
	User    *User         `json:"user"`
	APIKeys []*APIKeyInfo `json:"api_keys"`
}

// APIKeyInfo represents an API key with its associated quotas
type APIKeyInfo struct {
	APIKey        *APIKey    `json:"api_key"`
	ServiceQuotas []*Service `json:"service_quotas"`
}

// CheckUser retrieves and displays an existing user's API key(s) and service quotas
func (as *AdminService) CheckUser(ctx context.Context, email string) (*UserInfo, error) {
	// Get user by email
	user, err := as.queries.GetUserByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("user with email %s not found", email)
		}
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	// Get all assigned API keys for the user
	apiKeyRecords, err := as.queries.GetAssignedAPIKeysByUserID(ctx, user.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to get user's API keys: %w", err)
	}

	// Build API key info for each key
	var apiKeys []*APIKeyInfo
	for _, apiKeyRecord := range apiKeyRecords {
		// Get quotas for this API key
		quotaRecords, err := as.queries.GetAPIKeyQuotas(ctx, apiKeyRecord.ID)
		if err != nil {
			return nil, fmt.Errorf("failed to get API key quotas for key %d: %w", apiKeyRecord.ID, err)
		}

		// Map quota records to domain models
		var serviceQuotas []*Service
		for _, quota := range quotaRecords {
			serviceQuotas = append(serviceQuotas, &Service{
				ID:             quota.ServiceID,
				Name:           quota.ServiceName,
				ServiceName:    quota.ServiceName,
				InitialQuota:   quota.Available + quota.Consumed, // Total quota
				RemainingQuota: quota.Available,                  // Available quota
			})
		}

		// Add this API key with its quotas
		apiKeys = append(apiKeys, &APIKeyInfo{
			APIKey: &APIKey{
				ID:        apiKeyRecord.ID,
				KeyString: apiKeyRecord.KeyString,
				Status:    apiKeyRecord.Status,
				CreatedAt: apiKeyRecord.CreatedAt.Time,
			},
			ServiceQuotas: serviceQuotas,
		})
	}

	// Build the result
	userInfo := &UserInfo{
		User: &User{
			ID:        user.ID,
			Email:     user.Email,
			CreatedAt: user.CreatedAt.Time,
		},
		APIKeys: apiKeys,
	}

	return userInfo, nil
}
