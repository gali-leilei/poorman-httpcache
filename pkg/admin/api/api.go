package api

import (
	"crypto/rand"
	"embed"
	"encoding/hex"
	"encoding/json"
	"httpcache/pkg/admin"
	"httpcache/pkg/dbsqlc"
	"log/slog"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"
)

//go:generate go tool oapi-codegen -config cfg.yaml api.yaml

//go:embed index.html
//go:embed api.yaml
//go:embed web-components.min.js
//go:embed styles.min.css
var SwaggerAsset embed.FS

// Server implements ServerInterface
type Server struct {
	db           *pgx.Conn
	adminService *admin.AdminService
	queries      *dbsqlc.Queries
	logger       *slog.Logger
	adminKey     string
}

var _ ServerInterface = &Server{}

// NewServer creates a new API server instance
func NewServer(db *pgx.Conn, logger *slog.Logger, adminKey string) *Server {
	return &Server{
		db:           db,
		adminService: admin.NewAdminService(db),
		queries:      dbsqlc.New(db),
		logger:       logger,
		adminKey:     adminKey,
	}
}

// writeJSONResponse writes a JSON response with the given status code
func (s *Server) writeJSONResponse(w http.ResponseWriter, statusCode int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		s.logger.Error("failed to encode JSON response", "error", err)
	}
}

// writeJSONError writes a JSON error response
func (s *Server) writeJSONError(w http.ResponseWriter, statusCode int, message string, traces []string) {
	errorResp := ErrorResponse{
		Code:   statusCode,
		Msg:    message,
		Traces: traces,
	}
	s.writeJSONResponse(w, statusCode, errorResp)
}

// validateAdminKey validates the X-Admin-Key header for admin endpoints
func (s *Server) validateAdminKey(w http.ResponseWriter, r *http.Request) bool {
	adminKey := r.Header.Get("X-Admin-Key")
	if adminKey == "" {
		s.writeJSONError(w, http.StatusUnauthorized, "Missing admin credentials", []string{"X-Admin-Key header is required"})
		return false
	}

	if s.adminKey == "" {
		s.logger.Error("Admin key not configured on server")
		s.writeJSONError(w, http.StatusInternalServerError, "Admin authentication not configured", []string{"Server admin key not set"})
		return false
	}

	if adminKey != s.adminKey {
		s.writeJSONError(w, http.StatusUnauthorized, "Invalid admin credentials", []string{"X-Admin-Key header value is invalid"})
		return false
	}

	return true
}

// Ping handles GET /ping
func (s *Server) Ping(w http.ResponseWriter, r *http.Request) {
	pong := Pong{
		Ping: "pong",
	}
	s.writeJSONResponse(w, http.StatusOK, pong)
}

// ListUsers handles GET /admin/users - List all users
func (s *Server) ListUsers(w http.ResponseWriter, r *http.Request) {
	// Validate admin authentication
	if !s.validateAdminKey(w, r) {
		return
	}

	ctx := r.Context()

	// Get all users from database
	dbUsers, err := s.queries.GetAllUsers(ctx)
	if err != nil {
		s.logger.Error("failed to get users", "error", err)
		s.writeJSONError(w, http.StatusInternalServerError, "Failed to retrieve users", []string{err.Error()})
		return
	}

	// Convert dbsqlc models to API models
	var users []User
	for _, dbUser := range dbUsers {
		user := User{
			Id:        dbUser.ID,
			Email:     openapi_types.Email(dbUser.Email),
			CreatedAt: dbUser.CreatedAt.Time,
		}
		users = append(users, user)
	}

	s.writeJSONResponse(w, http.StatusOK, users)
}

// CreateUser handles POST /admin/users - Create a new user
func (s *Server) CreateUser(w http.ResponseWriter, r *http.Request) {
	// Validate admin authentication
	if !s.validateAdminKey(w, r) {
		return
	}

	ctx := r.Context()

	// Parse request body
	var req CreateUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeJSONError(w, http.StatusBadRequest, "Invalid request body", []string{err.Error()})
		return
	}

	// Create the user using AdminService
	user, err := s.adminService.CreateNewUser(ctx, string(req.Email), false)
	if err != nil {
		s.logger.Error("failed to create user", "email", req.Email, "error", err)
		s.writeJSONError(w, http.StatusInternalServerError, "Failed to create user", []string{err.Error()})
		return
	}

	// Convert admin model to API model
	apiUser := User{
		Id:        user.ID,
		Email:     openapi_types.Email(req.Email),
		CreatedAt: user.CreatedAt,
	}

	s.writeJSONResponse(w, http.StatusCreated, apiUser)
}

