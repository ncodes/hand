package tui

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/wandxy/hand/internal/datadir/files"
	"github.com/wandxy/hand/internal/profile"
)

type appTUIState struct {
	CurrentProfile string            `json:"current_profile,omitempty"`
	LastSessions   map[string]string `json:"last_sessions,omitempty"`
}

func loadLastSessionID() (string, error) {
	path := appTUIStatePath()
	if path == "" {
		return "", nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}

		return "", fmt.Errorf("read last session: %w", err)
	}

	var state appTUIState
	if err := json.Unmarshal(data, &state); err != nil {
		return "", fmt.Errorf("parse tui state: %w", err)
	}

	return strings.TrimSpace(state.LastSessions[getActiveProfileName()]), nil
}

func saveLastSessionID(id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil
	}

	path := appTUIStatePath()
	if path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create profile metadata dir: %w", err)
	}

	state, err := loadAppTUIState(path)
	if err != nil {
		return err
	}
	if state.LastSessions == nil {
		state.LastSessions = map[string]string{}
	}
	state.LastSessions[getActiveProfileName()] = id

	data := encodeAppTUIState(state)
	return os.WriteFile(path, append(data, '\n'), 0o600)
}

func encodeAppTUIState(state appTUIState) []byte {
	data, _ := json.MarshalIndent(state, "", "  ")

	return data
}

func loadAppTUIState(path string) (appTUIState, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return appTUIState{}, nil
		}

		return appTUIState{}, fmt.Errorf("read tui state: %w", err)
	}

	var state appTUIState
	if err := json.Unmarshal(data, &state); err != nil {
		return appTUIState{}, fmt.Errorf("parse tui state: %w", err)
	}

	return state, nil
}

func appTUIStatePath() string {
	active := profile.WithMetadataPaths(profile.Active())
	home := strings.TrimSpace(active.HomeDir)
	if home == "" {
		return ""
	}

	return filepath.Join(getProfileRootDir(active), files.StateFilename)
}

func getProfileRootDir(active profile.Profile) string {
	home := strings.TrimSpace(active.HomeDir)
	name := strings.TrimSpace(active.Name)
	if name != "" &&
		filepath.Base(home) == name &&
		filepath.Base(filepath.Dir(home)) == "profiles" {
		return filepath.Dir(filepath.Dir(home))
	}

	return home
}

func getActiveProfileName() string {
	name := strings.TrimSpace(profile.Active().Name)
	if name == "" {
		return profile.DefaultName
	}

	return name
}
