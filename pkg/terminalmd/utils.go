package terminalmd

import (
	"strings"

	goldast "github.com/yuin/goldmark/ast"
	extast "github.com/yuin/goldmark/extension/ast"
)

func isFenceLine(line string) bool {
	trimmed := strings.TrimSpace(line)
	return strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~")
}

func getTaskPrefix(paragraph *goldast.Paragraph) (string, bool) {
	if paragraph == nil {
		return "", false
	}
	checkbox, ok := paragraph.FirstChild().(*extast.TaskCheckBox)
	if !ok {
		return "", false
	}
	if checkbox.IsChecked {
		return "[x] ", true
	}
	return "[ ] ", true
}

// min returns the smaller of two ints.
func min(a int, b int) int {
	if a < b {
		return a
	}
	return b
}

// max returns the larger of two ints.
func max(a int, b int) int {
	if a > b {
		return a
	}
	return b
}