// ListUserApiKeys handles GET /admin/user/{userId}/keys - List API keys for a specific user
func (s *Server) ListUserApiKeys(w http.ResponseWriter, r *http.Request, userId int64) {
	// Validate admin authentication
	if !s.validateAdminKey(w, r) {
		return
	}

	ctx := r.Context()

	// Get API keys for the specific user
	dbAPIKeys, err := s.queries.GetAPIKeysByUserID(ctx, userId)
	if err != nil {
		s.logger.Error("failed to get API keys for user", "userId", userId, "error", err)
		s.writeJSONError(w, http.StatusInternalServerError, "Failed to retrieve API keys", []string{err.Error()})
		return
	}

	// Convert dbsqlc models to API models
	var apiKeys []ApiKey
	for _, dbKey := range dbAPIKeys {
		apiKey := ApiKey{
			Id:        dbKey.ID,
			KeyString: dbKey.KeyString,
			Status:    ApiKeyStatus(dbKey.Status),
			HasQuota:  dbKey.HasQuota,
			UserId:    dbKey.UserID,
			CreatedAt: dbKey.CreatedAt.Time,
			UpdatedAt: dbKey.UpdatedAt.Time,
		}
		apiKeys = append(apiKeys, apiKey)
	}

	s.writeJSONResponse(w, http.StatusOK, apiKeys)
}

// CreateUserApiKey handles POST /admin/user/{userId}/keys - Create a new API key for a specific user
func (s *Server) CreateUserApiKey(w http.ResponseWriter, r *http.Request, userId int64) {
	// Validate admin authentication
	if !s.validateAdminKey(w, r) {
		return
	}

	ctx := r.Context()

	// Parse request body
	var req CreateApiKeyForUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeJSONError(w, http.StatusBadRequest, "Invalid request body", []string{err.Error()})
		return
	}

	// Get user by querying all users and finding the matching ID
	// Note: There's no direct GetUserByID method available
	allUsers, err := s.queries.GetAllUsers(ctx)
	if err != nil {
		s.logger.Error("failed to get users", "error", err)
		s.writeJSONError(w, http.StatusInternalServerError, "Failed to retrieve users", []string{err.Error()})
		return
	}

	var targetUser *dbsqlc.Users
	for _, user := range allUsers {
		if user.ID == userId {
			targetUser = user
			break
		}
	}

	if targetUser == nil {
		s.writeJSONError(w, http.StatusNotFound, "User not found", []string{"User with specified ID does not exist"})
		return
	}

	// Create API key for the existing user using the admin service
	// Generate API key string (similar to InviteNewUser)
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		s.logger.Error("failed to generate random bytes", "error", err)
		s.writeJSONError(w, http.StatusInternalServerError, "Failed to generate API key", []string{err.Error()})
		return
	}
	keyString := "svc-miro-api01-" + hex.EncodeToString(bytes)

	// Convert dbsqlc user to admin user
	adminUser := &admin.User{
		ID:        targetUser.ID,
		Email:     targetUser.Email,
		CreatedAt: targetUser.CreatedAt.Time,
	}

	// Add key to user
	apiKey, err := s.adminService.AddKeyToUser(ctx, adminUser, keyString)
	if err != nil {
		s.logger.Error("failed to create API key for user", "userId", userId, "error", err)
		s.writeJSONError(w, http.StatusInternalServerError, "Failed to create API key", []string{err.Error()})
		return
	}

	// For now, return empty service quotas since AddKeyToUser doesn't initialize quotas
	// In a complete implementation, you'd need to initialize quotas separately if !req.HasQuota
	var serviceQuotas []ServiceQuota

	// Build response
	response := CreateApiKeyResponse{
		ApiKey:        apiKey.KeyString,
		UserEmail:     openapi_types.Email(targetUser.Email),
		ServiceQuotas: serviceQuotas,
	}

	s.writeJSONResponse(w, http.StatusCreated, response)
}

