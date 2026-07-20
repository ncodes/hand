package browser

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	browserdomain "github.com/wandxy/morph/internal/browser"
	"github.com/wandxy/morph/internal/config"
	"github.com/wandxy/morph/internal/permissions"
	"github.com/wandxy/morph/internal/profile"
	rpcclient "github.com/wandxy/morph/internal/rpc/client"
)

func TestNewCommand_ExposesBrowserOperatorCommands(t *testing.T) {
	command := NewCommand()
	names := make([]string, 0, len(command.Commands))
	for _, child := range command.Commands {
		names = append(names, child.Name)
	}
	require.ElementsMatch(t, []string{"status", "profiles", "sessions", "start", "stop", "config", "auth"}, names)
}

func TestBrowserAuthRotate_ReplacesCredentialAndRequiresRestart(t *testing.T) {
	originalProfile := profile.Active()
	originalRotate := rotateOwnerCredential
	originalOutput := browserOutput
	t.Cleanup(func() {
		profile.SetActive(originalProfile)
		rotateOwnerCredential = originalRotate
		browserOutput = originalOutput
	})
	home := t.TempDir()
	profile.SetActive(profile.Profile{Name: "default", HomeDir: home})
	rotatedHome := ""
	rotateOwnerCredential = func(profileHome string) ([]byte, error) {
		rotatedHome = profileHome
		return []byte("rotated"), nil
	}
	output := &bytes.Buffer{}
	browserOutput = output

	require.NoError(t, NewCommand().Run(context.Background(), []string{"browser", "auth", "rotate"}))
	require.Equal(t, home, rotatedHome)
	require.Contains(t, output.String(), "restart the daemon")
	require.Contains(t, output.String(), "reapprove browser attachments")
}

func TestBrowserCommands_RenderStatusProfilesSessionsAndConfig(t *testing.T) {
	now := time.Date(2026, 7, 19, 15, 0, 0, 0, time.Local)
	api := &browserCommandAPI{
		status: browserdomain.Status{
			Enabled: true,
			Profiles: []browserdomain.Profile{{
				Name: "default", Mode: "managed_ephemeral", Default: true, Available: true, Warning: "profile warning",
			}},
			Sessions: []browserdomain.Session{{
				ID: "browser_1", Profile: "default", State: browserdomain.SessionReady,
				LastActive: now, Warning: "session warning",
			}},
		},
		config: rpcclient.BrowserEffectiveConfig{
			Enabled: true, CapabilityEnabled: true, DefaultProfile: "default", NetworkStrict: true,
			PermissionPreset: permissions.PresetApproveForMe, ExecutableConfigured: true,
		},
	}
	configureBrowserCommandTest(t, api)

	for _, test := range []struct {
		args []string
		want string
	}{
		{args: []string{"browser", "status"}, want: "enabled: true\nprofiles: 1\nsessions: 1"},
		{args: []string{"browser", "profiles"}, want: "profile warning"},
		{args: []string{"browser", "sessions"}, want: "session warning"},
		{args: []string{"browser", "config"}, want: "permission preset: approve"},
	} {
		output := &bytes.Buffer{}
		previous := SetOutput(output)
		require.NoError(t, NewCommand().Run(context.Background(), test.args))
		SetOutput(previous)
		require.Contains(t, output.String(), test.want)
	}
}

func TestBrowserCommands_StartStopAndJSONOutput(t *testing.T) {
	api := &browserCommandAPI{
		status: browserdomain.Status{Enabled: true},
		start: browserdomain.Session{
			ID: "browser_1", State: browserdomain.SessionReady, Warning: "personal profile warning",
		},
		stop: browserdomain.Session{ID: "browser_1", State: browserdomain.SessionStopped},
	}
	configureBrowserCommandTest(t, api)
	output := &bytes.Buffer{}
	previous := SetOutput(output)
	t.Cleanup(func() { SetOutput(previous) })

	require.NoError(t, NewCommand().Run(context.Background(), []string{
		"browser", "start", "default", "--owner-session", "main",
	}))
	require.Equal(t, "default", api.startProfile)
	require.Equal(t, "main", api.startOwnerSession)
	require.Contains(t, output.String(), "browser_1")
	require.Contains(t, output.String(), "WARNING: personal profile warning")

	output.Reset()
	require.NoError(t, NewCommand().Run(context.Background(), []string{
		"browser", "stop", "browser_1", "--owner-session", "main",
	}))
	require.Equal(t, "browser_1", api.stopID)
	require.Equal(t, "main", api.stopOwnerSession)
	require.Contains(t, output.String(), "browser_1 stopped")

	output.Reset()
	require.NoError(t, NewCommand().Run(context.Background(), []string{"browser", "status", "--json"}))
	require.JSONEq(t, `{"enabled":true,"profiles":null,"sessions":null}`, output.String())
}

