package memory

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	defaultPinnedMaxChars     = 4000
	defaultPinnedMaxItemChars = 1000
	defaultPinnedFileName     = "memory.md"
)

var (
	getwd   = os.Getwd
	stat    = os.Stat
	readDir = os.ReadDir
)

func AutoPinnedFile() (string, bool, error) {
	root, err := getwd()
	if err != nil {
		return "", false, fmt.Errorf("resolve workspace root: %w", err)
	}
	return autoPinnedFileFromRoot(root)
}

func autoPinnedFileFromRoot(root string) (string, bool, error) {
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
		if strings.EqualFold(entry.Name(), defaultPinnedFileName) {
			return filepath.Join(root, entry.Name()), true, nil
		}
	}

	return "", false, nil
}

func normalizePinnedOptions(opts PinnedOptions) PinnedOptions {
	if opts.MaxChars <= 0 {
		opts.MaxChars = defaultPinnedMaxChars
	}
	if opts.MaxItemChars <= 0 {
		opts.MaxItemChars = defaultPinnedMaxItemChars
	}
	return opts
}

func pinnedEnabled(opts PinnedOptions) bool {
	return opts.Enabled == nil || *opts.Enabled
}

func (p *MemoryProvider) loadFilePinned() ([]MemoryItem, error) {
	file, ok, err := AutoPinnedFile()
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

	return []MemoryItem{{
		ID:     pinnedFileMemoryID(file),
		Kind:   KindPinned,
		Status: StatusActive,
		Title:  strings.TrimSpace(filepath.Base(file)),
		Text:   text,
		Metadata: map[string]string{
			"source": "file",
			"path":   file,
		},
	}}, nil
}

func (p *MemoryProvider) loadStorePinned(ctx context.Context, query SearchQuery) ([]MemoryItem, error) {
	storeQuery := query
	storeQuery.Text = ""
	storeQuery.Kinds = []Kind{KindPinned}
	storeQuery.Statuses = []Status{StatusActive}

	result, err := p.store.SearchMemory(ctx, storeQuery)
	if err != nil {
		return nil, err
	}

	items := make([]MemoryItem, 0, len(result.Hits))
	for _, hit := range result.Hits {
		items = append(items, hit.Item.Clone())
	}
	return items, nil
}

func (p *MemoryProvider) preparePinnedItems(
	ctx context.Context,
	items []MemoryItem,
	query SearchQuery,
) ([]MemoryItem, error) {
	if len(items) == 0 {
		return nil, nil
	}

	itemLimit := p.pinned.MaxItemChars
	if query.MaxChars > 0 && query.MaxChars < itemLimit {
		itemLimit = query.MaxChars
	}

	prepared := make([]MemoryItem, 0, len(items))
	remaining := p.pinned.MaxChars
	for _, item := range items {
		if item.Status != StatusActive {
			continue
		}
		if err := safetyScanItem(ctx, p.guardrails, item); err != nil {
			return nil, err
		}

		redacted, err := redactItem(ctx, p.guardrails, item)
		if err != nil {
			return nil, err
		}
		redacted.Text = strings.TrimSpace(redacted.Text)
		redacted.Title = strings.TrimSpace(redacted.Title)
		redacted = truncatePinnedItem(redacted, itemLimit)
		if redacted.Title == "" && redacted.Text == "" {
			continue
		}

		itemChars := countPinnedItemChars(redacted)
		if remaining <= 0 {
			break
		}
		if itemChars > remaining {
			redacted = truncatePinnedItem(redacted, remaining)
			itemChars = countPinnedItemChars(redacted)
		}

		prepared = append(prepared, redacted.Clone())
		remaining -= itemChars
	}

	return prepared, nil
}

func safetyScanItem(ctx context.Context, guardrails Guardrails, item MemoryItem) error {
	if guardrails == nil {
		return nil
	}
	return guardrails.SafetyScan(ctx, item)
}

func pinnedFileMemoryID(file string) string {
	return "pinned_file:" + strings.TrimSpace(file)
}

func countPinnedItemChars(item MemoryItem) int {
	return len([]rune(item.Title)) + len([]rune(item.Text))
}

func truncatePinnedItem(item MemoryItem, maxChars int) MemoryItem {
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
