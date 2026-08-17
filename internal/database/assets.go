package database

import (
	"context"
	"time"
)

type Asset struct {
	ID        int64
	OwnerID   int64
	SHA256    string
	MIMEType  string
	Size      int64
	Width     int
	Height    int
	CreatedAt time.Time
}

func UpsertAsset(ctx context.Context, db DBTX, asset Asset) (stored Asset, created bool, err error) {
	err = db.QueryRow(ctx, `
		INSERT INTO assets (owner_id, sha256, mime_type, size_bytes, width, height, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (owner_id, sha256) DO UPDATE SET sha256 = EXCLUDED.sha256
		RETURNING id, owner_id, sha256, mime_type, size_bytes, width, height, created_at, (xmax = 0)
	`, asset.OwnerID, asset.SHA256, asset.MIMEType, asset.Size, asset.Width, asset.Height, asset.CreatedAt).Scan(
		&stored.ID, &stored.OwnerID, &stored.SHA256, &stored.MIMEType, &stored.Size,
		&stored.Width, &stored.Height, &stored.CreatedAt, &created,
	)
	return stored, created, err
}

func FindAssetByID(ctx context.Context, db DBTX, id int64) (Asset, error) {
	var asset Asset
	err := db.QueryRow(ctx, `
		SELECT id, owner_id, sha256, mime_type, size_bytes, width, height, created_at
		FROM assets WHERE id = $1
	`, id).Scan(&asset.ID, &asset.OwnerID, &asset.SHA256, &asset.MIMEType, &asset.Size, &asset.Width, &asset.Height, &asset.CreatedAt)
	return asset, err
}

func FindAssetForOwnerByID(ctx context.Context, db DBTX, id, ownerID int64) (Asset, error) {
	var asset Asset
	err := db.QueryRow(ctx, `
		SELECT id, owner_id, sha256, mime_type, size_bytes, width, height, created_at
		FROM assets WHERE id = $1 AND owner_id = $2
	`, id, ownerID).Scan(&asset.ID, &asset.OwnerID, &asset.SHA256, &asset.MIMEType, &asset.Size, &asset.Width, &asset.Height, &asset.CreatedAt)
	return asset, err
}

func AssetUsageForOwner(ctx context.Context, db DBTX, ownerID int64) (count int, bytes int64, err error) {
	err = db.QueryRow(ctx, `
		SELECT COUNT(*), COALESCE(SUM(size_bytes), 0)
		FROM assets WHERE owner_id = $1
	`, ownerID).Scan(&count, &bytes)
	return count, bytes, err
}

func ExtendAssetLease(ctx context.Context, db DBTX, id, ownerID int64, until time.Time) error {
	_, err := db.Exec(ctx, `
		UPDATE assets
		SET lease_until = GREATEST(COALESCE(lease_until, '-infinity'::timestamptz), $3)
		WHERE id = $1 AND owner_id = $2
	`, id, ownerID, until)
	return err
}

func LockGlobalAssetQuota(ctx context.Context, db DBTX) error {
	_, err := db.Exec(ctx, `SELECT pg_advisory_xact_lock(1279214122)`)
	return err
}

func LockGlobalAssetQuotaSession(ctx context.Context, db DBTX) error {
	_, err := db.Exec(ctx, `SELECT pg_advisory_lock(1279214122)`)
	return err
}

func UnlockGlobalAssetQuotaSession(ctx context.Context, db DBTX) error {
	_, err := db.Exec(ctx, `SELECT pg_advisory_unlock(1279214122)`)
	return err
}

func AssetUsageTotal(ctx context.Context, db DBTX) (count int, bytes int64, err error) {
	err = db.QueryRow(ctx, `SELECT COUNT(*), COALESCE(SUM(size_bytes), 0) FROM assets`).Scan(&count, &bytes)
	return count, bytes, err
}

func AssetDigestExists(ctx context.Context, db DBTX, sha256, mimeType string) (bool, error) {
	var exists bool
	err := db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM assets WHERE sha256 = $1 AND mime_type = $2)`, sha256, mimeType).Scan(&exists)
	return exists, err
}

func ListAssets(ctx context.Context, db DBTX) ([]Asset, error) {
	rows, err := db.Query(ctx, `
		SELECT id, owner_id, sha256, mime_type, size_bytes, width, height, created_at
		FROM assets ORDER BY id
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	assets := make([]Asset, 0)
	for rows.Next() {
		var asset Asset
		if err := rows.Scan(&asset.ID, &asset.OwnerID, &asset.SHA256, &asset.MIMEType, &asset.Size, &asset.Width, &asset.Height, &asset.CreatedAt); err != nil {
			return nil, err
		}
		assets = append(assets, asset)
	}
	return assets, rows.Err()
}

func DeleteUnreferencedAssets(ctx context.Context, db DBTX, olderThan time.Time) error {
	_, err := db.Exec(ctx, `
		DELETE FROM assets AS asset
		WHERE NOT EXISTS (SELECT 1 FROM chapter_assets WHERE asset_id = asset.id)
		  AND NOT EXISTS (SELECT 1 FROM subject_import_assets WHERE asset_id = asset.id)
		  AND asset.created_at < $1
		  AND (asset.lease_until IS NULL OR asset.lease_until < NOW())
	`, olderThan)
	return err
}

func AssetIsPublished(ctx context.Context, db DBTX, id int64) (bool, error) {
	var published bool
	err := db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM chapter_assets WHERE asset_id = $1)`, id).Scan(&published)
	return published, err
}
