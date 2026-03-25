package promptio

import "unicode/utf8"

func TruncateMiddle(content string, maxLength int, marker string) string {
	if len(content) <= maxLength {
		return content
	}
	if maxLength <= 0 {
		return ""
	}
	if len(marker) >= maxLength {
		if utf8.ValidString(marker[:maxLength]) {
			return marker[:maxLength]
		}
		truncated := marker[:maxLength]
		for len(truncated) > 0 && !utf8.ValidString(truncated) {
			truncated = truncated[:len(truncated)-1]
		}
		return truncated
	}

	available := maxLength - len(marker)
	headLength := available / 2
	tailLength := available - headLength

	head := content[:headLength]
	if !utf8.ValidString(head) {
		for len(head) > 0 && !utf8.ValidString(head) {
			head = head[:len(head)-1]
		}
	}

	tailStart := len(content) - tailLength
	tail := content[tailStart:]
	if !utf8.ValidString(tail) {
		for tailStart < len(content) && !utf8.ValidString(tail) {
			tailStart++
			tail = content[tailStart:]
		}
	}

	return head + marker + tail
}
