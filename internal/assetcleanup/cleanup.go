package assetcleanup

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/benenen/lapin/internal/assetstore"
	"github.com/benenen/lapin/internal/database"
)

const unreferencedAssetGracePeriod = 24 * time.Hour

func Reconcile(ctx context.Context, pool *pgxpool.Pool, store *assetstore.Store) error {
	if store == nil {
		return nil
	}
	connection, err := pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("acquire asset cleanup connection: %w", err)
	}
	defer connection.Release()
	if err := database.LockGlobalAssetQuotaSession(ctx, connection); err != nil {
		return fmt.Errorf("lock asset cleanup: %w", err)
	}
	defer func() { _ = database.UnlockGlobalAssetQuotaSession(context.Background(), connection) }()

	tx, err := connection.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin asset cleanup: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := database.DeleteUnreferencedAssets(ctx, tx, time.Now().UTC().Add(-unreferencedAssetGracePeriod)); err != nil {
		return fmt.Errorf("delete unreferenced assets: %w", err)
	}
	storedAssets, err := database.ListAssets(ctx, tx)
	if err != nil {
		return fmt.Errorf("list referenced assets: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit asset cleanup: %w", err)
	}
	referenced := make([]assetstore.Blob, 0, len(storedAssets))
	for _, stored := range storedAssets {
		referenced = append(referenced, assetstore.Blob{SHA256: stored.SHA256, MIMEType: stored.MIMEType})
	}
	if err := store.Reconcile(referenced); err != nil {
		return fmt.Errorf("reconcile asset files: %w", err)
	}
	return nil
}

func RemoveDeleted(ctx context.Context, pool *pgxpool.Pool, store *assetstore.Store, deleted []database.Asset) error {
	if store == nil || len(deleted) == 0 {
		return nil
	}
	connection, err := pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("acquire asset deletion connection: %w", err)
	}
	defer connection.Release()
	if err := database.LockGlobalAssetQuotaSession(ctx, connection); err != nil {
		return fmt.Errorf("lock asset deletion: %w", err)
	}
	defer func() { _ = database.UnlockGlobalAssetQuotaSession(context.Background(), connection) }()
	for _, asset := range deleted {
		exists, err := database.AssetDigestExists(ctx, connection, asset.SHA256, asset.MIMEType)
		if err != nil {
			return fmt.Errorf("check shared asset digest: %w", err)
		}
		if exists {
			continue
		}
		if err := store.Remove(assetstore.Blob{SHA256: asset.SHA256, MIMEType: asset.MIMEType}); err != nil {
			return fmt.Errorf("remove deleted asset file: %w", err)
		}
	}
	return nil
}
