package admin

import (
	"context"
	"fmt"
)

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
