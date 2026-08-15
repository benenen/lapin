package identifier

import (
	"strings"
	"testing"
)

func TestCodecRoundTrip(t *testing.T) {
	codec, err := New("")
	if err != nil {
		t.Fatal(err)
	}
	encoded := codec.Encode(42)
	if encoded == "42" || len(encoded) < 10 {
		t.Fatalf("raw ID was not hidden: %q", encoded)
	}
	decoded, err := codec.Decode(encoded)
	if err != nil || decoded != 42 {
		t.Fatalf("decoded = %d, err = %v", decoded, err)
	}
	for _, invalid := range []string{"", "not-a-hashid", codec.Encode(0)} {
		if _, err := codec.Decode(invalid); err == nil {
			t.Fatalf("invalid HashID was accepted: %q", invalid)
		}
	}
	if _, err := codec.Decode(strings.Repeat("a", 65)); err == nil {
		t.Fatal("excessively long HashID was accepted")
	}
}
