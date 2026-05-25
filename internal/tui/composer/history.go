package composer

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// MaxPromptHistory is the package-level max prompt history constant.
const MaxPromptHistory = 100

type historyFile struct {
	Entries []string `json:"entries"`
}

// LoadHistory loads history.
func LoadHistory(path string) ([]string, error) {
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

	var file historyFile
	if err := json.Unmarshal(data, &file); err != nil {
		return nil, err
	}

	return NormalizeHistory(file.Entries), nil
}

// SaveHistory persists prompt history entries to disk.
func SaveHistory(path string, history []string) error {
	if strings.TrimSpace(path) == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}

	data, _ := json.MarshalIndent(historyFile{
		Entries: NormalizeHistory(history),
	}, "", "  ")

	return os.WriteFile(path, append(data, '\n'), 0o600)
}

// NormalizeHistory normalizes history.
func NormalizeHistory(history []string) []string {
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
	if len(normalized) > MaxPromptHistory {
		normalized = normalized[len(normalized)-MaxPromptHistory:]
	}

	return normalized
}
