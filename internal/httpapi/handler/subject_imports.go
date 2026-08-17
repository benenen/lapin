package handler

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"slices"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/benenen/lapin/internal/assetcleanup"
	"github.com/benenen/lapin/internal/database"
)

const maxImportAssets = 300
const maxImportAssetsPerBatch = 16
const maxActiveSubjectImports = 3
const subjectImportIdleTimeout = 24 * time.Hour

var assetURLPattern = regexp.MustCompile(`^/api/v1/assets/([A-Za-z0-9]{10,64})/content$`)

type subjectImportView struct {
	ID               string `json:"id"`
	Status           string `json:"status"`
	ExternalID       string `json:"external_id"`
	ExpectedChapters int    `json:"expected_chapters"`
	ExpectedAssets   int    `json:"expected_assets"`
	ReceivedChapters int    `json:"received_chapters"`
	ReceivedAssets   int    `json:"received_assets"`
	SubjectID        string `json:"subject_id,omitempty"`
}

type stagedChapterInput struct {
	ExternalID       string  `json:"external_id"`
	ParentExternalID *string `json:"parent_external_id"`
	Position         int     `json:"position"`
	Title            string  `json:"title"`
	Content          string  `json:"content"`
}

type stagedAssetView struct {
	Key      string `json:"key"`
	ID       string `json:"id"`
	URL      string `json:"url"`
	SHA256   string `json:"sha256"`
	MIMEType string `json:"mime_type"`
	Size     int64  `json:"size"`
	Width    int    `json:"width"`
	Height   int    `json:"height"`
}

