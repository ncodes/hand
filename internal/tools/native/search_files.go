package native

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	envtypes "github.com/wandxy/hand/internal/environment/types"
	"github.com/wandxy/hand/internal/guardrails"
	"github.com/wandxy/hand/internal/tools"
)

type contentMatch struct {
	Path   string `json:"path"`
	Line   int    `json:"line"`
	Column int    `json:"column"`
	Text   string `json:"text"`
}

func SearchFilesDefinition(dependencies envtypes.Runtime) tools.Definition {
	type input struct {
		Pattern       string `json:"pattern"`
		Path          string `json:"path"`
		CaseSensitive bool   `json:"case_sensitive"`
		IncludeHidden bool   `json:"include_hidden"`
		MaxResults    int    `json:"max_results"`
	}

	type match struct {
		Path   string `json:"path"`
		Line   int    `json:"line"`
		Column int    `json:"column"`
		Text   string `json:"text"`
	}

	return tools.Definition{
		Name:        "search_files",
		Description: "Search file contents under an allowed workspace root.",
		Groups:      []string{"core"},
		Requires:    tools.Capabilities{Filesystem: true},
		InputSchema: objectSchema(map[string]any{
			"pattern":        stringSchema("Text or pattern to search for within files."),
			"path":           stringSchema("Path relative to an allowed workspace root to search within. Defaults to the workspace root when omitted."),
			"case_sensitive": booleanSchema("When true, match text using case-sensitive search."),
			"include_hidden": booleanSchema("When true, include hidden files and directories in the search."),
			"max_results":    integerSchema("Maximum number of matches to return. Values outside the supported range are clamped."),
		}, "pattern"),
		Handler: tools.HandlerFunc(func(ctx context.Context, call tools.Call) (tools.Result, error) {
			var req input
			if result := decodeInput(call, &req); result.Error != "" {
				return result, nil
			}

			if strings.TrimSpace(req.Pattern) == "" {
				return toolError("invalid_input", "pattern is required"), nil
			}

			resolved, err := dependencies.FilePolicy().Resolve(req.Path)
			if err != nil {
				return fileError(err), nil
			}

			limit := req.MaxResults
			if limit <= 0 || limit > maxSearchResults {
				limit = maxSearchResults
			}

			matches, err := searchWithFallback(
				ctx,
				resolved,
				req.Pattern,
				req.CaseSensitive,
				req.IncludeHidden,
				limit,
			)
			if err != nil {
				return fileError(err), nil
			}

			out := make([]match, 0, len(matches))
			for _, item := range matches {
				out = append(out, match(item))
			}

			return encodeOutput(map[string]any{
				"root":    resolved.Root,
				"path":    normalizedDisplayPath(resolved.Relative),
				"pattern": req.Pattern,
				"matches": out,
			})
		}),
	}
}

func searchWithFallback(
	ctx context.Context,
	resolved guardrails.ResolvedPath,
	pattern string,
	caseSensitive bool,
	includeHidden bool,
	limit int,
) ([]contentMatch, error) {
	if _, err := lookPath("rg"); err == nil {
		if matches, searchErr := searchWithRipgrep(
			ctx,
			resolved,
			pattern,
			caseSensitive,
			includeHidden,
			limit,
		); searchErr == nil {
			return matches, nil
		}
	}

	return searchWithGo(resolved, pattern, caseSensitive, includeHidden, limit)
}

func searchWithRipgrep(
	ctx context.Context,
	resolved guardrails.ResolvedPath,
	pattern string,
	caseSensitive bool,
	includeHidden bool,
	limit int,
) ([]contentMatch, error) {
	args := []string{"--vimgrep", "--no-heading", "--color", "never", "--max-count", strconv.Itoa(limit)}
	if includeHidden {
		args = append(args, "--hidden")
	}
	if !caseSensitive {
		args = append(args, "-i")
	}
	args = append(args, pattern, resolved.Absolute)

	cmd := commandContext(ctx, "rg", args...)
	output, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
			return nil, nil
		}
		return nil, err
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	matches := make([]contentMatch, 0, len(lines))

	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}

		parts := strings.SplitN(line, ":", 4)
		if len(parts) != 4 {
			continue
		}

		rel, _ := filepath.Rel(resolved.Root, parts[0])
		lineNo, _ := strconv.Atoi(parts[1])
		column, _ := strconv.Atoi(parts[2])

		matches = append(matches, contentMatch{
			Path:   filepath.ToSlash(rel),
			Line:   lineNo,
			Column: column,
			Text:   parts[3],
		})

		if len(matches) >= limit {
			break
		}
	}

	return matches, nil
}

func searchWithGo(
	resolved guardrails.ResolvedPath,
	pattern string,
	caseSensitive bool,
	includeHidden bool,
	limit int,
) ([]contentMatch, error) {
	matches := make([]contentMatch, 0, limit)
	matchPattern := pattern
	if !caseSensitive {
		matchPattern = strings.ToLower(pattern)
	}

	err := walkDir(resolved.Absolute, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() {
			if path != resolved.Absolute {
				rel, _ := filepath.Rel(resolved.Root, path)
				if !includeHidden && hiddenPath(rel) {
					return filepath.SkipDir
				}
			}
			return nil
		}

		rel, _ := filepath.Rel(resolved.Root, path)

		if !includeHidden && hiddenPath(rel) {
			return nil
		}

		content, readErr := guardrails.ReadTextFile(path, maxReadBytes)
		if readErr != nil {
			return nil
		}

		scanner := bufio.NewScanner(bytes.NewReader(content))
		buffer := make([]byte, 0, 64*1024)
		scanner.Buffer(buffer, maxReadBytes)
		lineNo := 0

		for scanner.Scan() {
			lineNo++
			text := scanner.Text()
			haystack := text
			if !caseSensitive {
				haystack = strings.ToLower(text)
			}

			index := strings.Index(haystack, matchPattern)
			if index < 0 {
				continue
			}

			matches = append(matches, contentMatch{
				Path:   filepath.ToSlash(rel),
				Line:   lineNo,
				Column: index + 1,
				Text:   text,
			})

			if len(matches) >= limit {
				return errors.New("result limit reached")
			}
		}

		return nil
	})

	if err != nil && err.Error() != "result limit reached" {
		return nil, err
	}

	return matches, nil
}
