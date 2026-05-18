package tui

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/wandxy/hand/internal/datadir"
)

const maxPromptHistory = 100

var promptHistoryPath = func() string {
	return filepath.Join(datadir.DataDir(), "tui-history.json")
}

type promptHistoryFile struct {
	Entries []string `json:"entries"`
}

func loadPromptHistory() ([]string, error) {
	path := promptHistoryPath()
	if strings.TrimSpace(path) == "" {
		return nil, nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}

		return nil, err
	}

	var file promptHistoryFile
	if err := json.Unmarshal(data, &file); err != nil {
		return nil, err
	}

	return normalizePromptHistory(file.Entries), nil
}

func savePromptHistory(history []string) error {
	path := promptHistoryPath()
	if strings.TrimSpace(path) == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}

	data, _ := json.MarshalIndent(promptHistoryFile{
		Entries: normalizePromptHistory(history),
	}, "", "  ")

	return os.WriteFile(path, append(data, '\n'), 0o600)
}

func normalizePromptHistory(history []string) []string {
	normalized := make([]string, 0, len(history))
	for _, entry := range history {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		if len(normalized) > 0 && normalized[len(normalized)-1] == entry {
			continue
		}

		normalized = append(normalized, entry)
	}
	if len(normalized) > maxPromptHistory {
		normalized = normalized[len(normalized)-maxPromptHistory:]
	}

	return normalized
}

func (m *model) addPromptHistory(value string) tea.Cmd {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	if len(m.history) > 0 && m.history[len(m.history)-1] == value {
		m.historyAt = len(m.history)
		return nil
	}

	m.history = normalizePromptHistory(append(m.history, value))
	m.historyAt = len(m.history)
	if err := savePromptHistory(m.history); err != nil {
		return m.setStatus("prompt history unavailable")
	}

	return nil
}

func (m *model) showPreviousPrompt() {
	if len(m.history) == 0 {
		return
	}
	if m.historyAt == len(m.history) {
		m.draft = m.input.Value()
	}
	if m.historyAt > 0 {
		m.historyAt--
	}

	m.input.SetValue(m.history[m.historyAt])
	m.input.CursorEnd()
	m.resize()
}

func (m *model) showNextPrompt() {
	if len(m.history) == 0 || m.historyAt >= len(m.history) {
		return
	}

	m.historyAt++
	if m.historyAt == len(m.history) {
		m.input.SetValue(m.draft)
		m.draft = ""
	} else {
		m.input.SetValue(m.history[m.historyAt])
	}
	m.input.CursorEnd()
	m.resize()
}

func (m model) shouldUseHistoryKey(msg tea.KeyPressMsg) bool {
	switch msg.Key().Code {
	case tea.KeyUp, tea.KeyDown:
		return !strings.Contains(m.input.Value(), "\n")
	default:
		return false
	}
}
