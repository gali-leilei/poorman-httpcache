package admin_test

import (
	"context"
	"fmt"
	"log"

	"httpcache/pkg/admin"

	"github.com/jackc/pgx/v5"
)

// Example demonstrates how to use the InviteNewUser function
func ExampleAdminService_InviteNewUser() {
	// Connect to database (you would use your actual database connection)
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, "your-database-url")
	if err != nil {
		log.Fatal("Failed to connect to database:", err)
	}
	defer conn.Close(ctx)

	// Create admin service
	adminService := admin.NewAdminService(conn)

	// Invite a new user - this will:
	// 1. Check if user already exists (error if they do)
	// 2. Create the new user
	// 3. Generate and assign an API key
	// 4. Set up initial quotas for all services
	result, err := adminService.InviteNewUser(ctx, "user@example.com")
	if err != nil {
		log.Fatal("Failed to invite new user:", err)
	}

	fmt.Printf("New user invited successfully!\n")
	fmt.Printf("User ID: %d, Email: %s\n", result.User.ID, result.User.Email)
	fmt.Printf("API Key: %s (Status: %s)\n", result.APIKey.KeyString, result.APIKey.Status)
	fmt.Printf("Initial quotas set up for %d services\n", len(result.InitialQuotas))

	for _, quota := range result.InitialQuotas {
		fmt.Printf("  - %s: %d/%d quota remaining\n",
			quota.ServiceName, quota.RemainingQuota, quota.InitialQuota)
	}
}
