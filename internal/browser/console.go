package browser

import (
	"net/url"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	defaultConsoleLimit = 50
	maxConsoleMessages  = 200
	maxConsoleText      = 2_000
)

var (
	consoleANSIPattern   = regexp.MustCompile(`\x1b\[[0-?]*[ -/]*[@-~]`)
	consoleBearerPattern = regexp.MustCompile(`(?i)(bearer\s+)[^\s,;]+`)
	consoleURLPattern    = regexp.MustCompile(`(?i)\b(?:https?|wss?)://[^\s<>"']+`)
	consoleSecretPattern = regexp.MustCompile(
		`(?i)((?:"|')?(?:authorization|cookie|password|secret|token)(?:"|')?\s*[:=]\s*)(?:"[^"]*"|'[^']*'|[^\s,;}\]]+)`,
	)
)

func getSafeConsoleMessages(messages []ConsoleMessage, limit int) []ConsoleMessage {
	if limit < 1 || limit > maxConsoleMessages {
		limit = defaultConsoleLimit
	}
	if len(messages) > limit {
		messages = messages[len(messages)-limit:]
	}
	result := make([]ConsoleMessage, len(messages))
	for index, message := range messages {
		switch message.Level {
		case ConsoleDebug, ConsoleInfo, ConsoleWarn, ConsoleError:
		default:
			message.Level = ConsoleInfo
		}
		message.Text = sanitizeConsoleText(message.Text)
		message.Timestamp = message.Timestamp.UTC()
		result[index] = message
	}
	return result
}

func sanitizeConsoleText(value string) string {
	value = strings.ToValidUTF8(value, "")
	value = consoleANSIPattern.ReplaceAllString(value, "")
	value = strings.Map(func(character rune) rune {
		if character == '\n' || character == '\t' || !unicode.IsControl(character) {
			return character
		}
		return -1
	}, value)
	value = consoleURLPattern.ReplaceAllStringFunc(value, redactConsoleURL)
	value = consoleSecretPattern.ReplaceAllString(value, "$1[redacted]")
	value = consoleBearerPattern.ReplaceAllString(value, "$1[redacted]")
	value = strings.TrimSpace(value)
	if len(value) > maxConsoleText {
		cut := maxConsoleText
		for cut > 0 && !utf8.RuneStart(value[cut]) {
			cut--
		}
		value = value[:cut]
	}
	return value
}

func redactConsoleURL(raw string) string {
	candidate := strings.TrimRight(raw, ".,);]}")
	suffix := strings.TrimPrefix(raw, candidate)
	parsed, err := url.Parse(candidate)
	if err != nil || parsed.Hostname() == "" {
		return "[redacted-url]" + suffix
	}
	parsed.User = nil
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String() + suffix
}
