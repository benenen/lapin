package handler

import "testing"

func TestMarkdownAssetReferencesAcceptsOnlyLocalInlineAssets(t *testing.T) {
	references, valid := markdownAssetReferences(`before ![图](/api/v1/assets/abcdefghij/content "caption") after`)
	if !valid || len(references) != 1 || references[0] != "abcdefghij" {
		t.Fatalf("references = %#v, valid = %v", references, valid)
	}
	for _, content := range []string{
		`![tracker](https://tracker.example/pixel.png)`,
		`![inline](data:image/png;base64,AAAA)`,
		`![reference][asset]`,
		`![wrong](/api/v1/assets/short/content)`,
	} {
		if _, valid := markdownAssetReferences(content); valid {
			t.Fatalf("unsafe image reference was accepted: %s", content)
		}
	}
	if references, valid := markdownAssetReferences(`literal \![not an image](https://example.com)`); !valid || len(references) != 0 {
		t.Fatalf("escaped image marker = %#v, valid = %v", references, valid)
	}
}
