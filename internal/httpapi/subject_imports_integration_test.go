package httpapi_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	pngimage "image/png"
	"mime/multipart"
	"os"
	"path/filepath"
	"testing"

	"github.com/cloudwego/hertz/pkg/common/ut"
	"github.com/cloudwego/hertz/pkg/protocol"

	"github.com/benenen/lapin/internal/database"
	"github.com/benenen/lapin/internal/httpapi"
)

func TestAccessTokenCanStageAndAtomicallyCommitCourseWithAsset(t *testing.T) {
	h, assetDir := newTestAppWithAssetDir(t)
	register := performJSON(h, "POST", "/api/v1/auth/register", `{"email":"importer@example.com","name":"Importer","password":"correct horse battery staple"}`)
	assertStatus(t, register, 201)
	cookies, csrf := authState(t, register)
	tokenResponse := performJSON(h, "POST", "/api/v1/access-tokens", `{"name":"pdf-import"}`,
		ut.Header{Key: "Cookie", Value: cookies}, ut.Header{Key: "X-CSRF-Token", Value: csrf})
	assertStatus(t, tokenResponse, 201)
	var tokenPayload struct {
		Data struct {
			AccessToken string `json:"access_token"`
		} `json:"data"`
	}
	if err := json.Unmarshal(tokenResponse.Body(), &tokenPayload); err != nil {
		t.Fatal(err)
	}
	accessHeader := ut.Header{Key: "Authorization", Value: "Bearer " + tokenPayload.Data.AccessToken}

	begin := performJSON(h, "POST", "/openapi/v1/subject-imports", `{
		"idempotency_key":"fixture-v1","external_id":"fixture-course","title":"Fixture Course",
		"description":"atomic import","tags":["PDF"],"expected_chapters":2,"expected_assets":17
	}`, accessHeader)
	assertStatus(t, begin, 201)
	var beginPayload struct {
		Data struct {
			ID     string `json:"id"`
			Status string `json:"status"`
		} `json:"data"`
	}
	if err := json.Unmarshal(begin.Body(), &beginPayload); err != nil {
		t.Fatal(err)
	}
	if beginPayload.Data.ID == "" || beginPayload.Data.Status != "draft" {
		t.Fatalf("begin response = %s", begin.Body())
	}
	importID := beginPayload.Data.ID

	png, err := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=")
	if err != nil {
		t.Fatal(err)
	}
	invalidBatch := performMultipartAssetBatch(t, h, "/openapi/v1/subject-imports/"+importID+"/assets", []string{"valid-before-error", "invalid"}, []namedAsset{
		{filename: "valid.png", content: png},
		{filename: "invalid.png", content: []byte("not an image")},
	}, accessHeader)
	assertStatus(t, invalidBatch, 400)
	if storedAssetFileCount(t, assetDir) != 0 {
		t.Fatalf("failed batch left a published blob in %s", assetDir)
	}
	firstKeys := make([]string, 0, 16)
	firstFiles := make([]namedAsset, 0, 16)
	for index := 1; index <= 16; index++ {
		firstKeys = append(firstKeys, fmt.Sprintf("figure-%d", index))
		firstFiles = append(firstFiles, namedAsset{filename: fmt.Sprintf("pixel-%d.png", index), content: png})
	}
	asset := performMultipartAssetBatch(t, h, "/openapi/v1/subject-imports/"+importID+"/assets", firstKeys, firstFiles, accessHeader)
	assertStatus(t, asset, 201)
	var assetPayload struct {
		Data struct {
			Assets []struct {
				Key string `json:"key"`
				ID  string `json:"id"`
				URL string `json:"url"`
			} `json:"assets"`
		} `json:"data"`
	}
	if err := json.Unmarshal(asset.Body(), &assetPayload); err != nil {
		t.Fatal(err)
	}
	if len(assetPayload.Data.Assets) != 16 || assetPayload.Data.Assets[0].ID == "" || assetPayload.Data.Assets[0].URL == "" || assetPayload.Data.Assets[15].Key != "figure-16" {
		t.Fatalf("asset response = %s", asset.Body())
	}
	lastAsset := performMultipartAssetBatch(t, h, "/openapi/v1/subject-imports/"+importID+"/assets", []string{"figure-17"}, []namedAsset{{filename: "pixel-17.png", content: png}}, accessHeader)
	assertStatus(t, lastAsset, 201)
	var lastAssetPayload struct {
		Data struct {
			Assets []struct {
				URL string `json:"url"`
			} `json:"assets"`
		} `json:"data"`
	}
	if err := json.Unmarshal(lastAsset.Body(), &lastAssetPayload); err != nil || len(lastAssetPayload.Data.Assets) != 1 {
		t.Fatalf("last asset response = %s, err = %v", lastAsset.Body(), err)
	}

	chapters := performJSON(h, "POST", "/openapi/v1/subject-imports/"+importID+"/chapters", `{
		"batch_key":"chapters-1",
		"chapters":[
			{"external_id":"root","parent_external_id":null,"position":0,"title":"Root","content":""},
			{"external_id":"child","parent_external_id":"root","position":1,"title":"Child","content":"第一段。\n\n第二段。\n\n![图1](`+assetPayload.Data.Assets[0].URL+`)\n\n![图17](`+lastAssetPayload.Data.Assets[0].URL+`)"}
		]
	}`, accessHeader)
	assertStatus(t, chapters, 200)

	listBeforeCommit := performJSON(h, "GET", "/api/v1/subjects", `{}`, ut.Header{Key: "Cookie", Value: cookies})
	assertStatus(t, listBeforeCommit, 200)
	if string(listBeforeCommit.Body()) != `{"data":[]}` {
		t.Fatalf("draft course became visible: %s", listBeforeCommit.Body())
	}

	status := performJSON(h, "GET", "/openapi/v1/subject-imports/"+importID, `{}`, accessHeader)
	assertStatus(t, status, 200)
	if !containsJSONCounts(status.Body(), 2, 17) {
		t.Fatalf("status response = %s", status.Body())
	}

	commit := performJSON(h, "POST", "/openapi/v1/subject-imports/"+importID+"/commit", `{}`, accessHeader)
	assertStatus(t, commit, 200)
	var commitPayload struct {
		Data struct {
			Subject struct {
				ID         string `json:"id"`
				ExternalID string `json:"external_id"`
				Title      string `json:"title"`
			} `json:"subject"`
		} `json:"data"`
	}
	if err := json.Unmarshal(commit.Body(), &commitPayload); err != nil {
		t.Fatal(err)
	}
	if commitPayload.Data.Subject.ID == "" || commitPayload.Data.Subject.ExternalID != "fixture-course" || commitPayload.Data.Subject.Title != "Fixture Course" || bytes.Contains(commit.Body(), []byte(`"chapters"`)) || len(commit.Body()) > 16<<10 {
		t.Fatalf("commit response = %s", commit.Body())
	}
	assertStatus(t, performJSON(h, "POST", "/openapi/v1/subject-imports/"+importID+"/commit", `{}`, accessHeader), 200)
	manualView := performJSON(h, "GET", "/api/v1/subjects/"+commitPayload.Data.Subject.ID, `{}`, ut.Header{Key: "Cookie", Value: cookies})
	assertStatus(t, manualView, 200)
	var manualViewPayload struct {
		Data struct {
			Chapters []struct {
				ID         string `json:"id"`
				ExternalID string `json:"external_id"`
				Content    string `json:"content"`
			} `json:"chapters"`
		} `json:"data"`
	}
	if err := json.Unmarshal(manualView.Body(), &manualViewPayload); err != nil || len(manualViewPayload.Data.Chapters) != 2 {
		t.Fatalf("manual edit view = %s, err = %v", manualView.Body(), err)
	}
	rootChapterID := manualViewPayload.Data.Chapters[0].ID
	assertStatus(t, performJSON(h, "POST", "/api/v1/chapters/"+rootChapterID+"/update", `{"title":"Root Manual","content":"manual edit"}`,
		ut.Header{Key: "Cookie", Value: cookies}, ut.Header{Key: "X-CSRF-Token", Value: csrf}), 200)
	conflictingReuse := performJSON(h, "POST", "/openapi/v1/subject-imports", `{
		"idempotency_key":"fixture-v1","external_id":"fixture-course","title":"Changed title",
		"description":"atomic import","tags":["PDF"],"expected_chapters":2,"expected_assets":17
	}`, accessHeader)
	assertStatus(t, conflictingReuse, 409)
	stillCommitted := performJSON(h, "GET", "/openapi/v1/subject-imports/"+importID, `{}`, accessHeader)
	assertStatus(t, stillCommitted, 200)
	if !bytes.Contains(stillCommitted.Body(), []byte(`"status":"committed"`)) {
		t.Fatalf("conflicting idempotency request changed state: %s", stillCommitted.Body())
	}
	reopened := performJSON(h, "POST", "/openapi/v1/subject-imports", `{
		"idempotency_key":"fixture-v1","external_id":"fixture-course","title":"Fixture Course",
		"description":"atomic import","tags":["PDF"],"expected_chapters":2,"expected_assets":17
	}`, accessHeader)
	assertStatus(t, reopened, 200)
	if !bytes.Contains(reopened.Body(), []byte(`"status":"draft"`)) {
		t.Fatalf("committed import was not reopened: %s", reopened.Body())
	}
	conflictingDraft := performJSON(h, "POST", "/openapi/v1/subject-imports", `{
		"idempotency_key":"fixture-other","external_id":"fixture-course","title":"Fixture Course",
		"description":"atomic import","tags":["PDF"],"expected_chapters":2,"expected_assets":17
	}`, accessHeader)
	assertStatus(t, conflictingDraft, 409)
	assertStatus(t, performJSON(h, "POST", "/openapi/v1/subject-imports/"+importID+"/commit", `{}`, accessHeader), 200)
	restored := performJSON(h, "GET", "/api/v1/subjects/"+commitPayload.Data.Subject.ID, `{}`, ut.Header{Key: "Cookie", Value: cookies})
	assertStatus(t, restored, 200)
	if err := json.Unmarshal(restored.Body(), &manualViewPayload); err != nil || manualViewPayload.Data.Chapters[0].Content != "" || manualViewPayload.Data.Chapters[0].ID != rootChapterID {
		t.Fatalf("same bundle did not restore stable chapter = %s, err = %v", restored.Body(), err)
	}
	abortBegin := performJSON(h, "POST", "/openapi/v1/subject-imports", `{
		"idempotency_key":"abort-one","external_id":"abort-course","title":"Abort Course",
		"description":"","tags":[],"expected_chapters":1,"expected_assets":2
	}`, accessHeader)
	assertStatus(t, abortBegin, 201)
	var abortPayload struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(abortBegin.Body(), &abortPayload); err != nil || abortPayload.Data.ID == "" {
		t.Fatalf("abort begin = %s, err = %v", abortBegin.Body(), err)
	}
	var abortImage bytes.Buffer
	pixel := image.NewRGBA(image.Rect(0, 0, 1, 1))
	pixel.Set(0, 0, color.RGBA{R: 17, G: 34, B: 51, A: 255})
	if err := pngimage.Encode(&abortImage, pixel); err != nil {
		t.Fatal(err)
	}
	beforeAbortFiles := storedAssetFileCount(t, assetDir)
	directShared := performMultipart(t, h, "/api/v1/assets", "shared.png", abortImage.Bytes(),
		ut.Header{Key: "Cookie", Value: cookies}, ut.Header{Key: "X-CSRF-Token", Value: csrf})
	assertStatus(t, directShared, 201)
	var directSharedPayload struct {
		Data struct {
			URL string `json:"url"`
		} `json:"data"`
	}
	if err := json.Unmarshal(directShared.Body(), &directSharedPayload); err != nil || directSharedPayload.Data.URL == "" {
		t.Fatalf("direct shared asset = %s, err = %v", directShared.Body(), err)
	}
	afterDirectFiles := storedAssetFileCount(t, assetDir)
	var exclusiveImage bytes.Buffer
	pixel.Set(0, 0, color.RGBA{R: 68, G: 85, B: 102, A: 255})
	if err := pngimage.Encode(&exclusiveImage, pixel); err != nil {
		t.Fatal(err)
	}
	abortAsset := performMultipartAssetBatch(t, h, "/openapi/v1/subject-imports/"+abortPayload.Data.ID+"/assets", []string{"shared-image", "exclusive-image"}, []namedAsset{
		{filename: "shared.png", content: abortImage.Bytes()},
		{filename: "exclusive.png", content: exclusiveImage.Bytes()},
	}, accessHeader)
	assertStatus(t, abortAsset, 201)
	var abortAssetPayload struct {
		Data struct {
			Assets []struct {
				URL string `json:"url"`
			} `json:"assets"`
		} `json:"data"`
	}
	if err := json.Unmarshal(abortAsset.Body(), &abortAssetPayload); err != nil || len(abortAssetPayload.Data.Assets) != 2 {
		t.Fatalf("abort asset = %s, err = %v", abortAsset.Body(), err)
	}
	aborted := performJSON(h, "POST", "/openapi/v1/subject-imports/"+abortPayload.Data.ID+"/abort", `{}`, accessHeader)
	assertStatus(t, aborted, 200)
	if !bytes.Contains(aborted.Body(), []byte(`"status":"aborted"`)) {
		t.Fatalf("abort response = %s", aborted.Body())
	}
	if storedAssetFileCount(t, assetDir) != afterDirectFiles || afterDirectFiles != beforeAbortFiles+1 {
		t.Fatalf("abort did not preserve the leased direct asset and reclaim only the exclusive import asset")
	}
	assertStatus(t, ut.PerformRequest(h.Engine, "GET", directSharedPayload.Data.URL, nil, ut.Header{Key: "Cookie", Value: cookies}).Result(), 200)
	assertStatus(t, ut.PerformRequest(h.Engine, "GET", abortAssetPayload.Data.Assets[1].URL, nil, ut.Header{Key: "Cookie", Value: cookies}).Result(), 404)
	retriedAbort := performJSON(h, "POST", "/openapi/v1/subject-imports", `{
		"idempotency_key":"abort-one","external_id":"abort-course","title":"Abort Course",
		"description":"","tags":[],"expected_chapters":1,"expected_assets":2
	}`, accessHeader)
	assertStatus(t, retriedAbort, 200)
	if !bytes.Contains(retriedAbort.Body(), []byte(`"status":"draft"`)) || !containsJSONCounts(retriedAbort.Body(), 0, 0) {
		t.Fatalf("aborted task did not reopen: %s", retriedAbort.Body())
	}
	assertStatus(t, performJSON(h, "POST", "/openapi/v1/subject-imports/"+abortPayload.Data.ID+"/abort", `{}`, accessHeader), 200)
	expiring := performJSON(h, "POST", "/openapi/v1/subject-imports", `{
		"idempotency_key":"expire-one","external_id":"expire-course","title":"Expire Course",
		"description":"","tags":[],"expected_chapters":1,"expected_assets":0
	}`, accessHeader)
	assertStatus(t, expiring, 201)
	var expiringPayload struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(expiring.Body(), &expiringPayload); err != nil || expiringPayload.Data.ID == "" {
		t.Fatalf("expiring task = %s, err = %v", expiring.Body(), err)
	}
	assertStatus(t, performJSON(h, "POST", "/openapi/v1/subject-imports/"+expiringPayload.Data.ID+"/chapters", `{
		"batch_key":"will-expire","chapters":[{"external_id":"expired","parent_external_id":null,"position":0,"title":"Expired","content":"stale"}]
	}`, accessHeader), 200)
	expiringID, err := testIDCodec(t).Decode(expiringPayload.Data.ID)
	if err != nil {
		t.Fatal(err)
	}
	pool, err := database.Open(context.Background(), os.Getenv("TEST_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(context.Background(), `UPDATE subject_imports SET updated_at = NOW() - INTERVAL '25 hours' WHERE id = $1`, expiringID); err != nil {
		pool.Close()
		t.Fatal(err)
	}
	pool.Close()
	expired := performJSON(h, "GET", "/openapi/v1/subject-imports/"+expiringPayload.Data.ID, `{}`, accessHeader)
	assertStatus(t, expired, 200)
	if !bytes.Contains(expired.Body(), []byte(`"status":"aborted"`)) || !containsJSONCounts(expired.Body(), 0, 0) {
		t.Fatalf("idle task did not expire: %s", expired.Body())
	}

	viewer := performJSON(h, "POST", "/api/v1/auth/register", `{"email":"viewer@example.com","name":"Viewer","password":"correct horse battery staple"}`)
	assertStatus(t, viewer, 201)
	viewerCookies, _ := authState(t, viewer)
	preview := ut.PerformRequest(h.Engine, "GET", assetPayload.Data.Assets[0].URL, nil, ut.Header{Key: "Cookie", Value: viewerCookies}).Result()
	assertStatus(t, preview, 200)
}

func storedAssetFileCount(t *testing.T, root string) int {
	t.Helper()
	count := 0
	if err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.Type().IsRegular() && filepath.Ext(path) != "" {
			count++
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return count
}

type namedAsset struct {
	filename string
	content  []byte
}

func performMultipartAssetBatch(t *testing.T, h *httpapi.App, path string, keys []string, assets []namedAsset, headers ...ut.Header) *protocol.Response {
	t.Helper()
	if len(keys) != len(assets) {
		t.Fatal("asset keys and files must have the same length")
	}
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for index, asset := range assets {
		if err := writer.WriteField("key", keys[index]); err != nil {
			t.Fatal(err)
		}
		part, err := writer.CreateFormFile("file", asset.filename)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := part.Write(asset.content); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	allHeaders := append([]ut.Header{{Key: "Content-Type", Value: writer.FormDataContentType()}}, headers...)
	return ut.PerformRequest(h.Engine, "POST", path, &ut.Body{Body: bytes.NewReader(body.Bytes()), Len: body.Len()}, allHeaders...).Result()
}

func containsJSONCounts(body []byte, chapters, assets int) bool {
	var payload struct {
		Data struct {
			ReceivedChapters int `json:"received_chapters"`
			ReceivedAssets   int `json:"received_assets"`
		} `json:"data"`
	}
	return json.Unmarshal(body, &payload) == nil && payload.Data.ReceivedChapters == chapters && payload.Data.ReceivedAssets == assets
}
