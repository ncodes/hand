package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	cli "github.com/urfave/cli/v3"

	authcmd "github.com/wandxy/hand/cmd/auth"
	daemoncmd "github.com/wandxy/hand/cmd/daemon"
	doctorcmd "github.com/wandxy/hand/cmd/doctor"
	gatewaycmd "github.com/wandxy/hand/cmd/gateway"
	configcmd "github.com/wandxy/hand/cmd/hand/configcmd"
	profilecmd "github.com/wandxy/hand/cmd/profile"
	sessioncmd "github.com/wandxy/hand/cmd/session"
	tracecmd "github.com/wandxy/hand/cmd/trace"
	tuicmd "github.com/wandxy/hand/cmd/tui"
	handcli "github.com/wandxy/hand/internal/cli"
	"github.com/wandxy/hand/internal/config"
	"github.com/wandxy/hand/internal/profile"
	"github.com/wandxy/hand/pkg/logutils"
)

var log = logutils.Module("hand")

var (
	envFile                     = ".env"
	configFile                  = "config.yaml"
	rootOutput        io.Writer = os.Stdout
	runRootTUI                  = tuicmd.Run
	newRootChatAction           = handcli.NewMainAction
)

const rootHelpTemplate = `HAND_NAME:
   {{template "helpNameTemplate" .}}

USAGE:
   {{if .UsageText}}{{wrap .UsageText 3}}{{else}}{{.FullName}} {{if .VisibleFlags}}[global options]{{end}}{{if .VisibleCommands}} [command [command options]]{{end}}{{if .ArgsUsage}} {{.ArgsUsage}}{{else}}{{if .Arguments}} [arguments...]{{end}}{{end}}{{end}}{{if .Version}}{{if not .HideVersion}}

VERSION:
   {{.Version}}{{end}}{{end}}{{if .Description}}

DESCRIPTION:
   {{template "descriptionTemplate" .}}{{end}}
{{- if len .Authors}}

AUTHOR{{template "authorsTemplate" .}}{{end}}{{if .VisibleCommands}}

COMMANDS:{{template "visibleCommandCategoryTemplate" .}}{{end}}{{if .VisibleFlagCategories}}

GLOBAL OPTIONS:{{template "visibleFlagCategoryTemplate" .}}{{else if .VisibleFlags}}

GLOBAL OPTIONS:{{template "visibleFlagTemplate" .}}{{end}}

EXAMPLES:
   Start the interactive terminal UI:
      hand
      hand --profile work

   Start the daemon:
      hand daemon start
      hand --profile work daemon start
      hand profile use work
      hand --config ./config.yaml --trace.enabled daemon start

   Chat with the agent:
      hand --chat "summarize the failing tests"
      hand -c --profile work "continue"
      hand --chat --session ses_abc123 --instruct "be brief" "continue from the last debugging step"
      HAND_PROFILE=work hand session list

   Start the trace viewer:
      hand trace view
      hand --config ./config.yaml trace view --listen 127.0.0.1:9090
{{if .Copyright}}

COPYRIGHT:
   {{template "copyrightTemplate" .}}{{end}}
`

func main() {
	logutils.InitLogger("hand")

	if err := configureProfileDefaults(os.Args); err != nil {
		log.Fatal().Err(err).Msg("Failed to resolve profile")
	}

	envFile = getEnvFile(os.Args)
	if err := config.PreloadEnvFile(envFile); err != nil {
		log.Fatal().Err(err).Msg("Failed to preload environment")
	}

	cmd := newCommand()
	if err := cmd.Run(context.Background(), os.Args); err != nil {
		if exitErr, ok := errors.AsType[cli.ExitCoder](err); ok {
			os.Exit(exitErr.ExitCode())
		}
		if doctorcmd.IsCheckFailed(err) {
			os.Exit(1)
		}
		log.Fatal().Err(err).Msg("Failed to run")
	}
}

func newCommand() *cli.Command {
	var cmd *cli.Command
	cmd = &cli.Command{
		Name:                          "hand",
		Usage:                         "Run and manage your Hand daemon",
		Description:                   handcli.AppDescription,
		Version:                       formatRootVersion(),
		CustomRootCommandHelpTemplate: rootHelpTemplate,
		Flags: append(
			handcli.RootFlags(&envFile, &configFile),
			handcli.ChatFlag(),
			handcli.RequestInstructFlag()),
		Commands: []*cli.Command{
			authcmd.NewCommand(),
			newDatabaseCommand(),
			newVersionCommand(rootOutput),
			configcmd.NewCommand(rootOutput),
			doctorcmd.NewCommand(),
			gatewaycmd.NewCommand(),
			profilecmd.NewCommand(),
			sessioncmd.NewCommand(),
			tracecmd.NewCommand(),
			daemoncmd.NewCommand(),
		},
		Action: newRootAction(),
	}

	return cmd
}

func newRootAction() func(context.Context, *cli.Command) error {
	chatAction := newRootChatAction(handcli.MainActionOptions{
		Output: rootOutput,
	})

	return func(ctx context.Context, cmd *cli.Command) error {
		if cmd.Bool("chat") {
			return chatAction(ctx, cmd)
		}
		if cmd.Args().Len() > 0 {
			return fmt.Errorf("unexpected root arguments %q; use --chat or -c to send a one-shot chat request",
				strings.Join(cmd.Args().Slice(), " "))
		}

		return runRootTUI(ctx, cmd)
	}
}

func getEnvFile(args []string) string {
	if value := strings.TrimSpace(os.Getenv("HAND_ENV_FILE")); value != "" {
		return value
	}

	for i := range args {
		arg := strings.TrimSpace(args[i])
		if arg == "--env-file" && i+1 < len(args) {
			return strings.TrimSpace(args[i+1])
		}
		if value, ok := strings.CutPrefix(arg, "--env-file="); ok {
			return strings.TrimSpace(value)
		}
	}

	return envFile
}

func configureProfileDefaults(args []string) error {
	resolved, err := profile.Resolve(profile.ResolveOptions{Name: getProfileArg(args)})
	if err != nil {
		return err
	}

	profile.SetActive(resolved)
	envFile = resolved.EnvPath
	configFile = resolved.ConfigPath
	return nil
}

func getProfileArg(args []string) string {
	for i := range args {
		arg := strings.TrimSpace(args[i])
		if arg == "--" {
			return ""
		}
		if (arg == "--profile" || arg == "-p") && i+1 < len(args) {
			return strings.TrimSpace(args[i+1])
		}
		if value, ok := strings.CutPrefix(arg, "--profile="); ok {
			return strings.TrimSpace(value)
		}
		if value, ok := strings.CutPrefix(arg, "-p="); ok {
			return strings.TrimSpace(value)
		}
	}

	return ""
}
