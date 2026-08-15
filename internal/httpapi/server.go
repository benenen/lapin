package httpapi

import (
	"net"

	hertzapp "github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/benenen/lapin/internal/httpapi/handler"
	"github.com/benenen/lapin/internal/identifier"
	"github.com/benenen/lapin/web"
)

type Options struct {
	HostPorts         string
	SecureCookies     bool
	HashIDSalt        string
	TrustedProxyCIDRs []*net.IPNet
}

type App struct {
	*server.Hertz
}

func New(db *pgxpool.Pool, options Options) *App {
	if options.HostPorts == "" {
		options.HostPorts = ":8080"
	}
	h := server.Default(server.WithHostPorts(options.HostPorts))
	h.SetClientIPFunc(hertzapp.ClientIPWithOption(hertzapp.ClientIPOptions{
		RemoteIPHeaders: []string{"X-Forwarded-For", "X-Real-IP"},
		TrustedCIDRs:    options.TrustedProxyCIDRs,
	}))
	ids, err := identifier.New(options.HashIDSalt)
	if err != nil {
		panic(err)
	}
	app := &App{Hertz: h}
	handlers := handler.New(db, ids, handler.Options{SecureCookies: options.SecureCookies, TrustedProxyCIDRs: options.TrustedProxyCIDRs})
	app.routes(handlers)
	return app
}

func (a *App) routes(h *handler.Handler) {
	a.Use(h.SecurityHeaders())
	a.GET("/healthz", h.Health)

	api := a.Group("/api/v1")
	api.Use(h.APIRateLimit())
	api.POST("/auth/register", h.AuthRateLimit(), h.Register)
	api.POST("/auth/login", h.AuthRateLimit(), h.Login)
	api.POST("/auth/logout", h.RequireSession(true), h.Logout)
	api.GET("/me", h.RequireSession(false), h.Me)

	protected := api.Group("")
	protected.Use(h.RequireSession(false))
	protected.GET("/access-tokens", h.ListAccessTokens)
	protected.POST("/access-tokens", h.RequireCSRF(), h.CreateAccessToken)
	protected.DELETE("/access-tokens/:id", h.RequireCSRF(), h.RevokeAccessToken)
	protected.GET("/subjects", h.ListSubjects)
	protected.POST("/subjects", h.RequireCSRF(), h.CreateSubject)
	protected.GET("/subjects/:id", h.GetSubject)
	protected.POST("/subjects/:id/chapters", h.RequireCSRF(), h.CreateChapter)
	protected.PUT("/subjects/:id/tags", h.RequireCSRF(), h.ReplaceTags)
	protected.GET("/chapters/:id/annotations", h.ListAnnotations)
	protected.POST("/chapters/:id/annotations", h.RequireCSRF(), h.CreateAnnotation)
	protected.GET("/chapters/:id/whiteboards", h.ListWhiteboards)
	protected.PUT("/chapters/:id/whiteboard", h.RequireCSRF(), h.SaveWhiteboard)
	protected.GET("/chapters/:id/comments", h.ListComments)
	protected.POST("/chapters/:id/comments", h.RequireCSRF(), h.CreateComment)

	openapi := a.Group("/openapi/v1")
	openapi.Use(h.OpenAPIRateLimit())
	openapi.POST("/subjects/import", h.RequireAccessToken(), h.ImportSubject)

	a.NoRoute(web.Handler())
}
