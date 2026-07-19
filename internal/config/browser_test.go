package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestConfig_NormalizeAppliesSecureBrowserDefaults(t *testing.T) {
	cfg := NewDefaultConfig()
	cfg.Browser = BrowserConfig{}
	cfg.Normalize()

	require.False(t, cfg.Browser.Enabled)
	require.Equal(t, DefaultBrowserProfileName, cfg.Browser.DefaultProfile)
	require.Equal(t, []BrowserProfileConfig{{
		Name: DefaultBrowserProfileName,
		Mode: BrowserProfileManagedEphemeral,
	}}, cfg.Browser.Profiles)
	require.True(t, cfg.Browser.Network.StrictEnabled())
	require.True(t, filepath.IsAbs(cfg.Browser.ProfileRoot))
	require.True(t, filepath.IsAbs(cfg.Browser.TemporaryRoot))
	require.True(t, filepath.IsAbs(cfg.Browser.Artifacts.Root))
	require.NoError(t, cfg.ValidateRelaxed())
	profile, ok := cfg.Browser.Profile(DefaultBrowserProfileName)
	require.True(t, ok)
	require.Equal(t, BrowserProfileManagedEphemeral, profile.Mode)
	_, ok = cfg.Browser.Profile("missing")
	require.False(t, ok)
}

func TestLoad_ParsesNormalizesAndResolvesBrowserConfiguration(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte(`
browser:
  enabled: true
  executable: bin/chrome
  defaultProfile: persistent
  profileRoot: profiles
  temporaryRoot: tmp
  startTimeout: 2s
  inactivityTimeout: 3m
  cleanupInterval: 10s
  terminalRetention: 20m
  profiles:
    - name: persistent
      mode: managed_persistent
      directory: profiles/persistent
  network:
    strict: true
    developmentAllowedHosts: [LOCALHOST, localhost]
    developmentAllowedCIDRs: [127.0.0.0/8]
  artifacts:
    root: artifacts
    maxBytes: 100
    maxTotalBytes: 1000
    retention: 1h
`), 0o600))

	cfg, err := Load("", path)
	require.NoError(t, err)
	require.Equal(t, filepath.Join(directory, "bin", "chrome"), cfg.Browser.Executable)
	require.Equal(t, filepath.Join(directory, "profiles"), cfg.Browser.ProfileRoot)
	require.Equal(t, filepath.Join(directory, "profiles", "persistent"), cfg.Browser.Profiles[0].Directory)
	require.Equal(t, []string{"localhost"}, cfg.Browser.Network.DevelopmentAllowedHosts)
	require.Equal(t, 20*time.Minute, cfg.Browser.TerminalRetention)
	require.NoError(t, cfg.ValidateRelaxed())
}

func TestLoad_PreservesBrowserExecutableCommandName(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte("browser:\n  executable: chromium\n"), 0o600))

	cfg, err := Load("", path)
	require.NoError(t, err)
	require.Equal(t, "chromium", cfg.Browser.Executable)
}

