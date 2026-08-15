package handler

import (
	"encoding/json"
	"mime"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/common/utils"
)

const maxJSONBody = 1 << 20

func decodeJSON(c *app.RequestContext, target any) bool {
	mediaType, _, err := mime.ParseMediaType(string(c.Request.Header.ContentType()))
	if err != nil || mediaType != "application/json" {
		writeError(c, 415, "unsupported_media_type", "请求必须使用 application/json")
		return false
	}
	body := c.Request.Body()
	if len(body) == 0 || len(body) > maxJSONBody || json.Unmarshal(body, target) != nil {
		writeError(c, 400, "invalid_json", "请求内容不是有效的 JSON")
		return false
	}
	return true
}

func writeData(c *app.RequestContext, status int, data any) {
	c.JSON(status, utils.H{"data": data})
}

func writeError(c *app.RequestContext, status int, code, message string) {
	c.JSON(status, utils.H{"error": utils.H{"code": code, "message": message}})
}