func (h *Handler) BeginSubjectImport(ctx context.Context, c *app.RequestContext) {
	var request struct {
		IdempotencyKey   string   `json:"idempotency_key"`
		ExternalID       string   `json:"external_id"`
		Title            string   `json:"title"`
		Description      string   `json:"description"`
		Tags             []string `json:"tags"`
		ExpectedChapters int      `json:"expected_chapters"`
		ExpectedAssets   int      `json:"expected_assets"`
	}
	if !decodeJSON(c, &request) {
		return
	}
	request.IdempotencyKey = strings.TrimSpace(request.IdempotencyKey)
	request.ExternalID = strings.TrimSpace(request.ExternalID)
	request.Title = strings.TrimSpace(request.Title)
	request.Description = strings.TrimSpace(request.Description)
	tags, tagsOK := normalizeTags(request.Tags)
	if request.IdempotencyKey == "" || utf8.RuneCountInString(request.IdempotencyKey) > 160 || request.ExternalID == "" || utf8.RuneCountInString(request.ExternalID) > 160 || request.Title == "" || utf8.RuneCountInString(request.Title) > 200 || utf8.RuneCountInString(request.Description) > 4000 || !tagsOK || request.ExpectedChapters < 1 || request.ExpectedChapters > 100 || request.ExpectedAssets < 0 || request.ExpectedAssets > maxImportAssets {
		writeError(c, http.StatusBadRequest, errorCodeInvalidInput, "请检查导入任务信息")
		return
	}
	ownerID := currentUser(c).ID
	tx, err := h.db.Begin(ctx)
	if err != nil {
		writeError(c, http.StatusInternalServerError, errorCodeInternal, "创建导入任务失败")
		return
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := database.LockGlobalAssetQuota(ctx, tx); err != nil {
		writeError(c, http.StatusInternalServerError, errorCodeInternal, "创建导入任务失败")
		return
	}
	if err := database.LockUserForUpdate(ctx, tx, ownerID); err != nil {
		writeError(c, http.StatusInternalServerError, errorCodeInternal, "创建导入任务失败")
		return
	}
	expiredIDs, err := database.ListExpiredDraftSubjectImports(ctx, tx, ownerID, time.Now().UTC().Add(-subjectImportIdleTimeout))
	if err != nil {
		writeError(c, http.StatusInternalServerError, errorCodeInternal, "清理过期导入任务失败")
		return
	}
	deletedAssets := make([]database.Asset, 0)
	for _, expiredID := range expiredIDs {
		deleted, err := database.AbortAndClearSubjectImport(ctx, tx, expiredID, ownerID)
		if err != nil {
			writeError(c, http.StatusInternalServerError, errorCodeInternal, "清理过期导入任务失败")
			return
		}
		deletedAssets = append(deletedAssets, deleted...)
	}
	value, created, err := database.CreateOrFindSubjectImport(ctx, tx, database.SubjectImport{
		OwnerID: ownerID, IdempotencyKey: request.IdempotencyKey, ExternalID: request.ExternalID,
		Title: request.Title, Description: request.Description, Tags: tags,
		ExpectedChapters: request.ExpectedChapters, ExpectedAssets: request.ExpectedAssets, CreatedAt: time.Now().UTC(),
	})
	if err != nil {
		var postgresError *pgconn.PgError
		if errors.As(err, &postgresError) && postgresError.Code == "23505" {
			writeError(c, http.StatusConflict, errorCodeImportConflict, "该课程已有进行中的导入任务")
			return
		}
		writeError(c, http.StatusInternalServerError, errorCodeInternal, "创建导入任务失败")
		return
	}
	if value.ExternalID != request.ExternalID || value.Title != request.Title || value.Description != request.Description || !slices.Equal(value.Tags, tags) || value.ExpectedChapters != request.ExpectedChapters || value.ExpectedAssets != request.ExpectedAssets {
		writeError(c, http.StatusConflict, errorCodeImportConflict, "相同幂等键已用于其他导入内容")
		return
	}
	if created || value.Status == "committed" || value.Status == "aborted" {
		active, countErr := database.CountDraftSubjectImports(ctx, tx, ownerID)
		if countErr != nil {
			writeError(c, http.StatusInternalServerError, errorCodeInternal, "创建导入任务失败")
			return
		}
		if (created && active > maxActiveSubjectImports) || (!created && active >= maxActiveSubjectImports) {
			writeError(c, http.StatusConflict, errorCodeImportConflict, "进行中的导入任务过多")
			return
		}
	}
	if !created && (value.Status == "committed" || value.Status == "aborted") {
		if err := database.ReopenSubjectImport(ctx, tx, value.ID, ownerID); err != nil {
			var postgresError *pgconn.PgError
			if errors.As(err, &postgresError) && postgresError.Code == "23505" {
				writeError(c, http.StatusConflict, errorCodeImportConflict, "该课程已有进行中的导入任务")
				return
			}
			writeError(c, http.StatusInternalServerError, errorCodeInternal, "恢复导入任务失败")
			return
		}
		value.Status = "draft"
		value.SubjectID = nil
	} else if !created && value.Status == "draft" {
		if err := database.TouchSubjectImport(ctx, tx, value.ID); err != nil {
			writeError(c, http.StatusInternalServerError, errorCodeInternal, "恢复导入任务失败")
			return
		}
	}
	if err := tx.Commit(ctx); err != nil {
		writeError(c, http.StatusInternalServerError, errorCodeInternal, "创建导入任务失败")
		return
	}
	if !h.removeDeletedAssets(ctx, c, deletedAssets) {
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	view, err := h.subjectImportView(ctx, value)
	if err != nil {
		writeError(c, http.StatusInternalServerError, errorCodeInternal, "读取导入任务失败")
		return
	}
	writeData(c, status, view)
}

func (h *Handler) GetSubjectImport(ctx context.Context, c *app.RequestContext) {
	value, ok := h.findSubjectImport(ctx, c)
	if !ok {
		return
	}
	deletedAssets := make([]database.Asset, 0)
	if subjectImportExpired(value) {
		ownerID := currentUser(c).ID
		tx, err := h.db.Begin(ctx)
		if err != nil {
			writeError(c, http.StatusInternalServerError, errorCodeInternal, "清理过期导入任务失败")
			return
		}
		defer func() { _ = tx.Rollback(ctx) }()
		locked, err := database.LockSubjectImportForOwner(ctx, tx, value.ID, ownerID)
		if err != nil {
			writeError(c, http.StatusNotFound, errorCodeNotFound, "导入任务不存在")
			return
		}
		if subjectImportExpired(locked) {
			deleted, err := database.AbortAndClearSubjectImport(ctx, tx, value.ID, ownerID)
			if err != nil {
				writeError(c, http.StatusInternalServerError, errorCodeInternal, "清理过期导入任务失败")
				return
			}
			deletedAssets = append(deletedAssets, deleted...)
			value.Status = "aborted"
			value.SubjectID = nil
		}
		if err := tx.Commit(ctx); err != nil {
			writeError(c, http.StatusInternalServerError, errorCodeInternal, "清理过期导入任务失败")
			return
		}
		if !h.removeDeletedAssets(ctx, c, deletedAssets) {
			return
		}
	}
	view, err := h.subjectImportView(ctx, value)
	if err != nil {
		writeError(c, http.StatusInternalServerError, errorCodeInternal, "读取导入任务失败")
		return
	}
	writeData(c, http.StatusOK, view)
}

func (h *Handler) UploadSubjectImportChapters(ctx context.Context, c *app.RequestContext) {
	importID, ok := h.decodeSubjectImportID(c)
	if !ok {
		return
	}
	var request struct {
		BatchKey string               `json:"batch_key"`
		Chapters []stagedChapterInput `json:"chapters"`
	}
	if !decodeJSON(c, &request) {
		return
	}
	request.BatchKey = strings.TrimSpace(request.BatchKey)
	if request.BatchKey == "" || utf8.RuneCountInString(request.BatchKey) > 160 || len(request.Chapters) < 1 || len(request.Chapters) > 100 || !validStagedChapterBatch(request.Chapters) {
		writeError(c, http.StatusBadRequest, errorCodeInvalidInput, "章节批次无效")
		return
	}
	encoded, _ := json.Marshal(request.Chapters)
	digestBytes := sha256.Sum256(encoded)
	digest := hex.EncodeToString(digestBytes[:])
	tx, err := h.db.Begin(ctx)
	if err != nil {
		writeError(c, http.StatusInternalServerError, errorCodeInternal, "保存章节批次失败")
		return
	}
	defer func() { _ = tx.Rollback(ctx) }()
	value, err := database.LockSubjectImportForOwner(ctx, tx, importID, currentUser(c).ID)
	if err != nil {
		writeError(c, http.StatusNotFound, errorCodeNotFound, "导入任务不存在")
		return
	}
	if subjectImportExpired(value) {
		deleted, err := database.AbortAndClearSubjectImport(ctx, tx, value.ID, value.OwnerID)
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
	storedDigest, created, err := database.UpsertSubjectImportBatch(ctx, tx, importID, request.BatchKey, digest)
	if err != nil {
		writeError(c, http.StatusInternalServerError, errorCodeInternal, "保存章节批次失败")
		return
	}
	if storedDigest != digest {
		writeError(c, http.StatusConflict, errorCodeImportConflict, "相同批次 key 的内容不一致")
		return
	}
	if created {
		for _, chapter := range request.Chapters {
			contentHash := sha256.Sum256([]byte(chapter.Content))
			if err := database.UpsertSubjectImportChapter(ctx, tx, database.SubjectImportChapter{
				ImportID: importID, ExternalID: strings.TrimSpace(chapter.ExternalID), ParentExternalID: normalizedOptionalString(chapter.ParentExternalID),
				Position: chapter.Position, Title: strings.TrimSpace(chapter.Title), Content: chapter.Content, ContentSHA256: hex.EncodeToString(contentHash[:]),
			}); err != nil {
				writeError(c, http.StatusConflict, errorCodeImportConflict, "章节位置或 external_id 冲突")
				return
			}
		}
	}
	if err := database.TouchSubjectImport(ctx, tx, importID); err != nil {
		writeError(c, http.StatusInternalServerError, errorCodeInternal, "更新导入任务失败")
		return
	}
	if err := tx.Commit(ctx); err != nil {
		writeError(c, http.StatusInternalServerError, errorCodeInternal, "保存章节批次失败")
		return
	}
	value, err = database.FindSubjectImportForOwner(ctx, h.db, importID, currentUser(c).ID)
	if err != nil {
		writeError(c, http.StatusInternalServerError, errorCodeInternal, "读取导入任务失败")
		return
	}
	view, err := h.subjectImportView(ctx, value)
	if err != nil {
		writeError(c, http.StatusInternalServerError, errorCodeInternal, "读取导入任务失败")
		return
	}
	writeData(c, http.StatusOK, view)
}

func (h *Handler) AbortSubjectImport(ctx context.Context, c *app.RequestContext) {
	importID, ok := h.decodeSubjectImportID(c)
	if !ok {
		return
	}
	ownerID := currentUser(c).ID
	tx, err := h.db.Begin(ctx)
	if err != nil {
		writeError(c, http.StatusInternalServerError, errorCodeInternal, "终止导入任务失败")
		return
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := database.LockUserForUpdate(ctx, tx, ownerID); err != nil {
		writeError(c, http.StatusInternalServerError, errorCodeInternal, "终止导入任务失败")
		return
	}
	value, err := database.LockSubjectImportForOwner(ctx, tx, importID, ownerID)
	if err != nil {
		writeError(c, http.StatusNotFound, errorCodeNotFound, "导入任务不存在")
		return
	}
	if value.Status != "draft" {
		writeError(c, http.StatusConflict, errorCodeImportConflict, "只能终止进行中的导入任务")
		return
	}
	deleted, err := database.AbortAndClearSubjectImport(ctx, tx, importID, ownerID)
	if err != nil {
		writeError(c, http.StatusInternalServerError, errorCodeInternal, "终止导入任务失败")
		return
	}
	if err := tx.Commit(ctx); err != nil {
		writeError(c, http.StatusInternalServerError, errorCodeInternal, "终止导入任务失败")
		return
	}
	if !h.removeDeletedAssets(ctx, c, deleted) {
		return
	}
	value.Status = "aborted"
	view, err := h.subjectImportView(ctx, value)
	if err != nil {
		writeError(c, http.StatusInternalServerError, errorCodeInternal, "读取导入任务失败")
		return
	}
	writeData(c, http.StatusOK, view)
}

func (h *Handler) CommitSubjectImport(ctx context.Context, c *app.RequestContext) {
	importID, ok := h.decodeSubjectImportID(c)
	if !ok {
		return
	}
	tx, err := h.db.Begin(ctx)
	if err != nil {
		writeError(c, http.StatusInternalServerError, errorCodeInternal, "提交导入任务失败")
		return
	}
	defer func() { _ = tx.Rollback(ctx) }()
	value, err := database.LockSubjectImportForOwner(ctx, tx, importID, currentUser(c).ID)
	if err != nil {
		writeError(c, http.StatusNotFound, errorCodeNotFound, "导入任务不存在")
		return
	}
	if subjectImportExpired(value) {
		deleted, err := database.AbortAndClearSubjectImport(ctx, tx, value.ID, value.OwnerID)
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
	if value.Status == "committed" && value.SubjectID != nil {
		_ = tx.Rollback(ctx)
		h.writeCommittedImport(ctx, c, value, *value.SubjectID)
		return
	}
	if value.Status != "draft" {
		writeError(c, http.StatusConflict, errorCodeImportConflict, "导入任务已结束")
		return
	}
	chapterRecords, err := database.ListSubjectImportChapters(ctx, tx, importID)
	if err != nil {
		writeError(c, http.StatusInternalServerError, errorCodeInternal, "读取导入章节失败")
		return
	}
	assetIDs, err := database.ListSubjectImportAssetIDs(ctx, tx, importID)
	if err != nil {
		writeError(c, http.StatusInternalServerError, errorCodeInternal, "读取导入图片失败")
		return
	}
	if len(chapterRecords) != value.ExpectedChapters || len(assetIDs) != value.ExpectedAssets {
		writeError(c, http.StatusConflict, errorCodeImportIncomplete, "导入任务尚未接收全部章节和图片")
		return
	}
	chapterTree, assetLinks, valid := h.buildStagedChapterTree(chapterRecords, assetIDs)
	if !valid {
		writeError(c, http.StatusBadRequest, errorCodeInvalidInput, "导入章节树或图片引用无效")
		return
	}
	externalID := value.ExternalID
	subjectID, err := h.storeSubjectTx(ctx, tx, value.OwnerID, &externalID, value.Title, value.Description, value.Tags, chapterTree)
	if err != nil {
		writeError(c, http.StatusBadRequest, errorCodeInvalidInput, "导入课程内容无效")
		return
	}
	publishedChapters, err := database.ListChaptersBySubject(ctx, tx, subjectID)
	if err != nil {
		writeError(c, http.StatusInternalServerError, errorCodeInternal, "关联课程图片失败")
		return
	}
	chapterIDs := make(map[string]int64, len(publishedChapters))
	for _, chapter := range publishedChapters {
		if chapter.ExternalID != nil {
			chapterIDs[*chapter.ExternalID] = chapter.ID
		}
	}
	for externalChapterID, linkedAssets := range assetLinks {
		chapterID, exists := chapterIDs[externalChapterID]
		if !exists || database.ReplaceChapterAssets(ctx, tx, chapterID, linkedAssets) != nil {
			writeError(c, http.StatusInternalServerError, errorCodeInternal, "关联课程图片失败")
			return
		}
	}
	if err := database.MarkSubjectImportCommitted(ctx, tx, importID, subjectID); err != nil {
		writeError(c, http.StatusInternalServerError, errorCodeInternal, "提交导入任务失败")
		return
	}
	if err := tx.Commit(ctx); err != nil {
		writeError(c, http.StatusInternalServerError, errorCodeInternal, "提交导入任务失败")
		return
	}
	h.writeCommittedImport(ctx, c, value, subjectID)
}

func (h *Handler) removeDeletedAssets(ctx context.Context, c *app.RequestContext, deleted []database.Asset) bool {
	if err := assetcleanup.RemoveDeleted(ctx, h.db, h.assetStore, deleted); err != nil {
		writeError(c, http.StatusInternalServerError, errorCodeInternal, "清理未使用图片失败")
		return false
	}
	return true
}

func (h *Handler) writeCommittedImport(ctx context.Context, c *app.RequestContext, value database.SubjectImport, subjectID int64) {
	value.Status = "committed"
	value.SubjectID = &subjectID
	view, err := h.subjectImportView(ctx, value)
	if err != nil {
		writeError(c, http.StatusInternalServerError, errorCodeInternal, "读取已导入课程失败")
		return
	}
	subject := struct {
		ID         string `json:"id"`
		ExternalID string `json:"external_id"`
		Title      string `json:"title"`
	}{ID: h.ids.Encode(subjectID), ExternalID: value.ExternalID, Title: value.Title}
	writeData(c, http.StatusOK, map[string]any{"import": view, "subject": subject})
}

func (h *Handler) findSubjectImport(ctx context.Context, c *app.RequestContext) (database.SubjectImport, bool) {
	importID, ok := h.decodeSubjectImportID(c)
	if !ok {
		return database.SubjectImport{}, false
	}
	value, err := database.FindSubjectImportForOwner(ctx, h.db, importID, currentUser(c).ID)
	if err != nil {
		writeError(c, http.StatusNotFound, errorCodeNotFound, "导入任务不存在")
		return database.SubjectImport{}, false
	}
	return value, true
}

func (h *Handler) decodeSubjectImportID(c *app.RequestContext) (int64, bool) {
	value, err := h.ids.Decode(c.Param("id"))
	if err != nil {
		writeError(c, http.StatusNotFound, errorCodeNotFound, "导入任务不存在")
		return 0, false
	}
	return value, true
}

func (h *Handler) subjectImportView(ctx context.Context, value database.SubjectImport) (subjectImportView, error) {
	chapters, assets, err := database.CountSubjectImportParts(ctx, h.db, value.ID)
	if err != nil {
		return subjectImportView{}, err
	}
	view := subjectImportView{
		ID: h.ids.Encode(value.ID), Status: value.Status, ExternalID: value.ExternalID,
		ExpectedChapters: value.ExpectedChapters, ExpectedAssets: value.ExpectedAssets,
		ReceivedChapters: chapters, ReceivedAssets: assets,
	}
	if value.SubjectID != nil {
		view.SubjectID = h.ids.Encode(*value.SubjectID)
	}
	return view, nil
}

func validStagedChapterBatch(chapters []stagedChapterInput) bool {
	seenExternalIDs := make(map[string]struct{}, len(chapters))
	seenPositions := make(map[int]struct{}, len(chapters))
	for _, chapter := range chapters {
		externalID := strings.TrimSpace(chapter.ExternalID)
		title := strings.TrimSpace(chapter.Title)
		if externalID == "" || utf8.RuneCountInString(externalID) > 160 || title == "" || utf8.RuneCountInString(title) > 200 || utf8.RuneCountInString(chapter.Content) > 200_000 || chapter.Position < 0 {
			return false
		}
		if _, exists := seenExternalIDs[externalID]; exists {
			return false
		}
		if _, exists := seenPositions[chapter.Position]; exists {
			return false
		}
		seenExternalIDs[externalID] = struct{}{}
		seenPositions[chapter.Position] = struct{}{}
	}
	return true
}

func normalizedOptionalString(value *string) *string {
	if value == nil {
		return nil
	}
	normalized := strings.TrimSpace(*value)
	if normalized == "" {
		return nil
	}
	return &normalized
}

func subjectImportExpired(value database.SubjectImport) bool {
	return value.Status == "draft" && value.UpdatedAt.Before(time.Now().UTC().Add(-subjectImportIdleTimeout))
}

func (h *Handler) buildStagedChapterTree(records []database.SubjectImportChapter, allowedAssetIDs []int64) ([]chapterInput, map[string][]int64, bool) {
	if len(records) == 0 {
		return nil, nil, false
	}
	nodes := make(map[string]database.SubjectImportChapter, len(records))
	children := make(map[string][]string)
	roots := make([]string, 0)
	for position, record := range records {
		if record.Position != position {
			return nil, nil, false
		}
		nodes[record.ExternalID] = record
		if record.ParentExternalID == nil {
			roots = append(roots, record.ExternalID)
		} else {
			children[*record.ParentExternalID] = append(children[*record.ParentExternalID], record.ExternalID)
		}
	}
	for parent := range children {
		if _, exists := nodes[parent]; !exists {
			return nil, nil, false
		}
	}
	allowedAssets := make(map[int64]struct{}, len(allowedAssetIDs))
	for _, assetID := range allowedAssetIDs {
		allowedAssets[assetID] = struct{}{}
	}
	assetLinks := make(map[string][]int64, len(records))
	visiting := make(map[string]bool, len(records))
	built := make(map[string]bool, len(records))
	var build func(string) (chapterInput, bool)
	build = func(externalID string) (chapterInput, bool) {
		if visiting[externalID] || built[externalID] {
			return chapterInput{}, false
		}
		visiting[externalID] = true
		record := nodes[externalID]
		input := chapterInput{ExternalID: record.ExternalID, Title: record.Title, Content: record.Content}
		assetLinks[externalID] = nil
		references, valid := markdownAssetReferences(record.Content)
		if !valid {
			return chapterInput{}, false
		}
		for _, reference := range references {
			assetID, err := h.ids.Decode(reference)
			if err != nil {
				return chapterInput{}, false
			}
			if _, exists := allowedAssets[assetID]; !exists || slices.Contains(assetLinks[externalID], assetID) {
				if !exists {
					return chapterInput{}, false
				}
				continue
			}
			assetLinks[externalID] = append(assetLinks[externalID], assetID)
		}
		for _, childID := range children[externalID] {
			child, ok := build(childID)
			if !ok {
				return chapterInput{}, false
			}
			input.Children = append(input.Children, child)
		}
		visiting[externalID] = false
		built[externalID] = true
		return input, true
	}
	sort.Slice(roots, func(i, j int) bool { return nodes[roots[i]].Position < nodes[roots[j]].Position })
	tree := make([]chapterInput, 0, len(roots))
	for _, root := range roots {
		input, ok := build(root)
		if !ok {
			return nil, nil, false
		}
		tree = append(tree, input)
	}
	if len(built) != len(records) {
		return nil, nil, false
	}
	return tree, assetLinks, true
}
