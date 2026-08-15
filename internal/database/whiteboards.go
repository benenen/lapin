package database

import (
	"context"
	"encoding/json"
	"time"
)

type Whiteboard struct {
	ID        int64
	ChapterID int64
	UserID    int64
	Data      json.RawMessage
	UpdatedAt time.Time
}

func UpsertWhiteboard(ctx context.Context, db DBTX, whiteboard Whiteboard) (int64, error) {
	var id int64
	err := db.QueryRow(ctx, `
		INSERT INTO whiteboards (chapter_id, user_id, data, updated_at)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (chapter_id, user_id)
		DO UPDATE SET data = EXCLUDED.data, updated_at = EXCLUDED.updated_at
		RETURNING id
	`, whiteboard.ChapterID, whiteboard.UserID, whiteboard.Data, whiteboard.UpdatedAt).Scan(&id)
	return id, err
}

func ListWhiteboardsByChapterAndUser(ctx context.Context, db DBTX, chapterID, userID int64) ([]Whiteboard, error) {
	rows, err := db.Query(ctx, `
		SELECT id, chapter_id, user_id, data, updated_at
		FROM whiteboards WHERE chapter_id = $1 AND user_id = $2 ORDER BY updated_at DESC
	`, chapterID, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]Whiteboard, 0)
	for rows.Next() {
		var item Whiteboard
		if err := rows.Scan(&item.ID, &item.ChapterID, &item.UserID, &item.Data, &item.UpdatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}
