package lapincli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/benenen/lapin/internal/assetstore"
)

const (
	manifestVersionV1     = 1
	manifestVersionV2     = 2
	maxManifestBytes      = 1 << 20
	maxRequestBytes       = 1 << 20
	maxBundleAssetBytes   = 64 << 20
	maxBundleAssets       = 300
	maxChapterContentRune = 200_000
	maxChapterContentByte = maxChapterContentRune * utf8.UTFMax
)

type courseManifest struct {
	Version     int               `json:"version"`
	ExternalID  string            `json:"external_id"`
	Title       string            `json:"title"`
	Description string            `json:"description"`
	Tags        []string          `json:"tags"`
	Assets      []assetManifest   `json:"assets,omitempty"`
	Chapters    []chapterManifest `json:"chapters"`
}

type assetManifest struct {
	Key  string `json:"key"`
	File string `json:"file"`
}

type chapterManifest struct {
	ExternalID  string            `json:"external_id"`
	Title       string            `json:"title"`
	ContentFile string            `json:"content_file,omitempty"`
	Children    []chapterManifest `json:"children,omitempty"`
}

type importSubjectRequest struct {
	ExternalID  string                 `json:"external_id"`
	Title       string                 `json:"title"`
	Description string                 `json:"description"`
	Tags        []string               `json:"tags"`
	Chapters    []importChapterRequest `json:"chapters"`
}

type importChapterRequest struct {
	ExternalID string                 `json:"external_id"`
	Title      string                 `json:"title"`
	Content    string                 `json:"content"`
	Children   []importChapterRequest `json:"children,omitempty"`
}

type localAsset struct {
	Key      string
	Filename string
	Content  []byte
}

type loadedBundle struct {
	Version int
	Request importSubjectRequest
	Assets  []localAsset
}

type manifestLoader struct {
	root            *os.Root
	seenExternalIDs map[string]struct{}
	chapterCount    int
}

func loadManifest(path string) (importSubjectRequest, []byte, error) {
	bundle, err := loadBundle(path)
	if err != nil {
		return importSubjectRequest{}, nil, err
	}
	encoded, err := json.Marshal(bundle.Request)
	if err != nil {
		return importSubjectRequest{}, nil, fmt.Errorf("encode import request: %w", err)
	}
	if len(encoded) > maxRequestBytes {
		return importSubjectRequest{}, nil, fmt.Errorf("encoded import request exceeds %d bytes", maxRequestBytes)
	}
	return bundle.Request, encoded, nil
}

func loadBundle(path string) (loadedBundle, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return loadedBundle{}, fmt.Errorf("resolve manifest path: %w", err)
	}
	root, err := os.OpenRoot(filepath.Dir(absPath))
	if err != nil {
		return loadedBundle{}, fmt.Errorf("open manifest directory: %w", err)
	}
	defer root.Close()
	body, err := readRegularFile(root, filepath.Base(absPath), maxManifestBytes)
	if err != nil {
		return loadedBundle{}, fmt.Errorf("read manifest within its containing directory: %w", err)
	}
	if !utf8.Valid(body) {
		return loadedBundle{}, fmt.Errorf("manifest must be valid UTF-8")
	}
	var manifest courseManifest
	if err := decodeStrictJSON(body, &manifest); err != nil {
		return loadedBundle{}, fmt.Errorf("decode manifest: %w", err)
	}
	if manifest.Version != manifestVersionV1 && manifest.Version != manifestVersionV2 {
		return loadedBundle{}, fmt.Errorf("manifest version must be %d or %d", manifestVersionV1, manifestVersionV2)
	}
	if manifest.Version == manifestVersionV1 && len(manifest.Assets) != 0 {
		return loadedBundle{}, fmt.Errorf("manifest assets require version %d", manifestVersionV2)
	}

	externalID := strings.TrimSpace(manifest.ExternalID)
	title := strings.TrimSpace(manifest.Title)
	description := strings.TrimSpace(manifest.Description)
	if externalID == "" || utf8.RuneCountInString(externalID) > 160 {
		return loadedBundle{}, fmt.Errorf("course external_id is required and must not exceed 160 characters")
	}
	if title == "" || utf8.RuneCountInString(title) > 200 {
		return loadedBundle{}, fmt.Errorf("course title is required and must not exceed 200 characters")
	}
	if utf8.RuneCountInString(description) > 4000 {
		return loadedBundle{}, fmt.Errorf("course description must not exceed 4000 characters")
	}
	tags, err := normalizeManifestTags(manifest.Tags)
	if err != nil {
		return loadedBundle{}, err
	}

	loader := manifestLoader{
		root:            root,
		seenExternalIDs: make(map[string]struct{}),
	}
	chapters := make([]importChapterRequest, 0, len(manifest.Chapters))
	for _, chapter := range manifest.Chapters {
		loaded, err := loader.loadChapter(chapter)
		if err != nil {
			return loadedBundle{}, err
		}
		chapters = append(chapters, loaded)
	}

	request := importSubjectRequest{
		ExternalID:  externalID,
		Title:       title,
		Description: description,
		Tags:        tags,
		Chapters:    chapters,
	}
	assets := make([]localAsset, 0, len(manifest.Assets))
	seenAssetKeys := make(map[string]struct{}, len(manifest.Assets))
	totalAssetBytes := 0
	if len(manifest.Assets) > maxBundleAssets {
		return loadedBundle{}, fmt.Errorf("manifest must not contain more than %d assets", maxBundleAssets)
	}
	for _, input := range manifest.Assets {
		key := strings.TrimSpace(input.Key)
		if key == "" || utf8.RuneCountInString(key) > 160 {
			return loadedBundle{}, fmt.Errorf("asset key is required and must not exceed 160 characters")
		}
		if _, exists := seenAssetKeys[key]; exists {
			return loadedBundle{}, fmt.Errorf("duplicate asset key %q", key)
		}
		seenAssetKeys[key] = struct{}{}
		content, filename, err := loader.readAssetFile(input.File)
		if err != nil {
			return loadedBundle{}, fmt.Errorf("asset %q: %w", key, err)
		}
		totalAssetBytes += len(content)
		if totalAssetBytes > maxBundleAssetBytes {
			return loadedBundle{}, fmt.Errorf("bundle assets exceed %d bytes", maxBundleAssetBytes)
		}
		assets = append(assets, localAsset{Key: key, Filename: filename, Content: content})
	}
	return loadedBundle{Version: manifest.Version, Request: request, Assets: assets}, nil
}

