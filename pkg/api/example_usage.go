package api

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/jackc/pgx/v5"
)

// ExampleServerIntegration shows how to integrate the ServerInterface implementation
// with your existing HTTP server setup.
//
// To use this in your main.go:
// 1. Add DATABASE_URL environment variable for PostgreSQL connection
// 2. Add ADMIN_KEY environment variable for admin authentication
// 3. Connect to database and create the server instance
// 4. Add the API routes to your mux
//
// Example environment variables:
//
//	DATABASE_URL=postgres://user:password@localhost:5432/dbname?sslmode=disable
//	ADMIN_KEY=your-super-secret-admin-key
func ExampleServerIntegration() {
	// This is example code showing how to integrate with main.go
	// You would add this to your run() function after creating your mux

	var (
		ctx    = context.Background()
		logger = slog.Default()
		mux    = http.NewServeMux() // Your existing mux
	)

	// 1. Connect to database (add to your Config struct and env parsing)
	dbURL := "postgres://user:password@localhost:5432/dbname?sslmode=disable"
	db, err := pgx.Connect(ctx, dbURL)
	if err != nil {
		logger.Error("Failed to connect to database", "error", err)
		return
	}
	defer db.Close(ctx)

	// 2. Create the API server instance
	apiServer, err := NewServer(db, logger)
	if err != nil {
		logger.Error("Failed to create API server", "error", err)
		return
	}

	// 3. Add API routes to your existing mux
	// This creates all the admin endpoints with proper OpenAPI routing
	apiHandler := Handler(apiServer)
	mux.Handle("/api/", http.StripPrefix("/api", apiHandler))

	// Your existing routes remain the same:
	// mux.HandleFunc("/jina/", ...)
	// mux.HandleFunc("/serper/", ...)
	// mux.HandleFunc("/docs", ...)

	fmt.Println("API endpoints available at:")
	fmt.Println("  GET /api/ping               - Health check")
	fmt.Println("  GET /api/admin/users        - List all users (requires X-Admin-Key)")
	fmt.Println("  POST /api/admin/users       - Create new user (requires X-Admin-Key)")
	fmt.Println("  GET /api/admin/keys         - List all API keys (requires X-Admin-Key)")
	fmt.Println("  POST /api/admin/keys        - Create new API key (requires X-Admin-Key)")
}

// Example HTTP requests you can make after integration:

// 1. Health check (no auth required):
// curl http://localhost:8080/api/ping

// 2. List all users (admin auth required):
// curl -H "X-Admin-Key: your-super-secret-admin-key" http://localhost:8080/api/admin/users

// 3. Create new user:
// curl -X POST \
//   -H "X-Admin-Key: your-super-secret-admin-key" \
//   -H "Content-Type: application/json" \
//   -d '{"email": "newuser@example.com"}' \
//   http://localhost:8080/api/admin/users

// 4. List all API keys:
// curl -H "X-Admin-Key: your-super-secret-admin-key" http://localhost:8080/api/admin/keys

// 5. Create new API key:
// curl -X POST \
//   -H "X-Admin-Key: your-super-secret-admin-key" \
//   -H "Content-Type: application/json" \
//   -d '{"user_id": 1, "key_string": "sk-example-key-123", "has_quota": true}' \
//   http://localhost:8080/api/admin/keys
