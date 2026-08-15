package web

import (
	"context"
	"embed"
	"io/fs"
	"mime"
	"path"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/common/utils"
)

//go:embed dist/*
var embedded embed.FS

var assets = func() fs.FS {
	sub, err := fs.Sub(embedded, "dist")
	if err != nil {
		panic(err)
	}
	return sub
}()

// Handler serves the embedded Vite build and falls back to index.html for SPA routes.
func Handler() app.HandlerFunc {
	return func(_ context.Context, c *app.RequestContext) {
		requestPath := strings.TrimPrefix(string(c.Path()), "/")
		if strings.HasPrefix(requestPath, "api/") || strings.HasPrefix(requestPath, "openapi/") || requestPath == "healthz" {
			c.JSON(404, utils.H{"error": utils.H{"code": "not_found", "message": "接口不存在"}})
			return
		}
		if requestPath == "" {
			requestPath = "index.html"
		}
		requestPath = path.Clean(requestPath)
		if strings.HasPrefix(requestPath, "../") {
			c.JSON(404, utils.H{"error": utils.H{"code": "not_found", "message": "页面不存在"}})
			return
		}
		data, err := fs.ReadFile(assets, requestPath)
		if err != nil {
			requestPath = "index.html"
			data, err = fs.ReadFile(assets, requestPath)
		}
		if err != nil {
			c.JSON(500, utils.H{"error": utils.H{"code": "web_unavailable", "message": "Web 资源不可用"}})
			return
		}
		contentType := mime.TypeByExtension(path.Ext(requestPath))
		if contentType == "" {
			contentType = "application/octet-stream"
		}
		if requestPath == "index.html" {
			c.Response.Header.Set("Cache-Control", "no-cache")
		} else {
			c.Response.Header.Set("Cache-Control", "public, max-age=31536000, immutable")
		}
		c.Data(200, contentType, data)
	}
}
