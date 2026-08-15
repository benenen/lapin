package handler

import (
	"context"

	"github.com/cloudwego/hertz/pkg/app"
)

func (h *Handler) Health(ctx context.Context, c *app.RequestContext) {
	if err := h.db.Ping(ctx); err != nil {
		writeError(c, 503, "database_unavailable", "数据库暂不可用")
		return
	}
	writeData(c, 200, map[string]string{"status": "ok"})
}
