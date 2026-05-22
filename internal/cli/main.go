package cli

import (
	"context"
	"fmt"
	"io"
	"strings"

	urfavecli "github.com/urfave/cli/v3"

	"github.com/wandxy/hand/internal/config"
	rpcclient "github.com/wandxy/hand/internal/rpc/client"
	"github.com/wandxy/hand/internal/runtime"
	"github.com/wandxy/hand/pkg/logutils"
)

const (
	rootColorGray  = "\x1b[90m"
	rootColorReset = "\x1b[0m"
)

type NewChatClientFunc func(context.Context, *config.Config) (rpcclient.ChatClient, error)

type MainActionOptions struct {
	Output        io.Writer
	NewChatClient NewChatClientFunc
}

func NewMainAction(opts MainActionOptions) func(context.Context, *urfavecli.Command) error {
	output := opts.Output
	if output == nil {
		output = io.Discard
	}

	newChatClient := opts.NewChatClient
	if newChatClient == nil {
		newChatClient = newDefaultChatClient
	}

	return func(ctx context.Context, cmd *urfavecli.Command) error {
		message := strings.TrimSpace(strings.Join(cmd.Args().Slice(), " "))
		if message == "" {
			return urfavecli.ShowAppHelp(cmd)
		}

		cfg, inputs, err := LoadConfig(cmd)
		if err != nil {
			return err
		}

		ApplyConfigOverrides(cmd, cfg)
		AddStartupFilesystemRoots(cfg, inputs)

		endpoint, err := runtime.ResolveRPC(ctx, cmd, cfg)
		if err != nil {
			return err
		}
		cfg.RPC = endpoint

		config.Set(cfg)
		_ = logutils.ConfigureLogger("hand", cfg.Log.NoColor)
		logutils.SetLogLevel(cfg.Log.Level)

		client, err := newChatClient(ctx, cfg)
		if err != nil {
			return err
		}
		defer client.Close()

		instruct := ""
		if cmd.IsSet("instruct") {
			instruct = cfg.Session.Instruct
		}

		responseOptions := rpcclient.RespondOptions{
			Instruct:  instruct,
			SessionID: strings.TrimSpace(cmd.String("session")),
			Stream:    cfg.Models.Main.Stream,
		}
		if cfg.StreamEnabled() {
			responseOptions.OnEvent = func(event rpcclient.Event) {
				_, _ = fmt.Fprint(output, FormatChatEvent(cfg, event))
			}
		}

		reply, err := client.Respond(ctx, message, responseOptions)
		if err != nil {
			return err
		}

		if cfg.StreamEnabled() {
			_, err = fmt.Fprintln(output)
			return err
		}

		_, err = fmt.Fprintln(output, reply)
		return err
	}
}

func newDefaultChatClient(ctx context.Context, cfg *config.Config) (rpcclient.ChatClient, error) {
	return rpcclient.NewClient(ctx, rpcclient.Options{
		Address: cfg.RPC.Address,
		Port:    cfg.RPC.Port,
	})
}

func FormatChatEvent(cfg *config.Config, event rpcclient.Event) string {
	if event.TraceEvent != nil {
		return ""
	}
	if strings.TrimSpace(event.Channel) != "reasoning" || cfg == nil || cfg.Log.NoColor {
		return event.Text
	}

	return rootColorGray + event.Text + rootColorReset
}
