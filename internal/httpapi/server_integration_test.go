package httpapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/cloudwego/hertz/pkg/common/ut"
	"github.com/cloudwego/hertz/pkg/protocol"

	"github.com/benenen/lapin/internal/assetstore"
	"github.com/benenen/lapin/internal/database"
	"github.com/benenen/lapin/internal/httpapi"
	"github.com/benenen/lapin/internal/identifier"
)

func TestUserCanImportAndStudySubject(t *testing.T) {
	h := newTestApp(t)
	response := performJSON(h, "POST", "/api/v1/auth/register", `{"email":"alice@example.com","name":"Alice","avatar_url":"https://example.com/alice.png","password":"correct horse battery staple"}`)

	if response.StatusCode() != 201 {
		t.Fatalf("register status = %d, body = %s", response.StatusCode(), response.Body())
	}

	var payload struct {
		Data struct {
			User struct {
				ID        string   `json:"id"`
				Email     string   `json:"email"`
				AvatarURL string   `json:"avatar_url"`
				Roles     []string `json:"roles"`
			} `json:"user"`
			CSRFToken string `json:"csrf_token"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Data.User.Email != "alice@example.com" {
		t.Fatalf("registered email = %q", payload.Data.User.Email)
	}
	if _, err := strconv.ParseInt(payload.Data.User.ID, 10, 64); err == nil {
		t.Fatalf("user id leaked as raw integer: %q", payload.Data.User.ID)
	}
	if _, err := testIDCodec(t).Decode(payload.Data.User.ID); err != nil {
		t.Fatalf("user id is not a valid HashID: %q", payload.Data.User.ID)
	}
	if payload.Data.User.AvatarURL != "https://example.com/alice.png" || len(payload.Data.User.Roles) != 1 || payload.Data.User.Roles[0] != "learner" {
		t.Fatalf("registered profile = %#v", payload.Data.User)
	}
	cookieCount := 0
	response.Header.VisitAllCookie(func(_, _ []byte) { cookieCount++ })
	if cookieCount < 2 {
		t.Fatal("register response did not set session and CSRF cookies")
	}
	cookies := cookieHeader(response)

	response = performJSON(h, "POST", "/api/v1/access-tokens", `{"name":"integration"}`,
		ut.Header{Key: "Cookie", Value: cookies},
		ut.Header{Key: "X-CSRF-Token", Value: payload.Data.CSRFToken},
	)
	if response.StatusCode() != 201 {
		t.Fatalf("create token status = %d, body = %s", response.StatusCode(), response.Body())
	}
	var tokenPayload struct {
		Data struct {
			AccessToken string `json:"access_token"`
			Token       struct {
				ID string `json:"id"`
			} `json:"token"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body(), &tokenPayload); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(tokenPayload.Data.AccessToken, "lpn_") {
		t.Fatalf("access token = %q", tokenPayload.Data.AccessToken)
	}

	response = performJSON(h, "POST", "/openapi/v1/subjects/import", `{
		"external_id":"go-101",
		"title":"Go 入门",
		"description":"从语法到服务",
		"tags":["Go","后端"],
		"chapters":[{"external_id":"part-1","title":"语言基础","content":"","children":[{"external_id":"syntax","title":"基础语法","content":"package、变量与函数。"}]}]
	}`, ut.Header{Key: "Authorization", Value: "Bearer " + tokenPayload.Data.AccessToken})
	if response.StatusCode() != 200 {
		t.Fatalf("import status = %d, body = %s", response.StatusCode(), response.Body())
	}
	var importPayload struct {
		Data struct {
			ID       string `json:"id"`
			Chapters []struct {
				ID       string  `json:"id"`
				ParentID *string `json:"parent_id"`
			} `json:"chapters"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body(), &importPayload); err != nil {
		t.Fatal(err)
	}
	if importPayload.Data.ID == "" || len(importPayload.Data.Chapters) != 2 {
		t.Fatalf("unexpected imported subject: %s", response.Body())
	}

	chapterID := ""
	for _, chapter := range importPayload.Data.Chapters {
		if chapter.ParentID != nil {
			chapterID = chapter.ID
		}
	}
	if chapterID == "" {
		t.Fatalf("imported subject does not contain a child chapter: %s", response.Body())
	}
	response = performJSON(h, "POST", "/api/v1/chapters/"+chapterID+"/annotations", `{
		"start_offset":0,"end_offset":7,"quote":"package","note":"程序从包开始","color":"yellow"
	}`,
		ut.Header{Key: "Cookie", Value: cookies},
		ut.Header{Key: "X-CSRF-Token", Value: payload.Data.CSRFToken},
	)
	if response.StatusCode() != 201 {
		t.Fatalf("create annotation status = %d, body = %s", response.StatusCode(), response.Body())
	}
	var annotationPayload struct {
		Data struct {
			StartOffset int `json:"start_offset"`
			EndOffset   int `json:"end_offset"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body(), &annotationPayload); err != nil {
		t.Fatal(err)
	}
	if annotationPayload.Data.StartOffset != 0 || annotationPayload.Data.EndOffset != 7 {
		t.Fatalf("annotation offsets changed coordinate systems: %s", response.Body())
	}

	whiteboardDocument := validWhiteboardJSON(chapterID)
	response = performJSON(h, "POST", "/api/v1/chapters/"+chapterID+"/whiteboard", whiteboardDocument,
		ut.Header{Key: "Cookie", Value: cookies},
		ut.Header{Key: "X-CSRF-Token", Value: payload.Data.CSRFToken},
	)
	if response.StatusCode() != 200 {
		t.Fatalf("save whiteboard status = %d, body = %s", response.StatusCode(), response.Body())
	}
	var savedWhiteboard struct {
		Data struct {
			Data struct {
				Version int `json:"version"`
				Anchor  struct {
					ID string `json:"id"`
				} `json:"anchor"`
				Space struct {
					Width  float64 `json:"width"`
					Height float64 `json:"height"`
				} `json:"space"`
				Document struct {
					Type     string            `json:"type"`
					Elements []json.RawMessage `json:"elements"`
				} `json:"document"`
			} `json:"data"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body(), &savedWhiteboard); err != nil {
		t.Fatal(err)
	}
	if savedWhiteboard.Data.Data.Version != 3 || savedWhiteboard.Data.Data.Anchor.ID != chapterID || savedWhiteboard.Data.Data.Space.Width != 960 || savedWhiteboard.Data.Data.Space.Height != 640 || savedWhiteboard.Data.Data.Document.Type != "excalidraw" || len(savedWhiteboard.Data.Data.Document.Elements) != 3 {
		t.Fatalf("whiteboard contract was not preserved: %s", response.Body())
	}
	longChapterWhiteboard := strings.Replace(whiteboardDocument, `"space":{"width":960,"height":640,"fit":"contain"}`, `"space":{"width":960,"height":73680,"fit":"contain"}`, 1)
	response = performJSON(h, "POST", "/api/v1/chapters/"+chapterID+"/whiteboard", longChapterWhiteboard,
		ut.Header{Key: "Cookie", Value: cookies},
		ut.Header{Key: "X-CSRF-Token", Value: payload.Data.CSRFToken},
	)
	if response.StatusCode() != 200 {
		t.Fatalf("save whiteboard anchored to a long chapter status = %d, body = %s", response.StatusCode(), response.Body())
	}
	response = performJSON(h, "POST", "/api/v1/chapters/"+chapterID+"/whiteboard", whiteboardDocument,
		ut.Header{Key: "Cookie", Value: cookies},
		ut.Header{Key: "X-CSRF-Token", Value: payload.Data.CSRFToken},
	)
	assertStatus(t, response, 200)

	bobResponse := performJSON(h, "POST", "/api/v1/auth/register", `{"email":"bob@example.com","name":"Bob","password":"correct horse battery staple"}`)
	assertStatus(t, bobResponse, 201)
	bobCookies, _ := authState(t, bobResponse)
	response = performJSON(h, "GET", "/api/v1/chapters/"+chapterID+"/whiteboards", `{}`, ut.Header{Key: "Cookie", Value: bobCookies})
	assertStatus(t, response, 200)
	var bobWhiteboards struct {
		Data []json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(response.Body(), &bobWhiteboards); err != nil || len(bobWhiteboards.Data) != 0 {
		t.Fatalf("another user can read private whiteboard: body = %s, err = %v", response.Body(), err)
	}

	response = performJSON(h, "POST", "/api/v1/chapters/"+chapterID+"/comments", `{"body":"这一章很清楚"}`,
		ut.Header{Key: "Cookie", Value: cookies},
		ut.Header{Key: "X-CSRF-Token", Value: payload.Data.CSRFToken},
	)
	if response.StatusCode() != 201 {
		t.Fatalf("create comment status = %d, body = %s", response.StatusCode(), response.Body())
	}

	response = performJSON(h, "POST", "/openapi/v1/subjects/import", `{
		"external_id":"go-101","title":"Go 入门（更新）","description":"保持互动数据",
		"tags":["Go"],"chapters":[{"external_id":"part-1","title":"语言基础","content":"","children":[
			{"external_id":"new-before-syntax","title":"新增导读","content":"先读这里。"},
			{"external_id":"syntax","title":"基础语法（更新）","content":"package、变量与函数。"}
		]}]
	}`, ut.Header{Key: "Authorization", Value: "Bearer " + tokenPayload.Data.AccessToken})
	assertStatus(t, response, 200)
	var reimported struct {
		Data struct {
			Chapters []struct {
				ID    string `json:"id"`
				Title string `json:"title"`
			} `json:"chapters"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body(), &reimported); err != nil {
		t.Fatal(err)
	}
	stableChapterID := ""
	for _, chapter := range reimported.Data.Chapters {
		if chapter.Title == "基础语法（更新）" {
			stableChapterID = chapter.ID
		}
	}
	if stableChapterID != chapterID {
		t.Fatalf("stable chapter id changed after reordering: got %q, want %q", stableChapterID, chapterID)
	}
	response = performJSON(h, "POST", "/openapi/v1/subjects/import", `{
		"external_id":"go-101","title":"Go 入门（精简）","description":"保留未提交章节",
		"tags":["Go"],"chapters":[{"external_id":"syntax","title":"基础语法（精简）","content":"package、变量与函数。"}]
	}`, ut.Header{Key: "Authorization", Value: "Bearer " + tokenPayload.Data.AccessToken})
	assertStatus(t, response, 200)
	var compactImport struct {
		Data struct {
			Chapters []struct {
				ID       string `json:"id"`
				Position int    `json:"position"`
			} `json:"chapters"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body(), &compactImport); err != nil {
		t.Fatal(err)
	}
	positions := make(map[int]struct{}, len(compactImport.Data.Chapters))
	for _, chapter := range compactImport.Data.Chapters {
		if _, duplicate := positions[chapter.Position]; duplicate {
			t.Fatalf("reimport produced duplicate chapter position %d: %s", chapter.Position, response.Body())
		}
		positions[chapter.Position] = struct{}{}
	}

	for _, path := range []string{
		"/healthz",
		"/api/v1/me",
		"/api/v1/access-tokens",
		"/api/v1/subjects",
		"/api/v1/subjects/" + importPayload.Data.ID,
		"/api/v1/chapters/" + chapterID + "/annotations",
		"/api/v1/chapters/" + chapterID + "/whiteboards",
		"/api/v1/chapters/" + chapterID + "/comments",
	} {
		headers := []ut.Header{}
		if path != "/healthz" {
			headers = append(headers, ut.Header{Key: "Cookie", Value: cookies})
		}
		response = performJSON(h, "GET", path, `{}`, headers...)
		if response.StatusCode() != 200 {
			t.Fatalf("GET %s status = %d, body = %s", path, response.StatusCode(), response.Body())
		}
	}

	response = performJSON(h, "POST", "/api/v1/subjects", `{
		"title":"手动科目","description":"浏览器创建","tags":["学习"],
		"chapters":[{"title":"总览","content":"## 总览\n\n公式 $E = mc^2$"}]
	}`,
		ut.Header{Key: "Cookie", Value: cookies},
		ut.Header{Key: "X-CSRF-Token", Value: payload.Data.CSRFToken},
	)
	if response.StatusCode() != 201 {
		t.Fatalf("create subject status = %d, body = %s", response.StatusCode(), response.Body())
	}
	var manualSubject struct {
		Data struct {
			ID       string `json:"id"`
			Chapters []struct {
				ID      string `json:"id"`
				Content string `json:"content"`
			} `json:"chapters"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body(), &manualSubject); err != nil {
		t.Fatal(err)
	}
	if manualSubject.Data.Chapters[0].Content != "## 总览\n\n公式 $E = mc^2$" {
		t.Fatalf("Markdown content was changed by backend: %q", manualSubject.Data.Chapters[0].Content)
	}
	response = performJSON(h, "POST", "/api/v1/subjects/"+manualSubject.Data.ID+"/tags", `{"tags":["Go","树结构"]}`,
		ut.Header{Key: "Cookie", Value: cookies},
		ut.Header{Key: "X-CSRF-Token", Value: payload.Data.CSRFToken},
	)
	if response.StatusCode() != 200 {
		t.Fatalf("replace tags status = %d, body = %s", response.StatusCode(), response.Body())
	}
	response = performJSON(h, "POST", "/api/v1/subjects/"+manualSubject.Data.ID+"/chapters", `{
		"parent_id":"`+manualSubject.Data.Chapters[0].ID+`","title":"子章节","content":"子章节正文"
	}`,
		ut.Header{Key: "Cookie", Value: cookies},
		ut.Header{Key: "X-CSRF-Token", Value: payload.Data.CSRFToken},
	)
	if response.StatusCode() != 201 {
		t.Fatalf("create child chapter status = %d, body = %s", response.StatusCode(), response.Body())
	}

	response = performJSON(h, "POST", "/api/v1/access-tokens/"+tokenPayload.Data.Token.ID+"/revoke", `{}`,
		ut.Header{Key: "Cookie", Value: cookies},
		ut.Header{Key: "X-CSRF-Token", Value: payload.Data.CSRFToken},
	)
	if response.StatusCode() != 200 {
		t.Fatalf("revoke token status = %d, body = %s", response.StatusCode(), response.Body())
	}
	response = performJSON(h, "POST", "/openapi/v1/subjects/import", `{"external_id":"revoked","title":"不可导入","chapters":[]}`,
		ut.Header{Key: "Authorization", Value: "Bearer " + tokenPayload.Data.AccessToken},
	)
	if response.StatusCode() != 401 {
		t.Fatalf("revoked token import status = %d, body = %s", response.StatusCode(), response.Body())
	}

	response = performJSON(h, "POST", "/api/v1/auth/logout", `{}`,
		ut.Header{Key: "Cookie", Value: cookies},
		ut.Header{Key: "X-CSRF-Token", Value: payload.Data.CSRFToken},
	)
	if response.StatusCode() != 200 {
		t.Fatalf("logout status = %d, body = %s", response.StatusCode(), response.Body())
	}
	response = performJSON(h, "POST", "/api/v1/auth/login", `{"email":"alice@example.com","password":"correct horse battery staple"}`)
	if response.StatusCode() != 200 {
		t.Fatalf("login status = %d, body = %s", response.StatusCode(), response.Body())
	}
}

func TestAPIRejectsInvalidAndUnauthorizedRequests(t *testing.T) {
	h := newTestApp(t)

	assertStatus(t, performJSON(h, "POST", "/api/v1/auth/register", `{`), 400)
	assertStatus(t, performJSON(h, "POST", "/api/v1/auth/register", `{"email":"invalid","name":"","password":"short"}`), 400)
	aliceResponse := performJSON(h, "POST", "/api/v1/auth/register", `{"email":"alice@example.com","name":"Alice","password":"correct horse battery staple"}`)
	assertStatus(t, aliceResponse, 201)
	aliceCookies, aliceCSRF := authState(t, aliceResponse)
	assertStatus(t, performJSON(h, "POST", "/api/v1/auth/register", `{"email":"alice@example.com","name":"Alice 2","password":"correct horse battery staple"}`), 409)
	assertStatus(t, performJSON(h, "POST", "/api/v1/auth/register", `{"email":"avatar@example.com","name":"Avatar","avatar_url":"http://example.com/a.png","password":"correct horse battery staple"}`), 400)
	assertStatus(t, performJSON(h, "POST", "/api/v1/auth/login", `{"email":"alice@example.com","password":"wrong password"}`), 401)
	assertStatus(t, performJSON(h, "GET", "/api/v1/me", `{}`), 401)
	assertStatus(t, performJSON(h, "POST", "/api/v1/access-tokens", `{"name":"missing csrf"}`, ut.Header{Key: "Cookie", Value: aliceCookies}), 403)
	assertStatus(t, performJSON(h, "POST", "/openapi/v1/subjects/import", `{"external_id":"x","title":"X"}`, ut.Header{Key: "Authorization", Value: "Bearer lpn_invalid"}), 401)
	validTokenResponse := performJSON(h, "POST", "/api/v1/access-tokens", `{"name":"validation"}`,
		ut.Header{Key: "Cookie", Value: aliceCookies}, ut.Header{Key: "X-CSRF-Token", Value: aliceCSRF})
	assertStatus(t, validTokenResponse, 201)
	assertStatus(t, performJSON(h, "POST", "/api/v1/access-tokens", `{"name":""}`,
		ut.Header{Key: "Cookie", Value: aliceCookies}, ut.Header{Key: "X-CSRF-Token", Value: aliceCSRF}), 400)
	var validTokenPayload struct {
		Data struct {
			AccessToken string `json:"access_token"`
		} `json:"data"`
	}
	if err := json.Unmarshal(validTokenResponse.Body(), &validTokenPayload); err != nil {
		t.Fatal(err)
	}
	accessHeader := ut.Header{Key: "Authorization", Value: "Bearer " + validTokenPayload.Data.AccessToken}
	assertStatus(t, performJSON(h, "POST", "/openapi/v1/subjects/import", `{"external_id":"","title":"Bad"}`, accessHeader), 400)
	assertStatus(t, performJSON(h, "POST", "/openapi/v1/subjects/import", `{"external_id":"bad-subject","title":"","tags":[],"chapters":[]}`, accessHeader), 400)
	assertStatus(t, performJSON(h, "POST", "/api/v1/subjects", `{"title":"","tags":[]}`,
		ut.Header{Key: "Cookie", Value: aliceCookies}, ut.Header{Key: "X-CSRF-Token", Value: aliceCSRF}), 400)

	created := performJSON(h, "POST", "/api/v1/subjects", `{"title":"Alice subject","tags":["one"],"chapters":[{"title":"Root","content":"**A😀B** A😀B"}]}`,
		ut.Header{Key: "Cookie", Value: aliceCookies}, ut.Header{Key: "X-CSRF-Token", Value: aliceCSRF})
	assertStatus(t, created, 201)
	var subjectPayload struct {
		Data struct {
			ID       string `json:"id"`
			Chapters []struct {
				ID string `json:"id"`
			} `json:"chapters"`
		} `json:"data"`
	}
	if err := json.Unmarshal(created.Body(), &subjectPayload); err != nil {
		t.Fatal(err)
	}
	subjectID := subjectPayload.Data.ID
	chapterID := subjectPayload.Data.Chapters[0].ID
	formattedAnnotation := performJSON(h, "POST", "/api/v1/chapters/"+chapterID+"/annotations", `{"start_offset":1,"end_offset":3,"quote":"😀","note":"emoji","color":"yellow"}`,
		ut.Header{Key: "Cookie", Value: aliceCookies}, ut.Header{Key: "X-CSRF-Token", Value: aliceCSRF})
	assertStatus(t, formattedAnnotation, 201)
	var formattedAnnotationPayload struct {
		Data struct {
			StartOffset int `json:"start_offset"`
			EndOffset   int `json:"end_offset"`
		} `json:"data"`
	}
	if err := json.Unmarshal(formattedAnnotation.Body(), &formattedAnnotationPayload); err != nil {
		t.Fatal(err)
	}
	if formattedAnnotationPayload.Data.StartOffset != 1 || formattedAnnotationPayload.Data.EndOffset != 3 {
		t.Fatalf("formatted Markdown annotation offsets changed: %s", formattedAnnotation.Body())
	}

	bobResponse := performJSON(h, "POST", "/api/v1/auth/register", `{"email":"bob@example.com","name":"Bob","password":"correct horse battery staple"}`)
	assertStatus(t, bobResponse, 201)
	bobCookies, bobCSRF := authState(t, bobResponse)
	assertStatus(t, performJSON(h, "POST", "/api/v1/subjects/"+subjectID+"/chapters", `{"title":"Forbidden","content":""}`,
		ut.Header{Key: "Cookie", Value: bobCookies}, ut.Header{Key: "X-CSRF-Token", Value: bobCSRF}), 404)
	assertStatus(t, performJSON(h, "POST", "/api/v1/subjects/"+subjectID+"/tags", `{"tags":[""]}`,
		ut.Header{Key: "Cookie", Value: aliceCookies}, ut.Header{Key: "X-CSRF-Token", Value: aliceCSRF}), 400)
	missingID := testIDCodec(t).Encode(999999)
	assertStatus(t, performJSON(h, "POST", "/api/v1/subjects/"+subjectID+"/chapters", `{"parent_id":"`+missingID+`","title":"Bad parent","content":""}`,
		ut.Header{Key: "Cookie", Value: aliceCookies}, ut.Header{Key: "X-CSRF-Token", Value: aliceCSRF}), 400)
	assertStatus(t, performJSON(h, "POST", "/api/v1/subjects/"+subjectID+"/chapters", `{"title":"","content":""}`,
		ut.Header{Key: "Cookie", Value: aliceCookies}, ut.Header{Key: "X-CSRF-Token", Value: aliceCSRF}), 400)
	assertStatus(t, performJSON(h, "GET", "/api/v1/subjects/not-a-hashid", `{}`, ut.Header{Key: "Cookie", Value: aliceCookies}), 400)
	assertStatus(t, performJSON(h, "GET", "/api/v1/subjects/"+missingID, `{}`, ut.Header{Key: "Cookie", Value: aliceCookies}), 404)
	assertStatus(t, performJSON(h, "POST", "/api/v1/chapters/"+chapterID+"/annotations", `{"start_offset":0,"end_offset":99,"note":"bad","color":"orange"}`,
		ut.Header{Key: "Cookie", Value: bobCookies}, ut.Header{Key: "X-CSRF-Token", Value: bobCSRF}), 400)
	assertStatus(t, performJSON(h, "POST", "/api/v1/chapters/"+chapterID+"/annotations", `{"start_offset":100000,"end_offset":100001,"quote":"x","note":"bad range","color":"yellow"}`,
		ut.Header{Key: "Cookie", Value: bobCookies}, ut.Header{Key: "X-CSRF-Token", Value: bobCSRF}), 400)
	assertStatus(t, performJSON(h, "POST", "/api/v1/chapters/"+chapterID+"/whiteboard", `{"data":"not an object"}`,
		ut.Header{Key: "Cookie", Value: bobCookies}, ut.Header{Key: "X-CSRF-Token", Value: bobCSRF}), 400)
	assertStatus(t, performJSON(h, "POST", "/api/v1/chapters/"+chapterID+"/whiteboard", `{"data":{"strokes":"bad"}}`,
		ut.Header{Key: "Cookie", Value: bobCookies}, ut.Header{Key: "X-CSRF-Token", Value: bobCSRF}), 400)
	assertStatus(t, performJSON(h, "POST", "/api/v1/chapters/"+chapterID+"/whiteboard", `{"data":{"version":3,"anchor":{"type":"chapter","id":"wrong","content_revision":"sha256:x"},"space":{"width":960,"height":640,"fit":"contain"},"document":{"type":"excalidraw","version":2,"elements":[],"appState":{},"files":{}}}}`,
		ut.Header{Key: "Cookie", Value: bobCookies}, ut.Header{Key: "X-CSRF-Token", Value: bobCSRF}), 400)
	invalidWhiteboards := []string{
		`{"data":{"version":3,"anchor":{"type":"chapter","id":"` + chapterID + `","content_revision":""},"space":{"width":960,"height":640,"fit":"contain"},"document":{"type":"excalidraw","version":2,"elements":[],"appState":{},"files":{}}}}`,
		`{"data":{"version":3,"anchor":{"type":"chapter","id":"` + chapterID + `","content_revision":"sha256:x"},"space":{"width":99,"height":640,"fit":"contain"},"document":{"type":"excalidraw","version":2,"elements":[],"appState":{},"files":{}}}}`,
		`{"data":{"version":3,"anchor":{"type":"chapter","id":"` + chapterID + `","content_revision":"sha256:x"},"space":{"width":10001,"height":640,"fit":"contain"},"document":{"type":"excalidraw","version":2,"elements":[],"appState":{},"files":{}}}}`,
		`{"data":{"version":3,"anchor":{"type":"chapter","id":"` + chapterID + `","content_revision":"sha256:x"},"space":{"width":960,"height":200001,"fit":"contain"},"document":{"type":"excalidraw","version":2,"elements":[],"appState":{},"files":{}}}}`,
		`{"data":{"version":3,"anchor":{"type":"chapter","id":"` + chapterID + `","content_revision":"sha256:x"},"space":{"width":960,"height":640,"fit":"contain"},"document":{"type":"other","version":2,"elements":[],"appState":{},"files":{}}}}`,
		`{"data":{"version":3,"anchor":{"type":"chapter","id":"` + chapterID + `","content_revision":"sha256:x"},"space":{"width":960,"height":640,"fit":"contain"},"document":{"type":"excalidraw","version":2,"elements":null,"appState":{},"files":{}}}}`,
		`{"data":{"version":3,"anchor":{"type":"chapter","id":"` + chapterID + `","content_revision":"sha256:x"},"space":{"width":960,"height":640,"fit":"contain"},"document":{"type":"excalidraw","version":2,"elements":[null],"appState":{},"files":{}}}}`,
		`{"data":{"version":3,"anchor":{"type":"chapter","id":"` + chapterID + `","content_revision":"sha256:x"},"space":{"width":960,"height":640,"fit":"contain"},"document":{"type":"excalidraw","version":2,"elements":[],"appState":{},"files":{"image":{}}}}}`,
		`{"data":{"version":3,"anchor":{"type":"chapter","id":"` + chapterID + `","content_revision":"sha256:x"},"space":{"width":960,"height":640,"fit":"contain"},"document":{"type":"excalidraw","version":2,"elements":[{"id":"shape-1","type":"image"}],"appState":{"viewBackgroundColor":"transparent"},"files":{}}}}`,
		`{"data":{"version":3,"anchor":{"type":"chapter","id":"` + chapterID + `","content_revision":"sha256:x"},"space":{"width":960,"height":640,"fit":"contain"},"document":{"type":"excalidraw","version":2,"elements":[{"id":"duplicate","type":"freedraw"},{"id":"duplicate","type":"freedraw"}],"appState":{"viewBackgroundColor":"transparent"},"files":{}}}}`,
		`{"data":{"version":3,"anchor":{"type":"chapter","id":"` + chapterID + `","content_revision":"sha256:x"},"space":{"width":960,"height":640,"fit":"contain"},"document":{"type":"excalidraw","version":2,"elements":[{"id":"incomplete","type":"rectangle"}],"appState":{"viewBackgroundColor":"transparent"},"files":{}}}}`,
		`{"data":{"version":3,"anchor":{"type":"chapter","id":"` + chapterID + `","content_revision":"sha256:x"},"space":{"width":960,"height":640,"fit":"contain"},"document":{"type":"excalidraw","version":2,"elements":[{"id":"extreme","type":"rectangle","x":1e100,"y":0,"width":10,"height":10,"angle":0,"seed":1,"version":1,"versionNonce":1,"updated":1,"opacity":100,"strokeWidth":1,"roughness":1,"isDeleted":false,"locked":false}],"appState":{"viewBackgroundColor":"transparent"},"files":{}}}}`,
	}
	for _, body := range invalidWhiteboards {
		assertStatus(t, performJSON(h, "POST", "/api/v1/chapters/"+chapterID+"/whiteboard", body,
			ut.Header{Key: "Cookie", Value: bobCookies}, ut.Header{Key: "X-CSRF-Token", Value: bobCSRF}), 400)
	}
	assertStatus(t, performJSON(h, "POST", "/api/v1/chapters/"+chapterID+"/comments", `{"body":"   "}`,
		ut.Header{Key: "Cookie", Value: bobCookies}, ut.Header{Key: "X-CSRF-Token", Value: bobCSRF}), 400)
	for _, endpoint := range []struct {
		method string
		path   string
		body   string
	}{
		{method: "GET", path: "/api/v1/chapters/not-a-hashid/annotations", body: `{}`},
		{method: "POST", path: "/api/v1/chapters/not-a-hashid/annotations", body: `{"note":"x"}`},
		{method: "GET", path: "/api/v1/chapters/not-a-hashid/whiteboards", body: `{}`},
		{method: "POST", path: "/api/v1/chapters/not-a-hashid/whiteboard", body: `{"data":{}}`},
		{method: "GET", path: "/api/v1/chapters/not-a-hashid/comments", body: `{}`},
		{method: "POST", path: "/api/v1/chapters/not-a-hashid/comments", body: `{"body":"x"}`},
	} {
		assertStatus(t, performJSON(h, endpoint.method, endpoint.path, endpoint.body,
			ut.Header{Key: "Cookie", Value: bobCookies}, ut.Header{Key: "X-CSRF-Token", Value: bobCSRF}), 404)
	}
	assertStatus(t, performJSON(h, "POST", "/api/v1/access-tokens/not-a-hashid/revoke", `{}`,
		ut.Header{Key: "Cookie", Value: aliceCookies}, ut.Header{Key: "X-CSRF-Token", Value: aliceCSRF}), 400)
	assertStatus(t, performJSON(h, "POST", "/api/v1/access-tokens/"+missingID+"/revoke", `{}`,
		ut.Header{Key: "Cookie", Value: aliceCookies}, ut.Header{Key: "X-CSRF-Token", Value: aliceCSRF}), 404)
	assertStatus(t, performJSON(h, "POST", "/openapi/v1/subjects/import", `{"external_id":"","title":"Bad"}`, ut.Header{Key: "Authorization", Value: "bad"}), 401)
}

func TestAuthenticationEndpointsAreRateLimited(t *testing.T) {
	h := newTestApp(t)
	for attempt := 0; attempt < 10; attempt++ {
		response := performJSON(h, "POST", "/api/v1/auth/login", `{"email":"nobody`+strconv.Itoa(attempt)+`@example.com","password":"invalid password"}`,
			ut.Header{Key: "X-Forwarded-For", Value: "203.0.113." + strconv.Itoa(attempt+1)})
		assertStatus(t, response, 401)
	}
	assertStatus(t, performJSON(h, "POST", "/api/v1/auth/login", `{"email":"nobody10@example.com","password":"invalid password"}`,
		ut.Header{Key: "X-Forwarded-For", Value: "198.51.100.10"}), 429)
}

func TestAuthenticationRejectsCrossSiteAndNonJSONRequests(t *testing.T) {
	h := newTestApp(t)
	body := `{"email":"alice@example.com","name":"Alice","password":"correct horse battery staple"}`
	assertStatus(t, performJSON(h, "POST", "/api/v1/auth/register", body,
		ut.Header{Key: "Origin", Value: "https://evil.example"}), 403)

	response := ut.PerformRequest(h.Engine, "POST", "/api/v1/auth/register",
		&ut.Body{Body: strings.NewReader(body), Len: len(body)},
		ut.Header{Key: "Content-Type", Value: "text/plain"},
	).Result()
	assertStatus(t, response, 415)
}

func TestAccessTokenLimitIsAtomic(t *testing.T) {
	h := newTestApp(t)
	registered := performJSON(h, "POST", "/api/v1/auth/register", `{"email":"token-owner@example.com","name":"Token Owner","password":"correct horse battery staple"}`)
	assertStatus(t, registered, 201)
	cookies, csrf := authState(t, registered)

	statuses := make([]int, 11)
	var wait sync.WaitGroup
	for index := range statuses {
		wait.Add(1)
		go func() {
			defer wait.Done()
			response := performJSON(h, "POST", "/api/v1/access-tokens", `{"name":"token-`+strconv.Itoa(index)+`"}`,
				ut.Header{Key: "Cookie", Value: cookies}, ut.Header{Key: "X-CSRF-Token", Value: csrf})
			statuses[index] = response.StatusCode()
		}()
	}
	wait.Wait()
	created := 0
	rejected := 0
	for _, status := range statuses {
		switch status {
		case 201:
			created++
		case 409:
			rejected++
		default:
			t.Fatalf("unexpected concurrent token status %d: %#v", status, statuses)
		}
	}
	if created != 10 || rejected != 1 {
		t.Fatalf("token quota result = created %d, rejected %d, statuses %#v", created, rejected, statuses)
	}
}

func TestConcurrentChaptersHaveUniquePositions(t *testing.T) {
	h := newTestApp(t)
	registered := performJSON(h, "POST", "/api/v1/auth/register", `{"email":"chapters@example.com","name":"Chapter owner","password":"correct horse battery staple"}`)
	assertStatus(t, registered, 201)
	cookies, csrf := authState(t, registered)
	created := performJSON(h, "POST", "/api/v1/subjects", `{"title":"Concurrent chapters","chapters":[]}`,
		ut.Header{Key: "Cookie", Value: cookies}, ut.Header{Key: "X-CSRF-Token", Value: csrf})
	assertStatus(t, created, 201)
	var subject struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(created.Body(), &subject); err != nil {
		t.Fatal(err)
	}

	const count = 20
	var wg sync.WaitGroup
	statuses := make(chan int, count)
	for index := 0; index < count; index++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			response := performJSON(h, "POST", "/api/v1/subjects/"+subject.Data.ID+"/chapters", `{"title":"Chapter `+strconv.Itoa(index)+`","content":""}`,
				ut.Header{Key: "Cookie", Value: cookies}, ut.Header{Key: "X-CSRF-Token", Value: csrf})
			statuses <- response.StatusCode()
		}(index)
	}
	wg.Wait()
	close(statuses)
	for status := range statuses {
		if status != 201 {
			t.Fatalf("concurrent chapter status = %d", status)
		}
	}

	listed := performJSON(h, "GET", "/api/v1/subjects/"+subject.Data.ID, `{}`, ut.Header{Key: "Cookie", Value: cookies})
	assertStatus(t, listed, 200)
	var result struct {
		Data struct {
			Chapters []struct {
				Position int `json:"position"`
			} `json:"chapters"`
		} `json:"data"`
	}
	if err := json.Unmarshal(listed.Body(), &result); err != nil {
		t.Fatal(err)
	}
	positions := make(map[int]struct{}, count)
	for _, chapter := range result.Data.Chapters {
		if _, duplicate := positions[chapter.Position]; duplicate {
			t.Fatalf("concurrent chapter position %d is duplicated: %s", chapter.Position, listed.Body())
		}
		positions[chapter.Position] = struct{}{}
	}
}

func TestDatabaseTableLayoutAndConfigurationAccess(t *testing.T) {
	_ = newTestApp(t)
	ctx := context.Background()
	pool, err := database.Open(ctx, os.Getenv("TEST_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	roles, err := database.ListRoles(ctx, pool)
	if err != nil || len(roles) != 2 {
		t.Fatalf("roles = %#v, err = %v", roles, err)
	}
	configurationID, err := database.InsertConfiguration(ctx, pool, database.Configuration{Key1: "lapin", Key2: "study", Key3: "mode", Value: "focus"})
	if err != nil {
		t.Fatal(err)
	}
	configuration, err := database.FindConfigurationByID(ctx, pool, configurationID)
	if err != nil || configuration.Value != "focus" {
		t.Fatalf("configuration = %#v, err = %v", configuration, err)
	}
	configurations, err := database.ListConfigurationsByKeys(ctx, pool, "lapin", "study", "mode")
	if err != nil || len(configurations) != 1 {
		t.Fatalf("configurations = %#v, err = %v", configurations, err)
	}

	rows, err := pool.Query(ctx, `
		SELECT table_name FROM information_schema.columns
		WHERE table_schema = 'public' AND column_name = 'id'
		  AND data_type = 'bigint' AND is_identity = 'YES'
	`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	identityTables := make(map[string]bool)
	for rows.Next() {
		var table string
		if err := rows.Scan(&table); err != nil {
			t.Fatal(err)
		}
		identityTables[table] = true
	}
	for _, table := range []string{"users", "roles", "user_roles", "configurations", "sessions", "access_tokens", "subjects", "chapters", "tags", "annotations", "whiteboards", "comments"} {
		if !identityTables[table] {
			t.Errorf("%s.id is not a BIGINT identity", table)
		}
	}
}

func TestHTTPRoutesUseOnlyGetAndPost(t *testing.T) {
	h := newTestApp(t)
	for _, route := range h.Engine.Routes() {
		if route.Method != "GET" && route.Method != "POST" {
			t.Fatalf("route %s %s violates the GET/POST-only API convention", route.Method, route.Path)
		}
	}
}

func TestEmbeddedWebAndSPAFallback(t *testing.T) {
	h := newTestApp(t)
	root := performJSON(h, "GET", "/", `{}`)
	assertStatus(t, root, 200)
	if policy := string(root.Header.Peek("Content-Security-Policy")); !strings.Contains(policy, "img-src 'self' data: blob: https:") {
		t.Fatalf("CSP must allow whiteboard-generated blob images: %q", policy)
	}
	if policy := string(root.Header.Peek("Content-Security-Policy")); !strings.Contains(policy, "font-src 'self' data: https://esm.sh") {
		t.Fatalf("CSP must allow Excalidraw's self-hosted font fallback declaration: %q", policy)
	}
	if !strings.Contains(string(root.Body()), `<div id="app"></div>`) {
		t.Fatalf("embedded index is missing the Vue mount point: %s", root.Body())
	}
	assetPath := regexp.MustCompile(`src="(/assets/[^"]+\.js)"`).FindSubmatch(root.Body())
	if len(assetPath) != 2 {
		t.Fatalf("embedded index has no JavaScript asset: %s", root.Body())
	}
	assertStatus(t, performJSON(h, "GET", string(assetPath[1]), `{}`), 200)
	assertStatus(t, performJSON(h, "GET", "/subjects/example-hashid", `{}`), 200)
	assertStatus(t, performJSON(h, "GET", "/api/v1/does-not-exist", `{}`), 404)
}

func newTestApp(t *testing.T) *httpapi.App {
	t.Helper()
	app, _ := newTestAppWithAssetDir(t)
	return app
}

func newTestAppWithAssetDir(t *testing.T) (*httpapi.App, string) {
	t.Helper()
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	pool, err := database.Open(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if err := database.ResetForTest(ctx, pool); err != nil {
		t.Fatal(err)
	}
	if err := database.Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	if err := database.Migrate(ctx, pool); err != nil {
		t.Fatalf("second migration run must be idempotent: %v", err)
	}
	assetDir := t.TempDir()
	assets, err := assetstore.New(assetDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = assets.Close() })
	return httpapi.New(pool, httpapi.Options{AssetStore: assets}), assetDir
}

func testIDCodec(t *testing.T) *identifier.Codec {
	t.Helper()
	codec, err := identifier.New(identifier.DefaultSalt)
	if err != nil {
		t.Fatal(err)
	}
	return codec
}

func authState(t *testing.T, response *protocol.Response) (string, string) {
	t.Helper()
	var payload struct {
		Data struct {
			CSRFToken string `json:"csrf_token"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body(), &payload); err != nil {
		t.Fatal(err)
	}
	return cookieHeader(response), payload.Data.CSRFToken
}

func assertStatus(t *testing.T, response *protocol.Response, expected int) {
	t.Helper()
	if response.StatusCode() != expected {
		t.Fatalf("status = %d, want %d, body = %s", response.StatusCode(), expected, response.Body())
	}
}

func performJSON(h *httpapi.App, method, path, body string, headers ...ut.Header) *protocol.Response {
	bodyBytes := []byte(body)
	allHeaders := append([]ut.Header{{Key: "Content-Type", Value: "application/json"}}, headers...)
	return ut.PerformRequest(h.Engine, method, path, &ut.Body{Body: bytes.NewReader(bodyBytes), Len: len(bodyBytes)}, allHeaders...).Result()
}

func validWhiteboardJSON(chapterID string) string {
	return `{"data":{"version":3,"anchor":{"type":"chapter","id":"` + chapterID + `","content_revision":"sha256:test-content"},"space":{"width":960,"height":640,"fit":"contain"},"document":{"type":"excalidraw","version":2,"elements":[` +
		`{"x":24,"y":32,"id":"line-1","link":null,"seed":895096463,"type":"freedraw","angle":0,"index":"a0","width":120,"height":20,"locked":false,"points":[[0,0],[120,20]],"frameId":null,"opacity":100,"updated":1786895711611,"version":4,"groupIds":[],"fillStyle":"solid","isDeleted":false,"pressures":[],"roughness":1,"roundness":null,"strokeColor":"#1e1e1e","strokeStyle":"solid","strokeWidth":2,"versionNonce":1025703631,"boundElements":null,"backgroundColor":"transparent","simulatePressure":true,"lastCommittedPoint":[120,20]},` +
		`{"x":180,"y":64,"id":"shape-1","link":null,"seed":1832806106,"type":"rectangle","angle":0,"index":"a1","width":110,"height":70,"locked":false,"frameId":null,"opacity":100,"updated":1786897837352,"version":3,"groupIds":[],"fillStyle":"solid","isDeleted":false,"roughness":1,"roundness":{"type":3},"strokeColor":"#1e1e1e","strokeStyle":"solid","strokeWidth":2,"versionNonce":97133318,"boundElements":null,"backgroundColor":"transparent"},` +
		`{"x":340,"y":96,"id":"text-1","link":null,"seed":123456789,"type":"text","angle":0,"index":"a2","width":80,"height":25,"locked":false,"frameId":null,"opacity":100,"updated":1786897837353,"version":2,"groupIds":[],"fillStyle":"solid","isDeleted":false,"roughness":1,"roundness":null,"strokeColor":"#1e1e1e","strokeStyle":"solid","strokeWidth":2,"versionNonce":123456790,"boundElements":null,"backgroundColor":"transparent","text":"正文","fontSize":20,"fontFamily":1,"textAlign":"left","verticalAlign":"top","containerId":null,"originalText":"正文","autoResize":true,"lineHeight":1.25}` +
		`],"appState":{"viewBackgroundColor":"transparent"},"files":{}}}}`
}

func cookieHeader(response *protocol.Response) string {
	cookies := make([]string, 0, 2)
	response.Header.VisitAllCookie(func(_, value []byte) {
		cookies = append(cookies, strings.SplitN(string(value), ";", 2)[0])
	})
	return strings.Join(cookies, "; ")
}
