// Package profile resolves the active Hand profile identity and metadata paths.
package profile

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
)

const (
	// DefaultName is the profile name used when no explicit or environment profile is set.
	DefaultName = "default"

	// EnvName is the environment variable used to select the active profile.
	EnvName = "HAND_PROFILE"
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

// Resolve returns the active profile from explicit options, environment, or the default.
func Resolve(opts ResolveOptions) (Profile, error) {
	name, err := ResolveName(opts.Name, opts.Env)
	if err != nil {
		return Profile{}, err
	}

	homeDir, err := resolveHomeDir(opts.UserHomeDir)
	if err != nil {
		return Profile{}, err
	}

	profileHome := filepath.Join(homeDir, ".hand", "profiles", name)
	return WithMetadataPaths(Profile{Name: name, HomeDir: profileHome}), nil
}

// WithMetadataPaths returns profile with empty metadata paths filled from HomeDir.
func WithMetadataPaths(profile Profile) Profile {
	homeDir := strings.TrimSpace(profile.HomeDir)
	if homeDir == "" {
		return profile
	}
	if strings.TrimSpace(profile.ConfigPath) == "" {
		profile.ConfigPath = filepath.Join(homeDir, "config.yaml")
	}
	if strings.TrimSpace(profile.EnvPath) == "" {
		profile.EnvPath = filepath.Join(homeDir, ".env")
	}
	if strings.TrimSpace(profile.RuntimePath) == "" {
		profile.RuntimePath = filepath.Join(homeDir, "runtime.json")
	}
	if strings.TrimSpace(profile.PIDPath) == "" {
		profile.PIDPath = filepath.Join(homeDir, "hand.pid")
	}

	return profile
}

// SetActive stores profile as the active process-local profile.
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

// ResolveName returns the normalized profile name from explicit input, environment, or default.
func ResolveName(explicitName string, env map[string]string) (string, error) {
	name := strings.TrimSpace(explicitName)
	if name == "" {
		name = strings.TrimSpace(envValue(env, EnvName))
	}
	if name == "" {
		name = DefaultName
	}

	return NormalizeName(name)
}

// NormalizeName validates a profile name and returns its canonical lowercase form.
func NormalizeName(name string) (string, error) {
	name = strings.TrimSpace(name)
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
	name = strings.TrimSpace(name)
	return name != "" && validName.MatchString(name)
}

func envValue(env map[string]string, key string) string {
	if env != nil {
		return env[key]
	}

	return os.Getenv(key)
}

func resolveHomeDir(homeDir string) (string, error) {
	homeDir = strings.TrimSpace(homeDir)
	if homeDir == "" {
		var err error
		homeDir, err = userHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve user home dir: %w", err)
		}
	}
	if strings.TrimSpace(homeDir) == "" {
		return "", errors.New("home directory is required")
	}

	return filepath.Clean(homeDir), nil
}
