package pdf

import "testing"

// A PDF stores a bold run per text fragment, so a bold phrase that the extractor sees as several
// fragments used to emit "**结构化轨迹****第****3****轮**". Markdown renderers disagree about what a
// run of four asterisks between two words means, and the chapter showed the asterisks as
// literal text — "第3****轮".
func TestJoinPDFTextMergesAdjacentBoldRuns(t *testing.T) {
	for _, testCase := range []struct {
		name  string
		left  string
		right string
		want  string
	}{
		{name: "adjacent bold runs merge into one", left: "**结构化轨迹**", right: "**第**", want: "**结构化轨迹第**"},
		// The existing CJK/ASCII spacing rule applies inside the merged run too, exactly as it
		// already does for plain text: joinPDFText("第", "3") is "第 3".
		{name: "merging is repeatable across a whole line", left: "**结构化轨迹第**", right: "**3**", want: "**结构化轨迹第 3**"},
		{name: "bold followed by plain keeps both", left: "**结构化轨迹**", right: "轮", want: "**结构化轨迹**轮"},
		{name: "plain followed by bold keeps both", left: "轮", right: "**结构化轨迹**", want: "轮**结构化轨迹**"},
		{name: "latin words still take a separating space", left: "**Agent**", right: "**loop**", want: "**Agent loop**"},
		{name: "plain text is untouched", left: "上下文", right: "工程", want: "上下文工程"},
		{name: "an empty side is not turned into stray markers", left: "", right: "**第**", want: "**第**"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			if got := joinPDFText(testCase.left, testCase.right); got != testCase.want {
				t.Fatalf("joinPDFText(%q, %q) = %q, want %q", testCase.left, testCase.right, got, testCase.want)
			}
		})
	}
}
