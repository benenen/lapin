package pdf

import "testing"

func TestGenericBookProfileUsesLayoutInsteadOfChineseLabels(t *testing.T) {
	profile := GenericBookProfile()

	if got := profile.HeadingLevel("Architecture", 26); got != 2 {
		t.Fatalf("HeadingLevel() = %d, want 2", got)
	}
	if !profile.IsChapterHeading("Architecture", 2) {
		t.Fatal("a generic level-two heading should start a chapter")
	}
	if profile.IsFigureCaption("图 1-1 中文图题") {
		t.Fatal("the generic profile must not contain Chinese figure rules")
	}
	if got, _ := profile.ChapterIdentity("Architecture", 3); got != "chapter-03" {
		t.Fatalf("ChapterIdentity() = %q, want chapter-03", got)
	}
}

func TestChineseTechnicalBookProfileKeepsLanguageSpecificRules(t *testing.T) {
	profile := ChineseTechnicalBookProfile()

	if !profile.IsChapterHeading("第 3 章 Agent 架构", 2) {
		t.Fatal("Chinese chapter heading was not recognized")
	}
	if profile.IsChapterHeading("目录", 2) {
		t.Fatal("table-of-contents heading must not become a chapter")
	}
	if !profile.IsFigureCaption("图 3-2 执行流程") {
		t.Fatal("Chinese figure caption was not recognized")
	}
	if got := profile.NormalizeHeading("3.2执行流程"); got != "3.2 执行流程" {
		t.Fatalf("NormalizeHeading() = %q", got)
	}
	if got, _ := profile.ChapterIdentity("第 3 章 Agent 架构", 1); got != "chapter-03" {
		t.Fatalf("ChapterIdentity() = %q, want chapter-03", got)
	}
}

func TestDecodePDFInlineTextUsesXMLTokens(t *testing.T) {
	plain, bold, err := decodePDFInlineText(`甲 &amp; <b>乙 <i>丙</i></b>`)
	if err != nil {
		t.Fatal(err)
	}
	if plain != "甲 & 乙 丙" {
		t.Fatalf("plain = %q", plain)
	}
	if !bold {
		t.Fatal("bold markup was not detected")
	}
}

func TestDecodePDFInlineTextRejectsMalformedXML(t *testing.T) {
	if _, _, err := decodePDFInlineText(`<b>broken`); err == nil {
		t.Fatal("expected malformed XML to be rejected")
	}
}
