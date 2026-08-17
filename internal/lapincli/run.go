package lapincli

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const (
	exitSuccess = 0
	exitRemote  = 1
	exitUsage   = 2
)

type Dependencies struct {
	LookupEnv  func(string) (string, bool)
	HTTPClient *http.Client
	Stdout     io.Writer
	Stderr     io.Writer
}

func Run(ctx context.Context, args []string, dependencies Dependencies) int {
	if dependencies.LookupEnv == nil || dependencies.Stdout == nil || dependencies.Stderr == nil {
		return exitUsage
	}
	if len(args) == 1 && (args[0] == "help" || args[0] == "-h" || args[0] == "--help") {
		writeUsage(dependencies.Stdout)
		return exitSuccess
	}
	if len(args) >= 2 && args[0] == "course" && args[1] == "prepare-pdf" {
		return runPreparePDFCommand(ctx, args[2:], dependencies)
	}
	if len(args) < 2 || args[0] != "course" || args[1] != "import" {
		fmt.Fprintln(dependencies.Stderr, "usage error: expected 'course import'")
		writeUsage(dependencies.Stderr)
		return exitUsage
	}

	flags := flag.NewFlagSet("course import", flag.ContinueOnError)
	flags.SetOutput(dependencies.Stderr)
	manifestPath := flags.String("manifest", "", "path to a version 1 or 2 course manifest")
	baseURLFlag := flags.String("base-url", "", "Lapin server origin (or use LAPIN_BASE_URL)")
	flags.Usage = func() { writeImportUsage(dependencies.Stderr) }
	if err := flags.Parse(args[2:]); err != nil {
		if err == flag.ErrHelp {
			return exitSuccess
		}
		return exitUsage
	}
	if flags.NArg() != 0 || strings.TrimSpace(*manifestPath) == "" {
		fmt.Fprintln(dependencies.Stderr, "usage error: --manifest is required and no positional arguments are accepted")
		return exitUsage
	}
	token, ok := dependencies.LookupEnv("LAPIN_ACCESS_TOKEN")
	token = strings.TrimSpace(token)
	if !ok || token == "" {
		fmt.Fprintln(dependencies.Stderr, "configuration error: LAPIN_ACCESS_TOKEN is required")
		return exitUsage
	}
	if !strings.HasPrefix(token, "lpn_") {
		fmt.Fprintln(dependencies.Stderr, "configuration error: LAPIN_ACCESS_TOKEN must start with lpn_")
		return exitUsage
	}
	baseURL := strings.TrimSpace(*baseURLFlag)
	if baseURL == "" {
		if configured, exists := dependencies.LookupEnv("LAPIN_BASE_URL"); exists && strings.TrimSpace(configured) != "" {
			baseURL = strings.TrimSpace(configured)
		} else {
			baseURL = defaultBaseURL
		}
	}
	bundle, err := loadBundle(*manifestPath)
	if err != nil {
		fmt.Fprintf(dependencies.Stderr, "manifest error: %s\n", sanitizeDiagnostic(err.Error(), ""))
		return exitUsage
	}
	body, err := json.Marshal(bundle.Request)
	if err != nil {
		fmt.Fprintf(dependencies.Stderr, "manifest error: %s\n", sanitizeDiagnostic(err.Error(), ""))
		return exitUsage
	}
	var result importResult
	if bundle.Version == manifestVersionV2 || len(body) > maxRequestBytes {
		result, err = importStagedBundle(ctx, baseURL, token, bundle, dependencies.HTTPClient)
	} else {
		result, err = importCourse(ctx, baseURL, token, body, dependencies.HTTPClient)
	}
	if err != nil {
		fmt.Fprintf(dependencies.Stderr, "import error: %s\n", sanitizeDiagnostic(err.Error(), token))
		return exitRemote
	}
	result.ExternalID = bundle.Request.ExternalID
	result.Title = bundle.Request.Title
	result.ChapterCount = countImportedChapters(bundle.Request.Chapters)
	encoder := json.NewEncoder(dependencies.Stdout)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(result); err != nil {
		fmt.Fprintf(dependencies.Stderr, "output error: %v\n", err)
		return exitRemote
	}
	return exitSuccess
}

func countImportedChapters(chapters []importChapterRequest) int {
	total := 0
	stack := append([]importChapterRequest(nil), chapters...)
	for len(stack) > 0 {
		last := len(stack) - 1
		chapter := stack[last]
		stack = stack[:last]
		total++
		stack = append(stack, chapter.Children...)
	}
	return total
}

func writeUsage(writer io.Writer) {
	fmt.Fprintln(writer, "Usage:")
	fmt.Fprintln(writer, "  lapin-cli course import --manifest <course.json> [--base-url <origin>]")
	fmt.Fprintln(writer, "  lapin-cli course prepare-pdf --pdf <book.pdf> --output <bundle> --external-id <id> --title <title> [--profile <name>] [--reuse-chapter-tree <manifest>]")
}

func writeImportUsage(writer io.Writer) {
	writeUsage(writer)
	fmt.Fprintln(writer, "")
	fmt.Fprintln(writer, "Access Token is read only from LAPIN_ACCESS_TOKEN.")
}
