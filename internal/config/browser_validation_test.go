package config

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestConfig_ValidateRejectsUnresolvableBrowserRoot(t *testing.T) {
	directory := t.TempDir()
	loop := filepath.Join(directory, "loop")
	if err := os.Symlink(loop, loop); err != nil {
		t.Skip(err)
	}
	cfg := getValidBrowserConfig(t)
	cfg.Browser.ProfileRoot = loop

	require.EqualError(t, cfg.ValidateRelaxed(), "browser managed root could not be resolved")
}

func TestConfig_ValidateRejectsMissingBrowserProfileName(t *testing.T) {
	cfg := getValidBrowserConfig(t)
	cfg.Browser.Profiles[0].Name = ""
	cfg.Browser.DefaultProfile = ""

	require.EqualError(t, cfg.ValidateRelaxed(), "browser profile name is required")
}

func TestValidateBrowserNetwork_ValidatesDevelopmentExceptions(t *testing.T) {
	tests := []struct {
		name    string
		config  BrowserNetworkConfig
		wantErr string
	}{
		{
			name: "valid host and CIDR",
			config: BrowserNetworkConfig{
				DevelopmentAllowedHosts: []string{"LOCALHOST.", "bücher.example"},
				DevelopmentAllowedCIDRs: []string{"127.0.0.1/32", "::1/128"},
			},
		},
		{name: "empty host", config: BrowserNetworkConfig{DevelopmentAllowedHosts: []string{""}},
			wantErr: "browser development allowed host is invalid"},
		{name: "host with path", config: BrowserNetworkConfig{DevelopmentAllowedHosts: []string{"localhost/admin"}},
			wantErr: "browser development allowed host is invalid"},
		{name: "host with port", config: BrowserNetworkConfig{DevelopmentAllowedHosts: []string{"localhost:3000"}},
			wantErr: "browser development allowed host is invalid"},
		{name: "host with user information", config: BrowserNetworkConfig{DevelopmentAllowedHosts: []string{"user@localhost"}},
			wantErr: "browser development allowed host is invalid"},
		{name: "invalid international host", config: BrowserNetworkConfig{DevelopmentAllowedHosts: []string{"\u200d.example"}},
			wantErr: "browser development allowed host is invalid"},
		{name: "invalid CIDR", config: BrowserNetworkConfig{DevelopmentAllowedCIDRs: []string{"127.0.0.1"}},
			wantErr: "browser development allowed CIDR is invalid"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateBrowserNetwork(test.config)
			if test.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.EqualError(t, err, test.wantErr)
		})
	}
}

func TestValidateBrowserProfile_ValidatesModeSpecificSettings(t *testing.T) {
	root := filepath.Join(t.TempDir(), "profiles")
	persistent := filepath.Join(root, "persistent")
	tests := []struct {
		name     string
		profile  BrowserProfileConfig
		wantPath string
		wantErr  string
	}{
		{name: "ephemeral", profile: BrowserProfileConfig{Mode: BrowserProfileManagedEphemeral}},
		{name: "persistent", profile: BrowserProfileConfig{Mode: BrowserProfileManagedPersistent, Directory: persistent},
			wantPath: persistent},
		{name: "persistent endpoint", profile: BrowserProfileConfig{
			Mode: BrowserProfileManagedPersistent, Directory: persistent, CDPEndpoint: "https://example.com",
		}, wantErr: "managed persistent profile cannot set attachment configuration"},
		{name: "remote", profile: BrowserProfileConfig{
			Mode: BrowserProfileRemoteCDP, CDPEndpoint: "https://example.com", AttachmentScope: BrowserAttachmentBrowser,
		}},
		{name: "existing session", profile: BrowserProfileConfig{
			Mode: BrowserProfileExistingSession, CDPEndpoint: "ws://localhost:9222", DataIdentity: "profile-1",
			AttachmentScope: BrowserAttachmentBrowser,
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path, err := validateBrowserProfile(root, test.profile)
			if test.wantErr != "" {
				require.EqualError(t, err, test.wantErr)
				require.Empty(t, path)
				return
			}
			require.NoError(t, err)
			if test.wantPath == "" {
				require.Empty(t, path)
				return
			}
			expected, canonicalErr := getCanonicalPath(test.wantPath)
			require.NoError(t, canonicalErr)
			require.Equal(t, expected, path)
		})
	}
}

