package pdf

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/benenen/lapin/internal/documentconv"
)

// llmPageRules is the per-page conversion contract. It is deliberately strict
// about HTML, invented image links and page furniture, because a permissive
// prompt makes every page re-invent its own formatting.
//
//go:embed llm_prompt.md
var llmPageRules string

const llmOutlineRules = `You are given the extracted text of every page of a PDF, in order.
List the document's section headings, in document order, one per line, as:

<level><TAB><heading text exactly as printed>

level is an integer 1-6. When a heading carries a section number, level = the count
of dot-separated components in that number (1. -> 1, 1.1. -> 2, 2.2.1. -> 3).
Include ONLY real section headings. Exclude running headers, footers, page numbers,
table cells, figure captions, and table-of-contents entries.
Output nothing else.`

const (
	maxLLMPages           = 400
	maxLLMResponseBytes   = 4 << 20
	maxLLMOutlineRunes    = 32000
	maxLLMPageTextRunes   = 24000
	maxLLMRenderedPNG     = 24 << 20
	defaultLLMImageDPI    = 200
	defaultLLMWorkers     = 6
	defaultLLMNeighbour   = 600
	defaultLLMAttempts    = 3
	defaultLLMCallTimeout = 4 * time.Minute
	maxLLMImageDPI        = 600
	maxLLMWorkers         = 32
	bundleChapterLevel    = 2
)

// LLMOptions configures the vision-assisted converter. BaseURL must point at an
// OpenAI-compatible chat completions endpoint; APIKey may be empty for gateways
// that attach their own credentials.
type LLMOptions struct {
	Profile        Profile
	BaseURL        string
	APIKey         string
	Model          string
	OutlineModel   string
	Caller         string
	ImageDPI       int
	MaxWorkers     int
	NeighbourRunes int
	MaxAttempts    int
	HTTPClient     *http.Client
}

// LLMConverter renders each page, sends it to a vision model together with the
// document outline and its neighbouring pages, and assembles the replies into
// Lapin's course model. Unlike Converter it needs network access and is not
// deterministic, so callers must opt into it explicitly.
type LLMConverter struct {
	options LLMOptions
	client  *http.Client
}

var _ documentconv.Converter = (*LLMConverter)(nil)

func NewLLMConverter(options LLMOptions) (*LLMConverter, error) {
	if strings.TrimSpace(options.BaseURL) == "" {
		return nil, fmt.Errorf("LLM base URL is required")
	}
	parsed, err := url.Parse(strings.TrimSpace(options.BaseURL))
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return nil, fmt.Errorf("LLM base URL must be an http or https URL")
	}
	if strings.TrimSpace(options.Model) == "" {
		return nil, fmt.Errorf("LLM model is required")
	}
	if options.ImageDPI < 0 || options.ImageDPI > maxLLMImageDPI {
		return nil, fmt.Errorf("LLM image DPI must be between 1 and %d", maxLLMImageDPI)
	}
	if options.MaxWorkers < 0 || options.MaxWorkers > maxLLMWorkers {
		return nil, fmt.Errorf("LLM worker count must be between 1 and %d", maxLLMWorkers)
	}
	if options.NeighbourRunes < 0 {
		return nil, fmt.Errorf("LLM neighbour context must not be negative")
	}
	if options.MaxAttempts < 0 {
		return nil, fmt.Errorf("LLM attempt count must not be negative")
	}
	options.BaseURL = strings.TrimRight(strings.TrimSpace(options.BaseURL), "/")
	if options.Profile == nil {
		options.Profile = GenericBookProfile()
	}
	if options.OutlineModel == "" {
		options.OutlineModel = options.Model
	}
	if options.ImageDPI == 0 {
		options.ImageDPI = defaultLLMImageDPI
	}
	if options.MaxWorkers == 0 {
		options.MaxWorkers = defaultLLMWorkers
	}
	if options.NeighbourRunes == 0 {
		options.NeighbourRunes = defaultLLMNeighbour
	}
	if options.MaxAttempts == 0 {
		options.MaxAttempts = defaultLLMAttempts
	}
	client := options.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: defaultLLMCallTimeout}
	}
	return &LLMConverter{options: options, client: client}, nil
}

