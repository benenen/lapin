package database

import (
	"context"
	"time"
)

type Subject struct {
	ID          int64
	OwnerID     int64
	ExternalID  *string
	Title       string
	Description string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

func InsertSubject(ctx context.Context, db DBTX, subject Subject) (int64, error) {
	var id int64
	err := db.QueryRow(ctx, `
		INSERT INTO subjects (owner_id, title, description)
		VALUES ($1, $2, $3) RETURNING id
	`, subject.OwnerID, subject.Title, subject.Description).Scan(&id)
	return id, err
}

func UpsertExternalSubject(ctx context.Context, db DBTX, subject Subject) (int64, error) {
	var id int64
	err := db.QueryRow(ctx, `
		INSERT INTO subjects (owner_id, external_id, title, description)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (owner_id, external_id) WHERE external_id IS NOT NULL
		DO UPDATE SET title = EXCLUDED.title, description = EXCLUDED.description, updated_at = NOW()
		RETURNING id
	`, subject.OwnerID, subject.ExternalID, subject.Title, subject.Description).Scan(&id)
	return id, err
}

func ListSubjects(ctx context.Context, db DBTX) ([]Subject, error) {
	rows, err := db.Query(ctx, `
		SELECT id, owner_id, external_id, title, description, created_at, updated_at
		FROM subjects ORDER BY updated_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	subjects := make([]Subject, 0)
	for rows.Next() {
		var subject Subject
		if err := rows.Scan(&subject.ID, &subject.OwnerID, &subject.ExternalID, &subject.Title, &subject.Description, &subject.CreatedAt, &subject.UpdatedAt); err != nil {
			return nil, err
		}
		subjects = append(subjects, subject)
	}
	return subjects, rows.Err()
}

func FindSubjectByID(ctx context.Context, db DBTX, id int64) (Subject, error) {
	var subject Subject
	err := db.QueryRow(ctx, `
		SELECT id, owner_id, external_id, title, description, created_at, updated_at
		FROM subjects WHERE id = $1
	`, id).Scan(&subject.ID, &subject.OwnerID, &subject.ExternalID, &subject.Title, &subject.Description, &subject.CreatedAt, &subject.UpdatedAt)
	return subject, err
}

func IsSubjectOwner(ctx context.Context, db DBTX, subjectID, userID int64) (bool, error) {
	var exists bool
	err := db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM subjects WHERE id = $1 AND owner_id = $2)`, subjectID, userID).Scan(&exists)
	return exists, err
}

func LockSubjectForUpdate(ctx context.Context, db DBTX, subjectID int64) error {
	var id int64
	return db.QueryRow(ctx, `SELECT id FROM subjects WHERE id = $1 FOR UPDATE`, subjectID).Scan(&id)
}
