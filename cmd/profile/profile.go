package profilecmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	cli "github.com/urfave/cli/v3"

	"github.com/wandxy/hand/internal/config"
	"github.com/wandxy/hand/internal/profile"
)

var profileOutput io.Writer = os.Stdout

func SetOutput(w io.Writer) io.Writer {
	previous := profileOutput
	if w == nil {
		profileOutput = io.Discard
		return previous
	}
	profileOutput = w
	return previous
}

func NewCommand() *cli.Command {
	return &cli.Command{
		Name:  "profile",
		Usage: "Manage Hand profiles",
		Commands: []*cli.Command{
			newUseCommand(),
			newListCommand(),
			newCurrentCommand(),
			newInitCommand(),
			newPathCommand(),
			newDoctorCommand(),
		},
		Action: func(_ context.Context, cmd *cli.Command) error {
			return cli.ShowSubcommandHelp(cmd)
		},
	}
}

func newUseCommand() *cli.Command {
	return &cli.Command{
		Name:      "use",
		Usage:     "Set the machine-local current profile",
		ArgsUsage: "<name>",
		Action: func(_ context.Context, cmd *cli.Command) error {
			name := strings.TrimSpace(cmd.Args().First())
			if name == "" {
				return fmt.Errorf("profile name is required")
			}

			resolved, err := profile.Resolve(profile.ResolveOptions{Name: name})
			if err != nil {
				return err
			}
			if !pathExists(resolved.HomeDir) {
				return fmt.Errorf("profile %q does not exist; run `hand profile init %s` first", resolved.Name, resolved.Name)
			}

			name, err = profile.StoreCurrentName(resolved.Name, "")
			if err != nil {
				return err
			}

			_, err = fmt.Fprintln(profileOutput, name)
			return err
		},
	}
}

func newListCommand() *cli.Command {
	return &cli.Command{
		Name:  "list",
		Usage: "List existing profile directories",
		Action: func(_ context.Context, _ *cli.Command) error {
			names, err := profile.List("")
			if err != nil {
				return err
			}

			for _, name := range names {
				if _, err := fmt.Fprintln(profileOutput, name); err != nil {
					return err
				}
			}

			return nil
		},
	}
}

func newCurrentCommand() *cli.Command {
	return &cli.Command{
		Name:  "current",
		Usage: "Print the stored current profile",
		Action: func(_ context.Context, _ *cli.Command) error {
			name, ok, err := profile.LoadCurrentName("")
			if err != nil {
				return err
			}
			if !ok {
				name = profile.DefaultName
			}

			_, err = fmt.Fprintln(profileOutput, name)
			return err
		},
	}
}

func newInitCommand() *cli.Command {
	return &cli.Command{
		Name:      "init",
		Usage:     "Create a profile directory",
		ArgsUsage: "<name>",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:  "bare",
				Usage: "Create only the profile directory without config.yaml",
			},
		},
		Action: func(_ context.Context, cmd *cli.Command) error {
			name := strings.TrimSpace(cmd.Args().First())
			if name == "" {
				return fmt.Errorf("profile name is required")
			}

			resolved, err := profile.Init(name, "")
			if err != nil {
				return err
			}
			if !cmd.Bool("bare") {
				cfg := config.NewDefaultConfig()
				cfg.Name = resolved.Name
				if err := config.SaveYAML(resolved.ConfigPath, cfg); err != nil {
					return err
				}
			}

			_, err = fmt.Fprintln(profileOutput, resolved.HomeDir)
			return err
		},
	}
}

func newPathCommand() *cli.Command {
	return &cli.Command{
		Name:      "path",
		Usage:     "Print a profile home path",
		ArgsUsage: "[name]",
		Action: func(_ context.Context, cmd *cli.Command) error {
			resolved, err := loadCommandProfile(cmd.Args().First())
			if err != nil {
				return err
			}

			_, err = fmt.Fprintln(profileOutput, resolved.HomeDir)
			return err
		},
	}
}

func newDoctorCommand() *cli.Command {
	return &cli.Command{
		Name:      "doctor",
		Usage:     "Print profile paths and file status",
		ArgsUsage: "[name]",
		Action: func(_ context.Context, cmd *cli.Command) error {
			resolved, err := loadCommandProfile(cmd.Args().First())
			if err != nil {
				return err
			}

			lines := []string{
				"name=" + resolved.Name,
				"home=" + resolved.HomeDir,
				"config=" + resolved.ConfigPath,
				"env=" + resolved.EnvPath,
				"runtime=" + resolved.RuntimePath,
				"pid=" + resolved.PIDPath,
				fmt.Sprintf("home_exists=%t", pathExists(resolved.HomeDir)),
				fmt.Sprintf("config_exists=%t", pathExists(resolved.ConfigPath)),
				fmt.Sprintf("env_exists=%t", pathExists(resolved.EnvPath)),
				fmt.Sprintf("runtime_exists=%t", pathExists(resolved.RuntimePath)),
			}

			for _, line := range lines {
				if _, err := fmt.Fprintln(profileOutput, line); err != nil {
					return err
				}
			}

			return nil
		},
	}
}

func loadCommandProfile(name string) (profile.Profile, error) {
	name = strings.TrimSpace(name)
	if name != "" {
		return profile.Resolve(profile.ResolveOptions{Name: name})
	}

	return loadActiveProfile()
}

func loadActiveProfile() (profile.Profile, error) {
	active := profile.WithMetadataPaths(profile.Active())
	if strings.TrimSpace(active.HomeDir) != "" {
		return active, nil
	}

	resolved, err := profile.Resolve(profile.ResolveOptions{})
	if err != nil {
		return profile.Profile{}, err
	}
	profile.SetActive(resolved)

	return resolved, nil
}

func pathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
