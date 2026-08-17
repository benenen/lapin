package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
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
			Type     string                     `json:"type"`
			Version  int                        `json:"version"`
			Elements []json.RawMessage          `json:"elements"`
			AppState map[string]json.RawMessage `json:"appState"`
			Files    map[string]json.RawMessage `json:"files"`
		} `json:"document"`
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&document) != nil || decoder.Decode(&struct{}{}) != io.EOF || document.Version != 3 || document.Anchor.Type != "chapter" || document.Anchor.ID != chapterHashID {
		return false
	}
	if document.Anchor.ContentRevision == "" || len(document.Anchor.ContentRevision) > 128 || document.Space.Fit != "contain" {
		return false
	}
	if !validCanvasDimension(document.Space.Width) || !validCanvasDimension(document.Space.Height) || document.Document.Type != "excalidraw" || document.Document.Version != 2 {
		return false
	}
	if document.Document.Elements == nil || len(document.Document.Elements) > 5_000 || len(document.Document.AppState) != 1 || document.Document.Files == nil || len(document.Document.Files) != 0 {
		return false
	}
	var background string
	if json.Unmarshal(document.Document.AppState["viewBackgroundColor"], &background) != nil || background != "transparent" {
		return false
	}
	elementIDs := make(map[string]struct{}, len(document.Document.Elements))
	elementTypes := make(map[string]string, len(document.Document.Elements))
	validatedElements := make([]whiteboardElement, 0, len(document.Document.Elements))
	for _, record := range document.Document.Elements {
		if len(record) == 0 || len(record) > 100_000 {
			return false
		}
		elementID, ok := validWhiteboardElement(record)
		if !ok {
			return false
		}
		if _, exists := elementIDs[elementID]; exists {
			return false
		}
		elementIDs[elementID] = struct{}{}
		var element whiteboardElement
		if json.Unmarshal(record, &element) != nil {
			return false
		}
		elementTypes[elementID] = element.Type
		validatedElements = append(validatedElements, element)
	}
	return validWhiteboardElementReferences(validatedElements, elementTypes)
}

type whiteboardElement struct {
	ID                 string                    `json:"id"`
	Type               string                    `json:"type"`
	X                  *float64                  `json:"x"`
	Y                  *float64                  `json:"y"`
	Width              *float64                  `json:"width"`
	Height             *float64                  `json:"height"`
	Angle              *float64                  `json:"angle"`
	Seed               *int64                    `json:"seed"`
	Version            *int64                    `json:"version"`
	VersionNonce       *int64                    `json:"versionNonce"`
	Updated            *int64                    `json:"updated"`
	Opacity            *float64                  `json:"opacity"`
	StrokeWidth        *float64                  `json:"strokeWidth"`
	Roughness          *float64                  `json:"roughness"`
	IsDeleted          *bool                     `json:"isDeleted"`
	Locked             *bool                     `json:"locked"`
	Index              *string                   `json:"index"`
	StrokeColor        *string                   `json:"strokeColor"`
	BackgroundColor    *string                   `json:"backgroundColor"`
	FillStyle          *string                   `json:"fillStyle"`
	StrokeStyle        *string                   `json:"strokeStyle"`
	Roundness          *whiteboardRoundness      `json:"roundness"`
	GroupIDs           *[]string                 `json:"groupIds"`
	FrameID            *string                   `json:"frameId"`
	BoundElements      *[]whiteboardBoundElement `json:"boundElements"`
	Link               *string                   `json:"link"`
	Points             *[][]float64              `json:"points"`
	LastCommittedPoint []float64                 `json:"lastCommittedPoint"`
	StartBinding       *whiteboardBinding        `json:"startBinding"`
	EndBinding         *whiteboardBinding        `json:"endBinding"`
	StartArrowhead     *string                   `json:"startArrowhead"`
	EndArrowhead       *string                   `json:"endArrowhead"`
	Elbowed            *bool                     `json:"elbowed"`
	FixedSegments      *[]whiteboardFixedSegment `json:"fixedSegments"`
	StartIsSpecial     *bool                     `json:"startIsSpecial"`
	EndIsSpecial       *bool                     `json:"endIsSpecial"`
	Pressures          *[]float64                `json:"pressures"`
	SimulatePressure   *bool                     `json:"simulatePressure"`
	Text               *string                   `json:"text"`
	OriginalText       *string                   `json:"originalText"`
	FontSize           *float64                  `json:"fontSize"`
	FontFamily         *int64                    `json:"fontFamily"`
	TextAlign          *string                   `json:"textAlign"`
	VerticalAlign      *string                   `json:"verticalAlign"`
	LineHeight         *float64                  `json:"lineHeight"`
	ContainerID        *string                   `json:"containerId"`
	AutoResize         *bool                     `json:"autoResize"`
}

