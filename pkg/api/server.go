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
	adminService      *admin.AdminService
	db                *pgx.Conn
	logger            *slog.Logger
	adminKey          string
	adminUser         string
	adminPass         string
	trustTraefikAuth  bool
	traefikUserHeader string
}

// NewServer creates a new server instance
func NewServer(db *pgx.Conn, logger *slog.Logger) (*Server, error) {
	adminKey := os.Getenv("ADMIN_KEY")
	adminUser := os.Getenv("ADMIN_USERNAME")
	adminPass := os.Getenv("ADMIN_PASSWORD")
	trustTraefikAuth := os.Getenv("TRUST_TRAEFIK_AUTH") == "true"
	traefikUserHeader := os.Getenv("TRAEFIK_USER_HEADER")

	// Set default header if using Traefik auth but no custom header specified
	if trustTraefikAuth && traefikUserHeader == "" {
		traefikUserHeader = "X-Authenticated-User"
	}

	// Validation: require at least one authentication method
	hasDirectAuth := adminKey != "" || (adminUser != "" && adminPass != "")
	if !hasDirectAuth && !trustTraefikAuth {
		return nil, fmt.Errorf("authentication required: set ADMIN_KEY/ADMIN_USERNAME+ADMIN_PASSWORD or enable TRUST_TRAEFIK_AUTH=true")
	}

	return &Server{
		adminService:      admin.NewAdminService(db),
		db:                db,
		logger:            logger,
		adminKey:          adminKey,
		adminUser:         adminUser,
		adminPass:         adminPass,
		trustTraefikAuth:  trustTraefikAuth,
		traefikUserHeader: traefikUserHeader,
	}, nil
}

// validateAdminKey validates the X-Admin-Key header
func (s *Server) validateAdminKey(r *http.Request) bool {
	if s.adminKey == "" {
		return false
	}
	provided := r.Header.Get("X-Admin-Key")
	return provided == s.adminKey
}

// validateBasicAuth validates HTTP Basic Authentication
func (s *Server) validateBasicAuth(r *http.Request) bool {
	if s.adminUser == "" || s.adminPass == "" {
		return false
	}
	username, password, ok := r.BasicAuth()
	return ok && username == s.adminUser && password == s.adminPass
}

// validateTraefikAuth validates Traefik forwarded authentication
func (s *Server) validateTraefikAuth(r *http.Request) (bool, string) {
	if !s.trustTraefikAuth {
		return false, ""
	}

	// Check if Traefik forwarded an authenticated user
	user := r.Header.Get(s.traefikUserHeader)
	return user != "", user
}

// validateAdminAuth validates admin authentication (X-Admin-Key, Basic Auth, or Traefik forwarded)
func (s *Server) validateAdminAuth(r *http.Request) bool {
	// Try Traefik forwarded auth first
	if authenticated, _ := s.validateTraefikAuth(r); authenticated {
		return true
	}

	// Fall back to direct authentication methods
	return s.validateAdminKey(r) || s.validateBasicAuth(r)
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
	s.writeErrorWithTraces(w, statusCode, message, []string{})
}

// writeErrorWithTraces writes error response with traces
func (s *Server) writeErrorWithTraces(w http.ResponseWriter, statusCode int, message string, traces []string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	errorResponse := ErrorResponse{
		Code:   statusCode,
		Msg:    message,
		Traces: traces,
	}

	if err := json.NewEncoder(w).Encode(errorResponse); err != nil {
		s.logger.Error("Failed to encode error response", "error", err)
		// Fallback to plain text if JSON encoding fails
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("Internal server error"))
	}
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
	if !s.validateAdminAuth(r) {
		s.writeError(w, http.StatusUnauthorized, "Missing or invalid admin credentials")
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
	if !s.validateAdminAuth(r) {
		s.writeError(w, http.StatusUnauthorized, "Missing or invalid admin credentials")
		return
	}

	ctx := r.Context()

	// Parse request body
	var req CreateUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeErrorWithTraces(w, http.StatusBadRequest, "Invalid JSON request body", []string{"failed to parse JSON request body", err.Error()})
		return
	}

	// Validate email
	if req.Email == "" {
		s.writeErrorWithTraces(w, http.StatusBadRequest, "Email is required", []string{"validation failed for field 'email'", "email field cannot be empty"})
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
	if !s.validateAdminAuth(r) {
		s.writeError(w, http.StatusUnauthorized, "Missing or invalid admin credentials")
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
	if !s.validateAdminAuth(r) {
		s.writeError(w, http.StatusUnauthorized, "Missing or invalid admin credentials")
		return
	}

	ctx := r.Context()

	// Parse request body
	var req CreateApiKeyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeErrorWithTraces(w, http.StatusBadRequest, "Invalid JSON request body", []string{"failed to parse JSON request body", err.Error()})
		return
	}

	// Validate required fields
	if req.Email == "" {
		s.writeErrorWithTraces(w, http.StatusBadRequest, "email is required", []string{"validation failed for field 'email'", "email field cannot be empty"})
		return
	}

	// Use admin service to invite new user with API key
	// Pass !req.HasQuota as isServiceKey (service keys don't have quotas)
	result, err := s.adminService.InviteNewUser(ctx, string(req.Email), !req.HasQuota)
	if err != nil {
		s.logger.Error("Failed to create API key", "error", err, "email", req.Email)
		s.writeError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to create API key: %v", err))
		return
	}

	// Convert service quotas to API model
	var serviceQuotas []ServiceQuota
	for _, quota := range result.InitialQuotas {
		serviceQuotas = append(serviceQuotas, ServiceQuota{
			ServiceName:    quota.ServiceName,
			InitialQuota:   int(quota.InitialQuota),
			RemainingQuota: int(quota.RemainingQuota),
		})
	}

	// Create response
	response := CreateApiKeyResponse{
		ApiKey:        result.APIKey.KeyString,
		UserEmail:     openapi_types.Email(result.User.Email),
		ServiceQuotas: serviceQuotas,
	}

	s.writeJSON(w, http.StatusCreated, response)
}
