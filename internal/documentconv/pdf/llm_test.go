package pdf

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// tinyPNG is a 1x1 PNG that the fake pdftoppm writes for every requested page.
var tinyPNG = []byte{
	0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a,
	0x00, 0x00, 0x00, 0x0d, 'I', 'H', 'D', 'R',
	0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
	0x08, 0x06, 0x00, 0x00, 0x00, 0x1f, 0x15, 0xc4,
	0x89, 0x00, 0x00, 0x00, 0x0a, 'I', 'D', 'A', 'T',
	0x78, 0x9c, 0x63, 0x00, 0x01, 0x00, 0x00, 0x05,
	0x00, 0x01, 0x0d, 0x0a, 0x2d, 0xb4, 0x00, 0x00,
	0x00, 0x00, 'I', 'E', 'N', 'D', 0xae, 0x42, 0x60, 0x82,
}

// fakePopplerBin installs pdftotext and pdftoppm stubs on PATH. pdftotext writes
// pages separated by the form feed that poppler emits; pdftoppm writes one PNG
// for whichever page range it is asked for.
func fakePopplerBin(t *testing.T, pageTexts []string) string {
	t.Helper()
	binDirectory := t.TempDir()

	var pages strings.Builder
	for _, text := range pageTexts {
		pages.WriteString(text)
		pages.WriteString("\f")
	}
	textPath := filepath.Join(binDirectory, "pages.txt")
	if err := os.WriteFile(textPath, []byte(pages.String()), 0o600); err != nil {
		t.Fatal(err)
	}

	writeScript(t, filepath.Join(binDirectory, "pdftotext"),
		"#!/bin/sh\ncp "+textPath+" \"$4\"\n")
	// pdftoppm is called as: pdftoppm -png -r DPI -f N -l N source prefix
	pngPath := filepath.Join(binDirectory, "page.png")
	if err := os.WriteFile(pngPath, tinyPNG, 0o600); err != nil {
		t.Fatal(err)
	}
	writeScript(t, filepath.Join(binDirectory, "pdftoppm"),
		"#!/bin/sh\nprefix=\"${9}\"\ncp "+pngPath+" \"$prefix-01.png\"\n")

	t.Setenv("PATH", binDirectory+string(os.PathListSeparator)+os.Getenv("PATH"))
	return binDirectory
}

func writeScript(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o700); err != nil {
		t.Fatal(err)
	}
}

func writeFakePDF(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "source.pdf")
	if err := os.WriteFile(path, []byte("%PDF-1.4 fake"), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

type recordedRequest struct {
	Model    string `json:"model"`
	Caller   string `json:"caller"`
	Messages []struct {
		Role    string          `json:"role"`
		Content json.RawMessage `json:"content"`
	} `json:"messages"`
}

// stubGateway answers the outline call with outlineReply and every page call with
// the pageReplies entry whose key appears in that page's extracted text. Pages are
// converted concurrently, so replies must be matched by content, never by call order.
func stubGateway(t *testing.T, outlineReply string, pageReplies map[string]string) (*httptest.Server, *[]recordedRequest) {
	t.Helper()
	recorded := &[]recordedRequest{}
	var mutex sync.Mutex
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		var decoded recordedRequest
		if err := json.NewDecoder(request.Body).Decode(&decoded); err != nil {
			http.Error(writer, err.Error(), http.StatusBadRequest)
			return
		}
		mutex.Lock()
		*recorded = append(*recorded, decoded)
		mutex.Unlock()

		user := messageText(t, decoded.Messages[1].Content)
		reply := outlineReply
		if strings.Contains(user, "<pageContent>") {
			reply = ""
			for marker, pageReply := range pageReplies {
				if strings.Contains(pageContentOf(user), marker) {
					reply = pageReply
					break
				}
			}
		}
		writer.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(writer).Encode(map[string]any{
			"choices": []any{map[string]any{"message": map[string]any{"content": reply}}},
		})
	}))
	t.Cleanup(server.Close)
	return server, recorded
}

// pageContentOf isolates the <pageContent> block so that a marker appearing only in
// the neighbouring-page context does not select the wrong reply.
func pageContentOf(user string) string {
	start := strings.Index(user, "<pageContent>")
	end := strings.Index(user, "</pageContent>")
	if start < 0 || end < start {
		return ""
	}
	return user[start:end]
}

