package memory

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

type fakeDirEntry struct {
	name string
}

func (entry fakeDirEntry) Name() string {
	return entry.name
}

func (fakeDirEntry) IsDir() bool {
	return false
}

func (fakeDirEntry) Type() fs.FileMode {
	return 0
}

func (fakeDirEntry) Info() (fs.FileInfo, error) {
	return nil, nil
}

func TestAutoPinnedFileUsesWorkingDirectory(t *testing.T) {
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

	got, ok, err := AutoPinnedFile()
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, file, got)
}

func TestAutoPinnedFileReturnsWorkingDirectoryError(t *testing.T) {
	cwdErr := errors.New("cwd failed")
	previous := getwd
	t.Cleanup(func() {
		getwd = previous
	})
	getwd = func() (string, error) {
		return "", cwdErr
	}

	got, ok, err := AutoPinnedFile()
	require.ErrorIs(t, err, cwdErr)
	require.False(t, ok)
	require.Empty(t, got)
}

func TestAutoPinnedFileFromRoot(t *testing.T) {
	t.Run("empty root returns no files", func(t *testing.T) {
		got, ok := mustAutoPinnedFileFromRoot(t, "")
		require.False(t, ok)
		require.Empty(t, got)
	})

	t.Run("missing root returns no files", func(t *testing.T) {
		got, ok := mustAutoPinnedFileFromRoot(t, filepath.Join(t.TempDir(), "missing"))
		require.False(t, ok)
		require.Empty(t, got)
	})

	t.Run("file root returns no files", func(t *testing.T) {
		file := filepath.Join(t.TempDir(), "not-root")
		require.NoError(t, os.WriteFile(file, []byte("not a dir"), 0o600))
		got, ok := mustAutoPinnedFileFromRoot(t, file)
		require.False(t, ok)
		require.Empty(t, got)
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

		got, ok, err := autoPinnedFileFromRoot(t.TempDir())
		require.ErrorIs(t, err, statErr)
		require.Contains(t, err.Error(), "stat workspace root")
		require.False(t, ok)
		require.Empty(t, got)
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

		got, ok, err := autoPinnedFileFromRoot(t.TempDir())
		require.ErrorIs(t, err, readErr)
		require.Contains(t, err.Error(), "read workspace root")
		require.False(t, ok)
		require.Empty(t, got)
	})

	t.Run("directory named memory file is ignored", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, os.Mkdir(filepath.Join(dir, "memory.md"), 0o700))
		got, ok := mustAutoPinnedFileFromRoot(t, dir)
		require.False(t, ok)
		require.Empty(t, got)
	})

	t.Run("memory file match is case insensitive", func(t *testing.T) {
		dir := t.TempDir()
		file := filepath.Join(dir, "MEMORY.md")
		require.NoError(t, os.WriteFile(file, []byte("remember this"), 0o600))
		got, ok := mustAutoPinnedFileFromRoot(t, dir)
		require.True(t, ok)
		require.Equal(t, file, got)
	})
}

func TestNormalizePinnedOptions(t *testing.T) {
	enabled := true

	opts := normalizePinnedOptions(PinnedOptions{
		Enabled:      &enabled,
		MaxChars:     -1,
		MaxItemChars: 0,
	})

	require.Same(t, &enabled, opts.Enabled)
	require.Equal(t, defaultPinnedMaxChars, opts.MaxChars)
	require.Equal(t, defaultPinnedMaxItemChars, opts.MaxItemChars)
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
	t.Chdir(dir)
	file := filepath.Join(dir, "memory.md")
	require.NoError(t, os.WriteFile(file, []byte("from file"), 0o600))

	provider := defaultMemoryTestProvider(t, Options{})

	_, err := provider.Upsert(context.Background(), MemoryItem{Kind: KindPinned, Status: StatusActive, Text: "from db"})
	require.NoError(t, err)
	_, err = provider.Upsert(context.Background(), MemoryItem{Kind: KindSemantic, Status: StatusActive, Text: "semantic remember"})
	require.NoError(t, err)

	items, err := provider.LoadPinned(context.Background(), SearchQuery{Text: "remember"})
	require.NoError(t, err)
	require.Len(t, items, 2)
	require.Equal(t, KindPinned, items[0].Kind)
	require.Equal(t, "memory.md", items[0].Title)
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
		},
	})

	items, err := provider.LoadPinned(context.Background(), SearchQuery{})
	require.NoError(t, err)
	require.Empty(t, items)
}

func TestMemoryProvider_LoadPinnedSkipsEmptyFiles(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	file := filepath.Join(dir, "memory.md")
	require.NoError(t, os.WriteFile(file, []byte(" \n\t "), 0o600))

	provider := defaultMemoryTestProvider(t, Options{})

	items, err := provider.LoadPinned(context.Background(), SearchQuery{})
	require.NoError(t, err)
	require.Empty(t, items)
}

