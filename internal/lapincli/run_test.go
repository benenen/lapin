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
	"testing"
)

func TestRunImportsMarkdownCourse(t *testing.T) {
	root := t.TempDir()
	chaptersDir := filepath.Join(root, "chapters")
	if err := os.Mkdir(chaptersDir, 0o700); err != nil {
		t.Fatal(err)
	}
	markdown := "## 基础语法\n\n从 **package**、变量与函数开始。\n"
	if err := os.WriteFile(filepath.Join(chaptersDir, "syntax.md"), []byte(markdown), 0o600); err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(root, "course.json")
	manifest := `{
  "version": 1,
  "external_id": "go-101",
  "title": "Go 入门",
  "description": "从语法到服务",
  "tags": ["Go", "后端"],
  "chapters": [{
    "external_id": "part-1",
    "title": "语言基础",
    "children": [{
      "external_id": "syntax",
      "title": "基础语法",
      "content_file": "chapters/syntax.md"
    }]
  }]
}`
	if err := os.WriteFile(manifestPath, []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}

	const token = "lpn_test-token"
	var received importSubjectRequest
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", request.Method)
		}
		if request.URL.Path != "/openapi/v1/subjects/import" {
			t.Errorf("path = %s", request.URL.Path)
		}
		if got := request.Header.Get("Authorization"); got != "Bearer "+token {
			t.Errorf("Authorization = %q", got)
		}
		if got := request.Header.Get("Content-Type"); got != "application/json" {
			t.Errorf("Content-Type = %q", got)
		}
		if err := json.NewDecoder(request.Body).Decode(&received); err != nil {
			t.Errorf("decode request: %v", err)
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"data":{"id":"subjectHash","external_id":"go-101","title":"Go 入门","chapters":[{"id":"part"},{"id":"syntax"}]}}`))
	}))
	defer server.Close()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := Run(context.Background(), []string{"course", "import", "--manifest", manifestPath}, Dependencies{
		LookupEnv: func(key string) (string, bool) {
			switch key {
			case "LAPIN_ACCESS_TOKEN":
				return token, true
			case "LAPIN_BASE_URL":
				return server.URL, true
			default:
				return "", false
			}
		},
		HTTPClient: server.Client(),
		Stdout:     &stdout,
		Stderr:     &stderr,
	})

	if exitCode != 0 {
		t.Fatalf("exit code = %d, stderr = %q", exitCode, stderr.String())
	}
	if got, want := stdout.String(), "{\"subject_id\":\"subjectHash\",\"external_id\":\"go-101\",\"title\":\"Go 入门\",\"chapter_count\":2}\n"; got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
	if received.ExternalID != "go-101" || received.Title != "Go 入门" || len(received.Chapters) != 1 {
		t.Fatalf("request metadata = %#v", received)
	}
	parent := received.Chapters[0]
	if parent.Content != "" || len(parent.Children) != 1 {
		t.Fatalf("parent chapter = %#v", parent)
	}
	child := parent.Children[0]
	if child.ExternalID != "syntax" || child.Content != markdown {
		t.Fatalf("child chapter = %#v", child)
	}
}

func TestRunRequiresAccessTokenBeforeReadingManifest(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := Run(context.Background(), []string{"course", "import", "--manifest", "/does/not/exist.json"}, Dependencies{
		LookupEnv: func(string) (string, bool) { return "", false },
		Stdout:    &stdout,
		Stderr:    &stderr,
	})

	if exitCode != exitUsage {
		t.Fatalf("exit code = %d, want %d", exitCode, exitUsage)
	}
	if !strings.Contains(stderr.String(), "LAPIN_ACCESS_TOKEN is required") || strings.Contains(stderr.String(), "does/not/exist") {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestRunRejectsTokenFlagsWithoutEchoingTheirValues(t *testing.T) {
	const commandLineToken = "lpn_command-line-secret"
	var stderr bytes.Buffer
	exitCode := Run(context.Background(), []string{"course", "import", "--token=" + commandLineToken, "--manifest", "course.json"}, Dependencies{
		LookupEnv: func(key string) (string, bool) {
			if key == "LAPIN_ACCESS_TOKEN" {
				return "lpn_environment-token", true
			}
			return "", false
		},
		Stdout: &bytes.Buffer{},
		Stderr: &stderr,
	})

	if exitCode != exitUsage {
		t.Fatalf("exit code = %d, want %d", exitCode, exitUsage)
	}
	if strings.Contains(stderr.String(), commandLineToken) {
		t.Fatalf("stderr leaked token: %q", stderr.String())
	}
}

func TestRunHelpDoesNotRequireAccessToken(t *testing.T) {
	var stdout bytes.Buffer
	exitCode := Run(context.Background(), []string{"help"}, Dependencies{
		LookupEnv: func(string) (string, bool) { return "", false },
		Stdout:    &stdout,
		Stderr:    &bytes.Buffer{},
	})
	if exitCode != exitSuccess || !strings.Contains(stdout.String(), "lapin-cli course import") {
		t.Fatalf("exit code = %d, stdout = %q", exitCode, stdout.String())
	}
}
