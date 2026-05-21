package main

import (
	"context"
	"errors"
	"io"
	"os"
	"strings"

	"github.com/rs/zerolog/log"
	cli "github.com/urfave/cli/v3"

	doctorcmd "github.com/wandxy/hand/cmd/doctor"
	setconfigcmd "github.com/wandxy/hand/cmd/hand/setconfig"
	profilecmd "github.com/wandxy/hand/cmd/profile"
	sessioncmd "github.com/wandxy/hand/cmd/session"
	tracecmd "github.com/wandxy/hand/cmd/trace"
	tuicmd "github.com/wandxy/hand/cmd/tui"
	upcmd "github.com/wandxy/hand/cmd/up"
	handcli "github.com/wandxy/hand/internal/cli"
	"github.com/wandxy/hand/internal/config"
	"github.com/wandxy/hand/internal/profile"
	"github.com/wandxy/hand/pkg/logutils"
)

func init() {
	_ = logutils.InitLogger("hand")
}

var (
	envFile              = ".env"
	configFile           = "config.yaml"
	rootOutput io.Writer = os.Stdout
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
   Start the agent runtime:
      hand up
      hand --profile work up
      hand profile use work
      hand --config ./config.yaml --trace.enabled up

   Chat with the agent:
      hand "summarize the failing tests"
      hand --profile work "continue"
      hand --session ses_abc123 --instruct "be brief" "continue from the last debugging step"
      HAND_PROFILE=work hand session list

   Start the trace viewer:
      hand trace view
      hand --config ./config.yaml trace view --listen 127.0.0.1:9090
{{if .Copyright}}

COPYRIGHT:
   {{template "copyrightTemplate" .}}{{end}}
`

func main() {
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
		log.Fatal().Err(err).Msg("Failed to run")
	}
}

func newCommand() *cli.Command {
	var cmd *cli.Command
	cmd = &cli.Command{
		Name:                          "hand",
		Usage:                         "Run and manage your Hand daemon",
		Description:                   handcli.AppDescription,
		CustomRootCommandHelpTemplate: rootHelpTemplate,
		Flags:                         append(handcli.RootFlags(&envFile, &configFile), handcli.RequestInstructFlag()),
		Commands: []*cli.Command{
			newDatabaseCommand(),
			doctorcmd.NewCommand(),
			profilecmd.NewCommand(),
			sessioncmd.NewCommand(),
			setconfigcmd.NewCommand(rootOutput),
			tracecmd.NewCommand(),
			tuicmd.NewCommand(),
			upcmd.NewCommand(),
		},
		Action: handcli.NewMainAction(handcli.MainActionOptions{
			Output: rootOutput,
		}),
	}

	return cmd
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
