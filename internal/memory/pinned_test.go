package memory

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAutoPinnedFilesUsesWorkingDirectory(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "memory.md")
	require.NoError(t, os.WriteFile(file, []byte("remember this"), 0o600))

	previous := getwd
	t.Cleanup(func() {
		getwd = previous
	})
	getwd = func() (string, error) {
		return dir, nil
	}

	files, err := AutoPinnedFiles()
	require.NoError(t, err)
	require.Equal(t, []string{file}, files)
}

func TestAutoPinnedFilesReturnsWorkingDirectoryError(t *testing.T) {
	cwdErr := errors.New("cwd failed")
	previous := getwd
	t.Cleanup(func() {
		getwd = previous
	})
	getwd = func() (string, error) {
		return "", cwdErr
	}

	files, err := AutoPinnedFiles()
	require.ErrorIs(t, err, cwdErr)
	require.Empty(t, files)
}

func TestAutoPinnedFilesFromRoot(t *testing.T) {
	t.Run("empty root returns no files", func(t *testing.T) {
		require.Empty(t, mustAutoPinnedFilesFromRoot(t, ""))
	})

	t.Run("missing root returns no files", func(t *testing.T) {
		require.Empty(t, mustAutoPinnedFilesFromRoot(t, filepath.Join(t.TempDir(), "missing")))
	})

	t.Run("file root returns no files", func(t *testing.T) {
		file := filepath.Join(t.TempDir(), "not-root")
		require.NoError(t, os.WriteFile(file, []byte("not a dir"), 0o600))
		require.Empty(t, mustAutoPinnedFilesFromRoot(t, file))
	})

	t.Run("stat error is returned", func(t *testing.T) {
		statErr := errors.New("stat failed")
		previous := stat
		t.Cleanup(func() {
			stat = previous
		})
		stat = func(string) (os.FileInfo, error) {
			return nil, statErr
		}

		files, err := autoPinnedFilesFromRoot(t.TempDir())
		require.ErrorIs(t, err, statErr)
		require.Contains(t, err.Error(), "stat workspace root")
		require.Empty(t, files)
	})

	t.Run("read directory error is returned", func(t *testing.T) {
		readErr := errors.New("read failed")
		previous := readDir
		t.Cleanup(func() {
			readDir = previous
		})
		readDir = func(string) ([]os.DirEntry, error) {
			return nil, readErr
		}

		files, err := autoPinnedFilesFromRoot(t.TempDir())
		require.ErrorIs(t, err, readErr)
		require.Contains(t, err.Error(), "read workspace root")
		require.Empty(t, files)
	})

	t.Run("directory named memory file is ignored", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, os.Mkdir(filepath.Join(dir, "memory.md"), 0o700))
		require.Empty(t, mustAutoPinnedFilesFromRoot(t, dir))
	})

	t.Run("memory file match is case insensitive", func(t *testing.T) {
		dir := t.TempDir()
		file := filepath.Join(dir, "MEMORY.md")
		require.NoError(t, os.WriteFile(file, []byte("remember this"), 0o600))
		require.Equal(t, []string{file}, mustAutoPinnedFilesFromRoot(t, dir))
	})
}

func TestNormalizePinnedOptions(t *testing.T) {
	enabled := true

	opts := normalizePinnedOptions(PinnedOptions{
		Enabled:      &enabled,
		Files:        []string{" pinned.md ", "", "shared.md", "pinned.md"},
		MaxChars:     -1,
		MaxItemChars: 0,
	})

	require.Same(t, &enabled, opts.Enabled)
	require.Equal(t, []string{"pinned.md", "shared.md"}, opts.Files)
	require.Equal(t, defaultPinnedMaxChars, opts.MaxChars)
	require.Equal(t, defaultPinnedMaxItemChars, opts.MaxItemChars)
}

func TestCleanPinnedFilesReturnsNilForEmptyInput(t *testing.T) {
	require.Nil(t, cleanPinnedFiles(nil))
	require.Empty(t, cleanPinnedFiles([]string{" ", "\t"}))
}

func TestPinnedEnabled(t *testing.T) {
	enabled := true
	disabled := false

	require.True(t, pinnedEnabled(PinnedOptions{}))
	require.True(t, pinnedEnabled(PinnedOptions{Enabled: &enabled}))
	require.False(t, pinnedEnabled(PinnedOptions{Enabled: &disabled}))
}

