package datadir

import (
	"os"
	"path/filepath"
	"strings"
)

var (
	getenv      = os.Getenv
	userHomeDir = os.UserHomeDir
)

func ProjectHomeDir() string {
	if value := strings.TrimSpace(getenv("HAND_HOME")); value != "" {
		return value
	}

	home, err := userHomeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		return ".hand"
	}

	return filepath.Join(home, ".hand")
}

func HomeDir() string {
	return ProjectHomeDir()
}

func DataDir() string {
	return filepath.Join(ProjectHomeDir(), "data")
}

func DebugTraceDir() string {
	return filepath.Join(ProjectHomeDir(), "traces")
}

func StateDBPath() string {
	return filepath.Join(DataDir(), "state.db")
}

func SessionDBPath() string {
	return filepath.Join(DataDir(), "session.db")
}