// ListKeyServices handles GET /admin/user/{userId}/keys/{keyID}/services - List services associated with a specific API key
func (s *Server) ListKeyServices(w http.ResponseWriter, r *http.Request, userId int64, keyID int64) {
	// Validate admin authentication
	if !s.validateAdminKey(w, r) {
		return
	}

	ctx := r.Context()

	// Verify the API key exists and belongs to the user
	apiKey, err := s.queries.GetAPIKeyWithUser(ctx, keyID)
	if err != nil {
		s.logger.Error("failed to get API key", "keyID", keyID, "error", err)
		s.writeJSONError(w, http.StatusNotFound, "API key not found", []string{err.Error()})
		return
	}

	if apiKey.UserID != userId {
		s.writeJSONError(w, http.StatusNotFound, "API key not found for this user", []string{"API key does not belong to the specified user"})
		return
	}

	// Get quotas for this API key to determine which services are associated
	quotas, err := s.queries.GetAPIKeyQuotas(ctx, keyID)
	if err != nil {
		s.logger.Error("failed to get quotas for API key", "keyID", keyID, "error", err)
		s.writeJSONError(w, http.StatusInternalServerError, "Failed to retrieve quotas", []string{err.Error()})
		return
	}

	// Get all services to match quota service IDs to service details
	allServices, err := s.queries.GetAllServices(ctx)
	if err != nil {
		s.logger.Error("failed to get all services", "error", err)
		s.writeJSONError(w, http.StatusInternalServerError, "Failed to retrieve services", []string{err.Error()})
		return
	}

	// Build map of service ID to service details for efficient lookup
	serviceMap := make(map[int64]*dbsqlc.GetAllServicesRow)
	for _, svc := range allServices {
		serviceMap[svc.ID] = svc
	}

	// Convert quota information to services associated with this API key
	var services []Service
	for _, quota := range quotas {
		if svc, exists := serviceMap[quota.ServiceID]; exists {
			service := Service{
				Id:        svc.ID,
				Name:      svc.Name,
				Endpoint:  "",          // Services table doesn't have endpoint field
				CreatedAt: time.Time{}, // Using zero time as schema doesn't provide this in GetAllServices
			}
			// No description field in the actual services table
			services = append(services, service)
		}
	}

	s.writeJSONResponse(w, http.StatusOK, services)
}

// AssociateKeyService handles POST /admin/user/{userId}/keys/{keyID}/services - Associate a service with a specific API key
func (s *Server) AssociateKeyService(w http.ResponseWriter, r *http.Request, userId int64, keyID int64) {
	// Validate admin authentication
	if !s.validateAdminKey(w, r) {
		return
	}

	ctx := r.Context()

	// Parse request body
	var req AssociateServiceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeJSONError(w, http.StatusBadRequest, "Invalid request body", []string{err.Error()})
		return
	}

	// Verify the API key exists and belongs to the user
	apiKey, err := s.queries.GetAPIKeyWithUser(ctx, keyID)
	if err != nil {
		s.logger.Error("failed to get API key", "keyID", keyID, "error", err)
		s.writeJSONError(w, http.StatusNotFound, "API key not found", []string{err.Error()})
		return
	}

	if apiKey.UserID != userId {
		s.writeJSONError(w, http.StatusNotFound, "API key not found for this user", []string{"API key does not belong to the specified user"})
		return
	}

	// Get all services to find the target service
	allServices, err := s.queries.GetAllServices(ctx)
	if err != nil {
		s.logger.Error("failed to get services", "error", err)
		s.writeJSONError(w, http.StatusInternalServerError, "Failed to retrieve services", []string{err.Error()})
		return
	}

	// Find the target service
	var targetService *dbsqlc.GetAllServicesRow
	for _, svc := range allServices {
		if svc.ID == req.ServiceId {
			targetService = svc
			break
		}
	}

	if targetService == nil {
		s.writeJSONError(w, http.StatusNotFound, "Service not found", []string{"Service with specified ID does not exist"})
		return
	}

	// Associate the service with the API key using AdminService
	// Create APIKey object for the admin service call
	adminAPIKey := &admin.APIKey{
		ID:        apiKey.ID,
		KeyString: apiKey.KeyString,
		Status:    apiKey.Status,
		CreatedAt: apiKey.CreatedAt.Time,
	}

	// Create Service object for the admin service call
	adminService := &admin.Service{
		ID:           targetService.ID,
		Name:         targetService.Name,
		ServiceName:  targetService.Name,
		InitialQuota: targetService.DefaultQuota,
	}

	err = s.adminService.OpenServiceForKey(ctx, adminAPIKey, adminService)
	if err != nil {
		s.logger.Error("failed to associate service with API key", "keyID", keyID, "serviceId", req.ServiceId, "error", err)
		s.writeJSONError(w, http.StatusInternalServerError, "Failed to associate service", []string{err.Error()})
		return
	}

	// Return the service that was associated
	apiService := Service{
		Id:        targetService.ID,
		Name:      targetService.Name,
		Endpoint:  "",          // Services table doesn't have endpoint field
		CreatedAt: time.Time{}, // Using zero time as created_at isn't available in GetAllServices
	}
	// No description field in the actual services table

	s.writeJSONResponse(w, http.StatusCreated, apiService)
}

// NewHandlerWithMiddleware creates a new HTTP handler with custom middleware
func NewHandlerWithMiddleware(server *Server, middlewares ...MiddlewareFunc) http.Handler {
	return HandlerWithOptions(server, ChiServerOptions{
		Middlewares: middlewares,
	})
}
