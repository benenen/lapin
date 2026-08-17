package documentconv

import "context"

// Converter turns one source document into Lapin's format-neutral course model.
// Format packages such as documentconv/pdf implement this interface.
type Converter interface {
	Convert(context.Context, string) (Document, error)
}

type Document struct {
	Chapters []Chapter
	Assets   []Asset
	Warnings []string
}

type Chapter struct {
	ExternalID string
	Title      string
	Filename   string
	Markdown   string
}

type Asset struct {
	Key      string
	Filename string
	Content  []byte
}
