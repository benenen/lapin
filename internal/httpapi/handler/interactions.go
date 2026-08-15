package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"math"
	"strings"
	"time"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/cloudwego/hertz/pkg/app"

	"github.com/benenen/lapin/internal/database"
)

type annotationView struct {
	ID          string    `json:"id"`
	ChapterID   string    `json:"chapter_id"`
	UserID      string    `json:"user_id"`
	AuthorName  string    `json:"author_name"`
	StartOffset int       `json:"start_offset"`
	EndOffset   int       `json:"end_offset"`
	Quote       string    `json:"quote"`
	Note        string    `json:"note"`
	Color       string    `json:"color"`
	CreatedAt   time.Time `json:"created_at"`
}

func (h *Handler) CreateAnnotation(ctx context.Context, c *app.RequestContext) {
	chapterID, ok := h.existingChapterID(ctx, c)
	if !ok {
		return
	}
	var request struct {
		StartOffset int    `json:"start_offset"`
		EndOffset   int    `json:"end_offset"`
		Quote       string `json:"quote"`
		Note        string `json:"note"`
		Color       string `json:"color"`
	}
	if !decodeJSON(c, &request) {
		return
	}
	request.Note = strings.TrimSpace(request.Note)
	request.Color = strings.TrimSpace(request.Color)
	if request.Color == "" {
		request.Color = "yellow"
	}
	if request.StartOffset < 0 || request.EndOffset < request.StartOffset || request.Note == "" || utf8.RuneCountInString(request.Note) > 2000 || utf8.RuneCountInString(request.Quote) > 1000 || !allowedAnnotationColor(request.Color) {
		writeError(c, 400, "invalid_input", "标注范围、内容或颜色无效")
		return
	}
	chapter, err := database.FindChapterByID(ctx, h.db, chapterID)
	quoteLength := len(utf16.Encode([]rune(request.Quote)))
	if err != nil || request.EndOffset > len(utf16.Encode([]rune(chapter.Content))) || request.EndOffset-request.StartOffset != quoteLength {
		writeError(c, 400, "invalid_input", "标注未匹配当前章节正文")
		return
	}
	if request.Quote == "" {
		request.StartOffset = 0
		request.EndOffset = 0
	} else if !strings.Contains(chapter.Content, request.Quote) {
		writeError(c, 400, "invalid_input", "标注未匹配当前章节正文")
		return
	}
	user := currentUser(c)
	createdAt := time.Now().UTC()
	annotation := database.Annotation{
		ChapterID: chapterID, UserID: user.ID, StartOffset: request.StartOffset, EndOffset: request.EndOffset,
		Quote: request.Quote, Note: request.Note, Color: request.Color, CreatedAt: createdAt,
	}
	annotation.ID, err = database.InsertAnnotation(ctx, h.db, annotation)
	if err != nil {
		writeError(c, 500, "internal_error", "保存标注失败")
		return
	}
	writeData(c, 201, h.annotationView(annotation, user.Name))
}

func (h *Handler) ListAnnotations(ctx context.Context, c *app.RequestContext) {
	chapterID, ok := h.existingChapterID(ctx, c)
	if !ok {
		return
	}
	annotations, err := database.ListAnnotationsByChapter(ctx, h.db, chapterID)
	if err != nil {
		writeError(c, 500, "internal_error", "读取标注失败")
		return
	}
	items := make([]annotationView, 0, len(annotations))
	for _, annotation := range annotations {
		author, err := database.FindUserByID(ctx, h.db, annotation.UserID)
		if err != nil {
			writeError(c, 500, "internal_error", "读取标注失败")
			return
		}
		items = append(items, h.annotationView(annotation, author.DisplayName))
	}
	writeData(c, 200, items)
}