func (loader *manifestLoader) loadChapter(chapter chapterManifest) (importChapterRequest, error) {
	loader.chapterCount++
	if loader.chapterCount > 100 {
		return importChapterRequest{}, fmt.Errorf("manifest must not contain more than 100 chapters")
	}
	externalID := strings.TrimSpace(chapter.ExternalID)
	title := strings.TrimSpace(chapter.Title)
	if externalID == "" || utf8.RuneCountInString(externalID) > 160 {
		return importChapterRequest{}, fmt.Errorf("chapter external_id is required and must not exceed 160 characters")
	}
	if _, exists := loader.seenExternalIDs[externalID]; exists {
		return importChapterRequest{}, fmt.Errorf("duplicate chapter external_id %q", externalID)
	}
	loader.seenExternalIDs[externalID] = struct{}{}
	if title == "" || utf8.RuneCountInString(title) > 200 {
		return importChapterRequest{}, fmt.Errorf("chapter %q title is required and must not exceed 200 characters", externalID)
	}

	content := ""
	if strings.TrimSpace(chapter.ContentFile) != "" {
		loaded, err := loader.readContentFile(chapter.ContentFile)
		if err != nil {
			return importChapterRequest{}, fmt.Errorf("chapter %q: %w", externalID, err)
		}
		content = loaded
	}
	children := make([]importChapterRequest, 0, len(chapter.Children))
	for _, child := range chapter.Children {
		loaded, err := loader.loadChapter(child)
		if err != nil {
			return importChapterRequest{}, err
		}
		children = append(children, loaded)
	}
	return importChapterRequest{ExternalID: externalID, Title: title, Content: content, Children: children}, nil
}

func (loader *manifestLoader) readContentFile(path string) (string, error) {
	if filepath.IsAbs(path) {
		return "", fmt.Errorf("content_file must be relative to the manifest")
	}
	cleanPath := filepath.Clean(path)
	if cleanPath == "." || cleanPath == ".." || strings.HasPrefix(cleanPath, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("content_file must stay inside the manifest directory")
	}
	body, err := readRegularFile(loader.root, cleanPath, maxChapterContentByte)
	if err != nil {
		return "", fmt.Errorf("content_file must stay inside the manifest directory and name a readable regular file: %w", err)
	}
	if !utf8.Valid(body) {
		return "", fmt.Errorf("content_file must be valid UTF-8")
	}
	content := string(body)
	if utf8.RuneCountInString(content) > maxChapterContentRune {
		return "", fmt.Errorf("content_file must not exceed %d characters", maxChapterContentRune)
	}
	return content, nil
}

func (loader *manifestLoader) readAssetFile(path string) ([]byte, string, error) {
	if filepath.IsAbs(path) {
		return nil, "", fmt.Errorf("asset file must be relative to the manifest")
	}
	cleanPath := filepath.Clean(path)
	if cleanPath == "." || cleanPath == ".." || strings.HasPrefix(cleanPath, ".."+string(filepath.Separator)) {
		return nil, "", fmt.Errorf("asset file must stay inside the manifest directory")
	}
	body, err := readRegularFile(loader.root, cleanPath, assetstore.MaxAssetBytes)
	if err != nil {
		return nil, "", fmt.Errorf("asset file must stay inside the manifest directory and name a readable regular file: %w", err)
	}
	return body, filepath.Base(cleanPath), nil
}

func normalizeManifestTags(input []string) ([]string, error) {
	if len(input) > 20 {
		return nil, fmt.Errorf("manifest must not contain more than 20 tags")
	}
	seen := make(map[string]struct{}, len(input))
	result := make([]string, 0, len(input))
	for _, raw := range input {
		tag := strings.TrimSpace(raw)
		if tag == "" || utf8.RuneCountInString(tag) > 30 {
			return nil, fmt.Errorf("tags must be non-empty and must not exceed 30 characters")
		}
		key := strings.ToLower(tag)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, tag)
	}
	return result, nil
}

func decodeStrictJSON(body []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return fmt.Errorf("unexpected trailing JSON")
		}
		return err
	}
	return nil
}

func readRegularFile(root *os.Root, name string, limit int) ([]byte, error) {
	file, err := root.Open(name)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("not a regular file")
	}
	body, err := io.ReadAll(io.LimitReader(file, int64(limit)+1))
	if err != nil {
		return nil, err
	}
	if len(body) > limit {
		return nil, fmt.Errorf("file exceeds %d bytes", limit)
	}
	return body, nil
}
