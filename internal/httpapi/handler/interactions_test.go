package handler

import (
	"fmt"
	"strings"
	"testing"
)

func TestValidWhiteboardDataRejectsUnexpectedAppState(t *testing.T) {
	data := testWhiteboardData("chapter-1", `{"viewBackgroundColor":"transparent","collaborators":[]}`, nil)
	if validWhiteboardData(data, "chapter-1") {
		t.Fatal("unexpected appState field was accepted")
	}
}

func TestValidWhiteboardDataChecksElementReferences(t *testing.T) {
	rectangle := testWhiteboardElement("shape-1", "rectangle", `,"boundElements":[{"id":"arrow-1","type":"arrow"}]`)
	boundArrow := testWhiteboardElement(
		"arrow-1",
		"arrow",
		`,"points":[[0,0],[10,10]],"lastCommittedPoint":null,"startBinding":{"elementId":"shape-1","focus":0,"gap":1},"endBinding":null,"startArrowhead":null,"endArrowhead":"arrow","elbowed":false`,
	)
	if !validWhiteboardData(testWhiteboardData("chapter-1", `{"viewBackgroundColor":"transparent"}`, [][]byte{rectangle, boundArrow}), "chapter-1") {
		t.Fatal("valid element binding was rejected")
	}

	danglingArrow := testWhiteboardElement(
		"arrow-1",
		"arrow",
		`,"points":[[0,0],[10,10]],"lastCommittedPoint":null,"startBinding":{"elementId":"missing","focus":0,"gap":1},"endBinding":null,"startArrowhead":null,"endArrowhead":"arrow","elbowed":false`,
	)
	if validWhiteboardData(testWhiteboardData("chapter-1", `{"viewBackgroundColor":"transparent"}`, [][]byte{danglingArrow}), "chapter-1") {
		t.Fatal("dangling element binding was accepted")
	}

	framedRectangle := testWhiteboardElement("shape-1", "rectangle", `,"frameId":"missing-frame"`)
	if validWhiteboardData(testWhiteboardData("chapter-1", `{"viewBackgroundColor":"transparent"}`, [][]byte{framedRectangle}), "chapter-1") {
		t.Fatal("reference to a disabled frame was accepted")
	}
}

func TestValidWhiteboardElementRejectsMalformedRelationshipFields(t *testing.T) {
	tests := []struct {
		name  string
		extra string
	}{
		{name: "roundness", extra: `,"roundness":"rounded"`},
		{name: "bound elements", extra: `,"boundElements":[{"id":1,"type":"text"}]`},
		{name: "frame id", extra: `,"frameId":42`},
		{name: "link", extra: `,"link":false`},
		{name: "unknown field", extra: `,"selectedElementIds":{}`},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, ok := validWhiteboardElement(testWhiteboardElement("unsafe", "rectangle", test.extra)); ok {
				t.Fatalf("malformed %s field was accepted", test.name)
			}
		})
	}
}

func TestValidWhiteboardElementSupportsRelationshipFields(t *testing.T) {
	rectangle := testWhiteboardElement(
		"shape-1",
		"rectangle",
		`,"roundness":{"type":3,"value":12},"boundElements":[{"id":"arrow-1","type":"arrow"}],"frameId":"frame-1","link":"https://example.com"`,
	)
	if _, ok := validWhiteboardElement(rectangle); !ok {
		t.Fatal("valid generic relationship fields were rejected")
	}

	elbowArrow := testWhiteboardElement(
		"arrow-1",
		"arrow",
		`,"points":[[0,0],[10,10]],"lastCommittedPoint":[10,10],"startBinding":{"elementId":"shape-1","focus":0,"gap":1,"fixedPoint":[0.5,0.5]},"endBinding":null,"startArrowhead":"dot","endArrowhead":"arrow","elbowed":true,"fixedSegments":[{"start":[0,0],"end":[10,10],"index":0}],"startIsSpecial":null,"endIsSpecial":false`,
	)
	if _, ok := validWhiteboardElement(elbowArrow); !ok {
		t.Fatal("valid elbow arrow fields were rejected")
	}
}

func TestValidWhiteboardElementRejectsInvalidRelationshipValues(t *testing.T) {
	tests := []struct {
		name  string
		extra string
	}{
		{name: "roundness type", extra: `,"roundness":{"type":9}`},
		{name: "empty frame id", extra: `,"frameId":""`},
		{name: "bound type", extra: `,"boundElements":[{"id":"shape-1","type":"image"}]`},
		{name: "long link", extra: `,"link":"` + strings.Repeat("x", 2_049) + `"`},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, ok := validWhiteboardElement(testWhiteboardElement("unsafe", "rectangle", test.extra)); ok {
				t.Fatalf("invalid %s was accepted", test.name)
			}
		})
	}
}

