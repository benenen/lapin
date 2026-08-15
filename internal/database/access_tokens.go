package database

import (
	"context"
	"time"
)

type AccessToken struct {
	ID         int64
	UserID     int64
	Name       string
	Prefix     string
	TokenHash  []byte
	LastUsedAt *time.Time
	ExpiresAt  time.Time
	CreatedAt  time.Time
}

func InsertAccessToken(ctx context.Context, db DBTX, token AccessToken) (int64, error) {
	var id int64
	err := db.QueryRow(ctx, `
		INSERT INTO access_tokens (user_id, name, token_prefix, token_hash, expires_at, created_at)
		VALUES ($1, $2, $3, $4, $5, $6) RETURNING id
	`, token.UserID, token.Name, token.Prefix, token.TokenHash, token.ExpiresAt, token.CreatedAt).Scan(&id)
	return id, err
}

func ListActiveAccessTokens(ctx context.Context, db DBTX, userID int64) ([]AccessToken, error) {
	rows, err := db.Query(ctx, `
		SELECT id, user_id, name, token_prefix, last_used_at, expires_at, created_at
		FROM access_tokens
		WHERE user_id = $1 AND revoked_at IS NULL AND expires_at > NOW()
		ORDER BY created_at DESC
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	tokens := make([]AccessToken, 0)
	for rows.Next() {
		var token AccessToken
		if err := rows.Scan(&token.ID, &token.UserID, &token.Name, &token.Prefix, &token.LastUsedAt, &token.ExpiresAt, &token.CreatedAt); err != nil {
			return nil, err
		}
		tokens = append(tokens, token)
	}
	return tokens, rows.Err()
}

func CountActiveAccessTokens(ctx context.Context, db DBTX, userID int64) (int, error) {
	var count int
	err := db.QueryRow(ctx, `
		SELECT COUNT(*) FROM access_tokens
		WHERE user_id = $1 AND revoked_at IS NULL AND expires_at > NOW()
	`, userID).Scan(&count)
	return count, err
}

func RevokeAccessToken(ctx context.Context, db DBTX, id, userID int64) (bool, error) {
	result, err := db.Exec(ctx, `
		UPDATE access_tokens SET revoked_at = NOW()
		WHERE id = $1 AND user_id = $2 AND revoked_at IS NULL
	`, id, userID)
	return err == nil && result.RowsAffected() > 0, err
}

func FindActiveAccessTokenByHash(ctx context.Context, db DBTX, tokenHash []byte) (AccessToken, error) {
	var token AccessToken
	err := db.QueryRow(ctx, `
		SELECT id, user_id, name, token_prefix, last_used_at, expires_at, created_at
		FROM access_tokens
		WHERE token_hash = $1 AND revoked_at IS NULL AND expires_at > NOW()
	`, tokenHash).Scan(&token.ID, &token.UserID, &token.Name, &token.Prefix, &token.LastUsedAt, &token.ExpiresAt, &token.CreatedAt)
	return token, err
}

func TouchAccessToken(ctx context.Context, db DBTX, id int64) error {
	_, err := db.Exec(ctx, `UPDATE access_tokens SET last_used_at = NOW() WHERE id = $1`, id)
	return err
}
