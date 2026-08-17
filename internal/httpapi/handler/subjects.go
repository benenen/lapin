package handler

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/jackc/pgx/v5"

	"github.com/benenen/lapin/internal/database"
)

type chapterView struct {
	ID         string    `json:"id"`
	ParentID   *string   `json:"parent_id"`
	ExternalID *string   `json:"external_id,omitempty"`
	Position   int       `json:"position"`
	Title      string    `json:"title"`
	Content    string    `json:"content"`
	CreatedAt  time.Time `json:"created_at"`
}

type subjectView struct {
	ID          string        `json:"id"`
	OwnerID     string        `json:"owner_id"`
	OwnerName   string        `json:"owner_name"`
	ExternalID  *string       `json:"external_id"`
	Title       string        `json:"title"`
	Description string        `json:"description"`
	Tags        []string      `json:"tags"`
	Chapters    []chapterView `json:"chapters,omitempty"`
	CreatedAt   time.Time     `json:"created_at"`
	UpdatedAt   time.Time     `json:"updated_at"`
}

type chapterInput struct {
	ExternalID string         `json:"external_id,omitempty"`
	Title      string         `json:"title"`
	Content    string         `json:"content"`
	Children   []chapterInput `json:"children,omitempty"`
}

var errInvalidSubject = errors.New("invalid subject")

func (h *Handler) CreateSubject(ctx context.Context, c *app.RequestContext) {
	var request struct {
		Title       string         `json:"title"`
		Description string         `json:"description"`
		Tags        []string       `json:"tags"`
		Chapters    []chapterInput `json:"chapters"`
	}
	if !decodeJSON(c, &request) {
		return
	}
	subject, err := h.storeSubject(ctx, currentUser(c).ID, nil, request.Title, request.Description, request.Tags, request.Chapters)
	if errors.Is(err, errInvalidSubject) {
		writeError(c, 400, errorCodeInvalidInput, "请检查科目、标签和章节内容")
		return
	}
	if err != nil {
		writeError(c, 500, errorCodeInternal, "创建科目失败")
		return
	}
	writeData(c, 201, subject)
}

func (h *Handler) ImportSubject(ctx context.Context, c *app.RequestContext) {
	var request struct {
		ExternalID  string         `json:"external_id"`
		Title       string         `json:"title"`
		Description string         `json:"description"`
		Tags        []string       `json:"tags"`
		Chapters    []chapterInput `json:"chapters"`
	}
	if !decodeJSON(c, &request) {
		return
	}
	request.ExternalID = strings.TrimSpace(request.ExternalID)
	if request.ExternalID == "" || utf8.RuneCountInString(request.ExternalID) > 160 {
		writeError(c, 400, errorCodeInvalidInput, "external_id 不能为空且最多 160 字")
		return
	}
	subject, err := h.storeSubject(ctx, currentUser(c).ID, &request.ExternalID, request.Title, request.Description, request.Tags, request.Chapters)
	if errors.Is(err, errInvalidSubject) {
		writeError(c, 400, errorCodeInvalidInput, "导入章节必须提供唯一 external_id，并检查科目、标签和章节内容")
		return
	}
	if err != nil {
		writeError(c, 500, errorCodeInternal, "导入科目失败")
		return
	}
	writeData(c, 200, subject)
}

