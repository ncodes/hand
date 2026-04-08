package common

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/wandxy/hand/internal/tools"
)

var (
	LookPath       = exec.LookPath
	CommandContext = exec.CommandContext
	ReadDir        = os.ReadDir
	WriteFile      = os.WriteFile
	MkdirAll       = os.MkdirAll
	StatFile       = os.Stat
	WalkDir        = filepath.WalkDir
)

const (
	MaxListEntries   = 500
	MaxSearchResults = 200
	MaxReadBytes     = 256 * 1024
	MaxOutputBytes   = 256 * 1024
	DefaultTimeout   = 30
	MaxTimeout       = 120
)

func DecodeInput(call tools.Call, target any) tools.Result {
	if strings.TrimSpace(call.Input) == "" {
		call.Input = "{}"
	}
	if err := json.Unmarshal([]byte(call.Input), target); err != nil {
		return ToolError("invalid_input", "invalid tool input")
	}

	return tools.Result{}
}

func EncodeOutput(value any) (tools.Result, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return tools.Result{}, err
	}

	return tools.Result{Output: string(raw)}, nil
}

func ToolError(code, message string) tools.Result {
	return tools.Result{Error: tools.Error{Code: code, Message: message}.String()}
}

func FileError(err error) tools.Result {
	if err == nil {
		return tools.Result{}
	}

	switch {
	case errors.Is(err, os.ErrNotExist):
		return ToolError("not_found", "path not found")
	case errors.Is(err, os.ErrPermission):
		return ToolError("access_denied", "access denied")
	case errors.Is(err, fs.ErrInvalid):
		return ToolError("invalid_input", "path must be a file")
	case strings.Contains(err.Error(), "outside allowed roots"):
		return ToolError("path_outside_roots", "path is outside allowed roots")
	case strings.Contains(err.Error(), "size limit"):
		return ToolError("too_large", err.Error())
	case strings.Contains(err.Error(), "not text"):
		return ToolError("not_text", "file is not text")
	case strings.Contains(err.Error(), "directory"):
		return ToolError("invalid_input", "path must be a file")
	default:
		return ToolError("internal_error", err.Error())
	}
}

func HiddenPath(path string) bool {
	for part := range strings.SplitSeq(filepath.ToSlash(path), "/") {
		if strings.HasPrefix(part, ".") && part != "." && part != ".." {
			return true
		}
	}

	return false
}

func NormalizedDisplayPath(path string) string {
	if path == "" {
		return "."
	}

	return filepath.ToSlash(path)
}

func TrimOutput(value string, limit int) string {
	if len(value) <= limit {
		return value
	}

	return value[:limit]
}

func WithTimeoutSeconds(value int) int {
	if value <= 0 {
		return DefaultTimeout
	}
	if value > MaxTimeout {
		return MaxTimeout
	}

	return value
}

func JoinStrings(parts ...string) string {
	filtered := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		filtered = append(filtered, part)
	}

	return strings.Join(filtered, " ")
}

func ObjectSchema(properties map[string]any, required ...string) map[string]any {
	if properties == nil {
		properties = map[string]any{}
	}
	if required == nil {
		required = []string{}
	}

	schema := map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties":           properties,
		"required":             required,
	}

	return schema
}

func StringSchema(description string) map[string]any {
	return map[string]any{
		"type":        "string",
		"description": description,
	}
}

func BooleanSchema(description string) map[string]any {
	return map[string]any{
		"type":        "boolean",
		"description": description,
	}
}

func IntegerSchema(description string) map[string]any {
	return map[string]any{
		"type":        "integer",
		"description": description,
	}
}
