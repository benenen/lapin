package handler

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/jackc/pgx/v5"

	"github.com/benenen/lapin/internal/assetcleanup"
	"github.com/benenen/lapin/internal/assetstore"
	"github.com/benenen/lapin/internal/database"
)

func (h *Handler) UploadSubjectImportAsset(ctx context.Context, c *app.RequestContext) {
	if !acceptUnencodedMultipart(c) {
		return
	}
	importID, ok := h.decodeSubjectImportID(c)
	if !ok {
		return
	}
	form, err := c.MultipartForm()
	if err != nil {
		writeError(c, http.StatusBadRequest, errorCodeInvalidInput, "图片批次无效")
		return
	}
	keys := form.Value["key"]
	files := form.File["file"]
	if len(keys) < 1 || len(keys) > maxImportAssetsPerBatch || len(keys) != len(files) {
		writeError(c, http.StatusBadRequest, errorCodeInvalidInput, "图片批次无效")
		return
	}
	seenKeys := make(map[string]struct{}, len(keys))
	for index := range keys {
		keys[index] = strings.TrimSpace(keys[index])
		if keys[index] == "" || utf8.RuneCountInString(keys[index]) > 160 || files[index] == nil || files[index].Size < 1 || files[index].Size > assetstore.MaxAssetBytes {
			writeError(c, http.StatusBadRequest, errorCodeInvalidInput, "图片批次无效")
			return
		}
		if _, duplicate := seenKeys[keys[index]]; duplicate {
			writeError(c, http.StatusBadRequest, errorCodeInvalidInput, "图片批次包含重复 key")
			return
		}
		seenKeys[keys[index]] = struct{}{}
	}
	tx, err := h.db.Begin(ctx)
	if err != nil {
		writeError(c, http.StatusInternalServerError, errorCodeInternal, "保存导入图片失败")
		return
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := database.LockGlobalAssetQuota(ctx, tx); err != nil {
		writeError(c, http.StatusInternalServerError, errorCodeInternal, "保存导入图片失败")
		return
	}
	ownerID := currentUser(c).ID
	if err := database.LockUserForUpdate(ctx, tx, ownerID); err != nil {
		writeError(c, http.StatusInternalServerError, errorCodeInternal, "保存导入图片失败")
		return
	}
	value, err := database.LockSubjectImportForOwner(ctx, tx, importID, ownerID)
	if err != nil {
		writeError(c, http.StatusNotFound, errorCodeNotFound, "导入任务不存在")
		return
	}
	if subjectImportExpired(value) {
		deleted, err := database.AbortAndClearSubjectImport(ctx, tx, value.ID, ownerID)
		if err != nil || tx.Commit(ctx) != nil {
			writeError(c, http.StatusInternalServerError, errorCodeInternal, "清理过期导入任务失败")
			return
		}
		if !h.removeDeletedAssets(ctx, c, deleted) {
			return
		}
		writeError(c, http.StatusConflict, errorCodeImportConflict, "导入任务已过期，请重新开始")
		return
	}
	if value.Status != "draft" {
		writeError(c, http.StatusConflict, errorCodeImportConflict, "导入任务已结束")
		return
	}
	viewsByKey := make(map[string]stagedAssetView, len(keys))
	newIndexes := make([]int, 0, len(keys))
	var newBytes int64
	for index, key := range keys {
		existing, err := database.FindSubjectImportAsset(ctx, tx, value.ID, key)
		if err == nil {
			digest, hashErr := uploadedAssetSHA256(files[index])
			if hashErr != nil {
				writeError(c, http.StatusBadRequest, errorCodeInvalidInput, "读取上传图片失败")
				return
			}
			if digest != existing.SHA256 {
				writeError(c, http.StatusConflict, errorCodeImportConflict, "相同资源 key 已用于其他图片")
				return
			}
			assetView := h.assetView(existing)
			viewsByKey[key] = stagedAssetView{
				Key: key, ID: assetView.ID, URL: assetView.URL, SHA256: assetView.SHA256,
				MIMEType: assetView.MIMEType, Size: assetView.Size, Width: assetView.Width, Height: assetView.Height,
			}
			continue
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			writeError(c, http.StatusInternalServerError, errorCodeInternal, "读取导入图片失败")
			return
		}
		newIndexes = append(newIndexes, index)
		newBytes += files[index].Size
	}
	receivedAssets, importBytes, err := database.SubjectImportAssetUsage(ctx, tx, value.ID)
	if err != nil {
		writeError(c, http.StatusInternalServerError, errorCodeInternal, "读取导入任务失败")
		return
	}
	ownerAssets, ownerBytes, err := database.AssetUsageForOwner(ctx, tx, value.OwnerID)
	if err != nil {
		writeError(c, http.StatusInternalServerError, errorCodeInternal, "读取图片配额失败")
		return
	}
	totalAssets, totalBytes, err := database.AssetUsageTotal(ctx, tx)
	if err != nil {
		writeError(c, http.StatusInternalServerError, errorCodeInternal, "读取图片配额失败")
		return
	}
	if receivedAssets+len(newIndexes) > value.ExpectedAssets {
		writeError(c, http.StatusConflict, errorCodeImportConflict, "导入图片数量超过任务声明")
		return
	}
	if importBytes+newBytes > assetstore.MaxImportAssetBytes || ownerAssets+len(newIndexes) > assetstore.MaxOwnerAssets || ownerBytes+newBytes > assetstore.MaxOwnerAssetBytes {
		writeError(c, http.StatusRequestEntityTooLarge, errorCodeInvalidInput, "图片存储配额已用完")
		return
	}
	if totalAssets+len(newIndexes) > assetstore.MaxTotalAssets || totalBytes+newBytes > assetstore.MaxTotalAssetBytes {
		writeError(c, http.StatusInsufficientStorage, errorCodeInternal, "服务器图片存储空间不足")
		return
	}
	stagedByIndex := make(map[int]*assetstore.StagedBlob, len(newIndexes))
	for _, index := range newIndexes {
		staged, ok := h.stageUploadedAssetHeader(c, files[index])
		if !ok {
			for _, pending := range stagedByIndex {
				pending.Abort()
			}
			return
		}
		stagedByIndex[index] = staged
	}
	defer func() {
		for _, pending := range stagedByIndex {
			pending.Abort()
		}
	}()
	createdAny := false
	for _, index := range newIndexes {
		key := keys[index]
		asset, _, stored := h.upsertStagedAsset(ctx, c, tx, stagedByIndex[index])
		if !stored {
			return
		}
		storedAssetID, created, err := database.AssociateSubjectImportAsset(ctx, tx, value.ID, key, asset.ID)
		if err != nil {
			writeError(c, http.StatusInternalServerError, errorCodeInternal, "关联导入图片失败")
			return
		}
		if storedAssetID != asset.ID {
			writeError(c, http.StatusConflict, errorCodeImportConflict, "相同资源 key 已用于其他图片")
			return
		}
		createdAny = createdAny || created
		assetView := h.assetView(asset)
		viewsByKey[key] = stagedAssetView{
			Key: key, ID: assetView.ID, URL: assetView.URL, SHA256: assetView.SHA256,
			MIMEType: assetView.MIMEType, Size: assetView.Size, Width: assetView.Width, Height: assetView.Height,
		}
	}
	if err := database.TouchSubjectImport(ctx, tx, value.ID); err != nil {
		writeError(c, http.StatusInternalServerError, errorCodeInternal, "更新导入任务失败")
		return
	}
	for _, index := range newIndexes {
		if err := stagedByIndex[index].Publish(); err != nil {
			for _, published := range stagedByIndex {
				published.RollbackPublished()
			}
			writeError(c, http.StatusInternalServerError, errorCodeInternal, "发布导入图片失败")
			return
		}
	}
	if err := tx.Commit(ctx); err != nil {
		_ = tx.Rollback(ctx)
		_ = assetcleanup.Reconcile(ctx, h.db, h.assetStore)
		writeError(c, http.StatusInternalServerError, errorCodeInternal, "保存导入图片失败")
		return
	}
	status := http.StatusOK
	if createdAny {
		status = http.StatusCreated
	}
	views := make([]stagedAssetView, 0, len(keys))
	for _, key := range keys {
		views = append(views, viewsByKey[key])
	}
	writeData(c, status, map[string]any{"assets": views})
}
