package lapincli

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const stagedOnePixelPNGBase64 = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII="

func TestRunImportsV2BundleWithAssetsAndChapterBatches(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "assets"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "chapters"), 0o700); err != nil {
		t.Fatal(err)
	}
	png, err := base64.StdEncoding.DecodeString(stagedOnePixelPNGBase64)
	if err != nil {
		t.Fatal(err)
	}
	assetEntries := make([]string, 0, 17)
	for index := 1; index <= 17; index++ {
		filename := fmt.Sprintf("pixel-%02d.png", index)
		writeTestFile(t, filepath.Join(root, "assets", filename), png)
		assetEntries = append(assetEntries, fmt.Sprintf(`{"key":"pixel-%02d","file":"assets/%s"}`, index, filename))
	}
	for index := 1; index <= 6; index++ {
		content := strings.Repeat(string(rune('a'+index-1)), 180_000)
		if index == 1 {
			content = "![Pixel](lapin-asset://pixel-01)\n\n" + content
		}
		writeTestFile(t, filepath.Join(root, "chapters", string(rune('0'+index))+".md"), []byte(content))
	}
	manifest := `{
  "version":2,"external_id":"large-course","title":"Large Course","tags":["PDF"],
  "assets":[` + strings.Join(assetEntries, ",") + `],
  "chapters":[
    {"external_id":"one","title":"One","content_file":"chapters/1.md"},
    {"external_id":"two","title":"Two","content_file":"chapters/2.md"},
    {"external_id":"three","title":"Three","content_file":"chapters/3.md"},
    {"external_id":"four","title":"Four","content_file":"chapters/4.md"},
    {"external_id":"five","title":"Five","content_file":"chapters/5.md"},
    {"external_id":"six","title":"Six","content_file":"chapters/6.md"}
  ]
}`
	manifestPath := filepath.Join(root, "course.json")
	writeTestFile(t, manifestPath, []byte(manifest))

	var paths []string
	var chapterBatchSizes []int
	var assetBatchSizes []int
	var receivedContent string
	assetSequence := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "Bearer lpn_staged-test" {
			t.Errorf("Authorization = %q", request.Header.Get("Authorization"))
		}
		paths = append(paths, request.URL.Path)
		writer.Header().Set("Content-Type", "application/json")
		switch {
		case request.URL.Path == "/openapi/v1/subject-imports":
			_, _ = writer.Write([]byte(`{"data":{"id":"importHash1","status":"draft"}}`))
		case request.URL.Path == "/openapi/v1/subject-imports/importHash1/assets":
			if err := request.ParseMultipartForm(12 << 20); err != nil {
				t.Errorf("parse multipart: %v", err)
			}
			count := len(request.MultipartForm.Value["key"])
			assetBatchSizes = append(assetBatchSizes, count)
			views := make([]map[string]any, 0, count)
			for index := 0; index < count; index++ {
				key := request.MultipartForm.Value["key"][index]
				files := request.MultipartForm.File["file"]
				if index >= len(files) {
					t.Fatalf("missing file %d", index)
				}
				file, err := files[index].Open()
				if err != nil {
					t.Errorf("asset file: %v", err)
					continue
				}
				body, _ := io.ReadAll(file)
				_ = file.Close()
				if !bytes.Equal(body, png) {
					t.Error("asset bytes changed")
				}
				assetSequence++
				id := fmt.Sprintf("assetHash%02d", assetSequence)
				views = append(views, map[string]any{"key": key, "id": id, "url": "/api/v1/assets/" + id + "/content"})
			}
			_ = json.NewEncoder(writer).Encode(map[string]any{"data": map[string]any{"assets": views}})
		case strings.HasSuffix(request.URL.Path, "/chapters"):
			body, _ := io.ReadAll(request.Body)
			chapterBatchSizes = append(chapterBatchSizes, len(body))
			var payload struct {
				Chapters []struct {
					Content string `json:"content"`
				} `json:"chapters"`
			}
			if err := json.Unmarshal(body, &payload); err != nil {
				t.Errorf("decode chapters: %v", err)
			}
			for _, chapter := range payload.Chapters {
				receivedContent += chapter.Content
			}
			_, _ = writer.Write([]byte(`{"data":{"status":"draft"}}`))
		case strings.HasSuffix(request.URL.Path, "/commit"):
			_, _ = writer.Write([]byte(`{"data":{"import":{"id":"importHash1","status":"committed"},"subject":{"id":"subjectHash","external_id":"large-course","title":"Large Course"}}}`))
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := Run(context.Background(), []string{"course", "import", "--manifest", manifestPath}, Dependencies{
		LookupEnv: func(key string) (string, bool) {
			if key == "LAPIN_ACCESS_TOKEN" {
				return "lpn_staged-test", true
			}
			if key == "LAPIN_BASE_URL" {
				return server.URL, true
			}
			return "", false
		},
		HTTPClient: server.Client(), Stdout: &stdout, Stderr: &stderr,
	})
	if exitCode != exitSuccess {
		t.Fatalf("exit = %d, stderr = %q", exitCode, stderr.String())
	}
	if len(chapterBatchSizes) < 2 {
		t.Fatalf("chapter batches = %v, want at least 2", chapterBatchSizes)
	}
	if fmt.Sprint(assetBatchSizes) != "[16 1]" {
		t.Fatalf("asset batches = %v, want [16 1]", assetBatchSizes)
	}
	for _, size := range chapterBatchSizes {
		if size >= maxRequestBytes {
			t.Fatalf("chapter batch size = %d", size)
		}
	}
	if strings.Contains(receivedContent, "lapin-asset://") || !strings.Contains(receivedContent, "/api/v1/assets/assetHash01/content") {
		t.Fatal("asset placeholder was not replaced")
	}
	if paths[len(paths)-1] != "/openapi/v1/subject-imports/importHash1/commit" {
		t.Fatalf("last path = %q", paths[len(paths)-1])
	}
	if stdout.String() != "{\"subject_id\":\"subjectHash\",\"external_id\":\"large-course\",\"title\":\"Large Course\",\"chapter_count\":6}\n" {
		t.Fatalf("stdout = %q", stdout.String())
	}
}
