package personality

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/wandxy/hand/internal/config"
	"github.com/wandxy/hand/internal/constants"
	"github.com/wandxy/hand/internal/datadir"
	"github.com/wandxy/hand/internal/guardrails"
	"github.com/wandxy/hand/pkg/promptio"
)

const fileName = constants.PersonalityFileName
const maxContentLength = constants.PersonalityMaxContentLength

var (
	getwd              = os.Getwd
	readFile           = os.ReadFile
	resolveDisplayPath = getDisplayPath
)

type Result struct {
	Content string
	Found   bool
}

type LoadOptions struct {
	ProfileHome       string
	WorkspaceRoot     string
	PersonalityName   string
	PersonalityConfig config.PersonalityConfig
	AllowWorkspace    bool
}

func Load(opts LoadOptions) (Result, error) {
	opts.ProfileHome = strings.TrimSpace(opts.ProfileHome)
	if opts.ProfileHome == "" {
		opts.ProfileHome = datadir.ProjectHomeDir()
	}

	if opts.AllowWorkspace && strings.TrimSpace(opts.WorkspaceRoot) == "" {
		root, err := getwd()
		if err != nil {
			return Result{}, fmt.Errorf("resolve workspace root: %w", err)
		}
		opts.WorkspaceRoot = root
	}

	return loadWithOptions(opts)
}

func loadWithOptions(opts LoadOptions) (Result, error) {
	sections := make([]string, 0, 3)
	seenPaths := make(map[string]struct{}, 2)

	profileHome := strings.TrimSpace(opts.ProfileHome)
	workspaceRoot := strings.TrimSpace(opts.WorkspaceRoot)
	personalityName := strings.TrimSpace(opts.PersonalityName)

	if personalityName == "" {
		globalPath := filepath.Join(profileHome, fileName)
		globalSection, foundGlobal, err := loadFile(
			globalPath,
			workspaceRoot,
			seenPaths,
			loadFileOptions{Label: "Profile SOUL.md"},
		)
		if err != nil {
			return Result{}, err
		}
		if foundGlobal {
			sections = append(sections, globalSection)
		}
	} else {
		section, found, err := loadNamedPersonality(opts, seenPaths)
		if err != nil {
			return Result{}, err
		}
		if found {
			sections = append(sections, section)
		}
	}

	if opts.AllowWorkspace && workspaceRoot != "" {
		info, err := os.Stat(workspaceRoot)
		if err != nil {
			if !os.IsNotExist(err) {
				return Result{}, fmt.Errorf("stat workspace root %q: %w", workspaceRoot, err)
			}
		} else if info.IsDir() {
			workspacePath := filepath.Join(workspaceRoot, fileName)
			workspaceSection, foundWorkspace, err := loadFile(
				workspacePath,
				workspaceRoot,
				seenPaths,
				loadFileOptions{Label: "Workspace SOUL.md"},
			)
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

func loadNamedPersonality(
	opts LoadOptions,
	seenPaths map[string]struct{},
) (string, bool, error) {
	name := strings.TrimSpace(opts.PersonalityName)
	personalityConfig := opts.PersonalityConfig
	sections := make([]string, 0, 2)

	if strings.TrimSpace(personalityConfig.Soul) != "" {
		section, found, err := loadFile(
			personalityConfig.Soul,
			opts.WorkspaceRoot,
			seenPaths,
			loadFileOptions{Label: fmt.Sprintf("Personality %s SOUL.md", name), Required: true},
		)
		if err != nil {
			return "", false, err
		}
		if found {
			sections = append(sections, section)
		}
	}

	if strings.TrimSpace(personalityConfig.Instruct) != "" {
		displayName := fmt.Sprintf("personality:%s.instruct", name)
		scanned := guardrails.SafetyScan(strings.TrimSpace(personalityConfig.Instruct), displayName)
		sections = append(sections, fmt.Sprintf("## Personality %s instruct\n%s", name, scanned.Content))
	}

	if len(sections) == 0 {
		return "", false, nil
	}

	return strings.Join(sections, "\n\n"), true, nil
}

type loadFileOptions struct {
	Label    string
	Required bool
}

func loadFile(
	path string,
	workspaceRoot string,
	seenPaths map[string]struct{},
	opts loadFileOptions,
) (string, bool, error) {
	if absolutePath, err := filepath.Abs(path); err == nil {
		path = absolutePath
	}
	if _, ok := seenPaths[path]; ok {
		return "", false, nil
	}

	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			if opts.Required {
				return "", false, fmt.Errorf("personality file %q is required", path)
			}

			return "", false, nil
		}

		return "", false, fmt.Errorf("stat personality file %q: %w", path, err)
	}

	if info.IsDir() {
		if opts.Required {
			return "", false, fmt.Errorf("personality file %q is a directory", path)
		}

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
	label := strings.TrimSpace(opts.Label)
	if label == "" {
		label = displayPath
	}

	return fmt.Sprintf("## %s\n%s", label, scanned.Content), true, nil
}

func getDisplayPath(path, workspaceRoot string) (string, error) {
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
