package personality

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/wandxy/morph/internal/config"
	"github.com/wandxy/morph/internal/constants"
	"github.com/wandxy/morph/internal/datadir"
	"github.com/wandxy/morph/internal/guardrails"
	"github.com/wandxy/morph/pkg/promptio"
	"github.com/wandxy/morph/pkg/stringx"
)

const fileName = constants.PersonalityFileName
const maxContentLength = constants.PersonalityMaxContentLength

var (
	getwd              = os.Getwd
	readFile           = os.ReadFile
	resolveDisplayPath = getDisplayPath
)

// Result contains loaded instruction text and related metadata.
type Result struct {
	Content      string
	Found        bool
	SafetyEvents []guardrails.SafetyTracePayloadOptions
}

// LoadOptions selects the profile, workspace, and configured personality sources to load.
type LoadOptions struct {
	ProfileHome       string
	WorkspaceRoot     string
	PersonalityName   string
	PersonalityConfig config.PersonalityConfig
	AllowWorkspace    bool
}

// Load reads the configured personality instructions.
func Load(opts LoadOptions) (Result, error) {
	opts.ProfileHome = stringx.String(opts.ProfileHome).Trim()
	if opts.ProfileHome == "" {
		opts.ProfileHome = datadir.ProjectHomeDir()
	}

	if opts.AllowWorkspace && stringx.String(opts.WorkspaceRoot).Trim() == "" {
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
	safetyEvents := make([]guardrails.SafetyTracePayloadOptions, 0, 2)
	seenPaths := make(map[string]struct{}, 2)

	profileHome := stringx.String(opts.ProfileHome).Trim()
	workspaceRoot := stringx.String(opts.WorkspaceRoot).Trim()
	personalityName := stringx.String(opts.PersonalityName).Trim()

	if personalityName == "" {
		globalPath := filepath.Join(profileHome, fileName)
		globalSection, foundGlobal, events, err := loadFile(
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
		safetyEvents = append(safetyEvents, events...)
	} else {
		section, found, events, err := loadNamedPersonality(opts, seenPaths)
		if err != nil {
			return Result{}, err
		}
		if found {
			sections = append(sections, section)
		}
		safetyEvents = append(safetyEvents, events...)
	}

	if opts.AllowWorkspace && workspaceRoot != "" {
		info, err := os.Stat(workspaceRoot)
		if err != nil {
			if !os.IsNotExist(err) {
				return Result{}, fmt.Errorf("stat workspace root %q: %w", workspaceRoot, err)
			}
		} else if info.IsDir() {
			workspacePath := filepath.Join(workspaceRoot, fileName)
			workspaceSection, foundWorkspace, events, err := loadFile(
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
			safetyEvents = append(safetyEvents, events...)
		}
	}

	if len(sections) == 0 {
		return Result{SafetyEvents: safetyEvents}, nil
	}

	return Result{
		Content:      truncate(strings.Join(sections, "\n\n")),
		Found:        true,
		SafetyEvents: safetyEvents,
	}, nil
}

func loadNamedPersonality(
	opts LoadOptions,
	seenPaths map[string]struct{},
) (string, bool, []guardrails.SafetyTracePayloadOptions, error) {
	name := stringx.String(opts.PersonalityName).Trim()
	personalityConfig := opts.PersonalityConfig
	sections := make([]string, 0, 2)
	safetyEvents := make([]guardrails.SafetyTracePayloadOptions, 0, 2)

	if stringx.String(personalityConfig.Soul).Trim() != "" {
		section, found, events, err := loadFile(
			personalityConfig.Soul,
			opts.WorkspaceRoot,
			seenPaths,
			loadFileOptions{Label: fmt.Sprintf("Personality %s SOUL.md", name), Required: true},
		)
		if err != nil {
			return "", false, nil, err
		}
		if found {
			sections = append(sections, section)
		}
		safetyEvents = append(safetyEvents, events...)
	}

	if stringx.String(personalityConfig.Instruct).Trim() != "" {
		displayName := fmt.Sprintf("personality:%s.instruct", name)
		content := stringx.String(personalityConfig.Instruct).Trim()
		scanned := guardrails.SafetyScan(content, displayName)
		if scanned.Blocked {
			safetyEvents = append(safetyEvents, loadedContentSafetyEvent(displayName, content, scanned.Findings))
		}
		sections = append(sections, fmt.Sprintf("## Personality %s instruct\n%s", name, scanned.Content))
	}

	if len(sections) == 0 {
		return "", false, safetyEvents, nil
	}

	return strings.Join(sections, "\n\n"), true, safetyEvents, nil
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
) (string, bool, []guardrails.SafetyTracePayloadOptions, error) {
	if absolutePath, err := filepath.Abs(path); err == nil {
		path = absolutePath
	}
	if _, ok := seenPaths[path]; ok {
		return "", false, nil, nil
	}

	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			if opts.Required {
				return "", false, nil, fmt.Errorf("personality file %q is required", path)
			}

			return "", false, nil, nil
		}

		return "", false, nil, fmt.Errorf("stat personality file %q: %w", path, err)
	}

	if info.IsDir() {
		if opts.Required {
			return "", false, nil, fmt.Errorf("personality file %q is a directory", path)
		}

		return "", false, nil, nil
	}

	seenPaths[path] = struct{}{}

	content, err := readFile(path)
	if err != nil {
		return "", false, nil, fmt.Errorf("read personality file %q: %w", path, err)
	}
	contentText := stringx.String(string(content)).Trim()
	if contentText == "" {
		return "", false, nil, nil
	}

	displayPath, err := resolveDisplayPath(path, workspaceRoot)
	if err != nil {
		return "", false, nil, fmt.Errorf("resolve personality file path %q: %w", path, err)
	}

	scanned := guardrails.SafetyScan(contentText, displayPath)
	safetyEvents := []guardrails.SafetyTracePayloadOptions(nil)
	if scanned.Blocked {
		safetyEvents = append(safetyEvents, loadedContentSafetyEvent(displayPath, contentText, scanned.Findings))
	}
	label := stringx.String(opts.Label).Trim()
	if label == "" {
		label = displayPath
	}

	return fmt.Sprintf("## %s\n%s", label, scanned.Content), true, safetyEvents, nil
}

func loadedContentSafetyEvent(
	source string,
	content string,
	findings []guardrails.SafetyFinding,
) guardrails.SafetyTracePayloadOptions {
	return guardrails.SafetyTracePayloadOptions{
		Source:        source,
		Action:        "blocked",
		ContentLength: len([]rune(content)),
		Blocked:       true,
		Findings:      findings,
	}
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

	if cleanedHome := stringx.String(datadir.ProjectHomeDir()).Trim(); cleanedHome != "" {
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
