package handler

import (
	"context"
	"encoding/hex"
	"errors"
	"net/mail"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/benenen/lapin/internal/auth"
	"github.com/benenen/lapin/internal/database"
)

type userView struct {
	ID        string    `json:"id"`
	Email     string    `json:"email"`
	Name      string    `json:"name"`
	AvatarURL string    `json:"avatar_url"`
	Roles     []string  `json:"roles"`
	CreatedAt time.Time `json:"created_at"`
}

func (h *Handler) Register(ctx context.Context, c *app.RequestContext) {
	if !validAuthRequestOrigin(c) {
		writeError(c, 403, "invalid_origin", "请求来源无效")
		return
	}
	var request struct {
		Email     string `json:"email"`
		Name      string `json:"name"`
		AvatarURL string `json:"avatar_url"`
		Password  string `json:"password"`
	}
	if !decodeJSON(c, &request) {
		return
	}
	request.Email = strings.ToLower(strings.TrimSpace(request.Email))
	request.Name = strings.TrimSpace(request.Name)
	request.AvatarURL = strings.TrimSpace(request.AvatarURL)
	if !validEmail(request.Email) || len(request.Name) < 1 || len(request.Name) > 80 || !validAvatarURL(request.AvatarURL) || len(request.Password) < 8 || len(request.Password) > 128 {
		writeError(c, 400, "invalid_input", "请检查邮箱、昵称和密码（密码至少 8 位）")
		return
	}
	if !h.allowAccountRequest(c, accountLimiterKey("register", request.Email)) {
		return
	}

	passwordHash, err := h.hashPassword(ctx, request.Password)
	if err != nil {
		writeError(c, 503, "service_busy", "密码服务繁忙，请稍后重试")
		return
	}
	createdAt := time.Now().UTC()
	tx, err := h.db.Begin(ctx)
	if err != nil {
		writeError(c, 500, "internal_error", "服务暂时不可用")
		return
	}
	defer func() { _ = tx.Rollback(ctx) }()
	userID, err := database.InsertUser(ctx, tx, database.User{
		Email: request.Email, DisplayName: request.Name, AvatarURL: request.AvatarURL,
		PasswordHash: passwordHash, CreatedAt: createdAt,
	})
	if err != nil {
		var databaseError *pgconn.PgError
		if errors.As(err, &databaseError) && databaseError.Code == "23505" {
			writeError(c, 409, "email_exists", "该邮箱已注册")
		} else {
			writeError(c, 500, "internal_error", "服务暂时不可用")
		}
		return
	}
	if err := database.AssignRoleByCode(ctx, tx, userID, "learner"); err != nil {
		writeError(c, 500, "internal_error", "服务暂时不可用")
		return
	}
	if err := tx.Commit(ctx); err != nil {
		writeError(c, 500, "internal_error", "服务暂时不可用")
		return
	}

	csrfToken, err := h.createSession(ctx, c, userID)
	if err != nil {
		writeError(c, 500, "internal_error", "服务暂时不可用")
		return
	}
	user := userView{ID: h.ids.Encode(userID), Email: request.Email, Name: request.Name, AvatarURL: request.AvatarURL, Roles: []string{"learner"}, CreatedAt: createdAt}
	writeData(c, 201, map[string]any{"user": user, "csrf_token": csrfToken})
}

func (h *Handler) Login(ctx context.Context, c *app.RequestContext) {
	if !validAuthRequestOrigin(c) {
		writeError(c, 403, "invalid_origin", "请求来源无效")
		return
	}
	var request struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if !decodeJSON(c, &request) {
		return
	}
	request.Email = strings.ToLower(strings.TrimSpace(request.Email))
	if !validEmail(request.Email) || request.Password == "" || len(request.Password) > 128 {
		writeError(c, 401, "invalid_credentials", "邮箱或密码错误")
		return
	}
	if !h.allowAccountRequest(c, accountLimiterKey("login", request.Email)) {
		return
	}
	user, err := database.FindUserByEmail(ctx, h.db, request.Email)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		writeError(c, 500, "internal_error", "服务暂时不可用")
		return
	}
	encodedHash := dummyHash
	if err == nil {
		encodedHash = user.PasswordHash
	}
	verified, verifyErr := h.verifyPassword(ctx, encodedHash, request.Password)
	if verifyErr != nil {
		writeError(c, 503, "service_busy", "密码服务繁忙，请稍后重试")
		return
	}
	if err != nil || !verified {
		writeError(c, 401, "invalid_credentials", "邮箱或密码错误")
		return
	}
	roles, err := database.ListRoleCodesForUser(ctx, h.db, user.ID)
	if err != nil {
		writeError(c, 500, "internal_error", "服务暂时不可用")
		return
	}
	csrfToken, err := h.createSession(ctx, c, user.ID)
	if err != nil {
		writeError(c, 500, "internal_error", "服务暂时不可用")
		return
	}
	writeData(c, 200, map[string]any{
		"user":       userView{ID: h.ids.Encode(user.ID), Email: user.Email, Name: user.DisplayName, AvatarURL: user.AvatarURL, Roles: roles, CreatedAt: user.CreatedAt},
		"csrf_token": csrfToken,
	})
}

