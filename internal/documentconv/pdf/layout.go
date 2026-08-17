package pdf

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"html"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/benenen/lapin/internal/assetstore"
)

type pdfXMLDocument struct {
	Pages []pdfXMLPage `xml:"page"`
}

type pdfXMLPage struct {
	Number int           `xml:"number,attr"`
	Width  int           `xml:"width,attr"`
	Height int           `xml:"height,attr"`
	Fonts  []pdfXMLFont  `xml:"fontspec"`
	Texts  []pdfXMLText  `xml:"text"`
	Images []pdfXMLImage `xml:"image"`
}

type pdfXMLFont struct {
	ID     int    `xml:"id,attr"`
	Size   int    `xml:"size,attr"`
	Family string `xml:"family,attr"`
}

type pdfXMLText struct {
	Top      int    `xml:"top,attr"`
	Left     int    `xml:"left,attr"`
	Width    int    `xml:"width,attr"`
	Height   int    `xml:"height,attr"`
	Font     int    `xml:"font,attr"`
	InnerXML string `xml:",innerxml"`
}

type pdfXMLImage struct {
	Top     int    `xml:"top,attr"`
	Left    int    `xml:"left,attr"`
	Width   int    `xml:"width,attr"`
	Height  int    `xml:"height,attr"`
	Source  string `xml:"src,attr"`
	Caption string `xml:"-"`
}

type preparedPDFChapter struct {
	ExternalID string
	Title      string
	Filename   string
	Content    string
}

type preparedPDFAsset struct {
	Key      string
	Filename string
	Content  []byte
}

type semanticPDFBlock struct {
	Top          int
	Bottom       int
	Left         int
	Plain        string
	Markdown     string
	HeadingLevel int
	Image        *preparedPDFAsset
	Code         bool
	ListItem     bool
}

type pdfLine struct {
	Top      int
	Bottom   int
	Left     int
	Height   int
	FontSize int
	Code     bool
	Plain    string
	Markdown string
	Right    int
}

func parsePDFMarkdown(xmlBody []byte, xmlDirectory string, profile Profile) ([]preparedPDFChapter, []preparedPDFAsset, error) {
	var document pdfXMLDocument
	if err := xml.Unmarshal(xmlBody, &document); err != nil {
		return nil, nil, fmt.Errorf("parse pdftohtml XML: %w", err)
	}
	return parsePDFMarkdownDocument(document, xmlDirectory, profile)
}

func parsePDFMarkdownDocument(document pdfXMLDocument, xmlDirectory string, profile Profile) ([]preparedPDFChapter, []preparedPDFAsset, error) {
	if len(document.Pages) == 0 || len(document.Pages) > 2000 {
		return nil, nil, fmt.Errorf("PDF page count is not allowed")
	}
	chapters := make([]preparedPDFChapter, 0)
	assets := make([]preparedPDFAsset, 0)
	fonts := make(map[int]pdfXMLFont)
	var current *preparedPDFChapter
	for _, page := range document.Pages {
		if page.Width < 1 || page.Height < 1 || page.Width > 20_000 || page.Height > 20_000 {
			return nil, nil, fmt.Errorf("PDF page dimensions are not allowed")
		}
		for _, font := range page.Fonts {
			fonts[font.ID] = font
		}
		blocks, pageAssets, err := semanticBlocksForPage(page, xmlDirectory, fonts, profile)
		if err != nil {
			return nil, nil, err
		}
		assets = append(assets, pageAssets...)
		if len(assets) > maxPDFAssets || totalPDFAssetBytes(assets) > assetstore.MaxImportAssetBytes {
			return nil, nil, fmt.Errorf("PDF contains too many or too-large image assets")
		}
		for _, block := range blocks {
			if profile.IsChapterHeading(block.Plain, block.HeadingLevel) {
				if current != nil {
					current.Content = strings.TrimSpace(current.Content) + "\n"
					chapters = append(chapters, *current)
				}
				title := profile.NormalizeHeading(block.Plain)
				externalID, filename := profile.ChapterIdentity(title, len(chapters)+1)
				current = &preparedPDFChapter{ExternalID: externalID, Title: title, Filename: filename, Content: "## " + title + "\n\n"}
				continue
			}
			if current != nil {
				current.Content += block.Markdown
			}
		}
	}
	if current != nil {
		current.Content = strings.TrimSpace(current.Content) + "\n"
		chapters = append(chapters, *current)
	}
	if len(chapters) == 0 {
		return nil, nil, fmt.Errorf("PDF does not contain recognizable chapter headings")
	}
	return chapters, assets, nil
}