func TestConfig_ValidateRejectsUnsafeBrowserProfiles(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "profiles")
	valid := func() *Config {
		cfg := NewDefaultConfig()
		cfg.Normalize()
		cfg.Browser.ProfileRoot = root
		cfg.Browser.TemporaryRoot = filepath.Join(base, "tmp")
		cfg.Browser.Artifacts.Root = filepath.Join(base, "artifacts")
		return cfg
	}
	tests := []struct {
		name    string
		mutate  func(*Config)
		message string
	}{
		{
			name: "start timeout",
			mutate: func(cfg *Config) {
				cfg.Browser.StartTimeout = -1
			},
			message: "browser start timeout must be greater than zero",
		},
		{
			name: "inactivity timeout",
			mutate: func(cfg *Config) {
				cfg.Browser.InactivityTimeout = -1
			},
			message: "browser inactivity timeout must be greater than zero",
		},
		{
			name: "cleanup interval",
			mutate: func(cfg *Config) {
				cfg.Browser.CleanupInterval = -1
			},
			message: "browser cleanup interval must be greater than zero",
		},
		{
			name: "terminal retention",
			mutate: func(cfg *Config) {
				cfg.Browser.TerminalRetention = -1
			},
			message: "browser terminal retention must be greater than zero",
		},
		{
			name: "artifact max bytes",
			mutate: func(cfg *Config) {
				cfg.Browser.Artifacts.MaxBytes = -1
			},
			message: "browser artifact max bytes must be greater than zero",
		},
		{
			name: "artifact total bytes",
			mutate: func(cfg *Config) {
				cfg.Browser.Artifacts.MaxTotalBytes = cfg.Browser.Artifacts.MaxBytes - 1
			},
			message: "browser artifact total bytes must be greater than or equal to max bytes",
		},
		{
			name: "artifact retention",
			mutate: func(cfg *Config) {
				cfg.Browser.Artifacts.Retention = -1
			},
			message: "browser artifact retention must be greater than zero",
		},
		{
			name: "relative root",
			mutate: func(cfg *Config) {
				cfg.Browser.ProfileRoot = "relative"
			},
			message: "browser managed roots must be absolute",
		},
		{
			name: "overlapping managed roots",
			mutate: func(cfg *Config) {
				cfg.Browser.TemporaryRoot = filepath.Join(cfg.Browser.ProfileRoot, "tmp")
			},
			message: "browser managed roots must not overlap",
		},
		{
			name: "personal browser managed root",
			mutate: func(cfg *Config) {
				home, err := os.UserHomeDir()
				require.NoError(t, err)
				cfg.Browser.TemporaryRoot = filepath.Join(home, ".config", "google-chrome", "morph-temp")
			},
			message: "browser managed roots must not use a personal browser data root",
		},
		{
			name: "persistent outside root",
			mutate: func(cfg *Config) {
				cfg.Browser.Profiles = []BrowserProfileConfig{{
					Name: "bad", Mode: BrowserProfileManagedPersistent, Directory: filepath.Dir(root),
				}}
				cfg.Browser.DefaultProfile = "bad"
			},
			message: `browser profile "bad": managed persistent profile directory must be inside the browser profile root`,
		},
		{
			name: "overlapping persistent profiles",
			mutate: func(cfg *Config) {
				cfg.Browser.Profiles = []BrowserProfileConfig{
					{Name: "parent", Mode: BrowserProfileManagedPersistent, Directory: filepath.Join(root, "parent")},
					{Name: "child", Mode: BrowserProfileManagedPersistent, Directory: filepath.Join(root, "parent", "child")},
				}
				cfg.Browser.DefaultProfile = "parent"
			},
			message: "browser managed profile directories must not overlap",
		},
		{
			name: "endpoint credentials",
			mutate: func(cfg *Config) {
				cfg.Browser.Profiles = []BrowserProfileConfig{{
					Name: "remote", Mode: BrowserProfileRemoteCDP, CDPEndpoint: "https://user:secret@example.com",
				}}
				cfg.Browser.DefaultProfile = "remote"
			},
			message: `browser profile "remote": CDP endpoint must not contain inline credentials, query parameters, or fragments`,
		},
		{
			name: "missing remote endpoint",
			mutate: func(cfg *Config) {
				cfg.Browser.Profiles = []BrowserProfileConfig{{Name: "remote", Mode: BrowserProfileRemoteCDP}}
				cfg.Browser.DefaultProfile = "remote"
			},
			message: `browser profile "remote": CDP endpoint is required`,
		},
		{
			name: "invalid remote scheme",
			mutate: func(cfg *Config) {
				cfg.Browser.Profiles = []BrowserProfileConfig{{
					Name: "remote", Mode: BrowserProfileRemoteCDP, CDPEndpoint: "ftp://example.com",
				}}
				cfg.Browser.DefaultProfile = "remote"
			},
			message: `browser profile "remote": CDP endpoint scheme must be one of: http, https, ws, wss`,
		},
		{
			name: "ephemeral settings",
			mutate: func(cfg *Config) {
				cfg.Browser.Profiles[0].Directory = filepath.Join(root, "ephemeral")
			},
			message: `browser profile "default": managed ephemeral profile cannot set directory, CDP endpoint, or credential reference`,
		},
		{
			name: "persistent missing directory",
			mutate: func(cfg *Config) {
				cfg.Browser.Profiles[0].Mode = BrowserProfileManagedPersistent
			},
			message: `browser profile "default": managed persistent profile directory is required`,
		},
		{
			name: "remote directory",
			mutate: func(cfg *Config) {
				cfg.Browser.Profiles = []BrowserProfileConfig{{
					Name: "remote", Mode: BrowserProfileRemoteCDP,
					Directory: filepath.Join(root, "remote"), CDPEndpoint: "https://example.com",
				}}
				cfg.Browser.DefaultProfile = "remote"
			},
			message: `browser profile "remote": remote CDP profile cannot set a directory`,
		},
		{
			name: "remote credentials",
			mutate: func(cfg *Config) {
				cfg.Browser.Profiles = []BrowserProfileConfig{{
					Name: "remote", Mode: BrowserProfileRemoteCDP,
					CDPEndpoint: "https://example.com", CredentialRef: "BROWSER_CDP_TOKEN",
				}}
				cfg.Browser.DefaultProfile = "remote"
			},
			message: `browser profile "remote": CDP credential references require attachment support`,
		},
		{
			name: "duplicate profiles",
			mutate: func(cfg *Config) {
				cfg.Browser.Profiles = []BrowserProfileConfig{
					{Name: "same", Mode: BrowserProfileManagedEphemeral},
					{Name: "same", Mode: BrowserProfileManagedEphemeral},
				}
				cfg.Browser.DefaultProfile = "same"
			},
			message: "browser profile names must be unique",
		},
		{
			name: "invalid network",
			mutate: func(cfg *Config) {
				cfg.Browser.Network.DevelopmentAllowedCIDRs = []string{"not-a-cidr"}
			},
			message: "browser development allowed CIDR is invalid",
		},
		{
			name: "unknown default profile",
			mutate: func(cfg *Config) {
				cfg.Browser.DefaultProfile = "missing"
			},
			message: "browser default profile must reference a configured profile",
		},
		{
			name: "invalid profile mode",
			mutate: func(cfg *Config) {
				cfg.Browser.Profiles[0].Mode = "unknown"
			},
			message: `browser profile "default": mode must be one of: managed_ephemeral, managed_persistent, remote_cdp, existing_session`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cfg := valid()
			test.mutate(cfg)
			require.EqualError(t, cfg.ValidateRelaxed(), test.message)
		})
	}
}