func (h *Handler) annotationView(annotation database.Annotation, authorName string) annotationView {
	return annotationView{
		ID: h.ids.Encode(annotation.ID), ChapterID: h.ids.Encode(annotation.ChapterID), UserID: h.ids.Encode(annotation.UserID),
		AuthorName: authorName, StartOffset: annotation.StartOffset, EndOffset: annotation.EndOffset,
		Quote: annotation.Quote, Note: annotation.Note, Color: annotation.Color, CreatedAt: annotation.CreatedAt,
	}
}

type whiteboardView struct {
	ID         string          `json:"id"`
	ChapterID  string          `json:"chapter_id"`
	UserID     string          `json:"user_id"`
	AuthorName string          `json:"author_name"`
	Data       json.RawMessage `json:"data"`
	UpdatedAt  time.Time       `json:"updated_at"`
}

func (h *Handler) SaveWhiteboard(ctx context.Context, c *app.RequestContext) {
	chapterID, ok := h.existingChapterID(ctx, c)
	if !ok {
		return
	}
	var request struct {
		Data json.RawMessage `json:"data"`
	}
	if !decodeJSON(c, &request) {
		return
	}
	if len(request.Data) > 900_000 || !validWhiteboardData(request.Data, h.ids.Encode(chapterID)) {
		writeError(c, 400, "invalid_input", "白板数据无效或过大")
		return
	}
	user := currentUser(c)
	whiteboard := database.Whiteboard{ChapterID: chapterID, UserID: user.ID, Data: request.Data, UpdatedAt: time.Now().UTC()}
	var err error
	whiteboard.ID, err = database.UpsertWhiteboard(ctx, h.db, whiteboard)
	if err != nil {
		writeError(c, 500, "internal_error", "保存白板失败")
		return
	}
	writeData(c, 200, h.whiteboardView(whiteboard, user.Name))
}

func (h *Handler) ListWhiteboards(ctx context.Context, c *app.RequestContext) {
	chapterID, ok := h.existingChapterID(ctx, c)
	if !ok {
		return
	}
	user := currentUser(c)
	whiteboards, err := database.ListWhiteboardsByChapterAndUser(ctx, h.db, chapterID, user.ID)
	if err != nil {
		writeError(c, 500, "internal_error", "读取白板失败")
		return
	}
	items := make([]whiteboardView, 0, len(whiteboards))
	for _, whiteboard := range whiteboards {
		items = append(items, h.whiteboardView(whiteboard, user.Name))
	}
	writeData(c, 200, items)
}

func (h *Handler) whiteboardView(whiteboard database.Whiteboard, authorName string) whiteboardView {
	return whiteboardView{
		ID: h.ids.Encode(whiteboard.ID), ChapterID: h.ids.Encode(whiteboard.ChapterID), UserID: h.ids.Encode(whiteboard.UserID),
		AuthorName: authorName, Data: whiteboard.Data, UpdatedAt: whiteboard.UpdatedAt,
	}
}

type commentView struct {
	ID         string    `json:"id"`
	ChapterID  string    `json:"chapter_id"`
	UserID     string    `json:"user_id"`
	AuthorName string    `json:"author_name"`
	Body       string    `json:"body"`
	CreatedAt  time.Time `json:"created_at"`
}

func (h *Handler) CreateComment(ctx context.Context, c *app.RequestContext) {
	chapterID, ok := h.existingChapterID(ctx, c)
	if !ok {
		return
	}
	var request struct {
		Body string `json:"body"`
	}
	if !decodeJSON(c, &request) {
		return
	}
	request.Body = strings.TrimSpace(request.Body)
	if request.Body == "" || utf8.RuneCountInString(request.Body) > 2000 {
		writeError(c, 400, "invalid_input", "评论不能为空且最多 2000 字")
		return
	}
	user := currentUser(c)
	comment := database.Comment{ChapterID: chapterID, UserID: user.ID, Body: request.Body, CreatedAt: time.Now().UTC()}
	var err error
	comment.ID, err = database.InsertComment(ctx, h.db, comment)
	if err != nil {
		writeError(c, 500, "internal_error", "发表评论失败")
		return
	}
	writeData(c, 201, h.commentView(comment, user.Name))
}

