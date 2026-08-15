package database

import (
	"context"
	"time"
)

type Annotation struct {
	ID          int64
	ChapterID   int64
	UserID      int64
	StartOffset int
	EndOffset   int
	Quote       string
	Note        string
	Color       string
	CreatedAt   time.Time
}

func InsertAnnotation(ctx context.Context, db DBTX, annotation Annotation) (int64, error) {
	var id int64
	err := db.QueryRow(ctx, `
		INSERT INTO annotations (chapter_id, user_id, start_offset, end_offset, quote, note, color, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8) RETURNING id
	`, annotation.ChapterID, annotation.UserID, annotation.StartOffset, annotation.EndOffset, annotation.Quote, annotation.Note, annotation.Color, annotation.CreatedAt).Scan(&id)
	return id, err
}

func ListAnnotationsByChapter(ctx context.Context, db DBTX, chapterID int64) ([]Annotation, error) {
	rows, err := db.Query(ctx, `
		SELECT id, chapter_id, user_id, start_offset, end_offset, quote, note, color, created_at
		FROM annotations WHERE chapter_id = $1 ORDER BY created_at DESC
	`, chapterID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]Annotation, 0)
	for rows.Next() {
		var item Annotation
		if err := rows.Scan(&item.ID, &item.ChapterID, &item.UserID, &item.StartOffset, &item.EndOffset, &item.Quote, &item.Note, &item.Color, &item.CreatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}
