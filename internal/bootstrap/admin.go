package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/benenen/lapin/internal/auth"
	"github.com/benenen/lapin/internal/database"
)

const adminBootstrapLockID int64 = 1280332366

var ErrAdminEmailInUse = errors.New("ADMIN_EMAIL already belongs to a non-admin user")

type AdminCredentials struct {
	Email    string
	Password string
}

// EnsureAdmin creates the configured first administrator exactly once. Existing
// administrator passwords are never changed, and existing non-admin accounts
// are never promoted implicitly.
func EnsureAdmin(ctx context.Context, pool *pgxpool.Pool, credentials AdminCredentials) error {
	if credentials.Email == "" || credentials.Password == "" {
		return errors.New("administrator bootstrap credentials are required")
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin administrator bootstrap: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, adminBootstrapLockID); err != nil {
		return fmt.Errorf("lock administrator bootstrap: %w", err)
	}

	user, err := database.FindUserByEmail(ctx, tx, credentials.Email)
	if err == nil {
		roles, roleErr := database.ListRoleCodesForUser(ctx, tx, user.ID)
		if roleErr != nil {
			return fmt.Errorf("read administrator roles: %w", roleErr)
		}
		if !containsRole(roles, "admin") {
			return ErrAdminEmailInUse
		}
		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("commit administrator bootstrap: %w", err)
		}
		return nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("find administrator: %w", err)
	}

	passwordHash, err := auth.HashPassword(credentials.Password)
	if err != nil {
		return fmt.Errorf("hash administrator password: %w", err)
	}
	userID, err := database.InsertUser(ctx, tx, database.User{
		Email:        credentials.Email,
		DisplayName:  "Administrator",
		PasswordHash: passwordHash,
		CreatedAt:    time.Now().UTC(),
	})
	if err != nil {
		return fmt.Errorf("create administrator: %w", err)
	}
	if err := database.AssignRoleByCode(ctx, tx, userID, "admin"); err != nil {
		return fmt.Errorf("assign administrator role: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit administrator bootstrap: %w", err)
	}
	return nil
}

func containsRole(roles []string, wanted string) bool {
	for _, role := range roles {
		if role == wanted {
			return true
		}
	}
	return false
}
