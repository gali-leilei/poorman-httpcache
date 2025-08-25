package admin_test

import (
	"context"
	"log"

	"httpcache/pkg/admin"

	"github.com/jackc/pgx/v5"
)

// DemoWorkflowInviteAndCheck demonstrates a complete workflow: invite a user and then check their information
func DemoWorkflowInviteAndCheck() {
	// Connect to database (you would use your actual database connection)
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, "your-database-url")
	if err != nil {
		log.Fatal("Failed to connect to database:", err)
	}
	defer conn.Close(ctx)

	// Create admin service
	adminService := admin.NewAdminService(conn)

	// Step 1: Invite a new user (normal user key with quotas)
	log.Println("=== Inviting New User ===")
	inviteResult, err := adminService.InviteNewUser(ctx, "newuser@example.com", false)
	if err != nil {
		log.Fatal("Failed to invite new user:", err)
	}

	log.Printf("✅ User invited successfully!")
	log.Printf("   User ID: %d, Email: %s", inviteResult.User.ID, inviteResult.User.Email)
	log.Printf("   API Key: %s (Status: %s)", inviteResult.APIKey.KeyString, inviteResult.APIKey.Status)
	log.Printf("   Initial quotas set up for %d services", len(inviteResult.InitialQuotas))

	// Step 2: Check the user we just created
	log.Println("\n=== Checking User Information ===")
	err = adminService.PrintUserInfo(ctx, "newuser@example.com")
	if err != nil {
		log.Fatal("Failed to check user info:", err)
	}

	// Step 3: Try to invite the same user again (this should fail)
	log.Println("\n=== Attempting to Invite Same User Again ===")
	_, err = adminService.InviteNewUser(ctx, "newuser@example.com", false)
	if err != nil {
		log.Printf("✅ Expected error: %v", err) // This should fail with "user already exists"
	} else {
		log.Printf("❌ Unexpected: second invitation should have failed")
	}

	// Step 4: Check programmatic access to user info
	log.Println("\n=== Programmatic Access to User Info ===")
	userInfo, err := adminService.CheckUser(ctx, "newuser@example.com")
	if err != nil {
		log.Fatal("Failed to get user info:", err)
	}

	log.Printf("User: %s (ID: %d)", userInfo.User.Email, userInfo.User.ID)
	log.Printf("Number of API keys: %d", len(userInfo.APIKeys))

	for i, keyInfo := range userInfo.APIKeys {
		log.Printf("API Key %d:", i+1)
		log.Printf("  Key: %s", keyInfo.APIKey.KeyString)
		log.Printf("  Status: %s", keyInfo.APIKey.Status)
		log.Printf("  Services with quotas: %d", len(keyInfo.ServiceQuotas))

		for _, quota := range keyInfo.ServiceQuotas {
			log.Printf("    %s: %d/%d remaining",
				quota.ServiceName, quota.RemainingQuota, quota.InitialQuota)
		}
	}
}
