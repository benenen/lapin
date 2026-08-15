package database

import (
	"context"
	"time"
)

type Comment struct {
	ID        int64
	ChapterID int64
	UserID    int64
	Body      string
	CreatedAt time.Time
}

func InsertComment(ctx context.Context, db DBTX, comment Comment) (int64, error) {
	var id int64
	err := db.QueryRow(ctx, `
		INSERT INTO comments (chapter_id, user_id, body, created_at)
		VALUES ($1, $2, $3, $4) RETURNING id
	`, comment.ChapterID, comment.UserID, comment.Body, comment.CreatedAt).Scan(&id)
	return id, err
}

func ListCommentsByChapter(ctx context.Context, db DBTX, chapterID int64) ([]Comment, error) {
	rows, err := db.Query(ctx, `
		SELECT id, chapter_id, user_id, body, created_at
		FROM comments WHERE chapter_id = $1 ORDER BY created_at, id
	`, chapterID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]Comment, 0)
	for rows.Next() {
		var item Comment
		if err := rows.Scan(&item.ID, &item.ChapterID, &item.UserID, &item.Body, &item.CreatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}
