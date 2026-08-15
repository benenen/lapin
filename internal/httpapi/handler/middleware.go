package handler

import (
	"context"
	"crypto/subtle"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/cloudwego/hertz/pkg/app"

	"github.com/benenen/lapin/internal/auth"
	"github.com/benenen/lapin/internal/database"
)

const currentUserKey = "current_user"

type sessionUser struct {
	ID        int64
	Email     string
	Name      string
	AvatarURL string
	Roles     []string
	CreatedAt time.Time
}

func (h *Handler) SecurityHeaders() app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		c.Response.Header.Set("X-Content-Type-Options", "nosniff")
		c.Response.Header.Set("X-Frame-Options", "DENY")
		c.Response.Header.Set("Referrer-Policy", "no-referrer")
		c.Response.Header.Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data: blob: https:; connect-src 'self'; font-src 'self' data:; frame-ancestors 'none'")
		c.Next(ctx)
	}
}

func (h *Handler) AuthRateLimit() app.HandlerFunc {
	return h.rateLimit(h.authLimiter)
}

func (h *Handler) APIRateLimit() app.HandlerFunc {
	return h.rateLimit(h.apiLimiter)
}

func (h *Handler) OpenAPIRateLimit() app.HandlerFunc {
	return h.rateLimit(h.openAPILimiter)
}

func (h *Handler) rateLimit(limiter *fixedWindowLimiter) app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		allowed, retryAfter := limiter.allow(remoteAddressKey(c, h.clientIP), time.Now())
		if !allowed {
			seconds := int(retryAfter.Round(time.Second) / time.Second)
			if seconds < 1 {
				seconds = 1
			}
			c.Response.Header.Set("Retry-After", strconv.Itoa(seconds))
			writeError(c, 429, "rate_limited", "请求过于频繁，请稍后重试")
			c.Abort()
			return
		}
		c.Next(ctx)
	}
}

func remoteAddressKey(c *app.RequestContext, clientIP app.ClientIP) string {
	if address := net.ParseIP(strings.TrimSpace(clientIP(c))); address != nil {
		return address.String()
	}
	address := c.RemoteAddr().String()
	if host, _, err := net.SplitHostPort(address); err == nil && host != "" {
		return host
	}
	if address == "" {
		return "unknown"
	}
	return address
}

func (h *Handler) RequireSession(checkCSRF bool) app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		raw := string(c.Cookie("lapin_session"))
		if raw == "" {
			writeError(c, 401, "unauthenticated", "请先登录")
			c.Abort()
			return
		}
		session, err := database.FindActiveSessionByHash(ctx, h.db, auth.HashToken(raw))
		if err != nil {
			writeError(c, 401, "unauthenticated", "登录已失效")
			c.Abort()
			return
		}
		userRecord, err := database.FindUserByID(ctx, h.db, session.UserID)
		if err != nil {
			writeError(c, 401, "unauthenticated", "登录已失效")
			c.Abort()
			return
		}
		roles, err := database.ListRoleCodesForUser(ctx, h.db, userRecord.ID)
		if err != nil {
			writeError(c, 401, "unauthenticated", "登录已失效")
			c.Abort()
			return
		}
		if checkCSRF && !validCSRF(c, session.CSRFHash) {
			writeError(c, 403, "invalid_csrf", "请求校验失败，请刷新页面后重试")
			c.Abort()
			return
		}
		user := sessionUser{ID: userRecord.ID, Email: userRecord.Email, Name: userRecord.DisplayName, AvatarURL: userRecord.AvatarURL, Roles: roles, CreatedAt: userRecord.CreatedAt}
		c.Set(currentUserKey, user)
		c.Next(ctx)
	}
}

func (h *Handler) RequireCSRF() app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		raw := string(c.Cookie("lapin_session"))
		session, err := database.FindActiveSessionByHash(ctx, h.db, auth.HashToken(raw))
		if raw == "" || err != nil || !validCSRF(c, session.CSRFHash) {
			writeError(c, 403, "invalid_csrf", "请求校验失败，请刷新页面后重试")
			c.Abort()
			return
		}
		c.Next(ctx)
	}
}

func validCSRF(c *app.RequestContext, expectedHash []byte) bool {
	header := string(c.Request.Header.Peek("X-CSRF-Token"))
	cookie := string(c.Cookie("lapin_csrf"))
	return header != "" && header == cookie && subtle.ConstantTimeCompare(auth.HashToken(header), expectedHash) == 1
}

func (h *Handler) RequireAccessToken() app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		header := string(c.Request.Header.Peek("Authorization"))
		raw, ok := strings.CutPrefix(header, "Bearer ")
		if !ok || !strings.HasPrefix(raw, "lpn_") {
			writeError(c, 401, "invalid_access_token", "Access Token 无效")
			c.Abort()
			return
		}
		token, err := database.FindActiveAccessTokenByHash(ctx, h.db, auth.HashToken(raw))
		if err != nil {
			writeError(c, 401, "invalid_access_token", "Access Token 无效")
			c.Abort()
			return
		}
		userRecord, err := database.FindUserByID(ctx, h.db, token.UserID)
		if err != nil {
			writeError(c, 401, "invalid_access_token", "Access Token 无效")
			c.Abort()
			return
		}
		roles, err := database.ListRoleCodesForUser(ctx, h.db, userRecord.ID)
		if err != nil {
			writeError(c, 401, "invalid_access_token", "Access Token 无效")
			c.Abort()
			return
		}
		_ = database.TouchAccessToken(ctx, h.db, token.ID)
		user := sessionUser{ID: userRecord.ID, Email: userRecord.Email, Name: userRecord.DisplayName, AvatarURL: userRecord.AvatarURL, Roles: roles, CreatedAt: userRecord.CreatedAt}
		c.Set(currentUserKey, user)
		c.Next(ctx)
	}
}

func currentUser(c *app.RequestContext) sessionUser {
	value, _ := c.Get(currentUserKey)
	user, _ := value.(sessionUser)
	return user
}
