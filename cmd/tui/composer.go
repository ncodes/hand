package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
)

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

// submitPrompt routes a non-empty composer value to prompt or command handling.
func (m *model) submitPrompt() tea.Cmd {
	input := parseComposerInput(m.input.Value())
	if input.Kind == composerInputEmpty {
		return nil
	}
	if input.Kind == composerInputPrompt && m.responding {
		return m.setStatus("response already in progress")
	}

	cmd := m.addPromptHistory(input.Text)
	promptSubmitted := false
	switch input.Kind {
	case composerInputPrompt:
		m.messages = append(m.messages, "You: "+input.Text)
		cmd = tea.Batch(cmd, m.startResponse(input.Text))
		promptSubmitted = true
	case composerInputCommand:
		cmd = tea.Batch(cmd, m.handleSlashCommand(input))
	case composerInputLocalCommand:
		cmd = tea.Batch(cmd, m.handleLocalCommand(input))
	}
	if promptSubmitted {
		m.setTranscriptContent()
	} else if m.responding {
		m.setTranscriptContentForActiveTurn()
	} else {
		m.setTranscriptContent()
	}
	m.clearComposer()
	m.resize()

	return cmd
}

func (m *model) clearComposer() {
	m.input.SetValue("")
	m.historyAt = len(m.history)
	m.draft = ""
}
