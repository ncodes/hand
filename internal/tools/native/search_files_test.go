package native

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wandxy/hand/internal/guardrails"
	"github.com/wandxy/hand/internal/tools"
)

func TestSearchFiles_ToolUsesGoFallback(t *testing.T) {
	originalLookPath := lookPath
	t.Cleanup(func() {
		lookPath = originalLookPath
	})
	lookPath = func(string) (string, error) {
		return "", errors.New("missing")
	}

	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "main.go"), []byte("package main\nfunc main() { println(\"hello\") }\n"), 0o644))
	registry := registerTestRuntime(t, root, guardrails.CommandPolicy{})

	result, err := registry.Invoke(context.Background(), tools.Call{Name: "search_files", Input: `{"pattern":"println","path":"."}`})
	require.NoError(t, err)

	var payload struct {
		Path    string `json:"path"`
		Pattern string `json:"pattern"`
		Matches []struct {
			Path   string `json:"path"`
			Line   int    `json:"line"`
			Column int    `json:"column"`
			Text   string `json:"text"`
		} `json:"matches"`
	}
	require.NoError(t, json.Unmarshal([]byte(result.Output), &payload))
	require.Equal(t, ".", payload.Path)
	require.Equal(t, "println", payload.Pattern)
	require.Len(t, payload.Matches, 1)
	require.Equal(t, "main.go", payload.Matches[0].Path)
	require.Equal(t, 2, payload.Matches[0].Line)
	require.Equal(t, 15, payload.Matches[0].Column)
	require.Equal(t, `func main() { println("hello") }`, payload.Matches[0].Text)
}

func TestSearchFiles_ToolRejectsInvalidJSONInput(t *testing.T) {
	root := t.TempDir()
	registry := registerTestRuntime(t, root, guardrails.CommandPolicy{})

	result, err := registry.Invoke(context.Background(), tools.Call{Name: "search_files", Input: `{"pattern":`})
	require.NoError(t, err)

	var toolErr tools.Error
	require.NoError(t, json.Unmarshal([]byte(result.Error), &toolErr))
	require.Equal(t, "invalid_input", toolErr.Code)
	require.Equal(t, "invalid tool input", toolErr.Message)
}

func TestSearchFiles_ToolRequiresPattern(t *testing.T) {
	root := t.TempDir()
	registry := registerTestRuntime(t, root, guardrails.CommandPolicy{})

	result, err := registry.Invoke(context.Background(), tools.Call{Name: "search_files", Input: `{"pattern":"   ","path":"."}`})
	require.NoError(t, err)

	var toolErr tools.Error
	require.NoError(t, json.Unmarshal([]byte(result.Error), &toolErr))
	require.Equal(t, "invalid_input", toolErr.Code)
	require.Equal(t, "pattern is required", toolErr.Message)
}

func TestSearchFiles_ToolRejectsOutsideRoot(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "file.txt")
	require.NoError(t, os.WriteFile(outside, []byte("needle\n"), 0o644))
	registry := registerTestRuntime(t, root, guardrails.CommandPolicy{})

	result, err := registry.Invoke(context.Background(), tools.Call{Name: "search_files", Input: `{"pattern":"needle","path":` + quoteJSON(outside) + `}`})

	require.NoError(t, err)
	var toolErr tools.Error
	require.NoError(t, json.Unmarshal([]byte(result.Error), &toolErr))
	require.Equal(t, "path_outside_roots", toolErr.Code)
}

func TestSearchFiles_ToolReturnsWalkerErrors(t *testing.T) {
	originalLookPath := lookPath
	originalWalkDir := walkDir
	t.Cleanup(func() {
		lookPath = originalLookPath
		walkDir = originalWalkDir
	})
	lookPath = func(string) (string, error) {
		return "", errors.New("missing")
	}
	walkDir = func(string, fs.WalkDirFunc) error {
		return os.ErrPermission
	}

	root := t.TempDir()
	registry := registerTestRuntime(t, root, guardrails.CommandPolicy{})

	result, err := registry.Invoke(context.Background(), tools.Call{Name: "search_files", Input: `{"pattern":"needle","path":"."}`})

	require.NoError(t, err)
	var toolErr tools.Error
	require.NoError(t, json.Unmarshal([]byte(result.Error), &toolErr))
	require.Equal(t, "access_denied", toolErr.Code)
}

