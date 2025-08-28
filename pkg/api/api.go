package api

import (
	"embed"
	"encoding/json"
	"httpcache/pkg/admin"
	"httpcache/pkg/dbsqlc"
	"log/slog"
	"net/http"

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

// GetPing handles GET /ping
func (s *Server) GetPing(w http.ResponseWriter, r *http.Request) {
	pong := Pong{
		Ping: "pong",
	}
	s.writeJSONResponse(w, http.StatusOK, pong)
}

// GetAdminUsers handles GET /admin/users - List all users
func (s *Server) GetAdminUsers(w http.ResponseWriter, r *http.Request) {
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

// PostAdminUsers handles POST /admin/users - Create a new user
func (s *Server) PostAdminUsers(w http.ResponseWriter, r *http.Request) {
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
	result, err := s.adminService.InviteNewUser(ctx, string(req.Email), false)
	if err != nil {
		s.logger.Error("failed to create user", "email", req.Email, "error", err)
		s.writeJSONError(w, http.StatusInternalServerError, "Failed to create user", []string{err.Error()})
		return
	}

	// Convert admin model to API model
	apiUser := User{
		Id:        result.User.ID,
		Email:     openapi_types.Email(result.User.Email),
		CreatedAt: result.User.CreatedAt,
	}

	s.writeJSONResponse(w, http.StatusCreated, apiUser)
}

// GetAdminKeys handles GET /admin/keys - List all API keys
func (s *Server) GetAdminKeys(w http.ResponseWriter, r *http.Request) {
	// Validate admin authentication
	if !s.validateAdminKey(w, r) {
		return
	}

	ctx := r.Context()

	// Get all API keys from database
	dbAPIKeys, err := s.queries.GetAllAPIKeys(ctx)
	if err != nil {
		s.logger.Error("failed to get API keys", "error", err)
		s.writeJSONError(w, http.StatusInternalServerError, "Failed to retrieve API keys", []string{err.Error()})
		return
	}

	// Convert dbsqlc models to API models
	var apiKeys []ApiKey
	for _, dbKey := range dbAPIKeys {
		apiKey := ApiKey{
			Id:        dbKey.ID,
			KeyString: dbKey.KeyString,
			Status:    dbKey.Status,
			HasQuota:  dbKey.HasQuota,
			UserId:    dbKey.UserID,
			CreatedAt: dbKey.CreatedAt.Time,
			UpdatedAt: dbKey.UpdatedAt.Time,
		}
		apiKeys = append(apiKeys, apiKey)
	}

	s.writeJSONResponse(w, http.StatusOK, apiKeys)
}

// PostAdminKeys handles POST /admin/keys - Create a new API key
func (s *Server) PostAdminKeys(w http.ResponseWriter, r *http.Request) {
	// Validate admin authentication
	if !s.validateAdminKey(w, r) {
		return
	}

	ctx := r.Context()

	// Parse request body
	var req CreateApiKeyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeJSONError(w, http.StatusBadRequest, "Invalid request body", []string{err.Error()})
		return
	}

	// Create API key using AdminService (InviteNewUser creates user + API key)
	result, err := s.adminService.InviteNewUser(ctx, string(req.Email), !req.HasQuota)
	if err != nil {
		s.logger.Error("failed to create API key", "email", req.Email, "error", err)
		s.writeJSONError(w, http.StatusInternalServerError, "Failed to create API key", []string{err.Error()})
		return
	}

	// Convert service quotas to API format
	var serviceQuotas []ServiceQuota
	for _, sq := range result.InitialQuotas {
		serviceQuota := ServiceQuota{
			ServiceName:    sq.ServiceName,
			InitialQuota:   int(sq.InitialQuota),
			RemainingQuota: int(sq.RemainingQuota),
		}
		serviceQuotas = append(serviceQuotas, serviceQuota)
	}

	// Build response
	response := CreateApiKeyResponse{
		ApiKey:        result.APIKey.KeyString,
		UserEmail:     openapi_types.Email(result.User.Email),
		ServiceQuotas: serviceQuotas,
	}

	s.writeJSONResponse(w, http.StatusCreated, response)
}

// NewHandlerWithMiddleware creates a new HTTP handler with custom middleware
func NewHandlerWithMiddleware(server *Server, middlewares ...MiddlewareFunc) http.Handler {
	return HandlerWithOptions(server, ChiServerOptions{
		Middlewares: middlewares,
	})
}