type whiteboardRoundness struct {
	Type  int      `json:"type"`
	Value *float64 `json:"value,omitempty"`
}

type whiteboardBoundElement struct {
	ID   string `json:"id"`
	Type string `json:"type"`
}

type whiteboardBinding struct {
	ElementID  string          `json:"elementId"`
	Focus      *float64        `json:"focus"`
	Gap        *float64        `json:"gap"`
	FixedPoint json.RawMessage `json:"fixedPoint,omitempty"`
}

type whiteboardFixedSegment struct {
	Start []float64 `json:"start"`
	End   []float64 `json:"end"`
	Index *int64    `json:"index"`
}

func validWhiteboardElement(record []byte) (string, bool) {
	var element whiteboardElement
	decoder := json.NewDecoder(bytes.NewReader(record))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&element) != nil || decoder.Decode(&struct{}{}) != io.EOF || element.ID == "" || len(element.ID) > 128 || !allowedWhiteboardElementType(element.Type) {
		return "", false
	}
	if !validWhiteboardElementSchema(record, element) {
		return "", false
	}
	if !finiteInRange(element.X, -100_000, 100_000) || !finiteInRange(element.Y, -100_000, 100_000) ||
		!finiteInRange(element.Width, 0, 100_000) || !finiteInRange(element.Height, 0, 100_000) || !finiteInRange(element.Angle, -1_000, 1_000) {
		return "", false
	}
	if element.Seed == nil || *element.Seed < 0 || *element.Seed > math.MaxInt32 ||
		element.Version == nil || *element.Version < 1 || *element.Version > 1_000_000_000 ||
		element.VersionNonce == nil || *element.VersionNonce < 0 || *element.VersionNonce > math.MaxInt32 ||
		element.Updated == nil || *element.Updated < 0 || *element.Updated > 1_000_000_000_000_000 ||
		!finiteInRange(element.Opacity, 0, 100) || !finiteInRange(element.StrokeWidth, 0, 1_000) || !finiteInRange(element.Roughness, 0, 100) ||
		element.IsDeleted == nil || *element.IsDeleted || element.Locked == nil {
		return "", false
	}
	if !validElementString(element.Index, 64) || !validElementString(element.StrokeColor, 64) || !validElementString(element.BackgroundColor, 64) ||
		!validElementString(element.FillStyle, 64) || !validElementString(element.StrokeStyle, 64) || element.GroupIDs == nil || len(*element.GroupIDs) > 100 {
		return "", false
	}
	for _, groupID := range *element.GroupIDs {
		if groupID == "" || len(groupID) > 128 {
			return "", false
		}
	}
	if !validWhiteboardRelationships(element) {
		return "", false
	}
	if element.Type == "arrow" || element.Type == "line" || element.Type == "freedraw" {
		if !validElementPoints(element.Points) {
			return "", false
		}
	}
	if element.Type == "freedraw" {
		if element.Pressures == nil || len(*element.Pressures) > 10_000 || element.SimulatePressure == nil {
			return "", false
		}
		for _, pressure := range *element.Pressures {
			if math.IsNaN(pressure) || math.IsInf(pressure, 0) || pressure < 0 || pressure > 1 {
				return "", false
			}
		}
	}
	if element.Type == "text" {
		if element.Text == nil || len(*element.Text) > 50_000 || element.OriginalText == nil || len(*element.OriginalText) > 50_000 ||
			!finiteInRange(element.FontSize, 1, 1_000) || element.FontFamily == nil || *element.FontFamily < 1 || *element.FontFamily > 100 ||
			!oneOfElementString(element.TextAlign, "left", "center", "right") || !oneOfElementString(element.VerticalAlign, "top", "middle", "bottom") ||
			!finiteInRange(element.LineHeight, 0.1, 10) || element.AutoResize == nil || !validOptionalElementID(element.ContainerID) {
			return "", false
		}
	}
	return element.ID, true
}

