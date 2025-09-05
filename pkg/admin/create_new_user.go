package admin

import (
	"context"
	"fmt"
)

// CreateNewUser creates a new user in the system.
// This function will return an error if the user already exists.
func (as *AdminService) CreateNewUser(ctx context.Context, email string, isServiceKey bool) (int64, error) {
	// Start transaction
	tx, err := as.db.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() {
		if rollbackErr := tx.Rollback(ctx); rollbackErr != nil {
			// Rollback errors are typically expected after successful commits
			_ = rollbackErr // Acknowledge but don't propagate rollback errors
		}
	}()

	// Create queries with transaction context
	qtx := as.queries.WithTx(tx)

	// Check if user already exists
	_, err = qtx.GetUserByEmail(ctx, email)
	if err == nil {
		return 0, fmt.Errorf("user with email %s already exists", email)
	}

	// Create new user
	user, err := qtx.CreateUser(ctx, email)
	if err != nil {
		return 0, fmt.Errorf("failed to create user: %w", err)
	}

	// Commit transaction
	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("failed to commit transaction: %w", err)
	}

	return user.ID, nil
}
