// Package admin provides administrative operations for user and API key management.
package admin

import (
	"httpcache/pkg/dbsqlc"
	"time"

	"github.com/jackc/pgx/v5"
)

// AdminService provides administrative operations
type AdminService struct {
	db      *pgx.Conn
	queries *dbsqlc.Queries
}

// NewAdminService creates a new admin service
func NewAdminService(db *pgx.Conn) *AdminService {
	return &AdminService{
		db:      db,
		queries: dbsqlc.New(db),
	}
}

// User represents a user in the system
type User struct {
	ID        int64     `json:"id"`
	Email     string    `json:"email"`
	CreatedAt time.Time `json:"created_at"`
}

// APIKey represents an API key assigned to a user
type APIKey struct {
	ID        int64     `json:"id"`
	KeyString string    `json:"key_string"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}

// Service represents quota allocation for a service
type Service struct {
	ID             int64  `json:"id"`
	Name           string `json:"service_name"`
	ServiceName    string `json:"-"` // Alias for compatibility (not serialized)
	InitialQuota   int32  `json:"initial_quota"`
	RemainingQuota int32  `json:"remaining_quota"` // Available quota
}
