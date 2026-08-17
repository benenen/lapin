package pdf

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var (
	numberedHeadingPattern   = regexp.MustCompile(`^[0-9]+(?:\.[0-9]+)+\s+`)
	englishFigurePattern     = regexp.MustCompile(`(?i)^(?:figure|fig\.)\s*[0-9]+(?:[-.][0-9]+)?(?:\s|$)`)
	chineseTopLevelPattern   = regexp.MustCompile(`^第\s*([0-9]+)\s*章`)
	chineseFigurePattern     = regexp.MustCompile(`^图\s*[0-9]+(?:[-－][0-9]+)?(?:\s|$)`)
	compactChineseTopPattern = regexp.MustCompile(`^第\s*([0-9]+)\s*章\s*(.*)$`)
	compactSubheadingPattern = regexp.MustCompile(`^([0-9]+(?:\.[0-9]+)+)\s*(\S.*)$`)
)

// Profile contains document-structure rules that are not intrinsic to PDF.
// Language- and publisher-specific recognition belongs here, while the PDF
// extractor remains responsible only for coordinates, fonts, text and images.
type Profile interface {
	HeadingLevel(text string, fontSize int) int
	IsChapterHeading(text string, headingLevel int) bool
	IsFigureCaption(text string) bool
	NormalizeHeading(text string) string
	ChapterIdentity(title string, ordinal int) (externalID, filename string)
}

type genericBookProfile struct{}

// GenericBookProfile uses layout and common dotted-number headings only. It
// deliberately contains no Chinese book labels.
func GenericBookProfile() Profile {
	return genericBookProfile{}
}

func (genericBookProfile) HeadingLevel(text string, fontSize int) int {
	if fontSize >= 24 {
		return 2
	}
	if fontSize >= 18 || numberedHeadingPattern.MatchString(strings.TrimSpace(text)) {
		return 3
	}
	return 0
}

func (genericBookProfile) IsChapterHeading(_ string, headingLevel int) bool {
	return headingLevel == 2
}

func (genericBookProfile) IsFigureCaption(text string) bool {
	return englishFigurePattern.MatchString(strings.TrimSpace(text))
}

func (genericBookProfile) NormalizeHeading(text string) string {
	return strings.TrimSpace(text)
}

func (genericBookProfile) ChapterIdentity(_ string, ordinal int) (string, string) {
	id := fmt.Sprintf("chapter-%02d", ordinal)
	return id, id + ".md"
}

type chineseTechnicalBookProfile struct {
	genericBookProfile
}

// ChineseTechnicalBookProfile recognizes the conventions used by Chinese
// technical books, including “第 N 章”, dotted subsection numbers and “图 N-N”.
func ChineseTechnicalBookProfile() Profile {
	return chineseTechnicalBookProfile{}
}

func (chineseTechnicalBookProfile) IsChapterHeading(text string, headingLevel int) bool {
	if headingLevel != 2 {
		return false
	}
	text = strings.TrimSpace(text)
	return text == "引言" || strings.HasPrefix(text, "后记") || chineseTopLevelPattern.MatchString(text)
}

func (chineseTechnicalBookProfile) IsFigureCaption(text string) bool {
	return chineseFigurePattern.MatchString(strings.TrimSpace(text))
}

func (chineseTechnicalBookProfile) NormalizeHeading(text string) string {
	text = strings.TrimSpace(text)
	if matches := compactChineseTopPattern.FindStringSubmatch(text); len(matches) == 3 {
		return strings.TrimSpace("第 " + matches[1] + " 章 " + matches[2])
	}
	if matches := compactSubheadingPattern.FindStringSubmatch(text); len(matches) == 3 {
		return matches[1] + " " + matches[2]
	}
	return text
}

func (chineseTechnicalBookProfile) ChapterIdentity(title string, ordinal int) (string, string) {
	if strings.TrimSpace(title) == "引言" {
		return "introduction", "introduction.md"
	}
	if matches := chineseTopLevelPattern.FindStringSubmatch(title); len(matches) == 2 {
		number, _ := strconv.Atoi(matches[1])
		id := fmt.Sprintf("chapter-%02d", number)
		return id, id + ".md"
	}
	if strings.HasPrefix(strings.TrimSpace(title), "后记") {
		return "postscript", "postscript.md"
	}
	return genericBookProfile{}.ChapterIdentity(title, ordinal)
}
