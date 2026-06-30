package changelog

import (
	_ "embed"
	"strings"

	"github.com/wandxy/morph/pkg/stringx"
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
		if strings.HasPrefix(stringx.String(line).Trim(), "## ") {
			start = index
			break
		}
	}
	if start < 0 {
		return stringx.String(value).Trim()
	}

	end := len(lines)
	for index := start + 1; index < len(lines); index++ {
		if strings.HasPrefix(stringx.String(lines[index]).Trim(), "## ") {
			end = index
			break
		}
	}

	return stringx.String(strings.Join(lines[start:end], "\n")).Trim()
}
