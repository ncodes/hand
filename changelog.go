package changelog

import (
	_ "embed"
	"strings"
)

//go:embed CHANGELOG.md
var content string

func Latest() string {
	return latestSection(content)
}

func latestSection(value string) string {
	value = strings.ReplaceAll(value, "\r\n", "\n")
	lines := strings.Split(value, "\n")

	start := -1
	for index, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "## ") {
			start = index
			break
		}
	}
	if start < 0 {
		return strings.TrimSpace(value)
	}

	end := len(lines)
	for index := start + 1; index < len(lines); index++ {
		if strings.HasPrefix(strings.TrimSpace(lines[index]), "## ") {
			end = index
			break
		}
	}

	return strings.TrimSpace(strings.Join(lines[start:end], "\n"))
}