func (h *Handler) storeSubject(ctx context.Context, ownerID int64, externalID *string, title, description string, tags []string, chapters []chapterInput) (subjectView, error) {
	title = strings.TrimSpace(title)
	description = strings.TrimSpace(description)
	normalizedTags, ok := normalizeTags(tags)
	imported := externalID != nil
	if title == "" || utf8.RuneCountInString(title) > 200 || utf8.RuneCountInString(description) > 4000 || !ok || countChapters(chapters) > 100 || !validateChapterInputs(chapters, imported) {
		return subjectView{}, errInvalidSubject
	}

	tx, err := h.db.Begin(ctx)
	if err != nil {
		return subjectView{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	subject := database.Subject{OwnerID: ownerID, ExternalID: externalID, Title: title, Description: description}
	var subjectID int64
	if imported {
		subjectID, err = database.UpsertExternalSubject(ctx, tx, subject)
	} else {
		subjectID, err = database.InsertSubject(ctx, tx, subject)
	}
	if err != nil {
		return subjectView{}, err
	}
	if err := database.DeleteTagsBySubject(ctx, tx, subjectID); err != nil {
		return subjectView{}, err
	}
	for _, tag := range normalizedTags {
		if _, err := database.InsertTag(ctx, tx, subjectID, tag); err != nil {
			return subjectView{}, err
		}
	}
	position := 0
	importedChapterIDs := make([]int64, 0, countChapters(chapters))
	if err := storeChapterTree(ctx, tx, subjectID, nil, chapters, &position, imported, &importedChapterIDs); err != nil {
		return subjectView{}, err
	}
	if imported {
		if err := database.RepositionUnimportedChapters(ctx, tx, subjectID, importedChapterIDs, position); err != nil {
			return subjectView{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return subjectView{}, err
	}
	return h.fetchSubject(ctx, subjectID)
}

func (h *Handler) ListSubjects(ctx context.Context, c *app.RequestContext) {
	subjects, err := database.ListSubjects(ctx, h.db)
	if err != nil {
		writeError(c, 500, errorCodeInternal, "读取科目失败")
		return
	}
	views := make([]subjectView, 0, len(subjects))
	for _, subject := range subjects {
		view, err := h.subjectView(ctx, subject, false)
		if err != nil {
			writeError(c, 500, errorCodeInternal, "读取科目失败")
			return
		}
		views = append(views, view)
	}
	writeData(c, 200, views)
}

func (h *Handler) GetSubject(ctx context.Context, c *app.RequestContext) {
	id, err := h.ids.Decode(c.Param("id"))
	if err != nil {
		writeError(c, 400, errorCodeInvalidID, "科目 ID 无效")
		return
	}
	subject, err := h.fetchSubject(ctx, id)
	if err != nil {
		writeError(c, 404, errorCodeNotFound, "科目不存在")
		return
	}
	writeData(c, 200, subject)
}

func (h *Handler) UpdateSubject(ctx context.Context, c *app.RequestContext) {
	subjectID, err := h.ids.Decode(c.Param("id"))
	if err != nil {
		writeError(c, 400, errorCodeInvalidID, "科目 ID 无效")
		return
	}
	var request struct {
		Title       string `json:"title"`
		Description string `json:"description"`
	}
	if !decodeJSON(c, &request) {
		return
	}
	request.Title = strings.TrimSpace(request.Title)
	request.Description = strings.TrimSpace(request.Description)
	if request.Title == "" || utf8.RuneCountInString(request.Title) > 200 || utf8.RuneCountInString(request.Description) > 4000 {
		writeError(c, 400, errorCodeInvalidInput, "请检查科目标题和简介")
		return
	}
	subject, err := database.UpdateSubjectForOwner(ctx, h.db, subjectID, currentUser(c).ID, request.Title, request.Description)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(c, 404, errorCodeNotFound, "科目不存在或无权操作")
		return
	}
	if err != nil {
		writeError(c, 500, errorCodeInternal, "更新科目失败")
		return
	}
	view, err := h.subjectView(ctx, subject, true)
	if err != nil {
		writeError(c, 500, errorCodeInternal, "读取科目失败")
		return
	}
	writeData(c, 200, view)
}

func (h *Handler) fetchSubject(ctx context.Context, id int64) (subjectView, error) {
	subject, err := database.FindSubjectByID(ctx, h.db, id)
	if err != nil {
		return subjectView{}, err
	}
	return h.subjectView(ctx, subject, true)
}

func (h *Handler) subjectView(ctx context.Context, subject database.Subject, includeChapters bool) (subjectView, error) {
	owner, err := database.FindUserByID(ctx, h.db, subject.OwnerID)
	if err != nil {
		return subjectView{}, err
	}
	tags, err := database.ListTagsBySubject(ctx, h.db, subject.ID)
	if err != nil {
		return subjectView{}, err
	}
	view := subjectView{
		ID: h.ids.Encode(subject.ID), OwnerID: h.ids.Encode(subject.OwnerID), OwnerName: owner.DisplayName,
		ExternalID: subject.ExternalID, Title: subject.Title, Description: subject.Description,
		Tags: tags, CreatedAt: subject.CreatedAt, UpdatedAt: subject.UpdatedAt,
	}
	if !includeChapters {
		return view, nil
	}
	chapters, err := database.ListChaptersBySubject(ctx, h.db, subject.ID)
	if err != nil {
		return subjectView{}, err
	}
	view.Chapters = make([]chapterView, 0, len(chapters))
	for _, chapter := range chapters {
		view.Chapters = append(view.Chapters, h.chapterView(chapter))
	}
	return view, nil
}

func (h *Handler) chapterView(chapter database.Chapter) chapterView {
	var parentID *string
	if chapter.ParentID != nil {
		encoded := h.ids.Encode(*chapter.ParentID)
		parentID = &encoded
	}
	return chapterView{
		ID: h.ids.Encode(chapter.ID), ParentID: parentID, ExternalID: chapter.ExternalID,
		Position: chapter.Position, Title: chapter.Title, Content: chapter.Content, CreatedAt: chapter.CreatedAt,
	}
}

func (h *Handler) CreateChapter(ctx context.Context, c *app.RequestContext) {
	subjectID, err := h.ids.Decode(c.Param("id"))
	if err != nil || !h.isSubjectOwner(ctx, subjectID, currentUser(c).ID) {
		writeError(c, 404, errorCodeNotFound, "科目不存在或无权操作")
		return
	}
	var request struct {
		ParentID *string `json:"parent_id"`
		Title    string  `json:"title"`
		Content  string  `json:"content"`
	}
	if !decodeJSON(c, &request) {
		return
	}
	request.Title = strings.TrimSpace(request.Title)
	if request.Title == "" || utf8.RuneCountInString(request.Title) > 200 || utf8.RuneCountInString(request.Content) > 200_000 {
		writeError(c, 400, errorCodeInvalidInput, "请检查章节标题和内容")
		return
	}
	var parentID *int64
	if request.ParentID != nil {
		decoded, err := h.ids.Decode(*request.ParentID)
		if err != nil {
			writeError(c, 400, errorCodeInvalidParent, "父章节不属于当前科目")
			return
		}
		exists, err := database.ChapterExistsInSubject(ctx, h.db, decoded, subjectID)
		if err != nil || !exists {
			writeError(c, 400, errorCodeInvalidParent, "父章节不属于当前科目")
			return
		}
		parentID = &decoded
	}
	tx, err := h.db.Begin(ctx)
	if err != nil {
		writeError(c, 500, errorCodeInternal, "创建章节失败")
		return
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := database.LockSubjectForUpdate(ctx, tx, subjectID); err != nil {
		writeError(c, 500, errorCodeInternal, "创建章节失败")
		return
	}
	position, err := database.NextChapterPosition(ctx, tx, subjectID)
	if err != nil {
		writeError(c, 500, errorCodeInternal, "创建章节失败")
		return
	}
	chapter := database.Chapter{SubjectID: subjectID, ParentID: parentID, Position: position, Title: request.Title, Content: request.Content}
	chapter.ID, err = database.InsertChapter(ctx, tx, chapter)
	if err != nil {
		writeError(c, 500, errorCodeInternal, "创建章节失败")
		return
	}
	if err := tx.Commit(ctx); err != nil {
		writeError(c, 500, errorCodeInternal, "创建章节失败")
		return
	}
	chapter.CreatedAt = time.Now().UTC()
	writeData(c, 201, h.chapterView(chapter))
}

func (h *Handler) UpdateChapter(ctx context.Context, c *app.RequestContext) {
	chapterID, err := h.ids.Decode(c.Param("id"))
	if err != nil {
		writeError(c, 400, errorCodeInvalidID, "章节 ID 无效")
		return
	}
	var request struct {
		Title   string `json:"title"`
		Content string `json:"content"`
	}
	if !decodeJSON(c, &request) {
		return
	}
	request.Title = strings.TrimSpace(request.Title)
	if request.Title == "" || utf8.RuneCountInString(request.Title) > 200 || utf8.RuneCountInString(request.Content) > 200_000 {
		writeError(c, 400, errorCodeInvalidInput, "请检查章节标题和内容")
		return
	}
	chapter, err := database.UpdateChapterForOwner(ctx, h.db, chapterID, currentUser(c).ID, request.Title, request.Content)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(c, 404, errorCodeNotFound, "章节不存在或无权操作")
		return
	}
	if err != nil {
		writeError(c, 500, errorCodeInternal, "更新章节失败")
		return
	}
	writeData(c, 200, h.chapterView(chapter))
}

func (h *Handler) ReplaceTags(ctx context.Context, c *app.RequestContext) {
	subjectID, err := h.ids.Decode(c.Param("id"))
	if err != nil || !h.isSubjectOwner(ctx, subjectID, currentUser(c).ID) {
		writeError(c, 404, errorCodeNotFound, "科目不存在或无权操作")
		return
	}
	var request struct {
		Tags []string `json:"tags"`
	}
	if !decodeJSON(c, &request) {
		return
	}
	tags, ok := normalizeTags(request.Tags)
	if !ok {
		writeError(c, 400, errorCodeInvalidInput, "标签最多 20 个，每个最多 30 字")
		return
	}
	tx, err := h.db.Begin(ctx)
	if err != nil {
		writeError(c, 500, errorCodeInternal, "保存标签失败")
		return
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := database.DeleteTagsBySubject(ctx, tx, subjectID); err != nil {
		writeError(c, 500, errorCodeInternal, "保存标签失败")
		return
	}
	for _, tag := range tags {
		if _, err := database.InsertTag(ctx, tx, subjectID, tag); err != nil {
			writeError(c, 500, errorCodeInternal, "保存标签失败")
			return
		}
	}
	if err := tx.Commit(ctx); err != nil {
		writeError(c, 500, errorCodeInternal, "保存标签失败")
		return
	}
	writeData(c, 200, map[string]any{"tags": tags})
}

func (h *Handler) isSubjectOwner(ctx context.Context, subjectID, userID int64) bool {
	exists, err := database.IsSubjectOwner(ctx, h.db, subjectID, userID)
	return err == nil && exists
}

func normalizeTags(input []string) ([]string, bool) {
	if len(input) > 20 {
		return nil, false
	}
	seen := make(map[string]struct{}, len(input))
	result := make([]string, 0, len(input))
	for _, value := range input {
		value = strings.TrimSpace(value)
		if value == "" || utf8.RuneCountInString(value) > 30 {
			return nil, false
		}
		key := strings.ToLower(value)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, value)
	}
	sort.Strings(result)
	return result, true
}

func countChapters(chapters []chapterInput) int {
	total := 0
	stack := append([]chapterInput(nil), chapters...)
	for len(stack) > 0 {
		last := len(stack) - 1
		chapter := stack[last]
		stack = stack[:last]
		total++
		stack = append(stack, chapter.Children...)
	}
	return total
}

func validateChapterInputs(chapters []chapterInput, requireExternalID bool) bool {
	seenExternalIDs := make(map[string]struct{})
	stack := append([]chapterInput(nil), chapters...)
	for len(stack) > 0 {
		last := len(stack) - 1
		chapter := stack[last]
		stack = stack[:last]
		chapter.Title = strings.TrimSpace(chapter.Title)
		chapter.ExternalID = strings.TrimSpace(chapter.ExternalID)
		if chapter.Title == "" || utf8.RuneCountInString(chapter.Title) > 200 || utf8.RuneCountInString(chapter.Content) > 200_000 {
			return false
		}
		if requireExternalID {
			if chapter.ExternalID == "" || utf8.RuneCountInString(chapter.ExternalID) > 160 {
				return false
			}
			if _, exists := seenExternalIDs[chapter.ExternalID]; exists {
				return false
			}
			seenExternalIDs[chapter.ExternalID] = struct{}{}
		}
		stack = append(stack, chapter.Children...)
	}
	return true
}

func storeChapterTree(ctx context.Context, db database.DBTX, subjectID int64, parentID *int64, chapters []chapterInput, position *int, imported bool, importedIDs *[]int64) error {
	for _, input := range chapters {
		input.ExternalID = strings.TrimSpace(input.ExternalID)
		chapter := database.Chapter{
			SubjectID: subjectID, ParentID: parentID, Position: *position,
			Title: strings.TrimSpace(input.Title), Content: input.Content,
		}
		if imported {
			chapter.ExternalID = &input.ExternalID
		}
		var err error
		if imported {
			chapter.ID, err = database.UpsertExternalChapter(ctx, db, chapter)
		} else {
			chapter.ID, err = database.InsertChapter(ctx, db, chapter)
		}
		if err != nil {
			return err
		}
		if imported {
			*importedIDs = append(*importedIDs, chapter.ID)
		}
		(*position)++
		if err := storeChapterTree(ctx, db, subjectID, &chapter.ID, input.Children, position, imported, importedIDs); err != nil {
			return err
		}
	}
	return nil
}
