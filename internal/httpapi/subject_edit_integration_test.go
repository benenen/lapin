package httpapi_test

import (
	"encoding/json"
	"testing"

	"github.com/cloudwego/hertz/pkg/common/ut"
)

func TestSubjectOwnerCanEditSubjectAndChapter(t *testing.T) {
	h := newTestApp(t)
	aliceResponse := performJSON(h, "POST", "/api/v1/auth/register", `{"email":"alice@example.com","name":"Alice","password":"correct horse battery staple"}`)
	assertStatus(t, aliceResponse, 201)
	aliceCookies, aliceCSRF := authState(t, aliceResponse)

	created := performJSON(h, "POST", "/api/v1/subjects", `{"title":"Original subject","description":"Original description","tags":[],"chapters":[{"title":"Original chapter","content":"# Original"}]}`,
		ut.Header{Key: "Cookie", Value: aliceCookies}, ut.Header{Key: "X-CSRF-Token", Value: aliceCSRF})
	assertStatus(t, created, 201)
	var createdPayload struct {
		Data struct {
			ID       string `json:"id"`
			Chapters []struct {
				ID string `json:"id"`
			} `json:"chapters"`
		} `json:"data"`
	}
	if err := json.Unmarshal(created.Body(), &createdPayload); err != nil {
		t.Fatal(err)
	}
	subjectID := createdPayload.Data.ID
	chapterID := createdPayload.Data.Chapters[0].ID

	updatedSubject := performJSON(h, "POST", "/api/v1/subjects/"+subjectID+"/update", `{"title":" Updated subject ","description":"Updated description"}`,
		ut.Header{Key: "Cookie", Value: aliceCookies}, ut.Header{Key: "X-CSRF-Token", Value: aliceCSRF})
	assertStatus(t, updatedSubject, 200)
	updatedChapter := performJSON(h, "POST", "/api/v1/chapters/"+chapterID+"/update", `{"title":" Updated chapter ","content":"## Updated\n\n$E = mc^2$"}`,
		ut.Header{Key: "Cookie", Value: aliceCookies}, ut.Header{Key: "X-CSRF-Token", Value: aliceCSRF})
	assertStatus(t, updatedChapter, 200)

	response := performJSON(h, "GET", "/api/v1/subjects/"+subjectID, `{}`, ut.Header{Key: "Cookie", Value: aliceCookies})
	assertStatus(t, response, 200)
	var fetched struct {
		Data struct {
			Title       string `json:"title"`
			Description string `json:"description"`
			Chapters    []struct {
				ID      string `json:"id"`
				Title   string `json:"title"`
				Content string `json:"content"`
			} `json:"chapters"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body(), &fetched); err != nil {
		t.Fatal(err)
	}
	if fetched.Data.Title != "Updated subject" || fetched.Data.Description != "Updated description" {
		t.Fatalf("updated subject = %#v", fetched.Data)
	}
	if len(fetched.Data.Chapters) != 1 || fetched.Data.Chapters[0].ID != chapterID || fetched.Data.Chapters[0].Title != "Updated chapter" || fetched.Data.Chapters[0].Content != "## Updated\n\n$E = mc^2$" {
		t.Fatalf("updated chapter = %#v", fetched.Data.Chapters)
	}

	bobResponse := performJSON(h, "POST", "/api/v1/auth/register", `{"email":"bob@example.com","name":"Bob","password":"correct horse battery staple"}`)
	assertStatus(t, bobResponse, 201)
	bobCookies, bobCSRF := authState(t, bobResponse)
	assertStatus(t, performJSON(h, "POST", "/api/v1/subjects/"+subjectID+"/update", `{"title":"Stolen","description":""}`,
		ut.Header{Key: "Cookie", Value: bobCookies}, ut.Header{Key: "X-CSRF-Token", Value: bobCSRF}), 404)
	assertStatus(t, performJSON(h, "POST", "/api/v1/chapters/"+chapterID+"/update", `{"title":"Stolen","content":""}`,
		ut.Header{Key: "Cookie", Value: bobCookies}, ut.Header{Key: "X-CSRF-Token", Value: bobCSRF}), 404)
	assertStatus(t, performJSON(h, "POST", "/api/v1/subjects/"+subjectID+"/update", `{"title":"No CSRF","description":""}`,
		ut.Header{Key: "Cookie", Value: aliceCookies}), 403)
	assertStatus(t, performJSON(h, "POST", "/api/v1/subjects/"+subjectID+"/update", `{"title":"","description":""}`,
		ut.Header{Key: "Cookie", Value: aliceCookies}, ut.Header{Key: "X-CSRF-Token", Value: aliceCSRF}), 400)
	assertStatus(t, performJSON(h, "POST", "/api/v1/chapters/"+chapterID+"/update", `{"title":"","content":""}`,
		ut.Header{Key: "Cookie", Value: aliceCookies}, ut.Header{Key: "X-CSRF-Token", Value: aliceCSRF}), 400)
	assertStatus(t, performJSON(h, "POST", "/api/v1/subjects/not-a-hashid/update", `{"title":"Invalid","description":""}`,
		ut.Header{Key: "Cookie", Value: aliceCookies}, ut.Header{Key: "X-CSRF-Token", Value: aliceCSRF}), 400)
	assertStatus(t, performJSON(h, "POST", "/api/v1/chapters/not-a-hashid/update", `{"title":"Invalid","content":""}`,
		ut.Header{Key: "Cookie", Value: aliceCookies}, ut.Header{Key: "X-CSRF-Token", Value: aliceCSRF}), 400)
}