func TestBrowserCommands_ValidateInputAndPropagateFailures(t *testing.T) {
	api := &browserCommandAPI{err: errors.New("browser RPC failed")}
	configureBrowserCommandTest(t, api)

	err := NewCommand().Run(context.Background(), []string{"browser", "stop"})
	require.EqualError(t, err, "browser session id is required")
	err = NewCommand().Run(context.Background(), []string{"browser", "status"})
	require.EqualError(t, err, "browser RPC failed")
}

func TestSetOutput_UsesDiscardForNilWriter(t *testing.T) {
	previous := SetOutput(nil)
	require.NotNil(t, previous)
	SetOutput(previous)
}

type browserCommandClient struct {
	api    rpcclient.BrowserAPI
	closed bool
}

func (c *browserCommandClient) BrowserAPI() rpcclient.BrowserAPI {
	return c.api
}

func (c *browserCommandClient) Close() error {
	c.closed = true
	return nil
}

type browserCommandAPI struct {
	status            browserdomain.Status
	config            rpcclient.BrowserEffectiveConfig
	start             browserdomain.Session
	stop              browserdomain.Session
	err               error
	startProfile      string
	startOwnerSession string
	stopID            string
	stopOwnerSession  string
}

func (a *browserCommandAPI) Status(context.Context) (browserdomain.Status, error) {
	return a.status, a.err
}

func (a *browserCommandAPI) Profiles(context.Context) ([]browserdomain.Profile, error) {
	return a.status.Profiles, a.err
}

func (a *browserCommandAPI) Sessions(context.Context) ([]browserdomain.Session, error) {
	return a.status.Sessions, a.err
}

func (a *browserCommandAPI) Start(
	_ context.Context,
	profileName string,
	ownerSession string,
) (browserdomain.Session, error) {
	a.startProfile = profileName
	a.startOwnerSession = ownerSession
	return a.start, a.err
}

func (a *browserCommandAPI) Stop(
	_ context.Context,
	id string,
	ownerSession string,
) (browserdomain.Session, error) {
	a.stopID = id
	a.stopOwnerSession = ownerSession
	return a.stop, a.err
}

func (a *browserCommandAPI) ReadArtifact(
	context.Context,
	string,
	string,
	string,
) (browserdomain.ArtifactContent, error) {
	return browserdomain.ArtifactContent{}, a.err
}

func (a *browserCommandAPI) EffectiveConfig(context.Context) (rpcclient.BrowserEffectiveConfig, error) {
	return a.config, a.err
}

func configureBrowserCommandTest(t *testing.T, api rpcclient.BrowserAPI) {
	t.Helper()
	originalProfile := profile.Active()
	originalClient := newClient
	originalOutput := browserOutput
	t.Cleanup(func() {
		profile.SetActive(originalProfile)
		newClient = originalClient
		browserOutput = originalOutput
	})
	home := t.TempDir()
	cfg := config.NewDefaultConfig()
	configPath := filepath.Join(home, "config.yaml")
	require.NoError(t, config.SaveYAML(configPath, cfg))
	profile.SetActive(profile.WithMetadataPaths(profile.Profile{Name: "test", HomeDir: home, ConfigPath: configPath}))
	newClient = func(context.Context, *config.Config) (browserClient, error) {
		return &browserCommandClient{api: api}, nil
	}
}
