package main

import (
	"strings"
	"testing"
)

func TestTruncateOutputNoLimit(t *testing.T) {
	in := strings.Repeat("a", 50000)
	out := truncateOutput(in, 0)
	if out != in {
		t.Fatalf("expected no truncation when maxBytes=0")
	}
}

func TestTruncateOutputUnderLimit(t *testing.T) {
	in := "short"
	out := truncateOutput(in, 20000)
	if out != in {
		t.Fatalf("got %q", out)
	}
}

func TestTruncateOutputOverLimit(t *testing.T) {
	in := strings.Repeat("b", 30000)
	out := truncateOutput(in, 20000)
	if len(out) > 20000 {
		t.Fatalf("len=%d want <=20000", len(out))
	}
	if !strings.HasSuffix(out, truncationMark) {
		t.Fatalf("missing truncation mark: %q", out[len(out)-20:])
	}
}

func TestTruncateOutputUTF8Safe(t *testing.T) {
	// "ä" is 2 bytes in UTF-8; cut near a multi-byte boundary
	in := strings.Repeat("ä", 100)
	out := truncateOutput(in, 51)
	if len(out) > 51 {
		t.Fatalf("len=%d", len(out))
	}
	if len(in) > 51 && !strings.HasSuffix(out, truncationMark) {
		t.Fatalf("missing truncation mark: %q", out[len(out)-20:])
	}
	// must be valid UTF-8 (no panic / incomplete rune at end before mark)
	body := strings.TrimSuffix(out, truncationMark)
	for _, r := range body {
		if r == '\uFFFD' {
			t.Fatalf("replacement char in truncated body")
		}
	}
}
