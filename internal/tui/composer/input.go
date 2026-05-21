package composer

import "strings"

type InputKind int

const (
	InputEmpty InputKind = iota
	InputPrompt
	InputCommand
	InputLocalCommand
)

type Input struct {
	Kind InputKind
	Text string
	Name string
	Args string
}

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

func NormalizePaste(value string) string {
	return strings.TrimRight(value, "\r\n")
}
