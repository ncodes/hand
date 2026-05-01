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
)

func normalizePinnedOptions(opts PinnedOptions) PinnedOptions {
	opts.Files = cleanPinnedFiles(opts.Files)
	if opts.MaxChars <= 0 {
		opts.MaxChars = defaultPinnedMaxChars
	}
	if opts.MaxItemChars <= 0 {
		opts.MaxItemChars = defaultPinnedMaxItemChars
	}
	return opts
}

func cleanPinnedFiles(files []string) []string {
	if len(files) == 0 {
		return nil
	}

	seen := make(map[string]struct{}, len(files))
	cleaned := make([]string, 0, len(files))
	for _, file := range files {
		file = strings.TrimSpace(file)
		if file == "" {
			continue
		}
		if _, ok := seen[file]; ok {
			continue
		}
		seen[file] = struct{}{}
		cleaned = append(cleaned, file)
	}
	return cleaned
}

func pinnedEnabled(opts PinnedOptions) bool {
	return opts.Enabled == nil || *opts.Enabled
}

func (p *MemoryProvider) loadFilePinned() ([]MemoryItem, error) {
	files := p.pinned.Files
	if len(files) == 0 {
		return nil, nil
	}

	items := make([]MemoryItem, 0, len(files))
	for idx, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			return nil, fmt.Errorf("failed to read pinned memory file %q: %w", file, err)
		}

		text := strings.TrimSpace(string(data))
		if text == "" {
			continue
		}

		item := MemoryItem{
			ID:     pinnedFileMemoryID(idx, file),
			Kind:   KindPinned,
			Status: StatusActive,
			Title:  strings.TrimSpace(filepath.Base(file)),
			Text:   text,
			Metadata: map[string]string{
				"source": "file",
				"path":   file,
			},
		}
		items = append(items, item)
	}
	return items, nil
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

func pinnedFileMemoryID(idx int, file string) string {
	return fmt.Sprintf("pinned_file:%04d:%s", idx, strings.TrimSpace(file))
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
