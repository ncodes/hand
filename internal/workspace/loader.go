package workspace

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/wandxy/hand/internal/guardrails"
	"github.com/wandxy/hand/pkg/promptio"
)

const maxContentLength = 15000

var (
	defaultInstructionFiles = []string{"agents.md", "hand.md"}
	skippedDirectories      = map[string]struct{}{
		"node_modules": {},
		"__pycache__":  {},
		"venv":         {},
		".venv":        {},
		".git":         {},
		".gocache":     {},
		".plan":        {},
	}
	getwd = os.Getwd
)

type Result struct {
	Content string
	Found   bool
}

func DefaultInstructionFiles() []string {
	files := make([]string, len(defaultInstructionFiles))
	copy(files, defaultInstructionFiles)
	return files
}

func NormalizeRulePaths(files []string) []string {
	normalized := make([]string, 0, len(files))
	seen := make(map[string]struct{}, len(files))

	for _, file := range files {
		path := strings.TrimSpace(file)
		if path == "" {
			continue
		}
		path = filepath.Clean(path)
		if _, ok := seen[path]; ok {
			continue
		}
		seen[path] = struct{}{}
		normalized = append(normalized, path)
	}

	return normalized
}

func Load(files ...string) (Result, error) {
	root, err := getwd()
	if err != nil {
		return Result{}, fmt.Errorf("resolve workspace root: %w", err)
	}

	return LoadFromRoot(root, files...)
}

func LoadFromRoot(root string, files ...string) (Result, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return Result{}, nil
	}
	files = NormalizeRulePaths(files)

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

	defaultPaths, err := collectDefaultRuleFiles(root)
	if err != nil {
		return Result{}, err
	}

	configuredPaths, err := collectConfiguredRuleFiles(root, files)
	if err != nil {
		return Result{}, err
	}

	paths := mergePaths(defaultPaths, configuredPaths)
	if len(paths) == 0 {
		return Result{}, nil
	}

	sections := make([]string, 0, len(paths))
	for _, path := range paths {
		content, err := os.ReadFile(path)
		if err != nil {
			return Result{}, fmt.Errorf("read workspace rule %q: %w", path, err)
		}

		displayPath, err := displayPath(root, path)
		if err != nil {
			return Result{}, fmt.Errorf("resolve workspace rule path %q: %w", path, err)
		}

		scanned := guardrails.SafetyScan(string(content), displayPath)
		sections = append(sections, fmt.Sprintf("## %s\n%s", displayPath, scanned.Content))
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

	supportedRuleFiles := toFileSet(DefaultInstructionFiles())
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

func collectDefaultRuleFiles(root string) ([]string, error) {
	enabled, err := hasTopLevelRules(root)
	if err != nil {
		return nil, err
	}
	if !enabled {
		return nil, nil
	}

	return collectRuleFiles(root, DefaultInstructionFiles())
}

func collectConfiguredRuleFiles(root string, files []string) ([]string, error) {
	if len(files) == 0 {
		return nil, nil
	}

	paths := make([]string, 0, len(files))
	seen := make(map[string]struct{}, len(files))
	for _, configuredPath := range files {
		resolvedPath := configuredPath
		if !filepath.IsAbs(resolvedPath) {
			resolvedPath = filepath.Join(root, resolvedPath)
		}
		resolvedPath = filepath.Clean(resolvedPath)

		info, err := os.Stat(resolvedPath)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, fmt.Errorf("stat configured rule file %q: %w", configuredPath, err)
		}
		if info.IsDir() {
			continue
		}

		absolutePath, err := filepath.Abs(resolvedPath)
		if err == nil {
			resolvedPath = absolutePath
		}
		if _, ok := seen[resolvedPath]; ok {
			continue
		}
		seen[resolvedPath] = struct{}{}
		paths = append(paths, resolvedPath)
	}

	return paths, nil
}

func collectRuleFiles(root string, files []string) ([]string, error) {
	paths := make([]string, 0)
	supportedRuleFiles := toFileSet(files)

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

func mergePaths(groups ...[]string) []string {
	merged := make([]string, 0)
	seen := make(map[string]struct{})

	for _, group := range groups {
		for _, path := range group {
			key := path
			if absolutePath, err := filepath.Abs(path); err == nil {
				key = absolutePath
			}
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			merged = append(merged, path)
		}
	}

	return merged
}

func displayPath(root, path string) (string, error) {
	relativePath, err := filepath.Rel(root, path)
	if err != nil {
		return "", err
	}
	relativePath = filepath.ToSlash(relativePath)
	if relativePath == "." {
		return filepath.ToSlash(path), nil
	}
	if strings.HasPrefix(relativePath, "../") {
		return filepath.ToSlash(path), nil
	}
	return relativePath, nil
}

func depth(root, path string) int {
	relative, err := filepath.Rel(root, path)
	if err != nil {
		return 0
	}
	return strings.Count(filepath.ToSlash(relative), "/")
}

func truncate(content string) string {
	return promptio.TruncateMiddle(content, maxContentLength, "\n\n[... workspace rules truncated ...]\n\n")
}

func toFileSet(files []string) map[string]struct{} {
	supported := make(map[string]struct{}, len(files))
	for _, file := range files {
		supported[file] = struct{}{}
	}
	return supported
}