func validWhiteboardElementSchema(record []byte, element whiteboardElement) bool {
	var fields map[string]json.RawMessage
	if json.Unmarshal(record, &fields) != nil {
		return false
	}
	allowed := make(map[string]struct{})
	required := make(map[string]struct{})
	addWhiteboardFields(allowed, required,
		"id", "type", "x", "y", "strokeColor", "backgroundColor", "fillStyle", "strokeWidth", "strokeStyle", "roundness",
		"roughness", "opacity", "width", "height", "angle", "seed", "version", "versionNonce", "index", "isDeleted",
		"groupIds", "frameId", "boundElements", "updated", "link", "locked",
	)
	switch element.Type {
	case "rectangle", "diamond", "ellipse":
	case "line":
		addWhiteboardFields(allowed, required, "points", "lastCommittedPoint", "startBinding", "endBinding", "startArrowhead", "endArrowhead")
	case "arrow":
		addWhiteboardFields(allowed, required, "points", "lastCommittedPoint", "startBinding", "endBinding", "startArrowhead", "endArrowhead", "elbowed")
		addWhiteboardFields(allowed, nil, "fixedSegments", "startIsSpecial", "endIsSpecial")
		if element.Elbowed == nil {
			return false
		}
		if *element.Elbowed {
			addWhiteboardFields(nil, required, "fixedSegments", "startIsSpecial", "endIsSpecial")
		} else if hasAnyWhiteboardField(fields, "fixedSegments", "startIsSpecial", "endIsSpecial") {
			return false
		}
	case "freedraw":
		addWhiteboardFields(allowed, required, "points", "pressures", "simulatePressure", "lastCommittedPoint")
	case "text":
		addWhiteboardFields(allowed, required, "fontSize", "fontFamily", "text", "textAlign", "verticalAlign", "containerId", "originalText", "autoResize", "lineHeight")
	default:
		return false
	}
	for field := range fields {
		if _, ok := allowed[field]; !ok {
			return false
		}
	}
	for field := range required {
		if _, ok := fields[field]; !ok {
			return false
		}
	}
	return true
}

func addWhiteboardFields(allowed, required map[string]struct{}, fields ...string) {
	for _, field := range fields {
		if allowed != nil {
			allowed[field] = struct{}{}
		}
		if required != nil {
			required[field] = struct{}{}
		}
	}
}

func hasAnyWhiteboardField(fields map[string]json.RawMessage, names ...string) bool {
	for _, name := range names {
		if _, ok := fields[name]; ok {
			return true
		}
	}
	return false
}

func validWhiteboardElementReferences(elements []whiteboardElement, elementTypes map[string]string) bool {
	for _, element := range elements {
		if element.FrameID != nil {
			return false
		}
		if element.BoundElements != nil {
			for _, bound := range *element.BoundElements {
				if targetType, ok := elementTypes[bound.ID]; !ok || targetType != bound.Type {
					return false
				}
			}
		}
		if !validWhiteboardBindingReference(element.StartBinding, elementTypes) || !validWhiteboardBindingReference(element.EndBinding, elementTypes) {
			return false
		}
		if element.ContainerID != nil {
			targetType, ok := elementTypes[*element.ContainerID]
			if !ok || (targetType != "rectangle" && targetType != "diamond" && targetType != "ellipse" && targetType != "arrow") {
				return false
			}
		}
	}
	return true
}

func validWhiteboardBindingReference(binding *whiteboardBinding, elementTypes map[string]string) bool {
	if binding == nil {
		return true
	}
	targetType, ok := elementTypes[binding.ElementID]
	return ok && (targetType == "rectangle" || targetType == "diamond" || targetType == "ellipse" || targetType == "text")
}

