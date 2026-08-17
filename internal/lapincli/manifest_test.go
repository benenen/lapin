package lapincli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadManifestPreservesRecursiveOrderAndEmptyParents(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "one.md"), []byte("one\n"))
	writeTestFile(t, filepath.Join(root, "two.md"), []byte("two\n"))
	manifest := `{
  "version":1,"external_id":"tree","title":"Tree","tags":[],
  "chapters":[
    {"external_id":"part","title":"Part","children":[
      {"external_id":"one","title":"One","content_file":"one.md"},
      {"external_id":"two","title":"Two","content_file":"two.md"}
    ]},
    {"external_id":"last","title":"Last"}
  ]
}`
	manifestPath := filepath.Join(root, "course.json")
	writeTestFile(t, manifestPath, []byte(manifest))

	request, _, err := loadManifest(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	if got := []string{request.Chapters[0].ExternalID, request.Chapters[0].Children[0].ExternalID, request.Chapters[0].Children[1].ExternalID, request.Chapters[1].ExternalID}; strings.Join(got, ",") != "part,one,two,last" {
		t.Fatalf("chapter order = %v", got)
	}
	if request.Chapters[0].Content != "" || request.Chapters[1].Content != "" {
		t.Fatalf("empty parent content changed: %#v", request.Chapters)
	}
}

func TestLoadManifestRejectsInvalidDocumentsAndPaths(t *testing.T) {
	tests := []struct {
		name      string
		manifest  string
		prepare   func(t *testing.T, root string)
		wantError string
	}{
		{name: "unknown field", manifest: `{"version":1,"external_id":"x","title":"X","extra":true}`, wantError: "unknown field"},
		{name: "trailing JSON", manifest: `{"version":1,"external_id":"x","title":"X"}{}`, wantError: "trailing"},
		{name: "wrong version", manifest: `{"version":3,"external_id":"x","title":"X"}`, wantError: "version must be 1 or 2"},
		{name: "duplicate external id", manifest: `{"version":1,"external_id":"x","title":"X","chapters":[{"external_id":"same","title":"A"},{"external_id":"same","title":"B"}]}`, wantError: "duplicate chapter"},
		{name: "parent traversal", manifest: `{"version":1,"external_id":"x","title":"X","chapters":[{"external_id":"a","title":"A","content_file":"../outside.md"}]}`, wantError: "stay inside"},
		{name: "absolute path", manifest: `{"version":1,"external_id":"x","title":"X","chapters":[{"external_id":"a","title":"A","content_file":"/etc/passwd"}]}`, wantError: "must be relative"},
		{
			name:      "invalid UTF-8",
			manifest:  `{"version":1,"external_id":"x","title":"X","chapters":[{"external_id":"a","title":"A","content_file":"bad.md"}]}`,
			prepare:   func(t *testing.T, root string) { writeTestFile(t, filepath.Join(root, "bad.md"), []byte{0xff, 0xfe}) },
			wantError: "valid UTF-8",
		},
		{
			name:     "directory",
			manifest: `{"version":1,"external_id":"x","title":"X","chapters":[{"external_id":"a","title":"A","content_file":"docs"}]}`,
			prepare: func(t *testing.T, root string) {
				if err := os.Mkdir(filepath.Join(root, "docs"), 0o700); err != nil {
					t.Fatal(err)
				}
			},
			wantError: "regular file",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			if test.prepare != nil {
				test.prepare(t, root)
			}
			manifestPath := filepath.Join(root, "course.json")
			writeTestFile(t, manifestPath, []byte(test.manifest))
			_, _, err := loadManifest(manifestPath)
			if err == nil || !strings.Contains(err.Error(), test.wantError) {
				t.Fatalf("error = %v, want containing %q", err, test.wantError)
			}
		})
	}
}

func TestLoadManifestRejectsSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "secret.md")
	writeTestFile(t, outside, []byte("secret"))
	if err := os.Symlink(outside, filepath.Join(root, "linked.md")); err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(root, "course.json")
	writeTestFile(t, manifestPath, []byte(`{"version":1,"external_id":"x","title":"X","chapters":[{"external_id":"a","title":"A","content_file":"linked.md"}]}`))

	_, _, err := loadManifest(manifestPath)
	if err == nil || !strings.Contains(err.Error(), "stay inside") {
		t.Fatalf("error = %v", err)
	}
}

func TestLoadManifestRejectsManifestSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside-course.json")
	writeTestFile(t, outside, []byte(`{"version":1,"external_id":"outside","title":"Outside"}`))
	linkedManifest := filepath.Join(root, "course.json")
	if err := os.Symlink(outside, linkedManifest); err != nil {
		t.Fatal(err)
	}

	_, _, err := loadManifest(linkedManifest)
	if err == nil {
		t.Fatal("manifest symlink escaped its containing directory")
	}
}

func TestLoadManifestRejectsInvalidUTF8JSON(t *testing.T) {
	root := t.TempDir()
	body := append([]byte(`{"version":1,"external_id":"x","title":"`), 0xff)
	body = append(body, []byte(`"}`)...)
	manifestPath := filepath.Join(root, "course.json")
	writeTestFile(t, manifestPath, body)

	_, _, err := loadManifest(manifestPath)
	if err == nil || !strings.Contains(err.Error(), "valid UTF-8") {
		t.Fatalf("error = %v", err)
	}
}

func TestLoadManifestRejectsEncodedRequestOverAPILimit(t *testing.T) {
	root := t.TempDir()
	content := strings.Repeat("😀", 180_000)
	writeTestFile(t, filepath.Join(root, "one.md"), []byte(content))
	writeTestFile(t, filepath.Join(root, "two.md"), []byte(content))
	manifest := courseManifest{
		Version: 1, ExternalID: "large", Title: "Large",
		Chapters: []chapterManifest{
			{ExternalID: "one", Title: "One", ContentFile: "one.md"},
			{ExternalID: "two", Title: "Two", ContentFile: "two.md"},
		},
	}
	body, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(root, "course.json")
	writeTestFile(t, manifestPath, body)

	_, _, err = loadManifest(manifestPath)
	if err == nil || !strings.Contains(err.Error(), "encoded import request exceeds") {
		t.Fatalf("error = %v", err)
	}
}

func writeTestFile(t *testing.T, path string, body []byte) {
	t.Helper()
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
}
