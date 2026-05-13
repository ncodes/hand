package cli

import (
	"strings"

	cli "github.com/urfave/cli/v3"

	"github.com/wandxy/hand/internal/config"
	"github.com/wandxy/hand/internal/profile"
)

// ConfigInputs are the resolved profile-aware config inputs for a command.
type ConfigInputs struct {
	Profile    profile.Profile
	EnvPath    string
	ConfigPath string
}

// ResolveConfigInputs resolves the active profile before config and env loading.
func ResolveConfigInputs(cmd *cli.Command) (ConfigInputs, error) {
	profileName := getCommandProfile(cmd)
	resolved := profile.Active()
	if profileName != "" || strings.TrimSpace(resolved.HomeDir) == "" {
		var err error
		resolved, err = profile.Resolve(profile.ResolveOptions{Name: profileName})
		if err != nil {
			return ConfigInputs{}, err
		}
	}

	resolved = profile.WithMetadataPaths(resolved)
	profile.SetActive(resolved)
	inputs := ConfigInputs{
		Profile:    resolved,
		EnvPath:    resolved.EnvPath,
		ConfigPath: resolved.ConfigPath,
	}
	if cmd != nil {
		if cmd.IsSet("env-file") {
			inputs.EnvPath = strings.TrimSpace(cmd.String("env-file"))
		}
		if cmd.IsSet("config") {
			inputs.ConfigPath = strings.TrimSpace(cmd.String("config"))
		}
	}

	return inputs, nil
}

// LoadConfig loads config from the active profile unless command flags override the paths.
func LoadConfig(cmd *cli.Command) (*config.Config, ConfigInputs, error) {
	inputs, err := ResolveConfigInputs(cmd)
	if err != nil {
		return nil, ConfigInputs{}, err
	}

	cfg, err := config.Load(inputs.EnvPath, inputs.ConfigPath)
	if err != nil {
		return nil, inputs, err
	}

	return cfg, inputs, nil
}

func getCommandProfile(cmd *cli.Command) string {
	if cmd == nil || !cmd.IsSet("profile") {
		return ""
	}

	return strings.TrimSpace(cmd.String("profile"))
}
