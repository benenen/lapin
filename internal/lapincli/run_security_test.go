package lapincli

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunUsesLocalManifestMetadataForSuccessOutput(t *testing.T) {
	root := t.TempDir()
	manifestPath := filepath.Join(root, "course.json")
	writeTestFile(t, manifestPath, []byte(`{"version":1,"external_id":"local-course","title":"Local title","tags":[],"chapters":[]}`))
	const token = "lpn_success-metadata-secret"
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = writer.Write([]byte(`{"data":{"id":"safeSubjectID","external_id":"` + token + `","title":"` + token + `","chapters":["` + token + `"]}}`))
	}))
	defer server.Close()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := Run(context.Background(), []string{"course", "import", "--manifest", manifestPath}, Dependencies{
		LookupEnv: func(key string) (string, bool) {
			if key == "LAPIN_ACCESS_TOKEN" {
				return token, true
			}
			if key == "LAPIN_BASE_URL" {
				return server.URL, true
			}
			return "", false
		},
		HTTPClient: server.Client(),
		Stdout:     &stdout,
		Stderr:     &stderr,
	})

	if exitCode != exitSuccess {
		t.Fatalf("exit code = %d, stderr = %q", exitCode, stderr.String())
	}
	if strings.Contains(stdout.String(), token) || strings.Contains(stderr.String(), token) {
		t.Fatalf("output leaked token: stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
	want := "{\"subject_id\":\"safeSubjectID\",\"external_id\":\"local-course\",\"title\":\"Local title\",\"chapter_count\":0}\n"
	if stdout.String() != want {
		t.Fatalf("stdout = %q, want %q", stdout.String(), want)
	}
}
