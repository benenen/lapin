package httpapi_test

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"testing"

	"github.com/cloudwego/hertz/pkg/common/ut"
	"github.com/cloudwego/hertz/pkg/protocol"

	"github.com/benenen/lapin/internal/httpapi"
)

func TestUserCanUploadDeduplicatedAssetAndPreviewIt(t *testing.T) {
	h := newTestApp(t)
	register := performJSON(h, "POST", "/api/v1/auth/register", `{"email":"asset@example.com","name":"Asset Owner","password":"correct horse battery staple"}`)
	assertStatus(t, register, 201)
	cookies, csrf := authState(t, register)
	png, err := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=")
	if err != nil {
		t.Fatal(err)
	}

	first := performMultipart(t, h, "/api/v1/assets", "pixel.png", png,
		ut.Header{Key: "Cookie", Value: cookies},
		ut.Header{Key: "X-CSRF-Token", Value: csrf},
	)
	assertStatus(t, first, 201)
	var firstPayload struct {
		Data struct {
			ID       string `json:"id"`
			URL      string `json:"url"`
			SHA256   string `json:"sha256"`
			MIMEType string `json:"mime_type"`
			Size     int64  `json:"size"`
			Width    int    `json:"width"`
			Height   int    `json:"height"`
		} `json:"data"`
	}
	if err := json.Unmarshal(first.Body(), &firstPayload); err != nil {
		t.Fatal(err)
	}
	if firstPayload.Data.ID == "" || firstPayload.Data.URL != "/api/v1/assets/"+firstPayload.Data.ID+"/content" || firstPayload.Data.SHA256 != "431ced6916a2a21a156e38701afe55bbd7f88969fbbfc56d7fe099d47f265460" || firstPayload.Data.MIMEType != "image/png" || firstPayload.Data.Size != int64(len(png)) || firstPayload.Data.Width != 1 || firstPayload.Data.Height != 1 {
		t.Fatalf("asset response = %s", first.Body())
	}

	second := performMultipart(t, h, "/api/v1/assets", "renamed.png", png,
		ut.Header{Key: "Cookie", Value: cookies},
		ut.Header{Key: "X-CSRF-Token", Value: csrf},
	)
	assertStatus(t, second, 200)
	var secondPayload struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(second.Body(), &secondPayload); err != nil {
		t.Fatal(err)
	}
	if secondPayload.Data.ID != firstPayload.Data.ID {
		t.Fatalf("deduplicated asset ID = %q, want %q", secondPayload.Data.ID, firstPayload.Data.ID)
	}

	preview := ut.PerformRequest(h.Engine, "GET", firstPayload.Data.URL, nil,
		ut.Header{Key: "Cookie", Value: cookies},
	).Result()
	assertStatus(t, preview, 200)
	if !bytes.Equal(preview.Body(), png) || string(preview.Header.ContentType()) != "image/png" || string(preview.Header.Peek("X-Content-Type-Options")) != "nosniff" || string(preview.Header.Peek("ETag")) != `"431ced6916a2a21a156e38701afe55bbd7f88969fbbfc56d7fe099d47f265460"` {
		t.Fatalf("preview headers/body are invalid: content-type=%q headers=%s", preview.Header.ContentType(), preview.Header.Header())
	}
	assertStatus(t, ut.PerformRequest(h.Engine, "GET", firstPayload.Data.URL, nil).Result(), 401)

	encodedUpload := performMultipart(t, h, "/api/v1/assets", "pixel.png", png,
		ut.Header{Key: "Cookie", Value: cookies},
		ut.Header{Key: "X-CSRF-Token", Value: csrf},
		ut.Header{Key: "Content-Encoding", Value: "gzip"},
	)
	assertStatus(t, encodedUpload, 415)

	createdSubject := performJSON(h, "POST", "/api/v1/subjects", `{
		"title":"Course with image","chapters":[{"title":"Image chapter","content":"before upload"}]
	}`, ut.Header{Key: "Cookie", Value: cookies}, ut.Header{Key: "X-CSRF-Token", Value: csrf})
	assertStatus(t, createdSubject, 201)
	var subjectPayload struct {
		Data struct {
			Chapters []struct {
				ID string `json:"id"`
			} `json:"chapters"`
		} `json:"data"`
	}
	if err := json.Unmarshal(createdSubject.Body(), &subjectPayload); err != nil || len(subjectPayload.Data.Chapters) != 1 {
		t.Fatalf("created subject = %s, err = %v", createdSubject.Body(), err)
	}
	chapterID := subjectPayload.Data.Chapters[0].ID
	updatedChapter := performJSON(h, "POST", "/api/v1/chapters/"+chapterID+"/update", fmt.Sprintf(`{
		"title":"Image chapter","content":"![图](%s)"
	}`, firstPayload.Data.URL), ut.Header{Key: "Cookie", Value: cookies}, ut.Header{Key: "X-CSRF-Token", Value: csrf})
	assertStatus(t, updatedChapter, 200)

	viewer := performJSON(h, "POST", "/api/v1/auth/register", `{"email":"asset-viewer@example.com","name":"Asset Viewer","password":"correct horse battery staple"}`)
	assertStatus(t, viewer, 201)
	viewerCookies, viewerCSRF := authState(t, viewer)
	assertStatus(t, ut.PerformRequest(h.Engine, "GET", firstPayload.Data.URL, nil, ut.Header{Key: "Cookie", Value: viewerCookies}).Result(), 200)
	removedImage := performJSON(h, "POST", "/api/v1/chapters/"+chapterID+"/update", `{
		"title":"Image chapter","content":"image removed"
	}`, ut.Header{Key: "Cookie", Value: cookies}, ut.Header{Key: "X-CSRF-Token", Value: csrf})
	assertStatus(t, removedImage, 200)
	assertStatus(t, ut.PerformRequest(h.Engine, "GET", firstPayload.Data.URL, nil, ut.Header{Key: "Cookie", Value: viewerCookies}).Result(), 404)

	crossOwner := performJSON(h, "POST", "/api/v1/subjects", fmt.Sprintf(`{
		"title":"Cross owner","chapters":[{"title":"Chapter","content":"![图](%s)"}]
	}`, firstPayload.Data.URL), ut.Header{Key: "Cookie", Value: viewerCookies}, ut.Header{Key: "X-CSRF-Token", Value: viewerCSRF})
	assertStatus(t, crossOwner, 400)
	external := performJSON(h, "POST", "/api/v1/subjects", `{
		"title":"External tracker","chapters":[{"title":"Chapter","content":"![tracker](https://tracker.example/pixel.png)"}]
	}`, ut.Header{Key: "Cookie", Value: viewerCookies}, ut.Header{Key: "X-CSRF-Token", Value: viewerCSRF})
	assertStatus(t, external, 400)
}