func (converter *LLMConverter) Convert(ctx context.Context, sourcePath string) (documentconv.Document, error) {
	absolute, err := filepath.Abs(sourcePath)
	if err != nil {
		return documentconv.Document{}, fmt.Errorf("resolve PDF path: %w", err)
	}
	info, err := os.Stat(absolute)
	if err != nil || !info.Mode().IsRegular() || info.Size() < 1 || info.Size() > maxPDFBytes {
		return documentconv.Document{}, fmt.Errorf("PDF must be a readable regular file no larger than %d bytes", maxPDFBytes)
	}
	temporary, err := os.MkdirTemp("", "lapin-pdf-llm-")
	if err != nil {
		return documentconv.Document{}, fmt.Errorf("create PDF conversion directory: %w", err)
	}
	defer os.RemoveAll(temporary)

	pageTexts, err := extractLLMPageTexts(ctx, absolute, temporary)
	if err != nil {
		return documentconv.Document{}, err
	}

	var warnings []string
	outline, err := converter.requestOutline(ctx, pageTexts)
	if err != nil {
		return documentconv.Document{}, err
	}
	if strings.TrimSpace(outline) == "" {
		warnings = append(warnings, "the outline pass returned no headings; chapter splitting fell back to a single chapter")
	}

	pageMarkdown, pageWarnings, err := converter.convertPages(ctx, absolute, temporary, pageTexts, outline)
	if err != nil {
		return documentconv.Document{}, err
	}
	warnings = append(warnings, pageWarnings...)

	body := strings.Join(nonEmptyStrings(pageMarkdown), "\n\n")
	chapters := splitLLMChapters(body, outlineChapterLevel(outline), converter.options.Profile, strings.TrimSuffix(filepath.Base(absolute), filepath.Ext(absolute)))
	warnings = append(warnings, unbalancedFenceWarnings(chapters)...)
	if figures := strings.Count(body, "*[图"); figures > 0 {
		warnings = append(warnings, fmt.Sprintf("%d figure placeholders need artwork; the LLM engine does not extract images", figures))
	}
	return documentconv.Document{Chapters: chapters, Assets: nil, Warnings: warnings}, nil
}

// extractLLMPageTexts runs pdftotext once and splits its form-feed separated output.
func extractLLMPageTexts(ctx context.Context, sourcePath, temporary string) ([]string, error) {
	textPath := filepath.Join(temporary, "pages.txt")
	commandContext, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	command := exec.CommandContext(commandContext, "pdftotext", "-enc", "UTF-8", sourcePath, textPath)
	output, err := runBoundedPDFCommand(commandContext, command, temporary)
	if err != nil {
		return nil, fmt.Errorf("pdftotext failed: %s", boundedCommandOutput(output))
	}
	body, err := os.ReadFile(textPath)
	if err != nil {
		return nil, fmt.Errorf("read pdftotext output: %w", err)
	}
	pages := strings.Split(string(body), "\f")
	// pdftotext terminates the final page with a form feed, leaving a trailing empty field.
	if len(pages) > 0 && strings.TrimSpace(pages[len(pages)-1]) == "" {
		pages = pages[:len(pages)-1]
	}
	if len(pages) == 0 {
		return nil, fmt.Errorf("PDF contains no extractable pages")
	}
	if len(pages) > maxLLMPages {
		return nil, fmt.Errorf("PDF has %d pages; the LLM engine converts at most %d", len(pages), maxLLMPages)
	}
	for index, page := range pages {
		pages[index] = truncateRunes(page, maxLLMPageTextRunes)
	}
	return pages, nil
}

func (converter *LLMConverter) requestOutline(ctx context.Context, pageTexts []string) (string, error) {
	var builder strings.Builder
	for index, text := range pageTexts {
		fmt.Fprintf(&builder, "\n=== page %d ===\n%s", index+1, text)
	}
	outline, err := converter.chat(ctx, converter.options.OutlineModel, llmOutlineRules, []chatPart{
		{Type: "text", Text: truncateRunes(builder.String(), maxLLMOutlineRunes)},
	})
	if err != nil {
		return "", fmt.Errorf("outline pass: %w", err)
	}
	return strings.TrimSpace(outline), nil
}

