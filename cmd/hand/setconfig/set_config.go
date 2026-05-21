package setconfig

import (
	"context"
	"fmt"
	"io"
	"strings"

	cli "github.com/urfave/cli/v3"

	handcli "github.com/wandxy/hand/internal/cli"
	"github.com/wandxy/hand/internal/config"
)

func NewCommand(output io.Writer) *cli.Command {
	if output == nil {
		output = io.Discard
	}

	return &cli.Command{
		Name:      "set-config",
		Usage:     "Set a value in the selected profile config file",
		ArgsUsage: "<path> <value>|<path=value>...",
		Flags:     []cli.Flag{handcli.ProfileFlag()},
		Action: func(_ context.Context, cmd *cli.Command) error {
			updates, err := getSetConfigUpdates(cmd)
			if err != nil {
				return err
			}

			inputs, err := handcli.ResolveConfigInputs(cmd)
			if err != nil {
				return err
			}
			if err := config.PreloadEnvFile(inputs.EnvPath); err != nil {
				return err
			}

			updatedPaths, err := handcli.SetConfigValues(inputs.EnvPath, inputs.ConfigPath, updates)
			if err != nil {
				return err
			}

			for index, updatedPath := range updatedPaths {
				if _, err := fmt.Fprintf(output, "%s=%s\n", updatedPath, updates[index].Value); err != nil {
					return err
				}
			}
			return nil
		},
	}
}

func getSetConfigUpdates(cmd *cli.Command) ([]handcli.ConfigUpdate, error) {
	if cmd == nil {
		return nil, fmt.Errorf("config path and value are required")
	}

	args := cmd.Args().Slice()
	updates := make([]handcli.ConfigUpdate, 0, len(args))
	for index := 0; index < len(args); index++ {
		raw := strings.TrimSpace(args[index])
		if raw == "" {
			return nil, fmt.Errorf("config path and value are required")
		}

		path, value, ok := strings.Cut(raw, "=")
		if ok {
			path = strings.TrimSpace(path)
			if path == "" {
				return nil, fmt.Errorf("config path and value are required")
			}
			updates = append(updates, handcli.ConfigUpdate{Path: path, Value: strings.TrimSpace(value)})
			continue
		}

		if index+1 >= len(args) {
			return nil, fmt.Errorf("config path and value are required")
		}
		path = strings.TrimSpace(raw)
		if path == "" {
			return nil, fmt.Errorf("config path and value are required")
		}
		updates = append(updates, handcli.ConfigUpdate{Path: path, Value: strings.TrimSpace(args[index+1])})
		index++
	}

	if len(updates) == 0 {
		return nil, fmt.Errorf("config path and value are required")
	}

	return updates, nil
}