func TestMemoryProvider_LoadPinned(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "pinned.md")
	require.NoError(t, os.WriteFile(file, []byte("from file"), 0o600))

	provider := defaultMemoryTestProvider(t, Options{
		Pinned: PinnedOptions{Files: []string{file}},
	})

	_, err := provider.Upsert(context.Background(), MemoryItem{Kind: KindPinned, Status: StatusActive, Text: "from db"})
	require.NoError(t, err)
	_, err = provider.Upsert(context.Background(), MemoryItem{Kind: KindSemantic, Status: StatusActive, Text: "semantic remember"})
	require.NoError(t, err)

	items, err := provider.LoadPinned(context.Background(), SearchQuery{Text: "remember"})
	require.NoError(t, err)
	require.Len(t, items, 2)
	require.Equal(t, KindPinned, items[0].Kind)
	require.Equal(t, "pinned.md", items[0].Title)
	require.Equal(t, "from file", items[0].Text)
	require.Equal(t, map[string]string{"source": "file", "path": file}, items[0].Metadata)
	require.Equal(t, KindPinned, items[1].Kind)
	require.Equal(t, "from db", items[1].Text)
}

func TestMemoryProvider_LoadPinnedDisabled(t *testing.T) {
	enabled := false
	provider := defaultMemoryTestProvider(t, Options{
		Guardrails: &fakeGuardrails{searchErr: errors.New("search blocked"), safetyErr: errors.New("unsafe")},
		Pinned: PinnedOptions{
			Enabled: &enabled,
			Files:   []string{filepath.Join(t.TempDir(), "missing.md")},
		},
	})

	items, err := provider.LoadPinned(context.Background(), SearchQuery{})
	require.NoError(t, err)
	require.Empty(t, items)
}

func TestMemoryProvider_LoadPinnedSkipsEmptyFiles(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "empty.md")
	require.NoError(t, os.WriteFile(file, []byte(" \n\t "), 0o600))

	provider := defaultMemoryTestProvider(t, Options{
		Pinned: PinnedOptions{Files: []string{file}},
	})

	items, err := provider.LoadPinned(context.Background(), SearchQuery{})
	require.NoError(t, err)
	require.Empty(t, items)
}

func TestMemoryProvider_LoadPinnedAppliesItemAndTotalCharLimits(t *testing.T) {
	dir := t.TempDir()
	firstFile := filepath.Join(dir, "a")
	secondFile := filepath.Join(dir, "b")
	require.NoError(t, os.WriteFile(firstFile, []byte("abcdef"), 0o600))
	require.NoError(t, os.WriteFile(secondFile, []byte("xyz"), 0o600))

	provider := defaultMemoryTestProvider(t, Options{
		Pinned: PinnedOptions{
			Files:        []string{firstFile, secondFile},
			MaxChars:     5,
			MaxItemChars: 4,
		},
	})

	items, err := provider.LoadPinned(context.Background(), SearchQuery{})
	require.NoError(t, err)
	require.Len(t, items, 2)
	require.Equal(t, "a", items[0].Title)
	require.Equal(t, "abc", items[0].Text)
	require.Equal(t, "b", items[1].Title)
	require.Empty(t, items[1].Text)
}

func TestMemoryProvider_LoadPinnedAppliesQueryLimitAfterMerge(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "pinned.md")
	require.NoError(t, os.WriteFile(file, []byte("from file"), 0o600))

	provider := defaultMemoryTestProvider(t, Options{
		Pinned: PinnedOptions{Files: []string{file}},
	})
	_, err := provider.Upsert(context.Background(), MemoryItem{Kind: KindPinned, Status: StatusActive, Text: "from db"})
	require.NoError(t, err)

	items, err := provider.LoadPinned(context.Background(), SearchQuery{Limit: 1})
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Equal(t, "from file", items[0].Text)
}

func TestMemoryProvider_LoadPinnedUsesQueryCharLimitWhenSmaller(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "pinned.md")
	require.NoError(t, os.WriteFile(file, []byte("abcdef"), 0o600))

	provider := defaultMemoryTestProvider(t, Options{
		Pinned: PinnedOptions{
			Files:        []string{file},
			MaxChars:     100,
			MaxItemChars: 100,
		},
	})

	items, err := provider.LoadPinned(context.Background(), SearchQuery{MaxChars: 3})
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Equal(t, "pin", items[0].Title)
	require.Empty(t, items[0].Text)
}

func TestMemoryProvider_LoadPinnedSafetyScansAndRedacts(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "pinned.md")
	require.NoError(t, os.WriteFile(file, []byte("secret text"), 0o600))
	guardrails := &fakeGuardrails{redactText: "redacted"}

	provider := defaultMemoryTestProvider(t, Options{
		Guardrails: guardrails,
		Pinned:     PinnedOptions{Files: []string{file}},
	})

	items, err := provider.LoadPinned(context.Background(), SearchQuery{})
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Equal(t, "redacted", items[0].Text)
	require.Equal(t, 1, guardrails.validateSearchCalls)
	require.Equal(t, 1, guardrails.safetyScanCalls)
	require.Equal(t, 1, guardrails.redactCalls)
}