// convertPages renders and converts every page concurrently. Each page carries the
// outline plus its neighbours' text so that headings keep a stable level and a
// table split across a page break is reassembled exactly once.
func (converter *LLMConverter) convertPages(ctx context.Context, sourcePath, temporary string, pageTexts []string, outline string) ([]string, []string, error) {
	results := make([]string, len(pageTexts))
	warnings := make([]string, len(pageTexts))

	pageContext, cancel := context.WithCancel(ctx)
	defer cancel()

	var mutex sync.Mutex
	var firstErr error
	var waitGroup sync.WaitGroup
	semaphore := make(chan struct{}, converter.options.MaxWorkers)

	for index := range pageTexts {
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			select {
			case semaphore <- struct{}{}:
				defer func() { <-semaphore }()
			case <-pageContext.Done():
				return
			}
			markdown, err := converter.convertPage(pageContext, sourcePath, temporary, pageTexts, outline, index)
			mutex.Lock()
			defer mutex.Unlock()
			if err != nil {
				if firstErr == nil {
					firstErr = fmt.Errorf("page %d: %w", index+1, err)
					cancel()
				}
				return
			}
			results[index] = markdown
			if strings.TrimSpace(markdown) == "" {
				warnings[index] = fmt.Sprintf("page %d converted to empty Markdown", index+1)
			}
		}()
	}
	waitGroup.Wait()
	if firstErr != nil {
		return nil, nil, firstErr
	}
	return results, nonEmptyStrings(warnings), nil
}

func (converter *LLMConverter) convertPage(ctx context.Context, sourcePath, temporary string, pageTexts []string, outline string, index int) (string, error) {
	image, err := renderLLMPage(ctx, sourcePath, temporary, index+1, converter.options.ImageDPI)
	if err != nil {
		return "", err
	}
	previous, next := "(this is the first page)", "(this is the last page)"
	if index > 0 {
		previous = lastRunes(pageTexts[index-1], converter.options.NeighbourRunes)
	}
	if index < len(pageTexts)-1 {
		next = truncateRunes(pageTexts[index+1], converter.options.NeighbourRunes)
	}
	prompt := fmt.Sprintf(
		"<documentOutline>\n%s\n</documentOutline>\n\n"+
			"<previousPageTail>\n%s\n</previousPageTail>\n\n"+
			"<nextPageHead>\n%s\n</nextPageHead>\n\n"+
			"<pageNumber>%d of %d</pageNumber>\n\n"+
			"<pageContent>\n%s\n</pageContent>",
		outline, previous, next, index+1, len(pageTexts), pageTexts[index])

	markdown, err := converter.chat(ctx, converter.options.Model, llmPageRules, []chatPart{
		{Type: "text", Text: prompt},
		{Type: "image_url", ImageURL: &chatImageURL{URL: "data:image/png;base64," + base64.StdEncoding.EncodeToString(image)}},
	})
	if err != nil {
		return "", err
	}
	return stripMarkdownFence(markdown), nil
}

// renderLLMPage rasterizes one page and returns the PNG bytes. Each page renders
// into its own directory, which is removed immediately: pages convert concurrently,
// and the shared temporary-directory quota walk would otherwise race the cleanup of
// a sibling page.
func renderLLMPage(ctx context.Context, sourcePath, temporary string, pageNumber, dpi int) ([]byte, error) {
	pageDirectory := filepath.Join(temporary, fmt.Sprintf("page-%06d", pageNumber))
	if err := os.MkdirAll(pageDirectory, 0o700); err != nil {
		return nil, fmt.Errorf("create page render directory: %w", err)
	}
	defer os.RemoveAll(pageDirectory)

	prefix := filepath.Join(pageDirectory, "page")
	commandContext, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	page := strconv.Itoa(pageNumber)
	command := exec.CommandContext(commandContext, "pdftoppm",
		"-png", "-r", strconv.Itoa(dpi), "-f", page, "-l", page, sourcePath, prefix)
	output, err := runBoundedPDFCommand(commandContext, command, pageDirectory)
	if err != nil {
		return nil, fmt.Errorf("pdftoppm failed: %s", boundedCommandOutput(output))
	}
	// pdftoppm pads the page suffix according to the document's page count.
	matches, err := filepath.Glob(prefix + "*")
	if err != nil || len(matches) == 0 {
		return nil, fmt.Errorf("pdftoppm produced no image for page %d", pageNumber)
	}
	info, err := os.Stat(matches[0])
	if err != nil || info.Size() > maxLLMRenderedPNG {
		return nil, fmt.Errorf("rendered page %d is unreadable or larger than %d bytes", pageNumber, maxLLMRenderedPNG)
	}
	return os.ReadFile(matches[0])
}

