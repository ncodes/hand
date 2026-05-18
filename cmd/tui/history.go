package tui

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"

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