func TestMemoryProvider_LoadPinnedReturnsSafetyScanError(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "pinned.md")
	require.NoError(t, os.WriteFile(file, []byte("unsafe"), 0o600))
	safetyErr := errors.New("unsafe pinned memory")

	provider := defaultMemoryTestProvider(t, Options{
		Guardrails: &fakeGuardrails{safetyErr: safetyErr},
		Pinned:     PinnedOptions{Files: []string{file}},
	})

	items, err := provider.LoadPinned(context.Background(), SearchQuery{})
	require.ErrorIs(t, err, safetyErr)
	require.Empty(t, items)
}

func TestMemoryProvider_LoadPinnedReturnsFileReadError(t *testing.T) {
	provider := defaultMemoryTestProvider(t, Options{
		Pinned: PinnedOptions{Files: []string{filepath.Join(t.TempDir(), "missing.md")}},
	})

	items, err := provider.LoadPinned(context.Background(), SearchQuery{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to read pinned memory file")
	require.Empty(t, items)
}

func TestMemoryProvider_PreparePinnedItemsSkipsInactiveAndEmptyItems(t *testing.T) {
	provider := defaultMemoryTestProvider(t, Options{})

	items, err := provider.preparePinnedItems(context.Background(), []MemoryItem{
		{Kind: KindPinned, Status: StatusCandidate, Title: "candidate", Text: "skip"},
		{Kind: KindPinned, Status: StatusActive, Title: " ", Text: " "},
		{Kind: KindPinned, Status: StatusActive, Title: "keep", Text: "ok"},
	}, SearchQuery{})
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Equal(t, "keep", items[0].Title)
}

func TestMemoryProvider_PreparePinnedItemsStopsWhenTotalBudgetIsExhausted(t *testing.T) {
	provider := defaultMemoryTestProvider(t, Options{
		Pinned: PinnedOptions{MaxChars: 2, MaxItemChars: 10},
	})

	items, err := provider.preparePinnedItems(context.Background(), []MemoryItem{
		{Kind: KindPinned, Status: StatusActive, Title: "a", Text: "b"},
		{Kind: KindPinned, Status: StatusActive, Title: "c", Text: "d"},
	}, SearchQuery{})
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Equal(t, "a", items[0].Title)
	require.Equal(t, "b", items[0].Text)
}

func TestMemoryProvider_PreparePinnedItemsPropagatesRedactError(t *testing.T) {
	redactErr := errors.New("redact failed")
	provider := defaultMemoryTestProvider(t, Options{
		Guardrails: &fakeGuardrails{redactErr: redactErr},
	})

	items, err := provider.preparePinnedItems(context.Background(), []MemoryItem{{
		Kind:   KindPinned,
		Status: StatusActive,
		Text:   "secret",
	}}, SearchQuery{})
	require.ErrorIs(t, err, redactErr)
	require.Empty(t, items)
}

func TestPinnedFileMemoryID(t *testing.T) {
	require.Equal(t, "pinned_file:0007:/tmp/pinned.md", pinnedFileMemoryID(7, " /tmp/pinned.md "))
}

func TestPinnedItemChars(t *testing.T) {
	require.Equal(t, 4, countPinnedItemChars(MemoryItem{Title: "go", Text: "✓x"}))
}

func TestTruncatePinnedItem(t *testing.T) {
	item := truncatePinnedItem(MemoryItem{Title: "title", Text: "body"}, 0)
	require.Empty(t, item.Title)
	require.Empty(t, item.Text)

	item = truncatePinnedItem(MemoryItem{Title: "title", Text: "body"}, 3)
	require.Equal(t, "tit", item.Title)
	require.Empty(t, item.Text)

	item = truncatePinnedItem(MemoryItem{Title: "go", Text: "abcdef"}, 5)
	require.Equal(t, "go", item.Title)
	require.Equal(t, "abc", item.Text)
}

func TestTruncateRunes(t *testing.T) {
	require.Empty(t, truncateRunes("hello", 0))
	require.Equal(t, "hé", truncateRunes("héllo", 2))
	require.Equal(t, "short", truncateRunes("short", 10))
}

func mustAutoPinnedFilesFromRoot(t *testing.T, root string) []string {
	t.Helper()

	files, err := autoPinnedFilesFromRoot(root)
	require.NoError(t, err)
	return files
}
