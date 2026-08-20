package lapincli

import (
	"bytes"
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

// installFakePopplerForLLM puts pdftotext and pdftoppm stubs on PATH. pdftotext
// writes the given pages separated by poppler's form feed; pdftoppm writes a
// one-pixel PNG for whichever page it is asked to render.
func installFakePopplerForLLM(t *testing.T, pageTexts []string) {
	t.Helper()
	binDirectory := t.TempDir()

	textPath := filepath.Join(binDirectory, "pages.txt")
	writeTestFile(t, textPath, []byte(strings.Join(pageTexts, "\f")+"\f"))
	pngPath := filepath.Join(binDirectory, "page.png")
	writeTestFile(t, pngPath, []byte{
		0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a,
		0x00, 0x00, 0x00, 0x0d, 'I', 'H', 'D', 'R',
		0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
		0x08, 0x06, 0x00, 0x00, 0x00, 0x1f, 0x15, 0xc4,
		0x89, 0x00, 0x00, 0x00, 0x0a, 'I', 'D', 'A', 'T',
		0x78, 0x9c, 0x63, 0x00, 0x01, 0x00, 0x00, 0x05,
		0x00, 0x01, 0x0d, 0x0a, 0x2d, 0xb4, 0x00, 0x00,
		0x00, 0x00, 'I', 'E', 'N', 'D', 0xae, 0x42, 0x60, 0x82,
	})

	if err := os.WriteFile(filepath.Join(binDirectory, "pdftotext"),
		[]byte("#!/bin/sh\ncp "+textPath+" \"$4\"\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(binDirectory, "pdftoppm"),
		[]byte("#!/bin/sh\ncp "+pngPath+" \"${9}-01.png\"\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDirectory+string(os.PathListSeparator)+os.Getenv("PATH"))
}

// llmGateway answers the outline call with outlineReply and each page call with the
// reply whose marker appears in that page's <pageContent> block.
func llmGateway(t *testing.T, outlineReply string, pageReplies map[string]string) *httptest.Server {
	t.Helper()
	var mutex sync.Mutex
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		mutex.Lock()
		defer mutex.Unlock()
		var decoded struct {
			Messages []struct {
				Content json.RawMessage `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(request.Body).Decode(&decoded); err != nil {
			http.Error(writer, err.Error(), http.StatusBadRequest)
			return
		}
		var parts []struct {
			Text string `json:"text"`
		}
		user := ""
		if err := json.Unmarshal(decoded.Messages[1].Content, &parts); err == nil {
			for _, part := range parts {
				user += part.Text
			}
		} else {
			_ = json.Unmarshal(decoded.Messages[1].Content, &user)
		}

		reply := outlineReply
		if start := strings.Index(user, "<pageContent>"); start >= 0 {
			reply = ""
			block := user[start:]
			for marker, pageReply := range pageReplies {
				if strings.Contains(block, marker) {
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
	return server
}

func TestRunPreparePDFWithLLMEngineWritesBundleAndReportsWarnings(t *testing.T) {
	installFakePopplerForLLM(t, []string{"chapter one page", "chapter two page"})
	gateway := llmGateway(t, "1\t第 1 章 入门\n1\t第 2 章 进阶", map[string]string{
		"chapter one page": "# 第 1 章 入门\n\n## 1.1 起步\n\n正文一。\n\n*[图: 架构示意]*",
		"chapter two page": "# 第 2 章 进阶\n\n正文二。",
	})

	root := t.TempDir()
	pdfPath := filepath.Join(root, "fixture.pdf")
	writeTestFile(t, pdfPath, []byte("fake pdf"))
	outputDir := filepath.Join(root, "bundle")

	var stdout, stderr bytes.Buffer
	exitCode := Run(context.Background(), []string{
		"course", "prepare-pdf", "--pdf", pdfPath, "--output", outputDir,
		"--external-id", "fixture-pdf", "--title", "Fixture PDF",
		"--engine", "llm", "--llm-base-url", gateway.URL, "--llm-model", "test-vision",
	}, Dependencies{
		LookupEnv: func(string) (string, bool) { return "", false },
		Stdout:    &stdout, Stderr: &stderr,
	})
	if exitCode != exitSuccess {
		t.Fatalf("exit = %d, stderr = %q", exitCode, stderr.String())
	}

	markdown, err := os.ReadFile(filepath.Join(outputDir, "chapters", "chapter-01.md"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(markdown)
	if !strings.HasPrefix(text, "## 第 1 章 入门") || !strings.Contains(text, "### 1.1 起步") {
		t.Fatalf("chapter one was not written at bundle heading levels:\n%s", text)
	}
	if _, err := os.ReadFile(filepath.Join(outputDir, "chapters", "chapter-02.md")); err != nil {
		t.Fatalf("chapter two missing: %v", err)
	}

	var result preparePDFResult
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("decode result: %v (%s)", err, stdout.String())
	}
	if result.Chapters != 2 {
		t.Fatalf("Chapters = %d, want 2", result.Chapters)
	}
	// The LLM engine cannot extract artwork, so the figure placeholder must surface.
	if len(result.Warnings) == 0 || !strings.Contains(strings.Join(result.Warnings, " "), "figure placeholder") {
		t.Fatalf("Warnings did not mention the figure placeholder: %+v", result.Warnings)
	}
}

func TestRunPreparePDFLLMEngineReadsGatewaySettingsFromEnvironment(t *testing.T) {
	installFakePopplerForLLM(t, []string{"only page"})
	gateway := llmGateway(t, "1\t标题", map[string]string{"only page": "# 标题\n\n正文"})

	root := t.TempDir()
	pdfPath := filepath.Join(root, "fixture.pdf")
	writeTestFile(t, pdfPath, []byte("fake pdf"))

	var stdout, stderr bytes.Buffer
	exitCode := Run(context.Background(), []string{
		"course", "prepare-pdf", "--pdf", pdfPath, "--output", filepath.Join(root, "bundle"),
		"--external-id", "fixture-pdf", "--title", "Fixture PDF", "--engine", "llm",
	}, Dependencies{
		LookupEnv: func(name string) (string, bool) {
			switch name {
			case "LAPIN_LLM_BASE_URL":
				return gateway.URL, true
			case "LAPIN_LLM_MODEL":
				return "test-vision", true
			}
			return "", false
		},
		Stdout: &stdout, Stderr: &stderr,
	})
	if exitCode != exitSuccess {
		t.Fatalf("exit = %d, stderr = %q", exitCode, stderr.String())
	}
}

func TestRunPreparePDFLLMEngineRefusesIncompleteConfiguration(t *testing.T) {
	root := t.TempDir()
	pdfPath := filepath.Join(root, "fixture.pdf")
	writeTestFile(t, pdfPath, []byte("fake pdf"))

	var stdout, stderr bytes.Buffer
	exitCode := Run(context.Background(), []string{
		"course", "prepare-pdf", "--pdf", pdfPath, "--output", filepath.Join(root, "bundle"),
		"--external-id", "fixture-pdf", "--title", "Fixture PDF", "--engine", "llm",
	}, Dependencies{
		LookupEnv: func(string) (string, bool) { return "", false },
		Stdout:    &stdout, Stderr: &stderr,
	})
	if exitCode == exitSuccess {
		t.Fatal("prepare-pdf accepted --engine llm without a gateway URL or model")
	}
	if !strings.Contains(stderr.String(), "LAPIN_LLM_BASE_URL") {
		t.Fatalf("stderr did not name the missing setting: %q", stderr.String())
	}
}

func TestRunPreparePDFRejectsUnknownEngine(t *testing.T) {
	root := t.TempDir()
	pdfPath := filepath.Join(root, "fixture.pdf")
	writeTestFile(t, pdfPath, []byte("fake pdf"))

	var stdout, stderr bytes.Buffer
	exitCode := Run(context.Background(), []string{
		"course", "prepare-pdf", "--pdf", pdfPath, "--output", filepath.Join(root, "bundle"),
		"--external-id", "fixture-pdf", "--title", "Fixture PDF", "--engine", "magic",
	}, Dependencies{
		LookupEnv: func(string) (string, bool) { return "", false },
		Stdout:    &stdout, Stderr: &stderr,
	})
	if exitCode == exitSuccess {
		t.Fatal("prepare-pdf accepted an unknown --engine")
	}
}