type chatImageURL struct {
	URL string `json:"url"`
}

type chatPart struct {
	Type     string        `json:"type"`
	Text     string        `json:"text,omitempty"`
	ImageURL *chatImageURL `json:"image_url,omitempty"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content any    `json:"content"`
}

type chatRequest struct {
	Model    string        `json:"model"`
	Messages []chatMessage `json:"messages"`
	Caller   string        `json:"caller,omitempty"`
}

type chatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// chat posts one chat completion, retrying transient failures a bounded number of times.
func (converter *LLMConverter) chat(ctx context.Context, model, system string, parts []chatPart) (string, error) {
	// The prompt is full of <tag> markers, so HTML escaping would bloat every
	// request with \u003c sequences for no benefit.
	var payload bytes.Buffer
	encoder := json.NewEncoder(&payload)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(chatRequest{
		Model: model,
		Messages: []chatMessage{
			{Role: "system", Content: system},
			{Role: "user", Content: parts},
		},
		Caller: converter.options.Caller,
	}); err != nil {
		return "", fmt.Errorf("encode chat request: %w", err)
	}
	body := payload.Bytes()
	endpoint := converter.options.BaseURL + "/chat/completions"

	var lastErr error
	for attempt := 0; attempt < converter.options.MaxAttempts; attempt++ {
		if attempt > 0 {
			select {
			case <-time.After(time.Duration(attempt) * 2 * time.Second):
			case <-ctx.Done():
				return "", ctx.Err()
			}
		}
		content, err := converter.chatOnce(ctx, endpoint, body)
		if err == nil {
			return content, nil
		}
		lastErr = err
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
	}
	return "", lastErr
}

func (converter *LLMConverter) chatOnce(ctx context.Context, endpoint string, body []byte) (string, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("build chat request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	if converter.options.APIKey != "" {
		request.Header.Set("Authorization", "Bearer "+converter.options.APIKey)
	}
	response, err := converter.client.Do(request)
	if err != nil {
		return "", fmt.Errorf("call chat completions: %w", err)
	}
	defer response.Body.Close()
	payload, err := io.ReadAll(io.LimitReader(response.Body, maxLLMResponseBytes+1))
	if err != nil {
		return "", fmt.Errorf("read chat response: %w", err)
	}
	if len(payload) > maxLLMResponseBytes {
		return "", fmt.Errorf("chat response exceeds %d bytes", maxLLMResponseBytes)
	}
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("chat completions returned %d: %s", response.StatusCode, boundedCommandOutput(payload))
	}
	var decoded chatResponse
	if err := json.Unmarshal(payload, &decoded); err != nil {
		return "", fmt.Errorf("decode chat response: %w", err)
	}
	if decoded.Error != nil {
		return "", fmt.Errorf("chat completions error: %s", boundedCommandOutput([]byte(decoded.Error.Message)))
	}
	if len(decoded.Choices) == 0 {
		return "", fmt.Errorf("chat completions returned no choices")
	}
	return decoded.Choices[0].Message.Content, nil
}

var (
	markdownHeadingPattern = regexp.MustCompile(`^(#{1,6})\s+(.*)$`)
	outlineLevelPattern    = regexp.MustCompile(`^\s*([1-6])\s*\t`)
)

// outlineChapterLevel reports the shallowest heading level the outline uses, which
// is the level at which the document splits into chapters.
func outlineChapterLevel(outline string) int {
	level := 0
	for _, line := range strings.Split(outline, "\n") {
		matches := outlineLevelPattern.FindStringSubmatch(line)
		if matches == nil {
			continue
		}
		value, _ := strconv.Atoi(matches[1])
		if level == 0 || value < level {
			level = value
		}
	}
	if level == 0 {
		return 1
	}
	return level
}

type llmChapter struct {
	title string
	lines []string
}

// splitLLMChapters cuts the converted body at chapter-level headings and shifts every
// heading so that a chapter title sits at level two, matching the bundles the layout
// engine produces. Headings inside fenced code are left alone.
func splitLLMChapters(body string, chapterLevel int, profile Profile, fallbackTitle string) []documentconv.Chapter {
	shift := bundleChapterLevel - chapterLevel
	var collected []llmChapter
	current := llmChapter{title: ""}
	started := false
	fenced := false

	for _, line := range strings.Split(body, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			fenced = !fenced
		}
		if matches := markdownHeadingPattern.FindStringSubmatch(line); matches != nil && !fenced {
			level := len(matches[1])
			if level == chapterLevel {
				if started || len(nonBlank(current.lines)) > 0 {
					collected = append(collected, current)
				}
				current = llmChapter{title: profile.NormalizeHeading(matches[2])}
				started = true
			}
			line = strings.Repeat("#", clampHeadingLevel(level+shift)) + " " + matches[2]
		}
		current.lines = append(current.lines, line)
	}
	if started || len(nonBlank(current.lines)) > 0 {
		collected = append(collected, current)
	}

	chapters := make([]documentconv.Chapter, 0, len(collected))
	for index, chapter := range collected {
		title := chapter.title
		if title == "" {
			title = firstHeadingText(chapter.lines, profile, fallbackTitle)
		}
		externalID, filename := profile.ChapterIdentity(title, index+1)
		chapters = append(chapters, documentconv.Chapter{
			ExternalID: externalID,
			Title:      title,
			Filename:   filename,
			Markdown:   strings.TrimSpace(strings.Join(chapter.lines, "\n")) + "\n",
		})
	}
	return chapters
}

