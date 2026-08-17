package handler

import (
	"context"
	"strings"
	"time"

	"github.com/cloudwego/hertz/pkg/app"

	"github.com/benenen/lapin/internal/auth"
	"github.com/benenen/lapin/internal/database"
)

type accessTokenView struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	Prefix     string     `json:"prefix"`
	LastUsedAt *time.Time `json:"last_used_at"`
	ExpiresAt  time.Time  `json:"expires_at"`
	CreatedAt  time.Time  `json:"created_at"`
}

func (h *Handler) CreateAccessToken(ctx context.Context, c *app.RequestContext) {
	var request struct {
		Name string `json:"name"`
	}
	if !decodeJSON(c, &request) {
		return
	}
	request.Name = strings.TrimSpace(request.Name)
	if request.Name == "" || len(request.Name) > 80 {
		writeError(c, 400, errorCodeInvalidInput, "Token 名称不能为空且最多 80 字")
		return
	}
	userID := currentUser(c).ID
	tx, err := h.db.Begin(ctx)
	if err != nil {
		writeError(c, 500, errorCodeInternal, "创建 Token 失败")
		return
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := database.LockUserForUpdate(ctx, tx, userID); err != nil {
		writeError(c, 500, errorCodeInternal, "创建 Token 失败")
		return
	}
	count, err := database.CountActiveAccessTokens(ctx, tx, userID)
	if err != nil {
		writeError(c, 500, errorCodeInternal, "创建 Token 失败")
		return
	}
	if count >= 10 {
		writeError(c, 409, errorCodeTokenLimit, "最多保留 10 个有效 Token")
		return
	}
	raw, hash, err := auth.NewOpaqueToken("lpn_")
	if err != nil {
		writeError(c, 500, errorCodeInternal, "服务暂时不可用")
		return
	}
	prefix := raw
	if len(prefix) > 12 {
		prefix = prefix[:12]
	}
	createdAt := time.Now().UTC()
	expiresAt := createdAt.Add(90 * 24 * time.Hour)
	tokenID, err := database.InsertAccessToken(ctx, tx, database.AccessToken{
		UserID: userID, Name: request.Name, Prefix: prefix, TokenHash: hash,
		ExpiresAt: expiresAt, CreatedAt: createdAt,
	})
	if err != nil {
		writeError(c, 500, errorCodeInternal, "创建 Token 失败")
		return
	}
	if err := tx.Commit(ctx); err != nil {
		writeError(c, 500, errorCodeInternal, "创建 Token 失败")
		return
	}
	view := accessTokenView{ID: h.ids.Encode(tokenID), Name: request.Name, Prefix: prefix, ExpiresAt: expiresAt, CreatedAt: createdAt}
	writeData(c, 201, map[string]any{"access_token": raw, "token": view})
}

func (h *Handler) ListAccessTokens(ctx context.Context, c *app.RequestContext) {
	tokens, err := database.ListActiveAccessTokens(ctx, h.db, currentUser(c).ID)
	if err != nil {
		writeError(c, 500, errorCodeInternal, "读取 Token 失败")
		return
	}
	views := make([]accessTokenView, 0, len(tokens))
	for _, token := range tokens {
		views = append(views, accessTokenView{
			ID: h.ids.Encode(token.ID), Name: token.Name, Prefix: token.Prefix,
			LastUsedAt: token.LastUsedAt, ExpiresAt: token.ExpiresAt, CreatedAt: token.CreatedAt,
		})
	}
	writeData(c, 200, views)
}

func (h *Handler) RevokeAccessToken(ctx context.Context, c *app.RequestContext) {
	id, err := h.ids.Decode(c.Param("id"))
	if err != nil {
		writeError(c, 400, errorCodeInvalidID, "Token ID 无效")
		return
	}
	revoked, err := database.RevokeAccessToken(ctx, h.db, id, currentUser(c).ID)
	if err != nil || !revoked {
		writeError(c, 404, errorCodeNotFound, "Token 不存在")
		return
	}
	writeData(c, 200, map[string]bool{"revoked": true})
}
