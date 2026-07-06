// Package profile resolves the active Morph profile identity and metadata paths.
package profile

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"

	"github.com/wandxy/morph/internal/datadir/files"
	"github.com/wandxy/morph/pkg/str"
)

const (
	// DefaultName is the profile name used when no explicit or environment profile is set.
	DefaultName = "default"

	// EnvName is the environment variable used to select the active profile.
	EnvName = "MORPH_PROFILE"
)

const namePattern = `[a-zA-Z0-9][a-zA-Z0-9_-]{0,63}`

var validName = regexp.MustCompile(`^` + namePattern + `$`)

var (
	activeMu    sync.RWMutex
	active      Profile
	userHomeDir = os.UserHomeDir
)

// Profile describes the resolved profile identity and profile-local metadata paths.
type Profile struct {
	Name        string
	HomeDir     string
	ConfigPath  string
	EnvPath     string
	RuntimePath string
	PIDPath     string
}

// ResolveOptions controls profile resolution.
type ResolveOptions struct {
	Name        string
	Env         map[string]string
	UserHomeDir string
}

type stateFile map[string]any

// Resolve returns the active profile from explicit options, environment, stored current, or the default.
func Resolve(opts ResolveOptions) (Profile, error) {
	homeDir, err := resolveHomeDir(opts.UserHomeDir)
	if err != nil {
		return Profile{}, err
	}

	name, err := resolveNameWithStoredCurrent(opts.Name, opts.Env, homeDir)
	if err != nil {
		return Profile{}, err
	}

	profileHome := filepath.Join(homeDir, ".morph", "profiles", name)
	return WithMetadataPaths(Profile{Name: name, HomeDir: profileHome}), nil
}

// RootDir returns the machine-local Morph root for profile selectors and profiles.
func RootDir(homeDir string) (string, error) {
	homeDir, err := resolveHomeDir(homeDir)
	if err != nil {
		return "", err
	}

	return filepath.Join(homeDir, ".morph"), nil
}

// ProfilesDir returns the directory containing profile homes.
func ProfilesDir(homeDir string) (string, error) {
	root, err := RootDir(homeDir)
	if err != nil {
		return "", err
	}

	return filepath.Join(root, "profiles"), nil
}

// CurrentPath returns the machine-local app state path containing the current profile selector.
func CurrentPath(homeDir string) (string, error) {
	root, err := RootDir(homeDir)
	if err != nil {
		return "", err
	}

	return filepath.Join(root, files.StateFilename), nil
}

// LoadCurrentName returns the stored current profile name when configured.
func LoadCurrentName(homeDir string) (string, bool, error) {
	path, err := CurrentPath(homeDir)
	if err != nil {
		return "", false, err
	}

	state, err := loadStateFile(path)
	if err != nil {
		return "", false, err
	}
	stringValue1 := str.String(getStateFileString(state, "current_profile"))
	if value := stringValue1.Trim(); value != "" {
		name, err := NormalizeName(value)
		if err != nil {
			return "", false, err
		}

		return name, true, nil
	}

	return "", false, nil
}

// StoreCurrentName validates and stores the machine-local current profile name.
func StoreCurrentName(name string, homeDir string) (string, error) {
	name, err := NormalizeName(name)
	if err != nil {
		return "", err
	}

	path, err := CurrentPath(homeDir)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return "", fmt.Errorf("create profile selector dir: %w", err)
	}
	state, err := loadStateFile(path)
	if err != nil {
		return "", err
	}
	state["current_profile"] = name

	data := encodeStateFile(state)
	if err := os.WriteFile(path, append(data, '\n'), 0o600); err != nil {
		return "", fmt.Errorf("write current profile: %w", err)
	}

	return name, nil
}

func loadStateFile(path string) (stateFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return stateFile{}, nil
		}

		return nil, fmt.Errorf("read current profile: %w", err)
	}
	stringValue2 := str.String(string(data))
	if len(stringValue2.Trim()) == 0 {
		return stateFile{}, nil
	}

	var state stateFile
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("parse profile state: %w", err)
	}
	if state == nil {
		state = stateFile{}
	}

	return state, nil
}

func getStateFileString(state stateFile, key string) string {
	value, _ := state[key].(string)
	stringValue3 := str.String(value)
	return stringValue3.Trim()
}

func encodeStateFile(state stateFile) []byte {
	data, _ := json.MarshalIndent(state, "", "  ")
	return data
}

