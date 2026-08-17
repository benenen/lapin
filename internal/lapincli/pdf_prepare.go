package lapincli

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/benenen/lapin/internal/documentconv/pdf"
)

type preparePDFResult struct {
	Manifest string `json:"manifest"`
	Chapters int    `json:"chapters"`
	Assets   int    `json:"assets"`
}

func runPreparePDFCommand(ctx context.Context, args []string, dependencies Dependencies) int {
	flags := flag.NewFlagSet("course prepare-pdf", flag.ContinueOnError)
	flags.SetOutput(dependencies.Stderr)
	pdfPath := flags.String("pdf", "", "source PDF path")
	outputPath := flags.String("output", "", "output bundle directory")
	externalID := flags.String("external-id", "", "stable course external ID")
	title := flags.String("title", "", "course title")
	profileName := flags.String("profile", "zh-technical-book", "PDF layout profile: zh-technical-book or generic-book")
	reuseChapterTree := flags.String("reuse-chapter-tree", "", "existing manifest whose stable chapter external IDs and grouping should be reused")
	if err := flags.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return exitSuccess
		}
		return exitUsage
	}
	if flags.NArg() != 0 || strings.TrimSpace(*pdfPath) == "" || strings.TrimSpace(*outputPath) == "" || strings.TrimSpace(*externalID) == "" || strings.TrimSpace(*title) == "" {
		fmt.Fprintln(dependencies.Stderr, "usage error: --pdf, --output, --external-id and --title are required")
		return exitUsage
	}
	profile, err := selectPDFProfile(strings.TrimSpace(*profileName))
	if err != nil {
		fmt.Fprintf(dependencies.Stderr, "usage error: %s\n", err)
		return exitUsage
	}
	result, err := preparePDF(ctx, *pdfPath, *outputPath, strings.TrimSpace(*externalID), strings.TrimSpace(*title), profile, strings.TrimSpace(*reuseChapterTree))
	if err != nil {
		fmt.Fprintf(dependencies.Stderr, "PDF preparation error: %s\n", sanitizeDiagnostic(err.Error(), ""))
		return exitUsage
	}
	encoder := json.NewEncoder(dependencies.Stdout)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(result); err != nil {
		fmt.Fprintf(dependencies.Stderr, "output error: %v\n", err)
		return exitRemote
	}
	return exitSuccess
}

func selectPDFProfile(name string) (pdf.Profile, error) {
	switch name {
	case "generic-book":
		return pdf.GenericBookProfile(), nil
	case "zh-technical-book":
		return pdf.ChineseTechnicalBookProfile(), nil
	default:
		return nil, fmt.Errorf("--profile must be generic-book or zh-technical-book")
	}
}

func preparePDF(ctx context.Context, pdfPath, outputPath, externalID, title string, profile pdf.Profile, reuseChapterTree string) (preparePDFResult, error) {
	outputAbsolute, err := filepath.Abs(outputPath)
	if err != nil {
		return preparePDFResult{}, fmt.Errorf("resolve output directory: %w", err)
	}
	if entries, readErr := os.ReadDir(outputAbsolute); readErr == nil && len(entries) != 0 {
		return preparePDFResult{}, fmt.Errorf("output directory must be empty")
	} else if readErr != nil && !os.IsNotExist(readErr) {
		return preparePDFResult{}, fmt.Errorf("inspect output directory: %w", readErr)
	}
	if err := os.MkdirAll(outputAbsolute, 0o750); err != nil {
		return preparePDFResult{}, fmt.Errorf("create output directory: %w", err)
	}
	converter := pdf.New(pdf.Options{Profile: profile})
	document, err := converter.Convert(ctx, pdfPath)
	if err != nil {
		return preparePDFResult{}, err
	}
	chaptersDirectory := filepath.Join(outputAbsolute, "chapters")
	assetsDirectory := filepath.Join(outputAbsolute, "assets")
	if err := os.MkdirAll(chaptersDirectory, 0o750); err != nil {
		return preparePDFResult{}, fmt.Errorf("create chapter directory: %w", err)
	}
	if err := os.MkdirAll(assetsDirectory, 0o750); err != nil {
		return preparePDFResult{}, fmt.Errorf("create asset directory: %w", err)
	}
	manifestChapters := make([]chapterManifest, 0, len(document.Chapters))
	for _, chapter := range document.Chapters {
		if err := os.WriteFile(filepath.Join(chaptersDirectory, chapter.Filename), []byte(chapter.Markdown), 0o640); err != nil {
			return preparePDFResult{}, fmt.Errorf("write chapter Markdown: %w", err)
		}
		manifestChapters = append(manifestChapters, chapterManifest{
			ExternalID: chapter.ExternalID, Title: chapter.Title,
			ContentFile: filepath.ToSlash(filepath.Join("chapters", chapter.Filename)),
		})
	}
	if reuseChapterTree != "" {
		manifestChapters, err = reusePDFChapterTree(reuseChapterTree, manifestChapters)
		if err != nil {
			return preparePDFResult{}, err
		}
	}
	manifestAssets := make([]assetManifest, 0, len(document.Assets))
	for _, asset := range document.Assets {
		if err := os.WriteFile(filepath.Join(assetsDirectory, asset.Filename), asset.Content, 0o640); err != nil {
			return preparePDFResult{}, fmt.Errorf("write extracted PDF image: %w", err)
		}
		manifestAssets = append(manifestAssets, assetManifest{Key: asset.Key, File: filepath.ToSlash(filepath.Join("assets", asset.Filename))})
	}
	manifest := courseManifest{
		Version: manifestVersionV2, ExternalID: externalID, Title: title,
		Description: "由 PDF 文本块和图片生成的结构化课程", Tags: []string{"PDF"},
		Assets: manifestAssets, Chapters: manifestChapters,
	}
	manifestBody, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return preparePDFResult{}, fmt.Errorf("encode PDF manifest: %w", err)
	}
	manifestPath := filepath.Join(outputAbsolute, "course.json")
	if err := os.WriteFile(manifestPath, append(manifestBody, '\n'), 0o640); err != nil {
		return preparePDFResult{}, fmt.Errorf("write PDF manifest: %w", err)
	}
	if _, err := loadBundle(manifestPath); err != nil {
		return preparePDFResult{}, fmt.Errorf("validate generated PDF bundle: %w", err)
	}
	return preparePDFResult{Manifest: manifestPath, Chapters: countManifestChapters(manifestChapters), Assets: len(document.Assets)}, nil
}