func TestValidWhiteboardElementSupportsPrimaryToolbarShapes(t *testing.T) {
	tests := []struct {
		elementType string
		extra       string
	}{
		{elementType: "rectangle"},
		{elementType: "diamond"},
		{elementType: "ellipse"},
		{elementType: "arrow", extra: `,"points":[[0,0],[10,10]],"lastCommittedPoint":null,"startBinding":null,"endBinding":null,"startArrowhead":null,"endArrowhead":"arrow","elbowed":false`},
		{elementType: "line", extra: `,"points":[[0,0],[10,10]],"lastCommittedPoint":null,"startBinding":null,"endBinding":null,"startArrowhead":null,"endArrowhead":null`},
		{elementType: "freedraw", extra: `,"points":[[0,0],[10,10]],"pressures":[0.2,0.8],"simulatePressure":false,"lastCommittedPoint":[10,10]`},
		{elementType: "text", extra: `,"text":"正文","originalText":"正文","fontSize":20,"fontFamily":1,"textAlign":"left","verticalAlign":"top","lineHeight":1.25,"containerId":null,"autoResize":true`},
	}

	for _, test := range tests {
		t.Run(test.elementType, func(t *testing.T) {
			id, ok := validWhiteboardElement(testWhiteboardElement("element-"+test.elementType, test.elementType, test.extra))
			if !ok || id != "element-"+test.elementType {
				t.Fatalf("valid %s element rejected", test.elementType)
			}
		})
	}
}

func TestValidWhiteboardElementRejectsUnsafeShapeData(t *testing.T) {
	tests := []struct {
		name        string
		elementType string
		extra       string
	}{
		{name: "unsupported type", elementType: "image"},
		{name: "invalid point", elementType: "line", extra: `,"points":[[0,0,1]]`},
		{name: "missing points", elementType: "arrow"},
		{name: "incomplete arrow", elementType: "arrow", extra: `,"points":[[0,0],[10,10]]`},
		{name: "cross type field", elementType: "rectangle", extra: `,"points":[[0,0],[10,10]]`},
		{name: "invalid pressure", elementType: "freedraw", extra: `,"points":[[0,0]],"pressures":[2],"simulatePressure":false`},
		{name: "missing pressure mode", elementType: "freedraw", extra: `,"points":[[0,0]],"pressures":[]`},
		{name: "invalid text alignment", elementType: "text", extra: `,"text":"正文","originalText":"正文","fontSize":20,"fontFamily":1,"textAlign":"wide","verticalAlign":"top","lineHeight":1.25`},
		{name: "elbow binding without fixed point", elementType: "arrow", extra: `,"points":[[0,0],[10,10]],"lastCommittedPoint":null,"startBinding":{"elementId":"shape-1","focus":0,"gap":1},"endBinding":null,"startArrowhead":null,"endArrowhead":"arrow","elbowed":true,"fixedSegments":null,"startIsSpecial":null,"endIsSpecial":null`},
		{name: "plain binding with fixed point", elementType: "line", extra: `,"points":[[0,0],[10,10]],"lastCommittedPoint":null,"startBinding":{"elementId":"shape-1","focus":0,"gap":1,"fixedPoint":[0.5,0.5]},"endBinding":null,"startArrowhead":null,"endArrowhead":null`},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, ok := validWhiteboardElement(testWhiteboardElement("unsafe", test.elementType, test.extra)); ok {
				t.Fatalf("unsafe %s element accepted", test.elementType)
			}
		})
	}
}

func testWhiteboardElement(id, elementType, extra string) []byte {
	return []byte(fmt.Sprintf(
		`{"id":%q,"type":%q,"x":10,"y":20,"width":100,"height":50,"angle":0,"seed":1,"version":1,"versionNonce":2,"updated":3,"opacity":100,"strokeWidth":2,"roughness":1,"isDeleted":false,"locked":false,"index":"a0","strokeColor":"#1e1e1e","backgroundColor":"transparent","fillStyle":"solid","strokeStyle":"solid","roundness":null,"groupIds":[],"frameId":null,"boundElements":null,"link":null%s}`,
		id, elementType, extra,
	))
}

func testWhiteboardData(chapterID, appState string, elements [][]byte) []byte {
	encodedElements := make([]string, 0, len(elements))
	for _, element := range elements {
		encodedElements = append(encodedElements, string(element))
	}
	return []byte(fmt.Sprintf(
		`{"version":3,"anchor":{"type":"chapter","id":%q,"content_revision":"sha256:test"},"space":{"width":960,"height":640,"fit":"contain"},"document":{"type":"excalidraw","version":2,"elements":[%s],"appState":%s,"files":{}}}`,
		chapterID, strings.Join(encodedElements, ","), appState,
	))
}