func TestValidateBrowserProfile_RequiresExplicitAttachmentIdentityAndScope(t *testing.T) {
	tests := []struct {
		name    string
		profile BrowserProfileConfig
		wantErr string
	}{
		{
			name: "credential reference", profile: BrowserProfileConfig{
				Mode: BrowserProfileRemoteCDP, CDPEndpoint: "https://example.com",
				CredentialRef: "env:CDP_TOKEN", AttachmentScope: BrowserAttachmentBrowser,
			},
		},
		{
			name: "missing personal identity", profile: BrowserProfileConfig{
				Mode: BrowserProfileExistingSession, CDPEndpoint: "https://example.com",
				AttachmentScope: BrowserAttachmentBrowser,
			}, wantErr: "existing session data identity is required",
		},
		{
			name: "missing scope", profile: BrowserProfileConfig{
				Mode: BrowserProfileRemoteCDP, CDPEndpoint: "https://example.com",
			}, wantErr: "attachment scope must be one of: targets, context, browser",
		},
		{
			name: "target scope without targets", profile: BrowserProfileConfig{
				Mode: BrowserProfileRemoteCDP, CDPEndpoint: "https://example.com",
				AttachmentScope: BrowserAttachmentTargets,
			}, wantErr: "target attachment scope requires target IDs and no browser context ID",
		},
		{
			name: "context scope without context", profile: BrowserProfileConfig{
				Mode: BrowserProfileRemoteCDP, CDPEndpoint: "https://example.com",
				AttachmentScope: BrowserAttachmentContext,
			}, wantErr: "context attachment scope requires a browser context ID and no target IDs",
		},
		{
			name: "browser scope with target", profile: BrowserProfileConfig{
				Mode: BrowserProfileRemoteCDP, CDPEndpoint: "https://example.com",
				AttachmentScope: BrowserAttachmentBrowser, TargetIDs: []string{"target-1"},
			}, wantErr: "browser attachment scope cannot set a browser context ID or target IDs",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := validateBrowserProfile(t.TempDir(), test.profile)
			if test.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.EqualError(t, err, test.wantErr)
		})
	}
}

func TestValidateCDPEndpoint_ValidatesSupportedEndpointShape(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		wantErr string
	}{
		{name: "HTTP", value: "http://localhost:9222"},
		{name: "HTTPS", value: "https://browser.example.com"},
		{name: "WebSocket", value: "ws://localhost:9222/devtools/browser/id"},
		{name: "secure WebSocket", value: "wss://browser.example.com/devtools/browser/id"},
		{name: "malformed", value: "http://[::1", wantErr: "CDP endpoint is invalid"},
		{name: "missing host", value: "https:///devtools", wantErr: "CDP endpoint is invalid"},
		{name: "unsupported scheme", value: "ftp://browser.example.com",
			wantErr: "CDP endpoint scheme must be one of: http, https, ws, wss"},
		{name: "userinfo", value: "https://user@browser.example.com",
			wantErr: "CDP endpoint must not contain inline credentials, query parameters, or fragments"},
		{name: "query", value: "https://browser.example.com?token=secret",
			wantErr: "CDP endpoint must not contain inline credentials, query parameters, or fragments"},
		{name: "fragment", value: "https://browser.example.com/#secret",
			wantErr: "CDP endpoint must not contain inline credentials, query parameters, or fragments"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateCDPEndpoint(test.value)
			if test.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.EqualError(t, err, test.wantErr)
		})
	}
}

func TestValidateManagedProfilePath_RejectsPersonalBrowserDirectory(t *testing.T) {
	home, err := os.UserHomeDir()
	require.NoError(t, err)
	profile := filepath.Join(home, ".config", "google-chrome", "morph")

	path, err := validateManagedProfilePath(home, profile)

	require.Empty(t, path)
	require.EqualError(t, err, "managed persistent profile directory must not use a personal browser data root")
}

func TestValidateManagedProfilePath_RejectsRelativePaths(t *testing.T) {
	absolute := t.TempDir()
	tests := []struct {
		name       string
		root       string
		configured string
	}{
		{name: "relative root", root: "relative-root", configured: filepath.Join(absolute, "profile")},
		{name: "relative profile", root: absolute, configured: "relative-profile"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path, err := validateManagedProfilePath(test.root, test.configured)
			require.Empty(t, path)
			require.EqualError(t, err, "managed persistent profile paths must be absolute")
		})
	}

	path, err := getCanonicalPath("relative-profile")
	require.Empty(t, path)
	require.EqualError(t, err, "path must be absolute")
}

func TestValidateManagedProfilePath_PropagatesCanonicalizationErrors(t *testing.T) {
	root := t.TempDir()
	profile := filepath.Join(root, "profile")
	wantErr := errors.New("canonicalization failed")
	tests := []struct {
		name      string
		canonical func(string) (string, error)
	}{
		{
			name: "root",
			canonical: func(string) (string, error) {
				return "", wantErr
			},
		},
		{
			name: "profile",
			canonical: func(path string) (string, error) {
				if path == root {
					return root, nil
				}
				return "", wantErr
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path, err := validateManagedProfilePathWith(root, profile, test.canonical)
			require.Empty(t, path)
			require.ErrorIs(t, err, wantErr)
		})
	}
}

