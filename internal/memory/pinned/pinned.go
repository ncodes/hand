package pinned

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/wandxy/hand/internal/constants"
	state "github.com/wandxy/hand/internal/state/core"
)

const (
	defaultMaxChars     = constants.DefaultMemoryPinnedMaxChars
	defaultMaxItemChars = constants.DefaultMemoryPinnedItemChars
	defaultFileName     = constants.MemoryPinnedFileName
)

var (
	getwd   = os.Getwd
	stat    = os.Stat
	readDir = os.ReadDir
)

type Options struct {
	Enabled      *bool
	MaxChars     int
	MaxItemChars int
}

type SafetyScanner func(context.Context, state.MemoryItem) error

type Redactor func(context.Context, state.MemoryItem) (state.MemoryItem, error)

func AutoFile() (string, bool, error) {
	root, err := getwd()
	if err != nil {
		return "", false, fmt.Errorf("resolve workspace root: %w", err)
	}
	return autoFileFromRoot(root)
}

func NormalizeOptions(opts Options) Options {
	if opts.MaxChars <= 0 {
		opts.MaxChars = defaultMaxChars
	}
	if opts.MaxItemChars <= 0 {
		opts.MaxItemChars = defaultMaxItemChars
	}
	return opts
}

func Enabled(opts Options) bool {
	return opts.Enabled == nil || *opts.Enabled
}

func LoadFile() ([]state.MemoryItem, error) {
	file, ok, err := AutoFile()
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, nil
	}

	data, err := os.ReadFile(file)
	if err != nil {
		return nil, fmt.Errorf("failed to read pinned memory file %q: %w", file, err)
	}

	text := strings.TrimSpace(string(data))
	if text == "" {
		return nil, nil
	}

	return []state.MemoryItem{{
		ID:     fileMemoryID(file),
		Kind:   state.MemoryKindPinned,
		Status: state.MemoryStatusActive,
		Title:  strings.TrimSpace(filepath.Base(file)),
		Text:   text,
		Metadata: map[string]string{
			"source": "file",
			"path":   file,
		},
	}}, nil
}

func PrepareItems(
	ctx context.Context,
	items []state.MemoryItem,
	query state.MemorySearchQuery,
	opts Options,
	scan SafetyScanner,
	redact Redactor,
) ([]state.MemoryItem, error) {
	if len(items) == 0 {
		return nil, nil
	}

	itemLimit := opts.MaxItemChars
	if query.MaxChars > 0 && query.MaxChars < itemLimit {
		itemLimit = query.MaxChars
	}

	prepared := make([]state.MemoryItem, 0, len(items))
	remaining := opts.MaxChars
	for _, item := range items {
		if item.Status != state.MemoryStatusActive {
			continue
		}
		if scan != nil {
			if err := scan(ctx, item); err != nil {
				return nil, err
			}
		}

		redacted := item
		if redact != nil {
			var err error
			redacted, err = redact(ctx, item)
			if err != nil {
				return nil, err
			}
		}
		redacted.Text = strings.TrimSpace(redacted.Text)
		redacted.Title = strings.TrimSpace(redacted.Title)
		redacted = truncateItem(redacted, itemLimit)
		if redacted.Title == "" && redacted.Text == "" {
			continue
		}

		itemChars := countItemChars(redacted)
		if remaining <= 0 {
			break
		}
		if itemChars > remaining {
			redacted = truncateItem(redacted, remaining)
			itemChars = countItemChars(redacted)
		}

		prepared = append(prepared, redacted.Clone())
		remaining -= itemChars
	}

	return prepared, nil
}

func autoFileFromRoot(root string) (string, bool, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return "", false, nil
	}

	info, err := stat(root)
	if err != nil {
		if os.IsNotExist(err) {
			return "", false, nil
		}
		return "", false, fmt.Errorf("stat workspace root %q: %w", root, err)
	}
	if !info.IsDir() {
		return "", false, nil
	}

	entries, err := readDir(root)
	if err != nil {
		return "", false, fmt.Errorf("read workspace root %q: %w", root, err)
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if strings.EqualFold(entry.Name(), defaultFileName) {
			return filepath.Join(root, entry.Name()), true, nil
		}
	}

	return "", false, nil
}

func fileMemoryID(file string) string {
	return "pinned_file:" + strings.TrimSpace(file)
}

func countItemChars(item state.MemoryItem) int {
	return len([]rune(item.Title)) + len([]rune(item.Text))
}

func truncateItem(item state.MemoryItem, maxChars int) state.MemoryItem {
	if maxChars <= 0 {
		item.Title = ""
		item.Text = ""
		return item
	}

	titleRunes := []rune(item.Title)
	if len(titleRunes) >= maxChars {
		item.Title = string(titleRunes[:maxChars])
		item.Text = ""
		return item
	}

	item.Text = truncateRunes(item.Text, maxChars-len(titleRunes))
	return item
}

func truncateRunes(value string, maxChars int) string {
	if maxChars <= 0 {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= maxChars {
		return value
	}
	return string(runes[:maxChars])
}
