package database

import (
	"context"
	"time"
)

type Session struct {
	ID        int64
	UserID    int64
	TokenHash []byte
	CSRFHash  []byte
	ExpiresAt time.Time
}

func InsertSession(ctx context.Context, db DBTX, session Session) error {
	_, err := db.Exec(ctx, `
		INSERT INTO sessions (user_id, token_hash, csrf_hash, expires_at)
		VALUES ($1, $2, $3, $4)
	`, session.UserID, session.TokenHash, session.CSRFHash, session.ExpiresAt)
	return err
}

func FindActiveSessionByHash(ctx context.Context, db DBTX, tokenHash []byte) (Session, error) {
	var session Session
	err := db.QueryRow(ctx, `
		SELECT id, user_id, token_hash, csrf_hash, expires_at
		FROM sessions WHERE token_hash = $1 AND expires_at > NOW()
	`, tokenHash).Scan(&session.ID, &session.UserID, &session.TokenHash, &session.CSRFHash, &session.ExpiresAt)
	return session, err
}

func DeleteSessionByHash(ctx context.Context, db DBTX, tokenHash []byte) error {
	_, err := db.Exec(ctx, `DELETE FROM sessions WHERE token_hash = $1`, tokenHash)
	return err
}
