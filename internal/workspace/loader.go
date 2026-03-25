package workspace

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/wandxy/hand/internal/guardrails"
)

const maxContentLength = 15000

var (
	supportedRuleFiles = map[string]struct{}{
		"agents.md": {},
		"hand.md":   {},
		"claude.md": {},
	}
	skippedDirectories = map[string]struct{}{
		"node_modules": {},
		"__pycache__":  {},
		"venv":         {},
		".venv":        {},
		".git":         {},
		".gocache":     {},
		"plan":         {},
	}
	getwd = os.Getwd
)

type Result struct {
	Content string
	Found   bool
}

func Load() (Result, error) {
	root, err := getwd()
	if err != nil {
		return Result{}, fmt.Errorf("resolve workspace root: %w", err)
	}

	return LoadFromRoot(root)
}

func LoadFromRoot(root string) (Result, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return Result{}, nil
	}

	info, err := os.Stat(root)
	if err != nil {
		if os.IsNotExist(err) {
			return Result{}, nil
		}
		return Result{}, fmt.Errorf("stat workspace root %q: %w", root, err)
	}

	if !info.IsDir() {
		return Result{}, nil
	}

	enabled, err := hasTopLevelRules(root)
	if err != nil {
		return Result{}, err
	}
	if !enabled {
		return Result{}, nil
	}

	paths, err := collectRuleFiles(root)
	if err != nil {
		return Result{}, err
	}
	if len(paths) == 0 {
		return Result{}, nil
	}

	sections := make([]string, 0, len(paths))
	for _, path := range paths {
		content, err := os.ReadFile(path)
		if err != nil {
			return Result{}, fmt.Errorf("read workspace rule %q: %w", path, err)
		}

		relativePath, err := filepath.Rel(root, path)
		if err != nil {
			return Result{}, fmt.Errorf("resolve workspace rule path %q: %w", path, err)
		}

		scanned := guardrails.SafetyScan(string(content), filepath.ToSlash(relativePath))
		sections = append(sections, fmt.Sprintf("## %s\n%s", filepath.ToSlash(relativePath), scanned.Content))
	}

	return Result{
		Content: truncate(strings.Join(sections, "\n\n")),
		Found:   true,
	}, nil
}

func hasTopLevelRules(root string) (bool, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return false, fmt.Errorf("read workspace root %q: %w", root, err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if _, ok := supportedRuleFiles[strings.ToLower(entry.Name())]; ok {
			return true, nil
		}
	}

	return false, nil
}

func collectRuleFiles(root string) ([]string, error) {
	paths := make([]string, 0)

	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if entry.IsDir() {
			if path == root {
				return nil
			}

			name := entry.Name()
			if strings.HasPrefix(name, ".") {
				return filepath.SkipDir
			}
			if _, skip := skippedDirectories[name]; skip {
				return filepath.SkipDir
			}
			return nil
		}

		if _, ok := supportedRuleFiles[strings.ToLower(entry.Name())]; ok {
			paths = append(paths, path)
		}
		return nil
	})
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("walk workspace root %q: %w", root, err)
	}

	slices.SortFunc(paths, func(left, right string) int {
		leftDepth := depth(root, left)
		rightDepth := depth(root, right)
		if leftDepth != rightDepth {
			return leftDepth - rightDepth
		}
		return strings.Compare(filepath.ToSlash(left), filepath.ToSlash(right))
	})

	return paths, nil
}

func depth(root, path string) int {
	relative, err := filepath.Rel(root, path)
	if err != nil {
		return 0
	}
	return strings.Count(filepath.ToSlash(relative), "/")
}

func truncate(content string) string {
	if len(content) <= maxContentLength {
		return content
	}

	marker := "\n\n[... workspace rules truncated ...]\n\n"
	available := maxContentLength - len(marker)
	if available <= 0 {
		return marker
	}

	headLength := available / 2
	tailLength := available - headLength

	head := content[:headLength]
	if !utf8.ValidString(head) {
		for len(head) > 0 && !utf8.ValidString(head) {
			head = head[:len(head)-1]
		}
	}

	tailStart := len(content) - tailLength
	tail := content[tailStart:]
	if !utf8.ValidString(tail) {
		for tailStart < len(content) && !utf8.ValidString(tail) {
			tailStart++
			tail = content[tailStart:]
		}
	}

	return head + marker + tail
}