func TestMemoryProvider_LoadPinnedAppliesItemAndTotalCharLimits(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	file := filepath.Join(dir, "memory.md")
	require.NoError(t, os.WriteFile(file, []byte("abcdef"), 0o600))

	provider := defaultMemoryTestProvider(t, Options{
		Pinned: PinnedOptions{
			MaxChars:     5,
			MaxItemChars: 4,
		},
	})

	items, err := provider.LoadPinned(context.Background(), SearchQuery{})
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Equal(t, "memo", items[0].Title)
	require.Empty(t, items[0].Text)
}

func TestMemoryProvider_LoadPinnedAppliesQueryLimitAfterMerge(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	file := filepath.Join(dir, "memory.md")
	require.NoError(t, os.WriteFile(file, []byte("from file"), 0o600))

	provider := defaultMemoryTestProvider(t, Options{})
	_, err := provider.Upsert(context.Background(), MemoryItem{Kind: KindPinned, Status: StatusActive, Text: "from db"})
	require.NoError(t, err)

	items, err := provider.LoadPinned(context.Background(), SearchQuery{Limit: 1})
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Equal(t, "from file", items[0].Text)
}

func TestMemoryProvider_LoadPinnedUsesQueryCharLimitWhenSmaller(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	file := filepath.Join(dir, "memory.md")
	require.NoError(t, os.WriteFile(file, []byte("abcdef"), 0o600))

	provider := defaultMemoryTestProvider(t, Options{
		Pinned: PinnedOptions{
			MaxChars:     100,
			MaxItemChars: 100,
		},
	})

	items, err := provider.LoadPinned(context.Background(), SearchQuery{MaxChars: 3})
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Equal(t, "mem", items[0].Title)
	require.Empty(t, items[0].Text)
}

func TestMemoryProvider_LoadPinnedSafetyScansAndRedacts(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	file := filepath.Join(dir, "memory.md")
	require.NoError(t, os.WriteFile(file, []byte("secret text"), 0o600))
	guardrails := &fakeGuardrails{redactText: "redacted"}

	provider := defaultMemoryTestProvider(t, Options{
		Guardrails: guardrails,
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
	t.Chdir(dir)
	file := filepath.Join(dir, "memory.md")
	require.NoError(t, os.WriteFile(file, []byte("unsafe"), 0o600))
	safetyErr := errors.New("unsafe pinned memory")

	provider := defaultMemoryTestProvider(t, Options{
		Guardrails: &fakeGuardrails{safetyErr: safetyErr},
	})

	items, err := provider.LoadPinned(context.Background(), SearchQuery{})
	require.ErrorIs(t, err, safetyErr)
	require.Empty(t, items)
}

func TestMemoryProvider_LoadPinnedReturnsAutoPinnedFileDiscoveryError(t *testing.T) {
	cwdErr := errors.New("cwd failed")
	previous := getwd
	t.Cleanup(func() {
		getwd = previous
	})
	getwd = func() (string, error) {
		return "", cwdErr
	}

	provider := defaultMemoryTestProvider(t, Options{})
	items, err := provider.LoadPinned(context.Background(), SearchQuery{})
	require.ErrorIs(t, err, cwdErr)
	require.Empty(t, items)
}

func TestMemoryProvider_LoadPinnedReturnsAutoPinnedFileReadError(t *testing.T) {
	dir := t.TempDir()

	previousGetwd := getwd
	previousReadDir := readDir
	t.Cleanup(func() {
		getwd = previousGetwd
		readDir = previousReadDir
	})
	getwd = func() (string, error) {
		return dir, nil
	}
	readDir = func(string) ([]os.DirEntry, error) {
		return []os.DirEntry{fakeDirEntry{name: "memory.md"}}, nil
	}

	provider := defaultMemoryTestProvider(t, Options{})
	items, err := provider.LoadPinned(context.Background(), SearchQuery{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to read pinned memory file")
	require.Empty(t, items)
}

func TestMemoryProvider_PreparePinnedItemsReturnsNilWithoutItems(t *testing.T) {
	provider := defaultMemoryTestProvider(t, Options{})

	items, err := provider.preparePinnedItems(context.Background(), nil, SearchQuery{})
	require.NoError(t, err)
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

func TestMemoryProvider_PreparePinnedItemsTruncatesToRemainingBudget(t *testing.T) {
	provider := defaultMemoryTestProvider(t, Options{
		Pinned: PinnedOptions{MaxChars: 3, MaxItemChars: 10},
	})

	items, err := provider.preparePinnedItems(context.Background(), []MemoryItem{{
		Kind:   KindPinned,
		Status: StatusActive,
		Title:  "a",
		Text:   "bcdef",
	}}, SearchQuery{})
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Equal(t, "a", items[0].Title)
	require.Equal(t, "bc", items[0].Text)
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
	require.Equal(t, "pinned_file:/tmp/memory.md", pinnedFileMemoryID(" /tmp/memory.md "))
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

func mustAutoPinnedFileFromRoot(t *testing.T, root string) (string, bool) {
	t.Helper()

	file, ok, err := autoPinnedFileFromRoot(root)
	require.NoError(t, err)
	return file, ok
}
