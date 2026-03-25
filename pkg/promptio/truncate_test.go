package promptio

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestTruncateMiddleReturnsOriginalWhenWithinLimit(t *testing.T) {
	t.Parallel()

	content := "short content"

	got := TruncateMiddle(content, len(content), "[cut]")

	if got != content {
		t.Fatalf("expected original content, got %q", got)
	}
}

func TestTruncateMiddleReturnsEmptyWhenMaxLengthIsZero(t *testing.T) {
	t.Parallel()

	got := TruncateMiddle("content", 0, "[cut]")

	if got != "" {
		t.Fatalf("expected empty string, got %q", got)
	}
}

func TestTruncateMiddleTruncatesMarkerWhenMarkerExceedsLimit(t *testing.T) {
	t.Parallel()

	got := TruncateMiddle("content", 5, "abcdef")

	if got != "abcde" {
		t.Fatalf("expected truncated marker, got %q", got)
	}
}

func TestTruncateMiddleKeepsValidUTF8WhenMarkerExceedsLimit(t *testing.T) {
	t.Parallel()

	got := TruncateMiddle("content", 2, "🙂x")

	if !utf8.ValidString(got) {
		t.Fatalf("expected valid utf-8, got %q", got)
	}
	if got != "" {
		t.Fatalf("expected invalid partial rune to be trimmed away, got %q", got)
	}
}

func TestTruncateMiddlePreservesHeadAndTailAroundMarker(t *testing.T) {
	t.Parallel()

	content := strings.Repeat("a", 20) + strings.Repeat("b", 20)
	marker := "[cut]"

	got := TruncateMiddle(content, 25, marker)

	if len(got) != 25 {
		t.Fatalf("expected length 25, got %d", len(got))
	}
	if !strings.Contains(got, marker) {
		t.Fatalf("expected marker in %q", got)
	}
	if !strings.HasPrefix(got, strings.Repeat("a", 10)) {
		t.Fatalf("expected head content preserved, got %q", got)
	}
	if !strings.HasSuffix(got, strings.Repeat("b", 10)) {
		t.Fatalf("expected tail content preserved, got %q", got)
	}
}

func TestTruncateMiddleReturnsValidUTF8WhenCuttingContent(t *testing.T) {
	t.Parallel()

	content := strings.Repeat("🙂", 20)
	got := TruncateMiddle(content, 21, "[cut]")

	if !utf8.ValidString(got) {
		t.Fatalf("expected valid utf-8, got %q", got)
	}
	if !strings.Contains(got, "[cut]") {
		t.Fatalf("expected marker in %q", got)
	}
}
