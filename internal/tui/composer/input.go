package composer

import (
	"strings"

	"github.com/wandxy/morph/pkg/str"
)

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
	stringValue1 := str.String(value)
	text := stringValue1.Trim()
	if text == "" {
		return Input{Kind: InputEmpty}
	}

	if command, ok := strings.CutPrefix(text, "/"); ok {
		stringValue2 := str.String(command)
		name, args, _ := strings.Cut(stringValue2.Trim(), " ")
		stringValue3 := str.String(name)
		stringValue4 := str.String(args)
		return Input{
			Kind: InputCommand,
			Text: text,
			Name: stringValue3.Normalized(),
			Args: stringValue4.Trim(),
		}
	}

	if command, ok := strings.CutPrefix(text, "!"); ok {
		stringValue5 := str.String(command)
		return Input{
			Kind: InputLocalCommand,
			Text: text,
			Args: stringValue5.Trim(),
		}
	}

	return Input{Kind: InputPrompt, Text: text}
}

// NormalizePaste normalizes paste.
func NormalizePaste(value string) string {
	return strings.TrimRight(value, "\r\n")
}
