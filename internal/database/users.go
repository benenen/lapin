package database

import (
	"context"
	"time"
)

type User struct {
	ID           int64
	Email        string
	DisplayName  string
	AvatarURL    string
	PasswordHash string
	CreatedAt    time.Time
}

func InsertUser(ctx context.Context, db DBTX, user User) (int64, error) {
	var id int64
	err := db.QueryRow(ctx, `
		INSERT INTO users (email, display_name, avatar_url, password_hash, created_at)
		VALUES ($1, $2, $3, $4, $5) RETURNING id
	`, user.Email, user.DisplayName, user.AvatarURL, user.PasswordHash, user.CreatedAt).Scan(&id)
	return id, err
}

func FindUserByEmail(ctx context.Context, db DBTX, email string) (User, error) {
	var user User
	err := db.QueryRow(ctx, `
		SELECT id, email, display_name, avatar_url, password_hash, created_at
		FROM users WHERE email = $1
	`, email).Scan(&user.ID, &user.Email, &user.DisplayName, &user.AvatarURL, &user.PasswordHash, &user.CreatedAt)
	return user, err
}

func FindUserByID(ctx context.Context, db DBTX, id int64) (User, error) {
	var user User
	err := db.QueryRow(ctx, `
		SELECT id, email, display_name, avatar_url, password_hash, created_at
		FROM users WHERE id = $1
	`, id).Scan(&user.ID, &user.Email, &user.DisplayName, &user.AvatarURL, &user.PasswordHash, &user.CreatedAt)
	return user, err
}

func LockUserForUpdate(ctx context.Context, db DBTX, id int64) error {
	var lockedID int64
	return db.QueryRow(ctx, `SELECT id FROM users WHERE id = $1 FOR UPDATE`, id).Scan(&lockedID)
}
