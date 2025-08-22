package admin_test

import (
	"context"
	"log"

	"httpcache/pkg/admin"

	"github.com/jackc/pgx/v5"
)

// Example demonstrates how to use the CheckUser and PrintUserInfo functions
func ExampleAdminService_CheckUser() {
	// Connect to database (you would use your actual database connection)
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, "your-database-url")
	if err != nil {
		log.Fatal("Failed to connect to database:", err)
	}
	defer conn.Close(ctx)

	// Create admin service
	adminService := admin.NewAdminService(conn)

	// Check user information
	userInfo, err := adminService.CheckUser(ctx, "user@example.com")
	if err != nil {
		log.Fatal("Failed to check user:", err)
	}

	// Display structured information
	log.Printf("User: %s (ID: %d)", userInfo.User.Email, userInfo.User.ID)
	log.Printf("Found %d API key(s)", len(userInfo.APIKeys))

	for i, apiKeyInfo := range userInfo.APIKeys {
		log.Printf("API Key %d: %s (%s)", i+1, apiKeyInfo.APIKey.KeyString, apiKeyInfo.APIKey.Status)
		log.Printf("  %d service quotas configured", len(apiKeyInfo.ServiceQuotas))
	}
}

// Example demonstrates how to use the PrintUserInfo function for formatted output
func ExampleAdminService_PrintUserInfo() {
	// Connect to database (you would use your actual database connection)
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, "your-database-url")
	if err != nil {
		log.Fatal("Failed to connect to database:", err)
	}
	defer conn.Close(ctx)

	// Create admin service
	adminService := admin.NewAdminService(conn)

	// Print user information in a human-readable format
	// This will print:
	// === User Information ===
	// ID: 123
	// Email: user@example.com
	// Created: 2024-01-15 10:30:45
	//
	// === API Key #1 ===
	// ID: 456
	// Key: pmhc_abc123...
	// Status: assigned
	// Created: 2024-01-15 10:30:45
	//
	// === Service Quotas ===
	// Service: jina-search
	//   Initial: 1000
	//   Remaining: 750
	//   Used: 250 (25.0%)
	//
	// Service: serper-search
	//   Initial: 500
	//   Remaining: 400
	//   Used: 100 (20.0%)

	err = adminService.PrintUserInfo(ctx, "user@example.com")
	if err != nil {
		log.Fatal("Failed to print user info:", err)
	}
}

