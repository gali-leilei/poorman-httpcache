// Package admin provides administrative operations for user and API key management.
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
	APIKey        *APIKey         `json:"api_key"`
	ServiceQuotas []*ServiceQuota `json:"service_quotas"`
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
		var serviceQuotas []*ServiceQuota
		for _, quota := range quotaRecords {
			serviceQuotas = append(serviceQuotas, &ServiceQuota{
				ServiceName:    quota.ServiceName,
				InitialQuota:   quota.InitialQuota,
				RemainingQuota: quota.RemainingQuota,
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

// PrintUserInfo prints user information in a human-readable format
func (as *AdminService) PrintUserInfo(ctx context.Context, email string) error {
	userInfo, err := as.CheckUser(ctx, email)
	if err != nil {
		return err
	}

	// Print user basic info
	fmt.Printf("=== User Information ===\n")
	fmt.Printf("ID: %d\n", userInfo.User.ID)
	fmt.Printf("Email: %s\n", userInfo.User.Email)
	fmt.Printf("Created: %s\n", userInfo.User.CreatedAt.Format("2006-01-02 15:04:05"))
	fmt.Printf("\n")

	// Print API keys and quotas
	if len(userInfo.APIKeys) == 0 {
		fmt.Printf("No API keys found for this user.\n")
		return nil
	}

	for i, apiKeyInfo := range userInfo.APIKeys {
		fmt.Printf("=== API Key #%d ===\n", i+1)
		fmt.Printf("ID: %d\n", apiKeyInfo.APIKey.ID)
		fmt.Printf("Key: %s\n", apiKeyInfo.APIKey.KeyString)
		fmt.Printf("Status: %s\n", apiKeyInfo.APIKey.Status)
		fmt.Printf("Created: %s\n", apiKeyInfo.APIKey.CreatedAt.Format("2006-01-02 15:04:05"))
		fmt.Printf("\n")

		if len(apiKeyInfo.ServiceQuotas) == 0 {
			fmt.Printf("No service quotas configured.\n")
		} else {
			fmt.Printf("=== Service Quotas ===\n")
			for _, quota := range apiKeyInfo.ServiceQuotas {
				usage := quota.InitialQuota - quota.RemainingQuota
				usagePercent := float64(usage) / float64(quota.InitialQuota) * 100

				fmt.Printf("Service: %s\n", quota.ServiceName)
				fmt.Printf("  Initial: %d\n", quota.InitialQuota)
				fmt.Printf("  Remaining: %d\n", quota.RemainingQuota)
				fmt.Printf("  Used: %d (%.1f%%)\n", usage, usagePercent)
				fmt.Printf("\n")
			}
		}
	}

	return nil
}