func TestSearchFiles_ToolUsesGoFallbackForLongLines(t *testing.T) {
	originalLookPath := lookPath
	t.Cleanup(func() {
		lookPath = originalLookPath
	})
	lookPath = func(string) (string, error) {
		return "", errors.New("missing")
	}

	root := t.TempDir()
	longLine := strings.Repeat("a", 70*1024) + " needle"
	require.NoError(t, os.WriteFile(filepath.Join(root, "main.txt"), []byte(longLine+"\n"), 0o644))
	registry := registerTestRuntime(t, root, guardrails.CommandPolicy{})

	result, err := registry.Invoke(context.Background(), tools.Call{Name: "search_files", Input: `{"pattern":"needle","path":"."}`})

	require.NoError(t, err)
	var payload struct {
		Matches []struct {
			Path   string `json:"path"`
			Line   int    `json:"line"`
			Column int    `json:"column"`
		} `json:"matches"`
	}
	require.NoError(t, json.Unmarshal([]byte(result.Output), &payload))
	require.Len(t, payload.Matches, 1)
	require.Equal(t, "main.txt", payload.Matches[0].Path)
	require.Equal(t, 1, payload.Matches[0].Line)
	require.Equal(t, 70*1024+2, payload.Matches[0].Column)
}

func TestSearchFiles_ToolAppliesMaxResultsWithGoFallback(t *testing.T) {
	originalLookPath := lookPath
	t.Cleanup(func() {
		lookPath = originalLookPath
	})
	lookPath = func(string) (string, error) {
		return "", errors.New("missing")
	}

	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "main.txt"), []byte("Needle\nneedle\n"), 0o644))
	registry := registerTestRuntime(t, root, guardrails.CommandPolicy{})

	result, err := registry.Invoke(context.Background(), tools.Call{Name: "search_files", Input: `{"pattern":"needle","path":".","max_results":1}`})

	require.NoError(t, err)
	var payload struct {
		Matches []struct {
			Path   string `json:"path"`
			Line   int    `json:"line"`
			Column int    `json:"column"`
			Text   string `json:"text"`
		} `json:"matches"`
	}
	require.NoError(t, json.Unmarshal([]byte(result.Output), &payload))
	require.Len(t, payload.Matches, 1)
	require.Equal(t, 1, payload.Matches[0].Line)
	require.Equal(t, "Needle", payload.Matches[0].Text)
}

func TestSearchFiles_ToolUsesRipgrepWhenAvailable(t *testing.T) {
	originalLookPath := lookPath
	originalCommandContext := commandContext
	t.Cleanup(func() {
		lookPath = originalLookPath
		commandContext = originalCommandContext
	})
	lookPath = func(string) (string, error) {
		return "/usr/bin/rg", nil
	}
	commandContext = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		output := filepath.Join(args[len(args)-1], "main.go") + ":2:15:func main() { println(\"hello\") }\n"
		return exec.CommandContext(ctx, "sh", "-lc", "printf %s "+quoteJSON(output))
	}

	root := t.TempDir()
	registry := registerTestRuntime(t, root, guardrails.CommandPolicy{})

	result, err := registry.Invoke(context.Background(), tools.Call{Name: "search_files", Input: `{"pattern":"println","path":"."}`})

	require.NoError(t, err)
	var payload struct {
		Matches []struct {
			Path   string `json:"path"`
			Line   int    `json:"line"`
			Column int    `json:"column"`
			Text   string `json:"text"`
		} `json:"matches"`
	}
	require.NoError(t, json.Unmarshal([]byte(result.Output), &payload))
	require.Len(t, payload.Matches, 1)
	require.Equal(t, "main.go", payload.Matches[0].Path)
	require.Equal(t, 2, payload.Matches[0].Line)
	require.Equal(t, 15, payload.Matches[0].Column)
	require.Equal(t, "func main() { println(\"hello\") }\\n", payload.Matches[0].Text)
}

