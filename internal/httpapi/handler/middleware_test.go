package handler

import (
	"testing"

	"github.com/cloudwego/hertz/pkg/app"
)

func TestRemoteAddressKeyFallsBackToConnectionAddress(t *testing.T) {
	ctx := app.NewContext(0)
	key := remoteAddressKey(ctx, func(*app.RequestContext) string { return "not-an-ip" })
	if key != "0.0.0.0" {
		t.Fatalf("remote address key = %q, want connection address", key)
	}
}
