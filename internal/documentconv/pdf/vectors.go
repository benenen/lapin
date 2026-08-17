package pdf

import (
	"context"
	"fmt"
	"image"
	_ "image/jpeg"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"sort"

	"github.com/benenen/lapin/internal/assetstore"
)

func augmentVectorFigures(ctx context.Context, pdfPath, temporaryDirectory string, document *pdfXMLDocument, profile Profile) error {
	fonts := make(map[int]pdfXMLFont)
	for pageIndex := range document.Pages {
		page := &document.Pages[pageIndex]
		for _, font := range page.Fonts {
			fonts[font.ID] = font
		}
		lines, err := groupPDFLines(*page, fonts)
		if err != nil {
			return err
		}
		captions := make([]pdfLine, 0)
		for _, line := range lines {
			if profile.IsFigureCaption(line.Plain) && !hasNearbyPDFImage(page.Images, line.Top) {
				captions = append(captions, line)
			}
		}
		if len(captions) == 0 {
			continue
		}
		pageImage, err := renderPDFPage(ctx, pdfPath, temporaryDirectory, page.Number)
		if err != nil {
			return err
		}
		for captionIndex, caption := range captions {
			cropTop := max(90, caption.Top-450)
			cropBottom := max(cropTop+1, caption.Top-5)
			bounds := pageImage.Bounds()
			left := scalePDFCoordinate(60, page.Width, bounds.Dx())
			right := scalePDFCoordinate(max(61, page.Width-60), page.Width, bounds.Dx())
			top := scalePDFCoordinate(cropTop, page.Height, bounds.Dy())
			bottom := scalePDFCoordinate(cropBottom, page.Height, bounds.Dy())
			crop := image.Rect(left, top, max(left+1, right), max(top+1, bottom)).Intersect(bounds)
			if crop.Empty() {
				crop = bounds
			}
			filename := fmt.Sprintf("vector-page-%03d-%02d.png", page.Number, captionIndex+1)
			path := filepath.Join(temporaryDirectory, filename)
			file, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
			if err != nil {
				return fmt.Errorf("create vector figure image: %w", err)
			}
			subImage, ok := pageImage.(interface {
				SubImage(image.Rectangle) image.Image
			})
			if !ok {
				_ = file.Close()
				return fmt.Errorf("rendered PDF page cannot be cropped")
			}
			if err := png.Encode(file, subImage.SubImage(crop)); err != nil {
				_ = file.Close()
				return fmt.Errorf("encode vector figure image: %w", err)
			}
			if err := file.Close(); err != nil {
				return fmt.Errorf("close vector figure image: %w", err)
			}
			info, err := os.Stat(path)
			if err != nil || info.Size() < 1 || info.Size() > assetstore.MaxAssetBytes {
				return fmt.Errorf("rendered vector figure is invalid or too large")
			}
			if err := ensurePDFTemporaryDirectoryWithinLimits(temporaryDirectory); err != nil {
				return err
			}
			page.Images = append(page.Images, pdfXMLImage{
				Top: max(0, caption.Top-1), Left: 60, Width: max(1, page.Width-120), Height: 1, Source: filename, Caption: caption.Plain,
			})
		}
		sort.SliceStable(page.Images, func(i, j int) bool { return page.Images[i].Top < page.Images[j].Top })
	}
	return nil
}

func hasNearbyPDFImage(images []pdfXMLImage, captionTop int) bool {
	for _, candidate := range images {
		bottom := candidate.Top + candidate.Height
		if candidate.Top < captionTop && captionTop-bottom >= -10 && captionTop-bottom < 100 {
			return true
		}
	}
	return false
}

func renderPDFPage(ctx context.Context, pdfPath, temporaryDirectory string, page int) (image.Image, error) {
	prefix := filepath.Join(temporaryDirectory, fmt.Sprintf("render-page-%03d", page))
	command := exec.CommandContext(ctx, "pdftocairo", "-png", "-f", fmt.Sprint(page), "-l", fmt.Sprint(page), "-singlefile", "-r", "144", pdfPath, prefix)
	output, err := runBoundedPDFCommand(ctx, command, temporaryDirectory)
	if err != nil {
		return nil, fmt.Errorf("pdftocairo failed for page %d: %s", page, boundedCommandOutput(output))
	}
	renderedPath := prefix + ".png"
	file, err := os.Open(renderedPath)
	if err != nil {
		return nil, fmt.Errorf("open rendered PDF page: %w", err)
	}
	defer os.Remove(renderedPath)
	configuration, _, err := image.DecodeConfig(file)
	if err != nil || configuration.Width < 1 || configuration.Height < 1 || configuration.Width > maxPDFImageSide || configuration.Height > maxPDFImageSide || int64(configuration.Width)*int64(configuration.Height) > maxPDFImagePixels {
		_ = file.Close()
		return nil, fmt.Errorf("rendered PDF page dimensions are not allowed")
	}
	if _, err := file.Seek(0, 0); err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("rewind rendered PDF page: %w", err)
	}
	pageImage, _, err := image.Decode(file)
	_ = file.Close()
	if err != nil {
		return nil, fmt.Errorf("decode rendered PDF page: %w", err)
	}
	return pageImage, nil
}

func scalePDFCoordinate(value, sourceSize, targetSize int) int {
	if sourceSize <= 0 || targetSize <= 1 {
		return 0
	}
	scaled := int(float64(value) * float64(targetSize) / float64(sourceSize))
	if scaled < 0 {
		return 0
	}
	if scaled >= targetSize {
		return targetSize - 1
	}
	return scaled
}