func semanticBlocksForPage(page pdfXMLPage, xmlDirectory string, fonts map[int]pdfXMLFont, profile Profile) ([]semanticPDFBlock, []preparedPDFAsset, error) {
	lines, err := groupPDFLines(page, fonts)
	if err != nil {
		return nil, nil, err
	}
	blocks := make([]semanticPDFBlock, 0, len(lines)+len(page.Images))
	assets := make([]preparedPDFAsset, 0, len(page.Images))
	for _, line := range lines {
		level := profile.HeadingLevel(line.Plain, line.FontSize)
		plain := line.Plain
		markdown := line.Markdown
		if level != 0 {
			plain = profile.NormalizeHeading(plain)
			markdown = plain
		}
		listMarkdown, listItem := normalizePDFListItem(markdown)
		if listItem {
			markdown = listMarkdown
		}
		blocks = append(blocks, semanticPDFBlock{
			Top: line.Top, Bottom: line.Bottom, Left: line.Left, Plain: plain, Markdown: markdown,
			HeadingLevel: level, Code: line.Code, ListItem: listItem,
		})
	}
	for index, image := range page.Images {
		imagePath, err := containedPDFImagePath(xmlDirectory, image.Source)
		if err != nil {
			return nil, nil, err
		}
		file, err := os.Open(imagePath)
		if err != nil {
			return nil, nil, fmt.Errorf("read extracted PDF image: %w", err)
		}
		content, err := io.ReadAll(io.LimitReader(file, assetstore.MaxAssetBytes+1))
		_ = file.Close()
		if err != nil || len(content) < 1 || len(content) > assetstore.MaxAssetBytes {
			return nil, nil, fmt.Errorf("extracted PDF image is invalid or too large")
		}
		extension := strings.ToLower(filepath.Ext(imagePath))
		if extension != ".png" && extension != ".jpg" && extension != ".jpeg" {
			return nil, nil, fmt.Errorf("pdftohtml produced an unsupported image type")
		}
		if !validPDFImageAsset(content, extension) {
			return nil, nil, fmt.Errorf("pdftohtml produced an invalid image or disallowed dimensions")
		}
		key := fmt.Sprintf("page-%03d-image-%02d", page.Number, index+1)
		filename := key + extension
		asset := preparedPDFAsset{Key: key, Filename: filename, Content: content}
		assets = append(assets, asset)
		blocks = append(blocks, semanticPDFBlock{Top: image.Top, Bottom: image.Top + image.Height, Left: image.Left, Plain: image.Caption, Image: &asset})
	}
	sort.SliceStable(blocks, func(i, j int) bool {
		if blocks[i].Top != blocks[j].Top {
			return blocks[i].Top < blocks[j].Top
		}
		return blocks[i].Left < blocks[j].Left
	})
	return renderSemanticPDFBlocks(blocks, profile), assets, nil
}

func validPDFImageAsset(content []byte, extension string) bool {
	configuration, format, err := image.DecodeConfig(bytes.NewReader(content))
	if err != nil || configuration.Width < 1 || configuration.Height < 1 || configuration.Width > maxPDFImageSide || configuration.Height > maxPDFImageSide || int64(configuration.Width)*int64(configuration.Height) > maxPDFImagePixels {
		return false
	}
	if extension == ".png" {
		return format == "png"
	}
	return format == "jpeg"
}

func containedPDFImagePath(directory, source string) (string, error) {
	directoryAbsolute, err := filepath.Abs(directory)
	if err != nil {
		return "", fmt.Errorf("resolve PDF conversion directory: %w", err)
	}
	candidate := source
	if !filepath.IsAbs(candidate) {
		candidate = filepath.Join(directoryAbsolute, candidate)
	}
	candidate = filepath.Clean(candidate)
	relative, err := filepath.Rel(directoryAbsolute, candidate)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("PDF image path escapes the conversion directory")
	}
	return candidate, nil
}