func (h *Handler) ListComments(ctx context.Context, c *app.RequestContext) {
	chapterID, ok := h.existingChapterID(ctx, c)
	if !ok {
		return
	}
	comments, err := database.ListCommentsByChapter(ctx, h.db, chapterID)
	if err != nil {
		writeError(c, 500, "internal_error", "读取评论失败")
		return
	}
	items := make([]commentView, 0, len(comments))
	for _, comment := range comments {
		author, err := database.FindUserByID(ctx, h.db, comment.UserID)
		if err != nil {
			writeError(c, 500, "internal_error", "读取评论失败")
			return
		}
		items = append(items, h.commentView(comment, author.DisplayName))
	}
	writeData(c, 200, items)
}

func (h *Handler) commentView(comment database.Comment, authorName string) commentView {
	return commentView{
		ID: h.ids.Encode(comment.ID), ChapterID: h.ids.Encode(comment.ChapterID), UserID: h.ids.Encode(comment.UserID),
		AuthorName: authorName, Body: comment.Body, CreatedAt: comment.CreatedAt,
	}
}

func (h *Handler) existingChapterID(ctx context.Context, c *app.RequestContext) (int64, bool) {
	chapterID, err := h.ids.Decode(c.Param("id"))
	if err != nil {
		writeError(c, 404, "not_found", "章节不存在")
		return 0, false
	}
	exists, err := database.ChapterExists(ctx, h.db, chapterID)
	if err != nil || !exists {
		writeError(c, 404, "not_found", "章节不存在")
		return 0, false
	}
	return chapterID, true
}

func allowedAnnotationColor(value string) bool {
	return value == "yellow" || value == "green" || value == "blue" || value == "pink"
}

func validWhiteboardData(data []byte, chapterHashID string) bool {
	var document struct {
		Version int `json:"version"`
		Anchor  struct {
			Type            string `json:"type"`
			ID              string `json:"id"`
			ContentRevision string `json:"content_revision"`
		} `json:"anchor"`
		Space struct {
			Width  float64 `json:"width"`
			Height float64 `json:"height"`
			Fit    string  `json:"fit"`
		} `json:"space"`
		Document struct {
			Store  map[string]json.RawMessage `json:"store"`
			Schema json.RawMessage            `json:"schema"`
		} `json:"document"`
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&document) != nil || document.Version != 2 || document.Anchor.Type != "chapter" || document.Anchor.ID != chapterHashID {
		return false
	}
	if document.Anchor.ContentRevision == "" || len(document.Anchor.ContentRevision) > 128 || document.Space.Fit != "contain" {
		return false
	}
	if !validCanvasDimension(document.Space.Width) || !validCanvasDimension(document.Space.Height) || document.Document.Store == nil || len(document.Document.Store) > 5_000 {
		return false
	}
	var schema struct {
		SchemaVersion int            `json:"schemaVersion"`
		Sequences     map[string]int `json:"sequences"`
	}
	if json.Unmarshal(document.Document.Schema, &schema) != nil || schema.SchemaVersion < 1 || schema.Sequences == nil || schema.Sequences["com.tldraw.store"] < 1 || schema.Sequences["com.tldraw.document"] < 1 || schema.Sequences["com.tldraw.page"] < 1 {
		return false
	}
	hasDocument := false
	hasPage := false
	for key, record := range document.Document.Store {
		if key == "" || len(key) > 200 || len(record) > 100_000 {
			return false
		}
		var object struct {
			ID       string `json:"id"`
			TypeName string `json:"typeName"`
		}
		if json.Unmarshal(record, &object) != nil || object.ID != key || object.TypeName == "" || len(object.TypeName) > 64 {
			return false
		}
		hasDocument = hasDocument || object.TypeName == "document"
		hasPage = hasPage || object.TypeName == "page"
	}
	return hasDocument && hasPage
}

func validCanvasDimension(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0) && value >= 100 && value <= 10_000
}
