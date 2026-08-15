package database

import "context"

func DeleteTagsBySubject(ctx context.Context, db DBTX, subjectID int64) error {
	_, err := db.Exec(ctx, `DELETE FROM tags WHERE subject_id = $1`, subjectID)
	return err
}

func InsertTag(ctx context.Context, db DBTX, subjectID int64, name string) (int64, error) {
	var id int64
	err := db.QueryRow(ctx, `INSERT INTO tags (subject_id, name) VALUES ($1, $2) RETURNING id`, subjectID, name).Scan(&id)
	return id, err
}

func ListTagsBySubject(ctx context.Context, db DBTX, subjectID int64) ([]string, error) {
	rows, err := db.Query(ctx, `SELECT name FROM tags WHERE subject_id = $1 ORDER BY name`, subjectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	tags := make([]string, 0)
	for rows.Next() {
		var tag string
		if err := rows.Scan(&tag); err != nil {
			return nil, err
		}
		tags = append(tags, tag)
	}
	return tags, rows.Err()
}