func validWhiteboardRelationships(element whiteboardElement) bool {
	if element.Roundness != nil {
		if element.Roundness.Type < 1 || element.Roundness.Type > 3 ||
			(element.Roundness.Value != nil && !finiteInRange(element.Roundness.Value, 0, 100_000)) {
			return false
		}
	}
	if !validOptionalElementID(element.FrameID) || !validOptionalLink(element.Link) {
		return false
	}
	if element.BoundElements != nil {
		if len(*element.BoundElements) > 100 {
			return false
		}
		for _, bound := range *element.BoundElements {
			if bound.ID == "" || len(bound.ID) > 128 || (bound.Type != "arrow" && bound.Type != "text") {
				return false
			}
		}
	}
	requireFixedPoint := element.Type == "arrow" && element.Elbowed != nil && *element.Elbowed
	if !validWhiteboardBinding(element.StartBinding, requireFixedPoint) || !validWhiteboardBinding(element.EndBinding, requireFixedPoint) ||
		!validArrowhead(element.StartArrowhead) || !validArrowhead(element.EndArrowhead) || !validPoint(element.LastCommittedPoint, true) {
		return false
	}
	if element.FixedSegments != nil {
		if len(*element.FixedSegments) > 10_000 {
			return false
		}
		for _, segment := range *element.FixedSegments {
			if !validPoint(segment.Start, false) || !validPoint(segment.End, false) || segment.Index == nil || *segment.Index < 0 || *segment.Index > 10_000 {
				return false
			}
		}
	}
	return true
}

func validWhiteboardBinding(binding *whiteboardBinding, requireFixedPoint bool) bool {
	if binding == nil {
		return true
	}
	if binding.ElementID == "" || len(binding.ElementID) > 128 || !finiteInRange(binding.Focus, -10, 10) || !finiteInRange(binding.Gap, -100_000, 100_000) {
		return false
	}
	if !requireFixedPoint {
		return len(binding.FixedPoint) == 0
	}
	var point []float64
	return len(binding.FixedPoint) > 0 && string(binding.FixedPoint) != "null" && json.Unmarshal(binding.FixedPoint, &point) == nil && validPoint(point, false)
}

func validArrowhead(arrowhead *string) bool {
	if arrowhead == nil {
		return true
	}
	switch *arrowhead {
	case "arrow", "bar", "dot", "circle", "circle_outline", "triangle", "triangle_outline", "diamond", "diamond_outline", "crowfoot_one", "crowfoot_many", "crowfoot_one_or_many":
		return true
	default:
		return false
	}
}

func validOptionalElementID(value *string) bool {
	return value == nil || (*value != "" && len(*value) <= 128)
}

func validOptionalLink(value *string) bool {
	return value == nil || len(*value) <= 2_048
}

func validPoint(point []float64, optional bool) bool {
	if point == nil {
		return optional
	}
	return len(point) == 2 && !math.IsNaN(point[0]) && !math.IsInf(point[0], 0) && math.Abs(point[0]) <= 100_000 &&
		!math.IsNaN(point[1]) && !math.IsInf(point[1], 0) && math.Abs(point[1]) <= 100_000
}

func finiteInRange(value *float64, minimum, maximum float64) bool {
	return value != nil && !math.IsNaN(*value) && !math.IsInf(*value, 0) && *value >= minimum && *value <= maximum
}

func validElementString(value *string, maximum int) bool {
	return value != nil && *value != "" && len(*value) <= maximum
}

func oneOfElementString(value *string, allowed ...string) bool {
	if value == nil {
		return false
	}
	for _, candidate := range allowed {
		if *value == candidate {
			return true
		}
	}
	return false
}

func validElementPoints(points *[][]float64) bool {
	if points == nil || len(*points) == 0 || len(*points) > 10_000 {
		return false
	}
	for _, point := range *points {
		if len(point) != 2 || math.IsNaN(point[0]) || math.IsInf(point[0], 0) || math.Abs(point[0]) > 100_000 ||
			math.IsNaN(point[1]) || math.IsInf(point[1], 0) || math.Abs(point[1]) > 100_000 {
			return false
		}
	}
	return true
}

func allowedWhiteboardElementType(value string) bool {
	switch value {
	case "rectangle", "diamond", "ellipse", "arrow", "line", "freedraw", "text":
		return true
	default:
		return false
	}
}

func validCanvasDimension(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0) && value >= 100 && value <= 10_000
}