func newTestLLMConverter(t *testing.T, baseURL string, options ...func(*LLMOptions)) *LLMConverter {
	t.Helper()
	llmOptions := LLMOptions{
		BaseURL: baseURL,
		Model:   "test-vision",
		Caller:  "lapin_test",
	}
	for _, apply := range options {
		apply(&llmOptions)
	}
	converter, err := NewLLMConverter(llmOptions)
	if err != nil {
		t.Fatal(err)
	}
	return converter
}

func TestLLMConvertSplitsChaptersAndDemotesHeadingsToBundleLevels(t *testing.T) {
	fakePopplerBin(t, []string{"page one text", "page two text"})
	server, _ := stubGateway(t, "1\t第 1 章 入门\n1\t第 2 章 进阶", map[string]string{
		"page one text": "# 第 1 章 入门\n\n## 1.1 起步\n\n正文一。",
		"page two text": "# 第 2 章 进阶\n\n正文二。",
	})
	converter := newTestLLMConverter(t, server.URL, func(options *LLMOptions) {
		options.Profile = ChineseTechnicalBookProfile()
	})

	document, err := converter.Convert(context.Background(), writeFakePDF(t))
	if err != nil {
		t.Fatalf("Convert() error = %v", err)
	}
	if len(document.Chapters) != 2 {
		t.Fatalf("Chapters = %d, want 2: %+v", len(document.Chapters), document.Chapters)
	}
	first := document.Chapters[0]
	if first.ExternalID != "chapter-01" || first.Title != "第 1 章 入门" {
		t.Fatalf("first chapter = %q/%q, want chapter-01/第 1 章 入门", first.ExternalID, first.Title)
	}
	// The bundle convention is "## chapter, ### and deeper for subsections".
	if !strings.HasPrefix(first.Markdown, "## 第 1 章 入门") {
		t.Fatalf("chapter Markdown must start at level two, got:\n%s", first.Markdown)
	}
	if !strings.Contains(first.Markdown, "### 1.1 起步") {
		t.Fatalf("subsection was not demoted with its chapter, got:\n%s", first.Markdown)
	}
	if document.Chapters[1].ExternalID != "chapter-02" {
		t.Fatalf("second chapter external ID = %q", document.Chapters[1].ExternalID)
	}
}

func TestLLMConvertSendsOutlineAndNeighbouringPageContext(t *testing.T) {
	fakePopplerBin(t, []string{"表头在这一页", "数据行在这一页", "第三页"})
	server, recorded := stubGateway(t, "1\t标题", map[string]string{
		"表头在这一页":  "# 标题",
		"数据行在这一页": "内容二",
		"第三页":     "内容三",
	})
	converter := newTestLLMConverter(t, server.URL)

	if _, err := converter.Convert(context.Background(), writeFakePDF(t)); err != nil {
		t.Fatalf("Convert() error = %v", err)
	}
	if len(*recorded) != 4 {
		t.Fatalf("gateway calls = %d, want 1 outline + 3 pages", len(*recorded))
	}

	// The outline call is text-only and must carry every page's text.
	outlineUser := messageText(t, (*recorded)[0].Messages[1].Content)
	for _, want := range []string{"表头在这一页", "数据行在这一页", "第三页"} {
		if !strings.Contains(outlineUser, want) {
			t.Fatalf("outline call missing %q", want)
		}
	}

	// Page two must see page one's tail and page three's head.
	pageTwo := findPageRequest(t, *recorded, "数据行在这一页")
	for _, want := range []string{
		"<documentOutline>", "标题",
		"<previousPageTail>", "表头在这一页",
		"<nextPageHead>", "第三页",
	} {
		if !strings.Contains(pageTwo, want) {
			t.Fatalf("page two request missing %q in:\n%s", want, pageTwo)
		}
	}
}

func TestLLMConvertSendsRenderedPageImageAndSystemRules(t *testing.T) {
	fakePopplerBin(t, []string{"only page"})
	server, recorded := stubGateway(t, "1\t标题", map[string]string{"only page": "# 标题\n\n正文"})
	converter := newTestLLMConverter(t, server.URL)

	if _, err := converter.Convert(context.Background(), writeFakePDF(t)); err != nil {
		t.Fatalf("Convert() error = %v", err)
	}
	pageCall := (*recorded)[1]
	if pageCall.Model != "test-vision" || pageCall.Caller != "lapin_test" {
		t.Fatalf("page call model/caller = %q/%q", pageCall.Model, pageCall.Caller)
	}
	system := messageText(t, pageCall.Messages[0].Content)
	if !strings.Contains(system, "NO HTML") || !strings.Contains(system, "NEVER invent an image path") {
		t.Fatalf("system message is not the bundled prompt: %s", system)
	}
	user := messageText(t, pageCall.Messages[1].Content)
	if !strings.Contains(user, "data:image/png;base64,") {
		t.Fatalf("page call carried no rendered image: %s", user)
	}
}

