package composer

import "strings"

// InputKind classifies composer input before submission.
type InputKind int

const (
	InputEmpty InputKind = iota
	InputPrompt
	InputCommand
	InputLocalCommand
)

// Input describes input for input.
type Input struct {
	Kind InputKind
	Text string
	Name string
	Args string
}

// ParseInput classifies raw composer input as a command or chat message.
func ParseInput(value string) Input {
	text := strings.TrimSpace(value)
	if text == "" {
		return Input{Kind: InputEmpty}
	}

	if command, ok := strings.CutPrefix(text, "/"); ok {
		name, args, _ := strings.Cut(strings.TrimSpace(command), " ")
		return Input{
			Kind: InputCommand,
			Text: text,
			Name: strings.ToLower(strings.TrimSpace(name)),
			Args: strings.TrimSpace(args),
		}
	}

	if command, ok := strings.CutPrefix(text, "!"); ok {
		return Input{
			Kind: InputLocalCommand,
			Text: text,
			Args: strings.TrimSpace(command),
		}
	}

	return Input{Kind: InputPrompt, Text: text}
}

// NormalizePaste normalizes paste.
func NormalizePaste(value string) string {
	return strings.TrimRight(value, "\r\n")
}
