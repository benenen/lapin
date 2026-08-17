package database

import (
	"context"
	"encoding/json"
	"time"
)

type SubjectImport struct {
	ID               int64
	OwnerID          int64
	IdempotencyKey   string
	ExternalID       string
	Title            string
	Description      string
	Tags             []string
	ExpectedChapters int
	ExpectedAssets   int
	Status           string
	SubjectID        *int64
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

type SubjectImportChapter struct {
	ImportID         int64
	ExternalID       string
	ParentExternalID *string
	Position         int
	Title            string
	Content          string
	ContentSHA256    string
}

func CreateOrFindSubjectImport(ctx context.Context, db DBTX, value SubjectImport) (stored SubjectImport, created bool, err error) {
	tags, err := json.Marshal(value.Tags)
	if err != nil {
		return stored, false, err
	}
	var storedTags []byte
	err = db.QueryRow(ctx, `
		INSERT INTO subject_imports
			(owner_id, idempotency_key, external_id, title, description, tags, expected_chapters, expected_assets, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $9)
		ON CONFLICT (owner_id, idempotency_key) DO UPDATE SET idempotency_key = EXCLUDED.idempotency_key
		RETURNING id, owner_id, idempotency_key, external_id, title, description, tags,
		          expected_chapters, expected_assets, status, subject_id, created_at, updated_at, (xmax = 0)
	`, value.OwnerID, value.IdempotencyKey, value.ExternalID, value.Title, value.Description, tags,
		value.ExpectedChapters, value.ExpectedAssets, value.CreatedAt).Scan(
		&stored.ID, &stored.OwnerID, &stored.IdempotencyKey, &stored.ExternalID, &stored.Title,
		&stored.Description, &storedTags, &stored.ExpectedChapters, &stored.ExpectedAssets,
		&stored.Status, &stored.SubjectID, &stored.CreatedAt, &stored.UpdatedAt, &created,
	)
	if err == nil {
		err = json.Unmarshal(storedTags, &stored.Tags)
	}
	return stored, created, err
}

func FindSubjectImportForOwner(ctx context.Context, db DBTX, id, ownerID int64) (SubjectImport, error) {
	return findSubjectImport(ctx, db, id, ownerID, false)
}

func CountDraftSubjectImports(ctx context.Context, db DBTX, ownerID int64) (int, error) {
	var count int
	err := db.QueryRow(ctx, `SELECT COUNT(*) FROM subject_imports WHERE owner_id = $1 AND status = 'draft'`, ownerID).Scan(&count)
	return count, err
}

func ListExpiredDraftSubjectImports(ctx context.Context, db DBTX, ownerID int64, cutoff time.Time) ([]int64, error) {
	rows, err := db.Query(ctx, `
		SELECT id FROM subject_imports
		WHERE owner_id = $1 AND status = 'draft' AND updated_at < $2
		ORDER BY id FOR UPDATE
	`, ownerID, cutoff)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := make([]int64, 0)
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func ReopenSubjectImport(ctx context.Context, db DBTX, id, ownerID int64) error {
	_, err := db.Exec(ctx, `
		UPDATE subject_imports
		SET status = 'draft', subject_id = NULL, committed_at = NULL, updated_at = NOW()
		WHERE id = $1 AND owner_id = $2 AND status IN ('committed', 'aborted')
	`, id, ownerID)
	return err
}

func TouchSubjectImport(ctx context.Context, db DBTX, id int64) error {
	_, err := db.Exec(ctx, `UPDATE subject_imports SET updated_at = NOW() WHERE id = $1 AND status = 'draft'`, id)
	return err
}

func AbortAndClearSubjectImport(ctx context.Context, db DBTX, id, ownerID int64) ([]Asset, error) {
	rows, err := db.Query(ctx, `SELECT asset_id FROM subject_import_assets WHERE import_id = $1`, id)
	if err != nil {
		return nil, err
	}
	candidateAssetIDs := make([]int64, 0)
	for rows.Next() {
		var assetID int64
		if err := rows.Scan(&assetID); err != nil {
			rows.Close()
			return nil, err
		}
		candidateAssetIDs = append(candidateAssetIDs, assetID)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	if _, err := db.Exec(ctx, `DELETE FROM subject_import_batches WHERE import_id = $1`, id); err != nil {
		return nil, err
	}
	if _, err := db.Exec(ctx, `DELETE FROM subject_import_chapters WHERE import_id = $1`, id); err != nil {
		return nil, err
	}
	if _, err := db.Exec(ctx, `DELETE FROM subject_import_assets WHERE import_id = $1`, id); err != nil {
		return nil, err
	}
	deletedAssets := make([]Asset, 0)
	if len(candidateAssetIDs) != 0 {
		rows, err := db.Query(ctx, `
			DELETE FROM assets AS asset
			WHERE asset.id = ANY($1::bigint[])
			  AND NOT EXISTS (SELECT 1 FROM chapter_assets WHERE asset_id = asset.id)
			  AND NOT EXISTS (SELECT 1 FROM subject_import_assets WHERE asset_id = asset.id)
			  AND (asset.lease_until IS NULL OR asset.lease_until < NOW())
			RETURNING asset.id, asset.owner_id, asset.sha256, asset.mime_type,
			          asset.size_bytes, asset.width, asset.height, asset.created_at
		`, candidateAssetIDs)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var asset Asset
			if err := rows.Scan(&asset.ID, &asset.OwnerID, &asset.SHA256, &asset.MIMEType, &asset.Size, &asset.Width, &asset.Height, &asset.CreatedAt); err != nil {
				rows.Close()
				return nil, err
			}
			deletedAssets = append(deletedAssets, asset)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return nil, err
		}
		rows.Close()
	}
	_, err = db.Exec(ctx, `
		UPDATE subject_imports
		SET status = 'aborted', updated_at = NOW()
		WHERE id = $1 AND owner_id = $2
	`, id, ownerID)
	return deletedAssets, err
}

func LockSubjectImportForOwner(ctx context.Context, db DBTX, id, ownerID int64) (SubjectImport, error) {
	return findSubjectImport(ctx, db, id, ownerID, true)
}

func findSubjectImport(ctx context.Context, db DBTX, id, ownerID int64, lock bool) (SubjectImport, error) {
	query := `
		SELECT id, owner_id, idempotency_key, external_id, title, description, tags,
		       expected_chapters, expected_assets, status, subject_id, created_at, updated_at
		FROM subject_imports WHERE id = $1 AND owner_id = $2`
	if lock {
		query += ` FOR UPDATE`
	}
	var value SubjectImport
	var tags []byte
	err := db.QueryRow(ctx, query, id, ownerID).Scan(
		&value.ID, &value.OwnerID, &value.IdempotencyKey, &value.ExternalID, &value.Title,
		&value.Description, &tags, &value.ExpectedChapters, &value.ExpectedAssets,
		&value.Status, &value.SubjectID, &value.CreatedAt, &value.UpdatedAt,
	)
	if err == nil {
		err = json.Unmarshal(tags, &value.Tags)
	}
	return value, err
}

func UpsertSubjectImportBatch(ctx context.Context, db DBTX, importID int64, batchKey, digest string) (storedDigest string, created bool, err error) {
	err = db.QueryRow(ctx, `
		INSERT INTO subject_import_batches (import_id, batch_key, digest)
		VALUES ($1, $2, $3)
		ON CONFLICT (import_id, batch_key) DO UPDATE SET batch_key = EXCLUDED.batch_key
		RETURNING digest, (xmax = 0)
	`, importID, batchKey, digest).Scan(&storedDigest, &created)
	return storedDigest, created, err
}

func UpsertSubjectImportChapter(ctx context.Context, db DBTX, value SubjectImportChapter) error {
	_, err := db.Exec(ctx, `
		INSERT INTO subject_import_chapters
			(import_id, external_id, parent_external_id, position, title, content, content_sha256)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (import_id, external_id) DO UPDATE SET
			parent_external_id = EXCLUDED.parent_external_id,
			position = EXCLUDED.position,
			title = EXCLUDED.title,
			content = EXCLUDED.content,
			content_sha256 = EXCLUDED.content_sha256
	`, value.ImportID, value.ExternalID, value.ParentExternalID, value.Position, value.Title, value.Content, value.ContentSHA256)
	return err
}

func ListSubjectImportChapters(ctx context.Context, db DBTX, importID int64) ([]SubjectImportChapter, error) {
	rows, err := db.Query(ctx, `
		SELECT import_id, external_id, parent_external_id, position, title, content, content_sha256
		FROM subject_import_chapters WHERE import_id = $1 ORDER BY position
	`, importID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	values := make([]SubjectImportChapter, 0)
	for rows.Next() {
		var value SubjectImportChapter
		if err := rows.Scan(&value.ImportID, &value.ExternalID, &value.ParentExternalID, &value.Position, &value.Title, &value.Content, &value.ContentSHA256); err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	return values, rows.Err()
}

func AssociateSubjectImportAsset(ctx context.Context, db DBTX, importID int64, assetKey string, assetID int64) (storedAssetID int64, created bool, err error) {
	err = db.QueryRow(ctx, `
		INSERT INTO subject_import_assets (import_id, asset_key, asset_id)
		VALUES ($1, $2, $3)
		ON CONFLICT (import_id, asset_key) DO UPDATE SET asset_key = EXCLUDED.asset_key
		RETURNING asset_id, (xmax = 0)
	`, importID, assetKey, assetID).Scan(&storedAssetID, &created)
	return storedAssetID, created, err
}

func FindSubjectImportAsset(ctx context.Context, db DBTX, importID int64, assetKey string) (Asset, error) {
	var asset Asset
	err := db.QueryRow(ctx, `
		SELECT asset.id, asset.owner_id, asset.sha256, asset.mime_type, asset.size_bytes,
		       asset.width, asset.height, asset.created_at
		FROM subject_import_assets AS imported
		JOIN assets AS asset ON asset.id = imported.asset_id
		WHERE imported.import_id = $1 AND imported.asset_key = $2
	`, importID, assetKey).Scan(
		&asset.ID, &asset.OwnerID, &asset.SHA256, &asset.MIMEType, &asset.Size,
		&asset.Width, &asset.Height, &asset.CreatedAt,
	)
	return asset, err
}

func SubjectImportAssetUsage(ctx context.Context, db DBTX, importID int64) (count int, bytes int64, err error) {
	err = db.QueryRow(ctx, `
		SELECT COUNT(*), COALESCE(SUM(asset.size_bytes), 0)
		FROM subject_import_assets AS imported
		JOIN assets AS asset ON asset.id = imported.asset_id
		WHERE imported.import_id = $1
	`, importID).Scan(&count, &bytes)
	return count, bytes, err
}

func ListSubjectImportAssetIDs(ctx context.Context, db DBTX, importID int64) ([]int64, error) {
	rows, err := db.Query(ctx, `SELECT asset_id FROM subject_import_assets WHERE import_id = $1 ORDER BY asset_key`, importID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	values := make([]int64, 0)
	for rows.Next() {
		var value int64
		if err := rows.Scan(&value); err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	return values, rows.Err()
}

func CountSubjectImportParts(ctx context.Context, db DBTX, importID int64) (chapters, assets int, err error) {
	err = db.QueryRow(ctx, `
		SELECT
			(SELECT COUNT(*) FROM subject_import_chapters WHERE import_id = $1),
			(SELECT COUNT(*) FROM subject_import_assets WHERE import_id = $1)
	`, importID).Scan(&chapters, &assets)
	return chapters, assets, err
}

func MarkSubjectImportCommitted(ctx context.Context, db DBTX, importID, subjectID int64) error {
	_, err := db.Exec(ctx, `
		UPDATE subject_imports
		SET status = 'committed', subject_id = $2, committed_at = NOW(), updated_at = NOW()
		WHERE id = $1 AND status = 'draft'
	`, importID, subjectID)
	return err
}

func ReplaceChapterAssets(ctx context.Context, db DBTX, chapterID int64, assetIDs []int64) error {
	if _, err := db.Exec(ctx, `DELETE FROM chapter_assets WHERE chapter_id = $1`, chapterID); err != nil {
		return err
	}
	for _, assetID := range assetIDs {
		if _, err := db.Exec(ctx, `INSERT INTO chapter_assets (chapter_id, asset_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`, chapterID, assetID); err != nil {
			return err
		}
	}
	return nil
}
