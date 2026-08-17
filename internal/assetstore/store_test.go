package assetstore

import (
	"bytes"
	"encoding/base64"
	"io"
	"os"
	"path/filepath"
	"testing"
)

const onePixelPNGBase64 = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII="

func TestStoreShardsBySHA256AndDeduplicates(t *testing.T) {
	root := t.TempDir()
	store, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	png, err := base64.StdEncoding.DecodeString(onePixelPNGBase64)
	if err != nil {
		t.Fatal(err)
	}

	first, err := store.Save(bytes.NewReader(png))
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.Save(bytes.NewReader(png))
	if err != nil {
		t.Fatal(err)
	}

	const wantSHA = "431ced6916a2a21a156e38701afe55bbd7f88969fbbfc56d7fe099d47f265460"
	if first != second || first.SHA256 != wantSHA || first.MIMEType != "image/png" || first.Size != int64(len(png)) || first.Width != 1 || first.Height != 1 {
		t.Fatalf("stored asset = %#v, repeated = %#v", first, second)
	}
	wantPath := filepath.Join(root, "43", wantSHA+".png")
	info, err := os.Stat(wantPath)
	if err != nil {
		t.Fatal(err)
	}
	if !info.Mode().IsRegular() || info.Mode().Perm() != 0o640 {
		t.Fatalf("stored mode = %v", info.Mode())
	}
	entries, err := os.ReadDir(filepath.Dir(wantPath))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("shard contains %d files, want 1", len(entries))
	}

	reader, err := store.Open(first)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	got, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, png) {
		t.Fatal("stored bytes changed")
	}
}

func TestStoreRejectsUnsupportedOrOversizedContent(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	if _, err := store.Save(bytes.NewReader([]byte(`<svg xmlns="http://www.w3.org/2000/svg"><script/></svg>`))); err == nil {
		t.Fatal("SVG upload was accepted")
	}
	if _, err := store.Save(bytes.NewReader(bytes.Repeat([]byte("x"), MaxAssetBytes+1))); err == nil {
		t.Fatal("oversized upload was accepted")
	}
}

func TestStagedBlobIsNotPublishedUntilCommitAndAbortRemovesIt(t *testing.T) {
	root := t.TempDir()
	store, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	png, err := base64.StdEncoding.DecodeString(onePixelPNGBase64)
	if err != nil {
		t.Fatal(err)
	}
	staged, err := store.Stage(bytes.NewReader(png))
	if err != nil {
		t.Fatal(err)
	}
	finalPath := filepath.Join(root, staged.SHA256[:2], staged.SHA256+".png")
	if _, err := os.Stat(finalPath); !os.IsNotExist(err) {
		t.Fatalf("staged asset was published early: %v", err)
	}
	staged.Abort()
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("aborted stage left %d entries", len(entries))
	}
}

func TestStoreStartupRemovesInterruptedStages(t *testing.T) {
	root := t.TempDir()
	stale := filepath.Join(root, ".upload-0123456789abcdef")
	if err := os.WriteFile(stale, []byte("partial"), 0o600); err != nil {
		t.Fatal(err)
	}
	store, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Fatalf("interrupted stage was not removed: %v", err)
	}
}

func TestRollbackAndReconcileRemoveUnreferencedPublishedBlobs(t *testing.T) {
	root := t.TempDir()
	store, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	png, err := base64.StdEncoding.DecodeString(onePixelPNGBase64)
	if err != nil {
		t.Fatal(err)
	}
	staged, err := store.Stage(bytes.NewReader(png))
	if err != nil {
		t.Fatal(err)
	}
	if err := staged.Publish(); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, staged.SHA256[:2], staged.SHA256+".png")
	staged.RollbackPublished()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("rollback left published file: %v", err)
	}

	blob, err := store.Save(bytes.NewReader(png))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Reconcile([]Blob{blob}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("reconcile removed referenced file: %v", err)
	}
	if err := store.Reconcile(nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("reconcile left orphan file: %v", err)
	}
}
