package config

import "time"

const (
	BrowserProfileManagedEphemeral  = "managed_ephemeral"
	BrowserProfileManagedPersistent = "managed_persistent"
	BrowserProfileRemoteCDP         = "remote_cdp"
	BrowserProfileExistingSession   = "existing_session"

	DefaultBrowserProfileName = "default"
)

const (
	BrowserAttachmentTargets = "targets"
	BrowserAttachmentContext = "context"
	BrowserAttachmentBrowser = "browser"
)

const (
	defaultBrowserStartTimeout       = 15 * time.Second
	defaultBrowserInactivityTimeout  = 10 * time.Minute
	defaultBrowserCleanupInterval    = time.Minute
	defaultBrowserTerminalRetention  = 15 * time.Minute
	defaultBrowserArtifactRetention  = 24 * time.Hour
	defaultBrowserArtifactMaxBytes   = 25 << 20
	defaultBrowserArtifactTotalBytes = 250 << 20
)

type BrowserConfig struct {
	Enabled           bool                   `yaml:"enabled"`
	Executable        string                 `yaml:"executable"`
	DefaultProfile    string                 `yaml:"defaultProfile"`
	ProfileRoot       string                 `yaml:"profileRoot"`
	TemporaryRoot     string                 `yaml:"temporaryRoot"`
	StartTimeout      time.Duration          `yaml:"startTimeout"`
	InactivityTimeout time.Duration          `yaml:"inactivityTimeout"`
	CleanupInterval   time.Duration          `yaml:"cleanupInterval"`
	TerminalRetention time.Duration          `yaml:"terminalRetention"`
	Profiles          []BrowserProfileConfig `yaml:"profiles"`
	Network           BrowserNetworkConfig   `yaml:"network"`
	Artifacts         BrowserArtifactConfig  `yaml:"artifacts"`
}

type BrowserProfileConfig struct {
	Name             string   `yaml:"name"`
	Mode             string   `yaml:"mode"`
	Directory        string   `yaml:"directory"`
	CDPEndpoint      string   `yaml:"cdpEndpoint"`
	CredentialRef    string   `yaml:"credentialRef"`
	DataIdentity     string   `yaml:"dataIdentity"`
	AttachmentScope  string   `yaml:"attachmentScope"`
	BrowserContextID string   `yaml:"browserContextId"`
	TargetIDs        []string `yaml:"targetIds"`
}

type BrowserNetworkConfig struct {
	Strict                  *bool    `yaml:"strict"`
	DevelopmentAllowedHosts []string `yaml:"developmentAllowedHosts"`
	DevelopmentAllowedCIDRs []string `yaml:"developmentAllowedCIDRs"`
}

type BrowserArtifactConfig struct {
	Root          string        `yaml:"root"`
	MaxBytes      int64         `yaml:"maxBytes"`
	MaxTotalBytes int64         `yaml:"maxTotalBytes"`
	Retention     time.Duration `yaml:"retention"`
}

func (c BrowserConfig) Profile(name string) (BrowserProfileConfig, bool) {
	for _, profile := range c.Profiles {
		if profile.Name == name {
			return profile, true
		}
	}

	return BrowserProfileConfig{}, false
}

func (c BrowserNetworkConfig) StrictEnabled() bool {
	return c.Strict == nil || *c.Strict
}
