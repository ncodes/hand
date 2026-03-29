package native

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
	lookPath       = exec.LookPath
	commandContext = exec.CommandContext
	readDir        = os.ReadDir
	writeFile      = os.WriteFile
	mkdirAll       = os.MkdirAll
	statFile       = os.Stat
	walkDir        = filepath.WalkDir
)

const (
	maxListEntries   = 500
	maxSearchResults = 200
	maxReadBytes     = 256 * 1024
	maxOutputBytes   = 256 * 1024
	defaultTimeout   = 30
	maxTimeout       = 120
)

func decodeInput(call tools.Call, target any) tools.Result {
	if strings.TrimSpace(call.Input) == "" {
		call.Input = "{}"
	}
	if err := json.Unmarshal([]byte(call.Input), target); err != nil {
		return toolError("invalid_input", "invalid tool input")
	}
	return tools.Result{}
}

func encodeOutput(value any) (tools.Result, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return tools.Result{}, err
	}
	return tools.Result{Output: string(raw)}, nil
}

func toolError(code, message string) tools.Result {
	return tools.Result{Error: tools.Error{Code: code, Message: message}.String()}
}

func fileError(err error) tools.Result {
	if err == nil {
		return tools.Result{}
	}
	switch {
	case errors.Is(err, os.ErrNotExist):
		return toolError("not_found", "path not found")
	case errors.Is(err, os.ErrPermission):
		return toolError("access_denied", "access denied")
	case errors.Is(err, fs.ErrInvalid):
		return toolError("invalid_input", "path must be a file")
	case strings.Contains(err.Error(), "outside allowed roots"):
		return toolError("path_outside_roots", "path is outside allowed roots")
	case strings.Contains(err.Error(), "size limit"):
		return toolError("too_large", err.Error())
	case strings.Contains(err.Error(), "not text"):
		return toolError("not_text", "file is not text")
	case strings.Contains(err.Error(), "directory"):
		return toolError("invalid_input", "path must be a file")
	default:
		return toolError("internal_error", err.Error())
	}
}

func hiddenPath(path string) bool {
	for part := range strings.SplitSeq(filepath.ToSlash(path), "/") {
		if strings.HasPrefix(part, ".") && part != "." && part != ".." {
			return true
		}
	}
	return false
}

func normalizedDisplayPath(path string) string {
	if path == "" {
		return "."
	}
	return filepath.ToSlash(path)
}

func trimOutput(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	return value[:limit]
}

func withTimeoutSeconds(value int) int {
	if value <= 0 {
		return defaultTimeout
	}
	if value > maxTimeout {
		return maxTimeout
	}
	return value
}