func countManifestChapters(chapters []chapterManifest) int {
	total := 0
	stack := append([]chapterManifest(nil), chapters...)
	for len(stack) > 0 {
		last := len(stack) - 1
		chapter := stack[last]
		stack = stack[:last]
		total++
		stack = append(stack, chapter.Children...)
	}
	return total
}

func reusePDFChapterTree(manifestPath string, generated []chapterManifest) ([]chapterManifest, error) {
	absPath, err := filepath.Abs(manifestPath)
	if err != nil {
		return nil, fmt.Errorf("resolve reusable manifest: %w", err)
	}
	root, err := os.OpenRoot(filepath.Dir(absPath))
	if err != nil {
		return nil, fmt.Errorf("open reusable manifest directory: %w", err)
	}
	defer root.Close()
	body, err := readRegularFile(root, filepath.Base(absPath), maxManifestBytes)
	if err != nil {
		return nil, fmt.Errorf("read reusable manifest: %w", err)
	}
	var existing courseManifest
	if err := decodeStrictJSON(body, &existing); err != nil {
		return nil, fmt.Errorf("decode reusable manifest: %w", err)
	}
	byTitle := make(map[string]chapterManifest, len(generated))
	for _, chapter := range generated {
		if _, duplicate := byTitle[chapter.Title]; duplicate {
			return nil, fmt.Errorf("generated PDF contains duplicate chapter title %q", chapter.Title)
		}
		byTitle[chapter.Title] = chapter
	}
	usedTitles := make(map[string]struct{}, len(generated))
	seenExternalIDs := make(map[string]struct{})
	var reuse func(chapterManifest) (chapterManifest, bool, error)
	reuse = func(value chapterManifest) (chapterManifest, bool, error) {
		externalID := strings.TrimSpace(value.ExternalID)
		if externalID == "" {
			return chapterManifest{}, false, fmt.Errorf("reusable manifest contains an empty chapter external_id")
		}
		if _, duplicate := seenExternalIDs[externalID]; duplicate {
			return chapterManifest{}, false, fmt.Errorf("reusable manifest contains duplicate chapter external_id %q", externalID)
		}
		seenExternalIDs[externalID] = struct{}{}
		result := chapterManifest{ExternalID: externalID, Title: strings.TrimSpace(value.Title)}
		if generatedChapter, exists := byTitle[result.Title]; exists {
			result.ContentFile = generatedChapter.ContentFile
			usedTitles[result.Title] = struct{}{}
		}
		for _, child := range value.Children {
			reused, keep, err := reuse(child)
			if err != nil {
				return chapterManifest{}, false, err
			}
			if keep {
				result.Children = append(result.Children, reused)
			}
		}
		keep := result.ContentFile != "" || len(result.Children) != 0
		return result, keep, nil
	}
	result := make([]chapterManifest, 0, len(existing.Chapters)+len(generated))
	for _, chapter := range existing.Chapters {
		reused, keep, err := reuse(chapter)
		if err != nil {
			return nil, err
		}
		if keep {
			result = append(result, reused)
		}
	}
	for _, chapter := range generated {
		if _, used := usedTitles[chapter.Title]; !used {
			result = append(result, chapter)
		}
	}
	return result, nil
}