func firstHeadingText(lines []string, profile Profile, fallback string) string {
	for _, line := range lines {
		if matches := markdownHeadingPattern.FindStringSubmatch(line); matches != nil {
			return profile.NormalizeHeading(matches[2])
		}
	}
	return fallback
}

func clampHeadingLevel(level int) int {
	if level < 1 {
		return 1
	}
	if level > 6 {
		return 6
	}
	return level
}

func stripMarkdownFence(value string) string {
	value = strings.TrimSpace(value)
	if strings.HasPrefix(value, "```markdown") {
		value = strings.TrimSpace(strings.TrimPrefix(value, "```markdown"))
		value = strings.TrimSpace(strings.TrimSuffix(value, "```"))
	}
	return value
}

func truncateRunes(value string, limit int) string {
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit])
}

func lastRunes(value string, limit int) string {
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[len(runes)-limit:])
}

func nonEmptyStrings(values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			result = append(result, value)
		}
	}
	return result
}

func nonBlank(lines []string) []string {
	return nonEmptyStrings(lines)
}

// unbalancedFenceWarnings reports chapters that end inside a fenced code block. A
// page whose code sample wraps awkwardly can make the model close a fence mid
// command and emit a stray one; left alone the rest of the chapter renders as code.
func unbalancedFenceWarnings(chapters []documentconv.Chapter) []string {
	var warnings []string
	for _, chapter := range chapters {
		fences := 0
		for _, line := range strings.Split(chapter.Markdown, "\n") {
			if strings.HasPrefix(strings.TrimSpace(line), "```") {
				fences++
			}
		}
		if fences%2 != 0 {
			warnings = append(warnings, fmt.Sprintf(
				"chapter %s ends inside a code fence (%d fence markers); review it before importing",
				chapter.ExternalID, fences))
		}
	}
	return warnings
}
