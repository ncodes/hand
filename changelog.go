package changelog

import (
	_ "embed"
	"strings"

	"github.com/wandxy/morph/pkg/str"
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
		stringValue2 := str.String(line)
		if strings.HasPrefix(stringValue2.Trim(), "## ") {
			start = index
			break
		}
	}
	if start < 0 {
		stringValue3 := str.String(value)
		return stringValue3.Trim()
	}

	end := len(lines)
	for index := start + 1; index < len(lines); index++ {
		stringValue4 := str.String(lines[index])
		if strings.HasPrefix(stringValue4.Trim(), "## ") {
			end = index
			break
		}
	}
	stringValue1 := str.String(strings.Join(lines[start:end], "\n"))
	return stringValue1.Trim()
}
