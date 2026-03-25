package personality

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/wandxy/hand/internal/datadir"
	"github.com/wandxy/hand/internal/guardrails"
	"github.com/wandxy/hand/pkg/promptio"
)

const fileName = "SOUL.md"
const maxContentLength = 15000

var (
	getwd              = os.Getwd
	readFile           = os.ReadFile
	resolveDisplayPath = displayPath
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

	sections := make([]string, 0, 2)
	seenPaths := make(map[string]struct{}, 2)

	globalPath := filepath.Join(datadir.ProjectHomeDir(), fileName)
	globalSection, foundGlobal, err := loadFile(globalPath, "", seenPaths)
	if err != nil {
		return Result{}, err
	}
	if foundGlobal {
		sections = append(sections, globalSection)
	}

	if root != "" {
		info, err := os.Stat(root)
		if err != nil {
			if !os.IsNotExist(err) {
				return Result{}, fmt.Errorf("stat workspace root %q: %w", root, err)
			}
		} else if info.IsDir() {
			workspacePath := filepath.Join(root, fileName)
			workspaceSection, foundWorkspace, err := loadFile(workspacePath, root, seenPaths)
			if err != nil {
				return Result{}, err
			}
			if foundWorkspace {
				sections = append(sections, workspaceSection)
			}
		}
	}

	if len(sections) == 0 {
		return Result{}, nil
	}

	return Result{
		Content: truncate(strings.Join(sections, "\n\n")),
		Found:   true,
	}, nil
}

func loadFile(path, workspaceRoot string, seenPaths map[string]struct{}) (string, bool, error) {
	if absolutePath, err := filepath.Abs(path); err == nil {
		path = absolutePath
	}
	if _, ok := seenPaths[path]; ok {
		return "", false, nil
	}

	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", false, nil
		}
		return "", false, fmt.Errorf("stat personality file %q: %w", path, err)
	}
	if info.IsDir() {
		return "", false, nil
	}
	seenPaths[path] = struct{}{}

	content, err := readFile(path)
	if err != nil {
		return "", false, fmt.Errorf("read personality file %q: %w", path, err)
	}
	contentText := strings.TrimSpace(string(content))
	if contentText == "" {
		return "", false, nil
	}

	displayPath, err := resolveDisplayPath(path, workspaceRoot)
	if err != nil {
		return "", false, fmt.Errorf("resolve personality file path %q: %w", path, err)
	}

	scanned := guardrails.SafetyScan(contentText, displayPath)
	return fmt.Sprintf("## %s\n%s", displayPath, scanned.Content), true, nil
}

func displayPath(path, workspaceRoot string) (string, error) {
	if workspaceRoot != "" {
		relativePath, err := filepath.Rel(workspaceRoot, path)
		if err == nil {
			relativePath = filepath.ToSlash(relativePath)
			if relativePath != "." && !strings.HasPrefix(relativePath, "../") {
				return relativePath, nil
			}
		}
	}

	if cleanedHome := strings.TrimSpace(datadir.ProjectHomeDir()); cleanedHome != "" {
		relativePath, err := filepath.Rel(cleanedHome, path)
		if err == nil {
			relativePath = filepath.ToSlash(relativePath)
			if relativePath != "." && !strings.HasPrefix(relativePath, "../") {
				return relativePath, nil
			}
		}
	}

	return filepath.ToSlash(path), nil
}

func truncate(content string) string {
	return promptio.TruncateMiddle(content, maxContentLength, "\n\n[... personality overlay truncated ...]\n\n")
}