func TestLLMConvertReportsPageFailureRatherThanSilentlyDroppingContent(t *testing.T) {
	fakePopplerBin(t, []string{"page one"})
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		var decoded recordedRequest
		_ = json.NewDecoder(request.Body).Decode(&decoded)
		if len(decoded.Messages) > 0 && strings.Contains(string(decoded.Messages[0].Content), "NO HTML") {
			http.Error(writer, "upstream exploded", http.StatusInternalServerError)
			return
		}
		writer.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(writer).Encode(map[string]any{
			"choices": []any{map[string]any{"message": map[string]any{"content": "1\t标题"}}},
		})
	}))
	t.Cleanup(server.Close)

	converter := newTestLLMConverter(t, server.URL, func(options *LLMOptions) {
		options.MaxAttempts = 1
	})
	if _, err := converter.Convert(context.Background(), writeFakePDF(t)); err == nil {
		t.Fatal("Convert() succeeded even though a page conversion failed")
	}
}

func TestNewLLMConverterRejectsUnusableConfiguration(t *testing.T) {
	for name, options := range map[string]LLMOptions{
		"no base URL":      {Model: "m"},
		"no model":         {BaseURL: "http://127.0.0.1:1"},
		"bad scheme":       {BaseURL: "file:///etc/passwd", Model: "m"},
		"oversized DPI":    {BaseURL: "http://127.0.0.1:1", Model: "m", ImageDPI: 10_000},
		"negative workers": {BaseURL: "http://127.0.0.1:1", Model: "m", MaxWorkers: -1},
	} {
		if _, err := NewLLMConverter(options); err == nil {
			t.Fatalf("NewLLMConverter accepted %s", name)
		}
	}
}

// messageText flattens a chat message body, which is either a plain string or the
// multi-part array a page call sends.
func messageText(t *testing.T, raw json.RawMessage) string {
	t.Helper()
	var plain string
	if err := json.Unmarshal(raw, &plain); err == nil {
		return plain
	}
	var parts []struct {
		Type     string `json:"type"`
		Text     string `json:"text"`
		ImageURL *struct {
			URL string `json:"url"`
		} `json:"image_url"`
	}
	if err := json.Unmarshal(raw, &parts); err != nil {
		t.Fatalf("message content is neither a string nor parts: %s", raw)
	}
	var builder strings.Builder
	for _, part := range parts {
		builder.WriteString(part.Text)
		if part.ImageURL != nil {
			builder.WriteString(part.ImageURL.URL)
		}
	}
	return builder.String()
}

// findPageRequest returns the user message of the page call whose extracted text
// contains marker.
func findPageRequest(t *testing.T, recorded []recordedRequest, marker string) string {
	t.Helper()
	for _, request := range recorded {
		if len(request.Messages) < 2 {
			continue
		}
		user := messageText(t, request.Messages[1].Content)
		// Match inside <pageContent> only: a neighbouring page carries this text too.
		if strings.Contains(pageContentOf(user), marker) {
			return user
		}
	}
	t.Fatalf("no page request contained %q", marker)
	return ""
}

func TestLLMConvertWarnsWhenAChapterEndsInsideACodeFence(t *testing.T) {
	fakePopplerBin(t, []string{"page one"})
	// A long shell payload that wraps in the PDF makes the model close the fence
	// mid-command and emit a stray one, leaving the chapter unbalanced.
	server, _ := stubGateway(t, "1\t第 1 章 入门", map[string]string{
		"page one": "# 第 1 章 入门\n\n```bash\ncurl -d '{\"a\":1,\n```\n\n`}'`\n```\n",
	})
	converter := newTestLLMConverter(t, server.URL, func(options *LLMOptions) {
		options.Profile = ChineseTechnicalBookProfile()
	})

	document, err := converter.Convert(context.Background(), writeFakePDF(t))
	if err != nil {
		t.Fatalf("Convert() error = %v", err)
	}
	joined := strings.Join(document.Warnings, "\n")
	if !strings.Contains(joined, "code fence") || !strings.Contains(joined, "chapter-01") {
		t.Fatalf("unbalanced fence was not reported: %+v", document.Warnings)
	}
}