// Init creates the profile home directory and returns the resolved profile.
func Init(name string, homeDir string) (Profile, error) {
	resolved, err := Resolve(ResolveOptions{Name: name, UserHomeDir: homeDir})
	if err != nil {
		return Profile{}, err
	}
	if err := os.MkdirAll(resolved.HomeDir, 0o700); err != nil {
		return Profile{}, fmt.Errorf("create profile dir: %w", err)
	}

	return resolved, nil
}

// List returns profile names with existing profile directories.
func List(homeDir string) ([]string, error) {
	profilesDir, err := ProfilesDir(homeDir)
	if err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(profilesDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}

		return nil, fmt.Errorf("read profiles dir: %w", err)
	}

	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() || !IsValidName(entry.Name()) {
			continue
		}

		names = append(names, strings.ToLower(entry.Name()))
	}
	sort.Strings(names)

	return names, nil
}

// WithMetadataPaths returns profile with empty metadata paths filled from HomeDir.
func WithMetadataPaths(profile Profile) Profile {
	stringValue4 := str.String(profile.HomeDir)
	homeDir := stringValue4.Trim()
	if homeDir == "" {
		return profile
	}
	stringValue5 := str.String(profile.ConfigPath)
	if stringValue5.Trim() == "" {
		profile.ConfigPath = filepath.Join(homeDir, "config.yaml")
	}
	stringValue6 := str.String(profile.EnvPath)
	if stringValue6.Trim() == "" {
		profile.EnvPath = filepath.Join(homeDir, ".env")
	}
	stringValue7 := str.String(profile.RuntimePath)
	if stringValue7.Trim() == "" {
		profile.RuntimePath = filepath.Join(homeDir, "runtime.json")
	}
	stringValue8 := str.String(profile.PIDPath)
	if stringValue8.Trim() == "" {
		profile.PIDPath = filepath.Join(homeDir, "morph.pid")
	}

	return profile
}

// SetActive describes profile as the active process-local profile.
func SetActive(profile Profile) {
	activeMu.Lock()
	defer activeMu.Unlock()
	active = profile
}

// Active returns the active process-local profile.
func Active() Profile {
	activeMu.RLock()
	defer activeMu.RUnlock()
	return active
}

// ResolveName returns the normalized profile name from explicit input, environment, or default without reading stored current.
func ResolveName(explicitName string, env map[string]string) (string, error) {
	stringValue9 := str.String(explicitName)
	name := stringValue9.Trim()
	if name == "" {
		stringValue10 := str.String(envValue(env, EnvName))
		name = stringValue10.Trim()
	}
	if name == "" {
		name = DefaultName
	}

	return NormalizeName(name)
}

// NormalizeName validates a profile name and returns its canonical lowercase form.
func NormalizeName(name string) (string, error) {
	stringValue11 := str.String(name)
	name = stringValue11.Trim()
	if name == "" {
		return DefaultName, nil
	}
	if !validName.MatchString(name) {
		return "", fmt.Errorf("invalid profile name %q: must match %s", name, namePattern)
	}

	return strings.ToLower(name), nil
}

// IsValidName reports whether name is a non-empty path-safe profile name.
func IsValidName(name string) bool {
	stringValue12 := str.String(name)
	name = stringValue12.Trim()
	return name != "" && validName.MatchString(name)
}

func envValue(env map[string]string, key string) string {
	if env != nil {
		return env[key]
	}

	return os.Getenv(key)
}

func resolveNameWithStoredCurrent(explicitName string, env map[string]string, homeDir string) (string, error) {
	stringValue13 := str.String(explicitName)
	name := stringValue13.Trim()
	if name == "" {
		stringValue14 := str.String(envValue(env, EnvName))
		name = stringValue14.Trim()
	}
	if name != "" {
		return NormalizeName(name)
	}

	current, ok, err := LoadCurrentName(homeDir)
	if err != nil {
		return "", err
	}
	if ok {
		return current, nil
	}

	return DefaultName, nil
}

func resolveHomeDir(homeDir string) (string, error) {
	stringValue15 := str.String(homeDir)
	homeDir = stringValue15.Trim()
	if homeDir == "" {
		var err error
		homeDir, err = userHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve user home dir: %w", err)
		}
	}
	stringValue16 := str.String(homeDir)
	if stringValue16.Trim() == "" {
		return "", errors.New("home directory is required")
	}

	return filepath.Clean(homeDir), nil
}