func (h *Handler) Logout(ctx context.Context, c *app.RequestContext) {
	raw := string(c.Cookie("lapin_session"))
	_ = database.DeleteSessionByHash(ctx, h.db, auth.HashToken(raw))
	c.SetCookie("lapin_session", "", -1, "/", "", protocol.CookieSameSiteLaxMode, h.options.SecureCookies, true)
	c.SetCookie("lapin_csrf", "", -1, "/", "", protocol.CookieSameSiteLaxMode, h.options.SecureCookies, false)
	writeData(c, 200, map[string]bool{"logged_out": true})
}

func (h *Handler) Me(_ context.Context, c *app.RequestContext) {
	user := currentUser(c)
	writeData(c, 200, userView{ID: h.ids.Encode(user.ID), Email: user.Email, Name: user.Name, AvatarURL: user.AvatarURL, Roles: user.Roles, CreatedAt: user.CreatedAt})
}

func (h *Handler) createSession(ctx context.Context, c *app.RequestContext, userID int64) (string, error) {
	sessionToken, sessionHash, err := auth.NewOpaqueToken("ses_")
	if err != nil {
		return "", err
	}
	csrfToken, csrfHash, err := auth.NewOpaqueToken("csrf_")
	if err != nil {
		return "", err
	}
	expiresAt := time.Now().UTC().Add(7 * 24 * time.Hour)
	if err := database.InsertSession(ctx, h.db, database.Session{UserID: userID, TokenHash: sessionHash, CSRFHash: csrfHash, ExpiresAt: expiresAt}); err != nil {
		return "", err
	}
	c.SetCookie("lapin_session", sessionToken, 7*24*60*60, "/", "", protocol.CookieSameSiteLaxMode, h.options.SecureCookies, true)
	c.SetCookie("lapin_csrf", csrfToken, 7*24*60*60, "/", "", protocol.CookieSameSiteLaxMode, h.options.SecureCookies, false)
	return csrfToken, nil
}

func (h *Handler) allowAccountRequest(c *app.RequestContext, key string) bool {
	allowed, retryAfter := h.accountLimiter.allow(key, time.Now())
	if allowed {
		return true
	}
	seconds := int(retryAfter.Round(time.Second) / time.Second)
	if seconds < 1 {
		seconds = 1
	}
	c.Response.Header.Set("Retry-After", strconv.Itoa(seconds))
	writeError(c, 429, "rate_limited", "请求过于频繁，请稍后重试")
	return false
}

func (h *Handler) hashPassword(ctx context.Context, password string) (string, error) {
	if err := h.acquirePasswordWork(ctx); err != nil {
		return "", err
	}
	defer func() { <-h.passwordWork }()
	return auth.HashPassword(password)
}

func (h *Handler) verifyPassword(ctx context.Context, encodedHash, password string) (bool, error) {
	if err := h.acquirePasswordWork(ctx); err != nil {
		return false, err
	}
	defer func() { <-h.passwordWork }()
	return auth.VerifyPassword(encodedHash, password), nil
}

func (h *Handler) acquirePasswordWork(ctx context.Context) error {
	select {
	case h.passwordWork <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	default:
		return errors.New("password work queue is full")
	}
}

func accountLimiterKey(operation, email string) string {
	return operation + ":" + hex.EncodeToString(auth.HashToken(email))
}

func validEmail(email string) bool {
	address, err := mail.ParseAddress(email)
	return err == nil && strings.EqualFold(address.Address, email) && len(email) <= 254
}

func validAvatarURL(value string) bool {
	if value == "" {
		return true
	}
	if len(value) > 2048 {
		return false
	}
	parsed, err := url.ParseRequestURI(value)
	return err == nil && parsed.Scheme == "https" && parsed.Host != ""
}

func validAuthRequestOrigin(c *app.RequestContext) bool {
	origin := string(c.Request.Header.Peek("Origin"))
	if origin == "" {
		return true
	}
	parsed, err := url.ParseRequestURI(origin)
	return err == nil && (parsed.Scheme == "http" || parsed.Scheme == "https") && parsed.Host != "" && parsed.Host == string(c.Host())
}
