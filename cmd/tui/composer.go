package tui

import "strings"

type composerInputKind int

const (
	composerInputEmpty composerInputKind = iota
	composerInputPrompt
	composerInputCommand
	composerInputLocalCommand
)

type composerInput struct {
	Kind composerInputKind
	Text string
	Name string
	Args string
}

func parseComposerInput(value string) composerInput {
	text := strings.TrimSpace(value)
	if text == "" {
		return composerInput{Kind: composerInputEmpty}
	}

	if command, ok := strings.CutPrefix(text, "/"); ok {
		name, args, _ := strings.Cut(strings.TrimSpace(command), " ")
		return composerInput{
			Kind: composerInputCommand,
			Text: text,
			Name: strings.ToLower(strings.TrimSpace(name)),
			Args: strings.TrimSpace(args),
		}
	}

	if command, ok := strings.CutPrefix(text, "!"); ok {
		return composerInput{
			Kind: composerInputLocalCommand,
			Text: text,
			Args: strings.TrimSpace(command),
		}
	}

	return composerInput{Kind: composerInputPrompt, Text: text}
}

func normalizeComposerPaste(value string) string {
	return strings.TrimRight(value, "\r\n")
}
