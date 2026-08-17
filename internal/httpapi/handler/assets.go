package handler

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"time"

	"github.com/cloudwego/hertz/pkg/app"

	"github.com/benenen/lapin/internal/assetcleanup"
	"github.com/benenen/lapin/internal/assetstore"
	"github.com/benenen/lapin/internal/database"
)

type assetView struct {
	ID       string `json:"id"`
	URL      string `json:"url"`
	SHA256   string `json:"sha256"`
	MIMEType string `json:"mime_type"`
	Size     int64  `json:"size"`
	Width    int    `json:"width"`
	Height   int    `json:"height"`
}

const directAssetLeaseDuration = 24 * time.Hour

func (h *Handler) UploadAsset(ctx context.Context, c *app.RequestContext) {
	if !acceptUnencodedMultipart(c) {
		return
	}
	header, err := c.FormFile("file")
	if err != nil || header.Size < 1 || header.Size > assetstore.MaxAssetBytes {
		writeError(c, http.StatusBadRequest, errorCodeInvalidInput, "请选择有效的 PNG 或 JPEG 图片")
		return
	}
	tx, err := h.db.Begin(ctx)
	if err != nil {
		writeError(c, http.StatusInternalServerError, errorCodeInternal, "保存图片失败")
		return
	}
	defer func() { _ = tx.Rollback(ctx) }()
	ownerID := currentUser(c).ID
	if err := database.LockGlobalAssetQuota(ctx, tx); err != nil {
		writeError(c, http.StatusInternalServerError, errorCodeInternal, "保存图片失败")
		return
	}
	if err := database.LockUserForUpdate(ctx, tx, ownerID); err != nil {
		writeError(c, http.StatusInternalServerError, errorCodeInternal, "保存图片失败")
		return
	}
	count, usedBytes, err := database.AssetUsageForOwner(ctx, tx, ownerID)
	if err != nil {
		writeError(c, http.StatusInternalServerError, errorCodeInternal, "保存图片失败")
		return
	}
	if count >= assetstore.MaxOwnerAssets || usedBytes+header.Size > assetstore.MaxOwnerAssetBytes {
		writeError(c, http.StatusRequestEntityTooLarge, errorCodeInvalidInput, "图片存储配额已用完")
		return
	}
	totalCount, totalBytes, err := database.AssetUsageTotal(ctx, tx)
	if err != nil {
		writeError(c, http.StatusInternalServerError, errorCodeInternal, "保存图片失败")
		return
	}
	if totalCount >= assetstore.MaxTotalAssets || totalBytes+header.Size > assetstore.MaxTotalAssetBytes {
		writeError(c, http.StatusInsufficientStorage, errorCodeInternal, "服务器图片存储空间不足")
		return
	}
	leaseUntil := time.Now().UTC().Add(directAssetLeaseDuration)
	stored, created, _, ok := h.saveUploadedAssetHeader(ctx, c, tx, header, &leaseUntil)
	if !ok {
		return
	}
	if err := tx.Commit(ctx); err != nil {
		_ = tx.Rollback(ctx)
		_ = assetcleanup.Reconcile(ctx, h.db, h.assetStore)
		writeError(c, http.StatusInternalServerError, errorCodeInternal, "保存图片失败")
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	writeData(c, status, h.assetView(stored))
}

func (h *Handler) saveUploadedAssetHeader(ctx context.Context, c *app.RequestContext, db database.DBTX, header *multipart.FileHeader, leaseUntil *time.Time) (database.Asset, bool, *assetstore.StagedBlob, bool) {
	staged, ok := h.stageUploadedAssetHeader(c, header)
	if !ok {
		return database.Asset{}, false, nil, false
	}
	defer staged.Abort()
	stored, created, ok := h.upsertStagedAsset(ctx, c, db, staged)
	if !ok {
		return database.Asset{}, false, nil, false
	}
	if leaseUntil != nil {
		if err := database.ExtendAssetLease(ctx, db, stored.ID, stored.OwnerID, *leaseUntil); err != nil {
			writeError(c, http.StatusInternalServerError, errorCodeInternal, "保存图片租约失败")
			return database.Asset{}, false, nil, false
		}
	}
	if err := staged.Publish(); err != nil {
		writeError(c, http.StatusInternalServerError, errorCodeInternal, "发布图片失败")
		return database.Asset{}, false, nil, false
	}
	return stored, created, staged, true
}

func (h *Handler) stageUploadedAssetHeader(c *app.RequestContext, header *multipart.FileHeader) (*assetstore.StagedBlob, bool) {
	if h.assetStore == nil {
		writeError(c, http.StatusInternalServerError, errorCodeInternal, "资源存储不可用")
		return nil, false
	}
	if header == nil || header.Size < 1 || header.Size > assetstore.MaxAssetBytes {
		writeError(c, http.StatusBadRequest, errorCodeInvalidInput, "请选择有效的 PNG 或 JPEG 图片")
		return nil, false
	}
	file, err := header.Open()
	if err != nil {
		writeError(c, http.StatusBadRequest, errorCodeInvalidInput, "读取上传图片失败")
		return nil, false
	}
	defer file.Close()
	staged, err := h.assetStore.Stage(file)
	if err != nil {
		writeError(c, http.StatusBadRequest, errorCodeInvalidInput, "图片格式、大小或尺寸不符合要求")
		return nil, false
	}
	return staged, true
}

func (h *Handler) upsertStagedAsset(ctx context.Context, c *app.RequestContext, db database.DBTX, staged *assetstore.StagedBlob) (database.Asset, bool, bool) {
	blob := staged.Blob
	stored, created, err := database.UpsertAsset(ctx, db, database.Asset{
		OwnerID: currentUser(c).ID, SHA256: blob.SHA256, MIMEType: blob.MIMEType,
		Size: blob.Size, Width: blob.Width, Height: blob.Height, CreatedAt: time.Now().UTC(),
	})
	if err != nil {
		writeError(c, http.StatusInternalServerError, errorCodeInternal, "保存图片失败")
		return database.Asset{}, false, false
	}
	return stored, created, true
}

func acceptUnencodedMultipart(c *app.RequestContext) bool {
	if len(c.Request.Header.Peek("Content-Encoding")) == 0 {
		return true
	}
	writeError(c, http.StatusUnsupportedMediaType, errorCodeUnsupportedMedia, "图片上传不支持 Content-Encoding")
	return false
}

func uploadedAssetSHA256(header *multipart.FileHeader) (string, error) {
	file, err := header.Open()
	if err != nil {
		return "", err
	}
	defer file.Close()
	digest := sha256.New()
	written, err := io.Copy(digest, io.LimitReader(file, assetstore.MaxAssetBytes+1))
	if err != nil || written < 1 || written > assetstore.MaxAssetBytes {
		return "", fmt.Errorf("invalid uploaded asset")
	}
	return hex.EncodeToString(digest.Sum(nil)), nil
}

func (h *Handler) GetAssetContent(ctx context.Context, c *app.RequestContext) {
	assetID, err := h.ids.Decode(c.Param("id"))
	if err != nil {
		writeError(c, http.StatusNotFound, errorCodeNotFound, "图片不存在")
		return
	}
	asset, err := database.FindAssetByID(ctx, h.db, assetID)
	if err != nil {
		writeError(c, http.StatusNotFound, errorCodeNotFound, "图片不存在")
		return
	}
	if asset.OwnerID != currentUser(c).ID {
		published, publishErr := database.AssetIsPublished(ctx, h.db, asset.ID)
		if publishErr != nil || !published {
			writeError(c, http.StatusNotFound, errorCodeNotFound, "图片不存在")
			return
		}
	}
	if h.assetStore == nil {
		writeError(c, http.StatusInternalServerError, errorCodeInternal, "资源存储不可用")
		return
	}
	file, err := h.assetStore.Open(assetstore.Blob{SHA256: asset.SHA256, MIMEType: asset.MIMEType})
	if err != nil {
		writeError(c, http.StatusNotFound, errorCodeNotFound, "图片不存在")
		return
	}
	defer file.Close()
	body, err := io.ReadAll(io.LimitReader(file, assetstore.MaxAssetBytes+1))
	if err != nil || int64(len(body)) != asset.Size || len(body) > assetstore.MaxAssetBytes {
		writeError(c, http.StatusInternalServerError, errorCodeInternal, "读取图片失败")
		return
	}
	c.Response.Header.Set("Cache-Control", "private, max-age=31536000, immutable")
	c.Response.Header.Set("Content-Disposition", "inline")
	c.Response.Header.Set("ETag", fmt.Sprintf("%q", asset.SHA256))
	c.Response.Header.Set("X-Content-Type-Options", "nosniff")
	c.Data(http.StatusOK, asset.MIMEType, body)
}

func (h *Handler) assetView(asset database.Asset) assetView {
	id := h.ids.Encode(asset.ID)
	return assetView{
		ID: id, URL: "/api/v1/assets/" + id + "/content", SHA256: asset.SHA256,
		MIMEType: asset.MIMEType, Size: asset.Size, Width: asset.Width, Height: asset.Height,
	}
}