func TestValidateManagedProfilePath_RejectsCanonicalPathOutsideRoot(t *testing.T) {
	root := t.TempDir()
	profile := filepath.Join(root, "profile")

	path, err := validateManagedProfilePathWith(root, profile, func(path string) (string, error) {
		if path == root {
			return root, nil
		}
		return filepath.Dir(root), nil
	})

	require.Empty(t, path)
	require.EqualError(t, err, "managed persistent profile directory must be inside the browser profile root")
}

func TestHasPathSymlink_DetectsLinksAndInvalidComponents(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(root, "directory"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(root, "file"), []byte("content"), 0o600))
	link := filepath.Join(root, "link")
	if err := os.Symlink(filepath.Join(root, "directory"), link); err != nil {
		t.Skip(err)
	}

	require.False(t, hasPathSymlink(root, root))
	require.False(t, hasPathSymlink(root, filepath.Join(filepath.Dir(root), "outside")))
	require.False(t, hasPathSymlink(root, filepath.Join(root, "missing")))
	require.False(t, hasPathSymlink(root, filepath.Join(root, "directory")))
	require.True(t, hasPathSymlink(root, link))
	require.True(t, hasPathSymlink(root, filepath.Join(root, "file", "child")))
}

func TestGetCanonicalPath_ResolvesExistingLinksAndFutureSegments(t *testing.T) {
	root := t.TempDir()
	realDirectory := filepath.Join(root, "real")
	require.NoError(t, os.Mkdir(realDirectory, 0o700))
	link := filepath.Join(root, "link")
	if err := os.Symlink(realDirectory, link); err != nil {
		t.Skip(err)
	}

	resolved, err := getCanonicalPath(filepath.Join(link, "future", "child"))

	require.NoError(t, err)
	canonicalRealDirectory, err := getCanonicalPath(realDirectory)
	require.NoError(t, err)
	require.Equal(t, filepath.Join(canonicalRealDirectory, "future", "child"), resolved)
}

func TestGetCanonicalPath_RejectsSymbolicLinkLoop(t *testing.T) {
	loop := filepath.Join(t.TempDir(), "loop")
	if err := os.Symlink(loop, loop); err != nil {
		t.Skip(err)
	}

	resolved, err := getCanonicalPath(loop)

	require.Empty(t, resolved)
	require.Error(t, err)
}

func TestGetCanonicalPath_ReturnsErrorWhenNoAncestorCanBeResolved(t *testing.T) {
	resolved, err := getCanonicalPathWith(filepath.VolumeName(t.TempDir())+string(filepath.Separator), func(string) (string, error) {
		return "", os.ErrNotExist
	})

	require.Empty(t, resolved)
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestIsKnownPersonalBrowserPath_HandlesMissingHomeAndKnownDirectories(t *testing.T) {
	home, err := os.UserHomeDir()
	require.NoError(t, err)
	require.True(t, isKnownPersonalBrowserPath(filepath.Join(home, ".config", "google-chrome", "Default")))
	require.True(t, isKnownPersonalBrowserPath(filepath.Join(home, ".config", "BraveSoftware", "Brave-Browser", "Default")))
	require.True(t, isKnownPersonalBrowserPath(filepath.Join(home, "snap", "chromium", "common", "chromium", "Default")))
	require.True(t, isKnownPersonalBrowserPath(filepath.Join(
		home, "Library", "Application Support", "Google", "Chrome Canary", "Default",
	)))
	require.True(t, isKnownPersonalBrowserPath(filepath.Join(
		home, "AppData", "Local", "Microsoft", "Edge Dev", "User Data", "Default",
	)))
	require.False(t, isKnownPersonalBrowserPath(t.TempDir()))

	if runtime.GOOS == "windows" {
		t.Setenv("USERPROFILE", "")
	} else {
		t.Setenv("HOME", "")
	}
	require.False(t, isKnownPersonalBrowserPath(t.TempDir()))
}

func getValidBrowserConfig(t *testing.T) *Config {
	t.Helper()
	directory := t.TempDir()
	cfg := NewDefaultConfig()
	cfg.Normalize()
	cfg.Browser.ProfileRoot = filepath.Join(directory, "profiles")
	cfg.Browser.TemporaryRoot = filepath.Join(directory, "temporary")
	cfg.Browser.Artifacts.Root = filepath.Join(directory, "artifacts")
	return cfg
}
