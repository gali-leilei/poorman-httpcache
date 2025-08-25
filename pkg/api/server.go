package api

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"

	"httpcache/pkg/admin"
	"httpcache/pkg/dbsqlc"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"
)

// Server implements the ServerInterface
type Server struct {
	adminService *admin.AdminService
	db           *pgx.Conn
	logger       *slog.Logger
	adminKey     string
}

// NewServer creates a new server instance
func NewServer(db *pgx.Conn, logger *slog.Logger) (*Server, error) {
	adminKey := os.Getenv("ADMIN_KEY")
	if adminKey == "" {
		return nil, fmt.Errorf("ADMIN_KEY environment variable is required")
	}

	return &Server{
		adminService: admin.NewAdminService(db),
		db:           db,
		logger:       logger,
		adminKey:     adminKey,
	}, nil
}

// validateAdminKey validates the X-Admin-Key header
func (s *Server) validateAdminKey(r *http.Request) bool {
	provided := r.Header.Get("X-Admin-Key")
	return provided == s.adminKey
}

// writeJSON writes JSON response with proper content type
func (s *Server) writeJSON(w http.ResponseWriter, statusCode int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		s.logger.Error("Failed to encode JSON response", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

// writeError writes error response
func (s *Server) writeError(w http.ResponseWriter, statusCode int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(map[string]string{"error": message})
}

// mapDBUserToAPIUser converts database user to API user model
func mapDBUserToAPIUser(dbUser *dbsqlc.Users) User {
	return User{
		Id:        dbUser.ID,
		Email:     openapi_types.Email(dbUser.Email),
		CreatedAt: dbUser.CreatedAt.Time,
	}
}

// mapDBAPIKeyToAPIKey converts database API key to API key model
func mapDBAPIKeyToAPIKey(dbAPIKey *dbsqlc.ApiKeys) ApiKey {
	return ApiKey{
		Id:        dbAPIKey.ID,
		UserId:    dbAPIKey.UserID,
		KeyString: dbAPIKey.KeyString,
		Status:    dbAPIKey.Status,
		HasQuota:  dbAPIKey.HasQuota.Bool,
		CreatedAt: dbAPIKey.CreatedAt.Time,
		UpdatedAt: dbAPIKey.UpdatedAt.Time,
	}
}

// GetPing handles the ping endpoint
func (s *Server) GetPing(w http.ResponseWriter, r *http.Request) {
	s.writeJSON(w, http.StatusOK, Pong{Ping: "pong"})
}

// GetAdminUsers lists all users
func (s *Server) GetAdminUsers(w http.ResponseWriter, r *http.Request) {
	// Validate admin authentication
	if !s.validateAdminKey(r) {
		s.writeError(w, http.StatusUnauthorized, "Missing or invalid X-Admin-Key")
		return
	}

	ctx := r.Context()
	queries := dbsqlc.New(s.db)

	// Get all users from database
	dbUsers, err := queries.GetAllUsers(ctx)
	if err != nil {
		s.logger.Error("Failed to get users", "error", err)
		s.writeError(w, http.StatusInternalServerError, "Failed to retrieve users")
		return
	}

	// Convert to API models
	var users []User
	for _, dbUser := range dbUsers {
		users = append(users, mapDBUserToAPIUser(dbUser))
	}

	s.writeJSON(w, http.StatusOK, users)
}

// PostAdminUsers creates a new user
func (s *Server) PostAdminUsers(w http.ResponseWriter, r *http.Request) {
	// Validate admin authentication
	if !s.validateAdminKey(r) {
		s.writeError(w, http.StatusUnauthorized, "Missing or invalid X-Admin-Key")
		return
	}

	ctx := r.Context()

	// Parse request body
	var req CreateUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, "Invalid JSON request body")
		return
	}

	// Validate email
	if req.Email == "" {
		s.writeError(w, http.StatusBadRequest, "Email is required")
		return
	}

	// Use admin service to create user with API key
	result, err := s.adminService.InviteNewUser(ctx, string(req.Email), false)
	if err != nil {
		s.logger.Error("Failed to create user", "error", err, "email", req.Email)
		s.writeError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to create user: %v", err))
		return
	}

	// Convert admin result to API user model
	user := User{
		Id:        result.User.ID,
		Email:     openapi_types.Email(result.User.Email),
		CreatedAt: result.User.CreatedAt,
	}

	s.writeJSON(w, http.StatusCreated, user)
}

// GetAdminKeys lists all API keys
func (s *Server) GetAdminKeys(w http.ResponseWriter, r *http.Request) {
	// Validate admin authentication
	if !s.validateAdminKey(r) {
		s.writeError(w, http.StatusUnauthorized, "Missing or invalid X-Admin-Key")
		return
	}

	ctx := r.Context()
	queries := dbsqlc.New(s.db)

	// Get all API keys from database
	dbAPIKeys, err := queries.GetAllAPIKeys(ctx)
	if err != nil {
		s.logger.Error("Failed to get API keys", "error", err)
		s.writeError(w, http.StatusInternalServerError, "Failed to retrieve API keys")
		return
	}

	// Convert to API models
	var apiKeys []ApiKey
	for _, dbAPIKey := range dbAPIKeys {
		apiKeys = append(apiKeys, mapDBAPIKeyToAPIKey(dbAPIKey))
	}

	s.writeJSON(w, http.StatusOK, apiKeys)
}

// PostAdminKeys creates a new API key
func (s *Server) PostAdminKeys(w http.ResponseWriter, r *http.Request) {
	// Validate admin authentication
	if !s.validateAdminKey(r) {
		s.writeError(w, http.StatusUnauthorized, "Missing or invalid X-Admin-Key")
		return
	}

	ctx := r.Context()

	// Parse request body
	var req CreateApiKeyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, "Invalid JSON request body")
		return
	}

	// Validate required fields
	if req.UserId == 0 {
		s.writeError(w, http.StatusBadRequest, "user_id is required")
		return
	}
	if req.KeyString == "" {
		s.writeError(w, http.StatusBadRequest, "key_string is required")
		return
	}

	queries := dbsqlc.New(s.db)

	// Note: We'll let the database foreign key constraint validate the user_id
	// since we don't have a GetUserByID query available

	// Create the API key directly using the database queries
	var dbAPIKey *dbsqlc.ApiKeys
	var err error
	if req.HasQuota {
		dbAPIKey, err = queries.CreateUserAPIKey(ctx, &dbsqlc.CreateUserAPIKeyParams{
			UserID:    req.UserId,
			KeyString: req.KeyString,
		})
	} else {
		dbAPIKey, err = queries.CreateServiceKey(ctx, &dbsqlc.CreateServiceKeyParams{
			UserID:    req.UserId,
			KeyString: req.KeyString,
		})
	}

	if err != nil {
		s.logger.Error("Failed to create API key", "error", err, "user_id", req.UserId)
		s.writeError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to create API key: %v", err))
		return
	}

	// Convert to API model
	apiKey := mapDBAPIKeyToAPIKey(dbAPIKey)

	s.writeJSON(w, http.StatusCreated, apiKey)
}