func groupPDFLines(page pdfXMLPage, fonts map[int]pdfXMLFont) ([]pdfLine, error) {
	texts := append([]pdfXMLText(nil), page.Texts...)
	sort.SliceStable(texts, func(i, j int) bool {
		if abs(texts[i].Top-texts[j].Top) <= 2 {
			return texts[i].Left < texts[j].Left
		}
		return texts[i].Top < texts[j].Top
	})
	lines := make([]pdfLine, 0)
	for _, text := range texts {
		if text.Top < 90 || (page.Height > 0 && text.Top > page.Height-100) {
			continue
		}
		plain, bold, err := decodePDFInlineText(text.InnerXML)
		if err != nil {
			return nil, fmt.Errorf("parse PDF text markup: %w", err)
		}
		plain = strings.TrimSpace(plain)
		if plain == "" {
			continue
		}
		font := fonts[text.Font]
		markdown := plain
		if bold {
			markdown = "**" + plain + "**"
		}
		if len(lines) == 0 || abs(lines[len(lines)-1].Top-text.Top) > 2 {
			lines = append(lines, pdfLine{
				Top: text.Top, Bottom: text.Top + text.Height, Left: text.Left, Height: text.Height,
				FontSize: font.Size, Code: isCodeFont(font.Family), Plain: plain, Markdown: markdown,
				Right: text.Left + text.Width,
			})
			continue
		}
		line := &lines[len(lines)-1]
		if text.Left-line.Right > max(24, font.Size*2) {
			line.Plain += " | " + plain
			line.Markdown += " | " + markdown
		} else {
			line.Plain = joinPDFText(line.Plain, plain)
			line.Markdown = joinPDFText(line.Markdown, markdown)
		}
		line.Bottom = max(line.Bottom, text.Top+text.Height)
		line.Right = max(line.Right, text.Left+text.Width)
		line.FontSize = max(line.FontSize, font.Size)
		line.Code = line.Code && isCodeFont(font.Family)
	}
	return lines, nil
}

func totalPDFAssetBytes(assets []preparedPDFAsset) int {
	total := 0
	for _, asset := range assets {
		total += len(asset.Content)
	}
	return total
}

func normalizePDFListItem(value string) (string, bool) {
	trimmed := strings.TrimSpace(value)
	for _, marker := range []string{"•", "●", "▪", "◦", "-"} {
		if strings.HasPrefix(trimmed, marker) && strings.TrimSpace(strings.TrimPrefix(trimmed, marker)) != "" {
			return "- " + strings.TrimSpace(strings.TrimPrefix(trimmed, marker)), true
		}
	}
	end := 0
	for end < len(trimmed) && trimmed[end] >= '0' && trimmed[end] <= '9' {
		end++
	}
	if end > 0 {
		remainder := trimmed[end:]
		for _, marker := range []string{".", ")"} {
			withoutMarker := strings.TrimPrefix(remainder, marker)
			if withoutMarker != remainder && len(withoutMarker) > 0 && unicode.IsSpace(rune(withoutMarker[0])) && strings.TrimSpace(withoutMarker) != "" {
				return trimmed[:end] + ". " + strings.TrimSpace(withoutMarker), true
			}
		}
		if withoutMarker := strings.TrimPrefix(remainder, "、"); withoutMarker != remainder && strings.TrimSpace(withoutMarker) != "" {
			return trimmed[:end] + ". " + strings.TrimSpace(withoutMarker), true
		}
	}
	return value, false
}

func decodePDFInlineText(innerXML string) (string, bool, error) {
	decoder := xml.NewDecoder(strings.NewReader("<text>" + innerXML + "</text>"))
	var text strings.Builder
	bold := false
	for {
		token, err := decoder.Token()
		if err != nil {
			if err == io.EOF {
				break
			}
			return "", false, err
		}
		switch value := token.(type) {
		case xml.StartElement:
			if strings.EqualFold(value.Name.Local, "b") || strings.EqualFold(value.Name.Local, "strong") {
				bold = true
			}
		case xml.CharData:
			text.Write(value)
		}
	}
	return html.UnescapeString(text.String()), bold, nil
}