func TestSearchWithRipgrep_ReturnsNoMatchesForExitCodeOne(t *testing.T) {
	originalCommandContext := commandContext
	t.Cleanup(func() {
		commandContext = originalCommandContext
	})
	commandContext = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		return exec.CommandContext(ctx, "sh", "-lc", "exit 1")
	}

	root := t.TempDir()
	resolved := guardrails.ResolvedPath{Root: root, Absolute: root}
	matches, err := searchWithRipgrep(context.Background(), resolved, "needle", false, false, 10)

	require.NoError(t, err)
	require.Nil(t, matches)
}

func TestSearchWithRipgrep_ReturnsCommandErrors(t *testing.T) {
	originalCommandContext := commandContext
	t.Cleanup(func() {
		commandContext = originalCommandContext
	})
	commandContext = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		return exec.CommandContext(ctx, "sh", "-lc", "exit 2")
	}

	root := t.TempDir()
	resolved := guardrails.ResolvedPath{Root: root, Absolute: root}
	matches, err := searchWithRipgrep(context.Background(), resolved, "needle", true, true, 10)

	require.Nil(t, matches)
	require.Error(t, err)
}

func TestSearchWithRipgrep_SkipsMalformedLinesAndHonorsLimit(t *testing.T) {
	originalCommandContext := commandContext
	t.Cleanup(func() {
		commandContext = originalCommandContext
	})
	commandContext = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		output := "malformed\n\n" +
			filepath.Join(args[len(args)-1], "a.txt") + ":2:4:alpha\n" +
			filepath.Join(args[len(args)-1], "b.txt") + ":3:5:beta\n"
		return exec.CommandContext(ctx, "printf", output)
	}

	root := t.TempDir()
	resolved := guardrails.ResolvedPath{Root: root, Absolute: root}
	matches, err := searchWithRipgrep(context.Background(), resolved, "needle", false, false, 1)

	require.NoError(t, err)
	require.Len(t, matches, 1)
	require.Equal(t, "a.txt", matches[0].Path)
	require.Equal(t, 2, matches[0].Line)
	require.Equal(t, 4, matches[0].Column)
	require.Equal(t, "alpha", matches[0].Text)
}

func TestSearchWithGo_SkipsHiddenAndUnreadableFiles(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(root, ".hidden"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, ".hidden", "secret.txt"), []byte("needle\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, ".secret.txt"), []byte("needle\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "binary.bin"), []byte{0, 1, 2}, 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "visible.txt"), []byte("needle\n"), 0o644))
	resolved := guardrails.ResolvedPath{Root: root, Absolute: root}

	matches, err := searchWithGo(resolved, "needle", true, false, 10)

	require.NoError(t, err)
	require.Len(t, matches, 1)
	require.Equal(t, "visible.txt", matches[0].Path)
}

func TestSearchWithGo_PropagatesWalkerCallbackErrors(t *testing.T) {
	originalWalkDir := walkDir
	t.Cleanup(func() {
		walkDir = originalWalkDir
	})
	walkDir = func(root string, fn fs.WalkDirFunc) error {
		return fn(filepath.Join(root, "broken.txt"), nil, os.ErrPermission)
	}

	root := t.TempDir()
	resolved := guardrails.ResolvedPath{Root: root, Absolute: root}
	matches, err := searchWithGo(resolved, "needle", true, false, 10)

	require.Nil(t, matches)
	require.ErrorIs(t, err, os.ErrPermission)
}
