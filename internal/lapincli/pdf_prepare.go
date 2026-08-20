package lapincli

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/benenen/lapin/internal/documentconv"
	"github.com/benenen/lapin/internal/documentconv/pdf"
)

type preparePDFResult struct {
	Manifest string   `json:"manifest"`
	Chapters int      `json:"chapters"`
	Assets   int      `json:"assets"`
	Warnings []string `json:"warnings,omitempty"`
}

func runPreparePDFCommand(ctx context.Context, args []string, dependencies Dependencies) int {
	flags := flag.NewFlagSet("course prepare-pdf", flag.ContinueOnError)
	flags.SetOutput(dependencies.Stderr)
	pdfPath := flags.String("pdf", "", "source PDF path")
	outputPath := flags.String("output", "", "output bundle directory")
	externalID := flags.String("external-id", "", "stable course external ID")
	title := flags.String("title", "", "course title")
	profileName := flags.String("profile", "zh-technical-book", "PDF layout profile: zh-technical-book or generic-book")
	engineName := flags.String("engine", "layout", "extraction engine: layout (offline, deterministic) or llm (vision model, needs a gateway)")
	llmBaseURL := flags.String("llm-base-url", "", "OpenAI-compatible chat completions base URL (or use LAPIN_LLM_BASE_URL)")
	llmModel := flags.String("llm-model", "", "vision model used for page conversion (or use LAPIN_LLM_MODEL)")
	llmOutlineModel := flags.String("llm-outline-model", "", "model used for the outline pass (or use LAPIN_LLM_OUTLINE_MODEL, defaults to --llm-model)")
	llmDPI := flags.Int("llm-dpi", 0, "page render DPI for the llm engine")
	llmWorkers := flags.Int("llm-workers", 0, "concurrent page conversions for the llm engine")
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
	converter, err := selectPDFConverter(strings.TrimSpace(*engineName), profile, llmSettings{
		baseURL:      settingOrEnv(dependencies, *llmBaseURL, "LAPIN_LLM_BASE_URL"),
		model:        settingOrEnv(dependencies, *llmModel, "LAPIN_LLM_MODEL"),
		outlineModel: settingOrEnv(dependencies, *llmOutlineModel, "LAPIN_LLM_OUTLINE_MODEL"),
		apiKey:       environmentValue(dependencies, "LAPIN_LLM_API_KEY"),
		imageDPI:     *llmDPI,
		workers:      *llmWorkers,
		httpClient:   dependencies.HTTPClient,
	})
	if err != nil {
		fmt.Fprintf(dependencies.Stderr, "usage error: %s\n", err)
		return exitUsage
	}
	result, err := preparePDF(ctx, converter, *pdfPath, *outputPath, strings.TrimSpace(*externalID), strings.TrimSpace(*title), strings.TrimSpace(*reuseChapterTree))
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

// llmSettings carries the gateway configuration for the llm engine. The API key is
// read from the environment only, matching how the CLI handles the Access Token.
type llmSettings struct {
	baseURL      string
	model        string
	outlineModel string
	apiKey       string
	imageDPI     int
	workers      int
	httpClient   *http.Client
}

func settingOrEnv(dependencies Dependencies, flagValue, name string) string {
	if trimmed := strings.TrimSpace(flagValue); trimmed != "" {
		return trimmed
	}
	return environmentValue(dependencies, name)
}

func environmentValue(dependencies Dependencies, name string) string {
	if dependencies.LookupEnv == nil {
		return ""
	}
	value, exists := dependencies.LookupEnv(name)
	if !exists {
		return ""
	}
	return strings.TrimSpace(value)
}

// selectPDFConverter picks the extraction engine. The layout engine is the default
// because it is deterministic and needs no network; the llm engine restores tables,
// code fences and heading levels that layout heuristics cannot recover, at the cost
// of a gateway round trip per page.
func selectPDFConverter(engine string, profile pdf.Profile, settings llmSettings) (documentconv.Converter, error) {
	switch engine {
	case "", "layout":
		return pdf.New(pdf.Options{Profile: profile}), nil
	case "llm":
		if settings.baseURL == "" {
			return nil, fmt.Errorf("--engine llm requires --llm-base-url or LAPIN_LLM_BASE_URL")
		}
		if settings.model == "" {
			return nil, fmt.Errorf("--engine llm requires --llm-model or LAPIN_LLM_MODEL")
		}
		return pdf.NewLLMConverter(pdf.LLMOptions{
			Profile:      profile,
			BaseURL:      settings.baseURL,
			APIKey:       settings.apiKey,
			Model:        settings.model,
			OutlineModel: settings.outlineModel,
			Caller:       "lapin_cli_prepare_pdf",
			ImageDPI:     settings.imageDPI,
			MaxWorkers:   settings.workers,
			HTTPClient:   settings.httpClient,
		})
	default:
		return nil, fmt.Errorf("--engine must be layout or llm")
	}
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

func preparePDF(ctx context.Context, converter documentconv.Converter, pdfPath, outputPath, externalID, title, reuseChapterTree string) (preparePDFResult, error) {
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
	return preparePDFResult{
		Manifest: manifestPath,
		Chapters: countManifestChapters(manifestChapters),
		Assets:   len(document.Assets),
		Warnings: document.Warnings,
	}, nil
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
