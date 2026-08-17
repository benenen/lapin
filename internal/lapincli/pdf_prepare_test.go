package lapincli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunPreparePDFRestoresParagraphsHeadingsAndImageAssets(t *testing.T) {
	root := t.TempDir()
	fakeBin := filepath.Join(root, "bin")
	if err := os.Mkdir(fakeBin, 0o700); err != nil {
		t.Fatal(err)
	}
	fakePDFToHTML := filepath.Join(fakeBin, "pdftohtml")
	script := `#!/usr/bin/env bash
set -euo pipefail
output="${@: -1}"
mkdir -p "$(dirname "$output")"
cat >"$output" <<'XML'
<?xml version="1.0" encoding="UTF-8"?>
<pdf2xml>
  <page number="1" top="0" left="0" height="1262" width="892">
    <fontspec id="0" size="26" family="Heading" color="#1e396b"/>
    <fontspec id="1" size="15" family="Body" color="#000000"/>
    <fontspec id="2" size="22" family="Heading" color="#1e396b"/>
		<fontspec id="3" size="14" family="Courier" color="#000000"/>
    <text top="120" left="300" width="260" height="26" font="0">第 1 章 示例</text>
    <text top="200" left="115" width="680" height="15" font="1">第一段跨视觉行的前半，</text>
    <text top="223" left="85" width="680" height="15" font="1">后半应当合并。</text>
    <text top="270" left="115" width="680" height="15" font="1">第二段必须独立。</text>
		<text top="310" left="115" width="300" height="15" font="1">• 第一项</text>
		<text top="333" left="115" width="300" height="15" font="1">• 第二项</text>
    <text top="380" left="90" width="200" height="22" font="2">1.1 小节标题</text>
		<text top="425" left="115" width="300" height="14" font="3">{"tool":"search"}</text>
		<text top="447" left="115" width="300" height="14" font="3">return result</text>
		<text top="485" left="115" width="120" height="15" font="1">参数</text>
		<text top="485" left="420" width="180" height="15" font="1">说明</text>
		<text top="508" left="115" width="120" height="15" font="1">timeout</text>
		<text top="508" left="420" width="180" height="15" font="1">超时时间</text>
    <image top="550" left="100" width="300" height="120" src="source-1_1.png"/>
    <text top="680" left="320" width="180" height="15" font="1">图 1-1 示例图片</text>
    <text top="1220" left="440" width="10" height="15" font="1">1</text>
  </page>
  <page number="2" top="0" left="0" height="1262" width="892">
    <fontspec id="1" size="15" family="Body" color="#000000"/>
    <text top="300" left="220" width="300" height="15" font="1">输入 → Agent → 输出</text>
    <text top="520" left="320" width="180" height="15" font="1">图 1-2 矢量流程图</text>
    <text top="1220" left="440" width="10" height="15" font="1">2</text>
  </page>
</pdf2xml>
XML
printf '%s' 'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=' | base64 -d >"$(dirname "$output")/source-1_1.png"
`
	if err := os.WriteFile(fakePDFToHTML, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	fakePDFToCairo := filepath.Join(fakeBin, "pdftocairo")
	cairoScript := `#!/usr/bin/env bash
set -euo pipefail
output="${@: -1}"
printf '%s' 'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=' | base64 -d >"${output}.png"
`
	if err := os.WriteFile(fakePDFToCairo, []byte(cairoScript), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"))
	pdfPath := filepath.Join(root, "fixture.pdf")
	writeTestFile(t, pdfPath, []byte("fake pdf"))
	outputDir := filepath.Join(root, "bundle")
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := Run(context.Background(), []string{
		"course", "prepare-pdf", "--pdf", pdfPath, "--output", outputDir,
		"--external-id", "fixture-pdf", "--title", "Fixture PDF",
	}, Dependencies{
		LookupEnv: func(string) (string, bool) { return "", false },
		Stdout:    &stdout, Stderr: &stderr,
	})
	if exitCode != exitSuccess {
		t.Fatalf("exit = %d, stderr = %q", exitCode, stderr.String())
	}
	markdown, err := os.ReadFile(filepath.Join(outputDir, "chapters", "chapter-01.md"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(markdown)
	for _, expected := range []string{
		"## 第 1 章 示例",
		"第一段跨视觉行的前半，后半应当合并。\n\n第二段必须独立。",
		"### 1.1 小节标题",
		"- 第一项\n- 第二项",
		"```\n{\"tool\":\"search\"}\nreturn result\n```",
		"参数 | 说明 timeout | 超时时间",
		"![图 1-1 示例图片](lapin-asset://page-001-image-01)",
		"![图 1-2 矢量流程图](lapin-asset://page-002-image-01)",
	} {
		if !strings.Contains(text, expected) {
			t.Fatalf("Markdown missing %q:\n%s", expected, text)
		}
	}
	if strings.Count(text, "图 1-2 矢量流程图") != 2 {
		t.Fatalf("vector figure caption should appear only as image alt and visible caption:\n%s", text)
	}
	manifest, err := os.ReadFile(filepath.Join(outputDir, "course.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(manifest), `"version": 2`) || !strings.Contains(string(manifest), `"key": "page-001-image-01"`) || !strings.Contains(string(manifest), `"key": "page-002-image-01"`) || !strings.Contains(string(manifest), `"content_file": "chapters/chapter-01.md"`) {
		t.Fatalf("manifest = %s", manifest)
	}
	if _, err := os.Stat(filepath.Join(outputDir, "assets", "page-001-image-01.png")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(outputDir, "assets", "page-002-image-01.png")); err != nil {
		t.Fatal(err)
	}
}

func TestReusePDFChapterTreePreservesStableExternalIDsAndGrouping(t *testing.T) {
	root := t.TempDir()
	manifestPath := filepath.Join(root, "course.json")
	writeTestFile(t, manifestPath, []byte(`{
		"version":2,"external_id":"book","title":"Book","assets":[],
		"chapters":[{"external_id":"part-stable","title":"Part","children":[
			{"external_id":"chapter-stable","title":"Chapter One","content_file":"old.md"}
		]}]
	}`))

	result, err := reusePDFChapterTree(manifestPath, []chapterManifest{
		{ExternalID: "chapter-01", Title: "Chapter One", ContentFile: "chapters/chapter-01.md"},
		{ExternalID: "appendix", Title: "Appendix", ContentFile: "chapters/appendix.md"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 2 || result[0].ExternalID != "part-stable" || len(result[0].Children) != 1 || result[0].Children[0].ExternalID != "chapter-stable" || result[0].Children[0].ContentFile != "chapters/chapter-01.md" || result[1].ExternalID != "appendix" {
		t.Fatalf("reused chapter tree = %#v", result)
	}
}
