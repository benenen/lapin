package pdf

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"github.com/benenen/lapin/internal/documentconv"
)

const (
	maxPDFBytes          = 100 << 20
	maxPDFXML            = 64 << 20
	maxPDFAssets         = 300
	maxPDFTemporaryBytes = 128 << 20
	maxPDFTemporaryFiles = 1200
	maxPDFCommandOutput  = 64 << 10
	maxPDFImageSide      = 8192
	maxPDFImagePixels    = 40_000_000
)

type Options struct {
	Profile Profile
}

type Converter struct {
	profile Profile
}

var _ documentconv.Converter = (*Converter)(nil)

func New(options Options) *Converter {
	profile := options.Profile
	if profile == nil {
		profile = GenericBookProfile()
	}
	return &Converter{profile: profile}
}

// Convert uses the language-neutral book profile. Call New with a specific
// Profile when the source follows language- or publisher-specific conventions.
func Convert(ctx context.Context, sourcePath string) (documentconv.Document, error) {
	return New(Options{}).Convert(ctx, sourcePath)
}

func (converter *Converter) Convert(ctx context.Context, sourcePath string) (documentconv.Document, error) {
	absolute, err := filepath.Abs(sourcePath)
	if err != nil {
		return documentconv.Document{}, fmt.Errorf("resolve PDF path: %w", err)
	}
	info, err := os.Stat(absolute)
	if err != nil || !info.Mode().IsRegular() || info.Size() < 1 || info.Size() > maxPDFBytes {
		return documentconv.Document{}, fmt.Errorf("PDF must be a readable regular file no larger than %d bytes", maxPDFBytes)
	}
	temporary, err := os.MkdirTemp("", "lapin-pdf-")
	if err != nil {
		return documentconv.Document{}, fmt.Errorf("create PDF conversion directory: %w", err)
	}
	defer os.RemoveAll(temporary)
	xmlPath := filepath.Join(temporary, "source.xml")
	conversionContext, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	command := exec.CommandContext(conversionContext, "pdftohtml", "-xml", "-hidden", "-nodrm", absolute, xmlPath)
	output, err := runBoundedPDFCommand(conversionContext, command, temporary)
	if err != nil {
		return documentconv.Document{}, fmt.Errorf("pdftohtml failed: %s", boundedCommandOutput(output))
	}
	xmlFile, err := os.Open(xmlPath)
	if err != nil {
		return documentconv.Document{}, fmt.Errorf("open pdftohtml output: %w", err)
	}
	xmlBody, err := io.ReadAll(io.LimitReader(xmlFile, maxPDFXML+1))
	_ = xmlFile.Close()
	if err != nil || len(xmlBody) > maxPDFXML {
		return documentconv.Document{}, fmt.Errorf("pdftohtml XML is invalid or too large")
	}
	var raw pdfXMLDocument
	if err := xml.Unmarshal(xmlBody, &raw); err != nil {
		return documentconv.Document{}, fmt.Errorf("parse pdftohtml XML: %w", err)
	}
	if err := augmentVectorFigures(conversionContext, absolute, temporary, &raw, converter.profile); err != nil {
		return documentconv.Document{}, err
	}
	chapters, assets, err := parsePDFMarkdownDocument(raw, temporary, converter.profile)
	if err != nil {
		return documentconv.Document{}, err
	}
	result := documentconv.Document{
		Chapters: make([]documentconv.Chapter, 0, len(chapters)),
		Assets:   make([]documentconv.Asset, 0, len(assets)),
	}
	for _, chapter := range chapters {
		result.Chapters = append(result.Chapters, documentconv.Chapter{
			ExternalID: chapter.ExternalID, Title: chapter.Title, Filename: chapter.Filename, Markdown: chapter.Content,
		})
	}
	for _, asset := range assets {
		result.Assets = append(result.Assets, documentconv.Asset{Key: asset.Key, Filename: asset.Filename, Content: asset.Content})
	}
	return result, nil
}

type boundedPDFOutput struct {
	value []byte
}

func (output *boundedPDFOutput) Write(value []byte) (int, error) {
	remaining := maxPDFCommandOutput - len(output.value)
	if remaining > 0 {
		output.value = append(output.value, value[:min(remaining, len(value))]...)
	}
	return len(value), nil
}

func runBoundedPDFCommand(ctx context.Context, command *exec.Cmd, temporaryDirectory string) ([]byte, error) {
	output := &boundedPDFOutput{value: make([]byte, 0, 4096)}
	command.Stdout = output
	command.Stderr = output
	if err := command.Start(); err != nil {
		return output.value, err
	}
	done := make(chan error, 1)
	go func() { done <- command.Wait() }()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case err := <-done:
			if limitErr := ensurePDFTemporaryDirectoryWithinLimits(temporaryDirectory); limitErr != nil {
				return output.value, limitErr
			}
			return output.value, err
		case <-ticker.C:
			if err := ensurePDFTemporaryDirectoryWithinLimits(temporaryDirectory); err != nil {
				_ = command.Process.Kill()
				<-done
				return output.value, err
			}
		case <-ctx.Done():
			_ = command.Process.Kill()
			<-done
			return output.value, ctx.Err()
		}
	}
}

func ensurePDFTemporaryDirectoryWithinLimits(directory string) error {
	files := 0
	var bytes int64
	err := filepath.WalkDir(directory, func(_ string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !entry.Type().IsRegular() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		files++
		bytes += info.Size()
		if files > maxPDFTemporaryFiles || bytes > maxPDFTemporaryBytes {
			return fmt.Errorf("PDF conversion temporary output exceeds its limit")
		}
		return nil
	})
	return err
}

func boundedCommandOutput(output []byte) string {
	const maxRunes = 512
	var builder strings.Builder
	count := 0
	for _, value := range string(output) {
		if count == maxRunes {
			builder.WriteString("...")
			break
		}
		if unicode.IsControl(value) || unicode.In(value, unicode.Cf) {
			builder.WriteByte(' ')
		} else {
			builder.WriteRune(value)
		}
		count++
	}
	return strings.TrimSpace(builder.String())
}
