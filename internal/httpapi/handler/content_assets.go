package handler

import (
	"context"
	"strings"

	"github.com/benenen/lapin/internal/database"
)

func (h *Handler) replaceChapterAssetsFromContent(ctx context.Context, db database.DBTX, ownerID, chapterID int64, content string) error {
	references, valid := markdownAssetReferences(content)
	if !valid {
		return errInvalidSubject
	}
	assetIDs := make([]int64, 0, len(references))
	seen := make(map[int64]struct{}, len(references))
	for _, encodedID := range references {
		assetID, err := h.ids.Decode(encodedID)
		if err != nil {
			return errInvalidSubject
		}
		if _, exists := seen[assetID]; exists {
			continue
		}
		if _, err := database.FindAssetForOwnerByID(ctx, db, assetID, ownerID); err != nil {
			return errInvalidSubject
		}
		seen[assetID] = struct{}{}
		assetIDs = append(assetIDs, assetID)
	}
	return database.ReplaceChapterAssets(ctx, db, chapterID, assetIDs)
}

// markdownAssetReferences accepts the standard inline image form emitted by
// Tiptap and lapin-cli. Reference-style, data, blob and remote images are
// rejected so rendering a course never contacts a third-party image host.
func markdownAssetReferences(content string) ([]string, bool) {
	result := make([]string, 0)
	for offset := 0; offset < len(content); {
		relative := strings.Index(content[offset:], "![")
		if relative < 0 {
			break
		}
		start := offset + relative
		if escapedMarkdownByte(content, start) {
			offset = start + 2
			continue
		}
		labelEnd := findUnescapedMarkdownByte(content, start+2, ']')
		if labelEnd < 0 {
			return nil, false
		}
		if labelEnd+1 >= len(content) || content[labelEnd+1] != '(' {
			if labelEnd+1 < len(content) && content[labelEnd+1] == '[' {
				return nil, false
			}
			offset = labelEnd + 1
			continue
		}
		destinationEnd := findUnescapedMarkdownByte(content, labelEnd+2, ')')
		if destinationEnd < 0 {
			return nil, false
		}
		destination := strings.TrimSpace(content[labelEnd+2 : destinationEnd])
		fields := strings.Fields(destination)
		if len(fields) < 1 {
			return nil, false
		}
		url := strings.Trim(fields[0], "<>")
		matches := assetURLPattern.FindStringSubmatch(url)
		if len(matches) != 2 {
			return nil, false
		}
		if len(fields) > 1 {
			title := strings.TrimSpace(destination[len(fields[0]):])
			if len(title) < 2 || !((title[0] == '"' && title[len(title)-1] == '"') || (title[0] == '\'' && title[len(title)-1] == '\'')) {
				return nil, false
			}
		}
		result = append(result, matches[1])
		offset = destinationEnd + 1
	}
	return result, true
}

func findUnescapedMarkdownByte(value string, start int, target byte) int {
	for index := start; index < len(value); index++ {
		if value[index] == target && !escapedMarkdownByte(value, index) {
			return index
		}
	}
	return -1
}

func escapedMarkdownByte(value string, index int) bool {
	backslashes := 0
	for index--; index >= 0 && value[index] == '\\'; index-- {
		backslashes++
	}
	return backslashes%2 == 1
}