func TestConfig_ValidateRejectsSymbolicLinkBrowserProfile(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "profiles")
	require.NoError(t, os.MkdirAll(root, 0o700))
	external := t.TempDir()
	link := filepath.Join(root, "linked")
	if err := os.Symlink(external, link); err != nil {
		t.Skip(err)
	}
	cfg := NewDefaultConfig()
	cfg.Normalize()
	cfg.Browser.ProfileRoot = root
	cfg.Browser.TemporaryRoot = filepath.Join(base, "tmp")
	cfg.Browser.Artifacts.Root = filepath.Join(base, "artifacts")
	cfg.Browser.Profiles = []BrowserProfileConfig{{
		Name: "linked", Mode: BrowserProfileManagedPersistent, Directory: link,
	}}
	cfg.Browser.DefaultProfile = "linked"

	require.EqualError(
		t,
		cfg.ValidateRelaxed(),
		`browser profile "linked": managed persistent profile directory must not traverse a symbolic link or junction`,
	)
}

func TestConfigClone_BrowserConfigurationDoesNotAlias(t *testing.T) {
	first := NewDefaultConfig()
	first.Browser.Network.DevelopmentAllowedHosts = []string{"localhost"}
	first.Browser.Profiles = append(first.Browser.Profiles, BrowserProfileConfig{Name: "other", Mode: BrowserProfileManagedEphemeral})
	second := cloneConfig(*first)

	first.Browser.Network.DevelopmentAllowedHosts[0] = "changed"
	first.Browser.Profiles[0].Name = "changed"
	*first.Browser.Network.Strict = false

	require.Equal(t, []string{"localhost"}, second.Browser.Network.DevelopmentAllowedHosts)
	require.Equal(t, DefaultBrowserProfileName, second.Browser.Profiles[0].Name)
	require.True(t, second.Browser.Network.StrictEnabled())
}
