package database

import (
	"context"
	"time"
)

type Chapter struct {
	ID         int64
	SubjectID  int64
	ParentID   *int64
	ExternalID *string
	Position   int
	Title      string
	Content    string
	CreatedAt  time.Time
}

func InsertChapter(ctx context.Context, db DBTX, chapter Chapter) (int64, error) {
	var id int64
	err := db.QueryRow(ctx, `
		INSERT INTO chapters (subject_id, parent_id, external_id, position, title, content)
		VALUES ($1, $2, $3, $4, $5, $6) RETURNING id
	`, chapter.SubjectID, chapter.ParentID, chapter.ExternalID, chapter.Position, chapter.Title, chapter.Content).Scan(&id)
	return id, err
}

func UpsertExternalChapter(ctx context.Context, db DBTX, chapter Chapter) (int64, error) {
	var id int64
	err := db.QueryRow(ctx, `
		INSERT INTO chapters (subject_id, parent_id, external_id, position, title, content)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (subject_id, external_id) WHERE external_id IS NOT NULL
		DO UPDATE SET parent_id = EXCLUDED.parent_id, position = EXCLUDED.position,
		              title = EXCLUDED.title, content = EXCLUDED.content
		RETURNING id
	`, chapter.SubjectID, chapter.ParentID, chapter.ExternalID, chapter.Position, chapter.Title, chapter.Content).Scan(&id)
	return id, err
}

func ListChaptersBySubject(ctx context.Context, db DBTX, subjectID int64) ([]Chapter, error) {
	rows, err := db.Query(ctx, `
		SELECT id, subject_id, parent_id, external_id, position, title, content, created_at
		FROM chapters WHERE subject_id = $1 ORDER BY position, id
	`, subjectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	chapters := make([]Chapter, 0)
	for rows.Next() {
		var chapter Chapter
		if err := rows.Scan(&chapter.ID, &chapter.SubjectID, &chapter.ParentID, &chapter.ExternalID, &chapter.Position, &chapter.Title, &chapter.Content, &chapter.CreatedAt); err != nil {
			return nil, err
		}
		chapters = append(chapters, chapter)
	}
	return chapters, rows.Err()
}

func FindChapterByID(ctx context.Context, db DBTX, chapterID int64) (Chapter, error) {
	var chapter Chapter
	err := db.QueryRow(ctx, `
		SELECT id, subject_id, parent_id, external_id, position, title, content, created_at
		FROM chapters WHERE id = $1
	`, chapterID).Scan(&chapter.ID, &chapter.SubjectID, &chapter.ParentID, &chapter.ExternalID, &chapter.Position, &chapter.Title, &chapter.Content, &chapter.CreatedAt)
	return chapter, err
}

func UpdateChapterForOwner(ctx context.Context, db DBTX, chapterID, ownerID int64, title, content string) (Chapter, error) {
	var chapter Chapter
	err := db.QueryRow(ctx, `
		UPDATE chapters AS chapter
		SET title = $3, content = $4
		FROM subjects AS subject
		WHERE chapter.id = $1 AND chapter.subject_id = subject.id AND subject.owner_id = $2
		RETURNING chapter.id, chapter.subject_id, chapter.parent_id, chapter.external_id,
		          chapter.position, chapter.title, chapter.content, chapter.created_at
	`, chapterID, ownerID, title, content).Scan(
		&chapter.ID, &chapter.SubjectID, &chapter.ParentID, &chapter.ExternalID,
		&chapter.Position, &chapter.Title, &chapter.Content, &chapter.CreatedAt,
	)
	return chapter, err
}

func ChapterExists(ctx context.Context, db DBTX, chapterID int64) (bool, error) {
	var exists bool
	err := db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM chapters WHERE id = $1)`, chapterID).Scan(&exists)
	return exists, err
}

func ChapterExistsInSubject(ctx context.Context, db DBTX, chapterID, subjectID int64) (bool, error) {
	var exists bool
	err := db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM chapters WHERE id = $1 AND subject_id = $2)`, chapterID, subjectID).Scan(&exists)
	return exists, err
}

func NextChapterPosition(ctx context.Context, db DBTX, subjectID int64) (int, error) {
	var position int
	err := db.QueryRow(ctx, `SELECT COALESCE(MAX(position), -1) + 1 FROM chapters WHERE subject_id = $1`, subjectID).Scan(&position)
	return position, err
}

func RepositionUnimportedChapters(ctx context.Context, db DBTX, subjectID int64, importedIDs []int64, start int) error {
	_, err := db.Exec(ctx, `
		WITH untouched AS (
			SELECT id, ROW_NUMBER() OVER (ORDER BY position, id) - 1 AS offset
			FROM chapters
			WHERE subject_id = $1 AND NOT (id = ANY($2::bigint[]))
		)
		UPDATE chapters AS chapter
		SET position = $3 + untouched.offset
		FROM untouched
		WHERE chapter.id = untouched.id
	`, subjectID, importedIDs, start)
	return err
}