func renderSemanticPDFBlocks(blocks []semanticPDFBlock, profile Profile) []semanticPDFBlock {
	result := make([]semanticPDFBlock, 0, len(blocks))
	paragraph := semanticPDFBlock{}
	codeLines := make([]string, 0)
	listLines := make([]string, 0)
	flushParagraph := func() {
		if paragraph.Markdown == "" {
			return
		}
		paragraph.Markdown = strings.TrimSpace(paragraph.Markdown) + "\n\n"
		result = append(result, paragraph)
		paragraph = semanticPDFBlock{}
	}
	flushCode := func() {
		if len(codeLines) == 0 {
			return
		}
		if len(codeLines) == 1 {
			result = append(result, semanticPDFBlock{Markdown: "`" + strings.ReplaceAll(codeLines[0], "`", "\\`") + "`\n\n"})
		} else {
			result = append(result, semanticPDFBlock{Markdown: "```\n" + strings.Join(codeLines, "\n") + "\n```\n\n"})
		}
		codeLines = codeLines[:0]
	}
	flushList := func() {
		if len(listLines) == 0 {
			return
		}
		result = append(result, semanticPDFBlock{Markdown: strings.Join(listLines, "\n") + "\n\n"})
		listLines = listLines[:0]
	}
	flushStructured := func() {
		flushParagraph()
		flushCode()
		flushList()
	}
	var pendingImage *semanticPDFBlock
	for _, block := range blocks {
		if block.Image != nil {
			flushStructured()
			if pendingImage != nil {
				caption := pendingImage.Plain
				if caption == "" {
					caption = "PDF 图片"
				}
				pendingImage.Markdown = fmt.Sprintf("![%s](lapin-asset://%s)\n\n", caption, pendingImage.Image.Key)
				result = append(result, *pendingImage)
			}
			copy := block
			pendingImage = &copy
			continue
		}
		if pendingImage != nil {
			caption := pendingImage.Plain
			if caption == "" {
				caption = fmt.Sprintf("第 %d 页图片", pendingImage.Top)
			}
			if profile.IsFigureCaption(block.Plain) && block.Top-pendingImage.Bottom < 80 {
				caption = block.Plain
				block = semanticPDFBlock{}
			}
			pendingImage.Markdown = fmt.Sprintf("![%s](lapin-asset://%s)\n\n*%s*\n\n", caption, pendingImage.Image.Key, caption)
			pendingImage.Plain = caption
			result = append(result, *pendingImage)
			pendingImage = nil
			if block.Markdown == "" {
				continue
			}
		}
		if block.Code {
			flushParagraph()
			flushList()
			codeLines = append(codeLines, block.Plain)
			continue
		}
		flushCode()
		if block.ListItem {
			flushParagraph()
			listLines = append(listLines, block.Markdown)
			continue
		}
		flushList()
		if block.HeadingLevel != 0 {
			flushParagraph()
			block.Markdown = strings.Repeat("#", block.HeadingLevel) + " " + block.Plain + "\n\n"
			result = append(result, block)
			continue
		}
		if paragraph.Markdown != "" && block.Top-paragraph.Bottom >= max(18, block.Bottom-block.Top+3) {
			flushParagraph()
		}
		if paragraph.Markdown == "" {
			paragraph = block
		} else {
			paragraph.Markdown = joinPDFText(paragraph.Markdown, block.Markdown)
			paragraph.Plain = joinPDFText(paragraph.Plain, block.Plain)
			paragraph.Bottom = block.Bottom
		}
	}
	if pendingImage != nil {
		caption := pendingImage.Plain
		if caption == "" {
			caption = "PDF 图片"
		}
		pendingImage.Markdown = fmt.Sprintf("![%s](lapin-asset://%s)\n\n", caption, pendingImage.Image.Key)
		result = append(result, *pendingImage)
	}
	flushStructured()
	return result
}

func joinPDFText(left, right string) string {
	left = strings.TrimSpace(left)
	right = strings.TrimSpace(right)
	if left == "" {
		return right
	}
	if right == "" {
		return left
	}
	last, _ := utf8.DecodeLastRuneInString(left)
	first, _ := utf8.DecodeRuneInString(right)
	if (isASCIIWord(last) && unicode.IsLetter(first)) || (unicode.IsLetter(last) && isASCIIWord(first)) {
		return left + " " + right
	}
	return left + right
}

func isASCIIWord(value rune) bool {
	return value < utf8.RuneSelf && (unicode.IsLetter(value) || unicode.IsDigit(value))
}

func isCodeFont(family string) bool {
	family = strings.ToLower(family)
	return strings.Contains(family, "mono") || strings.Contains(family, "menlo") || strings.Contains(family, "courier")
}

func abs(value int) int {
	if value < 0 {
		return -value
	}
	return value
}
