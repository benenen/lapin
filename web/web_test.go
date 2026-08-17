package web

import (
	"bytes"
	"regexp"
	"strings"
	"testing"

	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/ut"
	"github.com/cloudwego/hertz/pkg/protocol"
)

func TestEmbeddedAssetsAndFallback(t *testing.T) {
	h := server.Default()
	h.NoRoute(Handler())

	root := performRequest(h, "/")
	if root.StatusCode() != 200 || !strings.Contains(string(root.Body()), `<div id="app"></div>`) {
		t.Fatalf("root response = %d, %s", root.StatusCode(), root.Body())
	}
	asset := regexp.MustCompile(`src="(/assets/[^"]+\.js)"`).FindSubmatch(root.Body())
	if len(asset) != 2 {
		t.Fatalf("asset not found in %s", root.Body())
	}
	if response := performRequest(h, string(asset[1])); response.StatusCode() != 200 {
		t.Fatalf("asset response = %d", response.StatusCode())
	}
	if response := performRequest(h, "/subjects/example-hashid"); response.StatusCode() != 200 {
		t.Fatalf("SPA fallback response = %d", response.StatusCode())
	}
	if response := performRequest(h, "/api/missing"); response.StatusCode() != 404 {
		t.Fatalf("API fallback response = %d", response.StatusCode())
	}
}

func performRequest(h *server.Hertz, path string) *protocol.Response {
	body := &ut.Body{Body: bytes.NewReader(nil), Len: 0}
	return ut.PerformRequest(h.Engine, "GET", path, body).Result()
}