func performMultipart(t *testing.T, h *httpapi.App, path, filename string, content []byte, headers ...ut.Header) *protocol.Response {
	t.Helper()
	body, contentType := multipartBody(t, filename, content)
	allHeaders := append([]ut.Header{{Key: "Content-Type", Value: contentType}}, headers...)
	return ut.PerformRequest(h.Engine, "POST", path, &ut.Body{Body: bytes.NewReader(body), Len: len(body)}, allHeaders...).Result()
}

func multipartBody(t *testing.T, filename string, content []byte) ([]byte, string) {
	return multipartBodyWithFields(t, filename, content, nil)
}

func performMultipartFields(t *testing.T, h *httpapi.App, path, filename string, content []byte, fields map[string]string, headers ...ut.Header) *protocol.Response {
	t.Helper()
	body, contentType := multipartBodyWithFields(t, filename, content, fields)
	allHeaders := append([]ut.Header{{Key: "Content-Type", Value: contentType}}, headers...)
	return ut.PerformRequest(h.Engine, "POST", path, &ut.Body{Body: bytes.NewReader(body), Len: len(body)}, allHeaders...).Result()
}

func multipartBodyWithFields(t *testing.T, filename string, content []byte, fields map[string]string) ([]byte, string) {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for key, value := range fields {
		if err := writer.WriteField(key, value); err != nil {
			t.Fatal(err)
		}
	}
	part, err := writer.CreateFormFile("file", filename)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return body.Bytes(), writer.FormDataContentType()
}
