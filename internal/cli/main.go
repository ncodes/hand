package cli

import (
	"context"
	"fmt"
	"io"
	"strings"

	urfavecli "github.com/urfave/cli/v3"

	"github.com/wandxy/morph/internal/config"
	"github.com/wandxy/morph/internal/constants"
	modelprovider "github.com/wandxy/morph/internal/model/provider"
	provider_ollama "github.com/wandxy/morph/internal/model/provider_ollama"
	rpcclient "github.com/wandxy/morph/internal/rpc/client"
	"github.com/wandxy/morph/internal/runtime"
	"github.com/wandxy/morph/pkg/logutils"
)

const (
	rootColorGray  = "\x1b[90m"
	rootColorReset = "\x1b[0m"
)

// NewChatClientFunc creates a chat client for CLI commands.
type NewChatClientFunc func(context.Context, *config.Config) (rpcclient.ChatClient, error)

// EnsureDaemonFunc ensures the daemon is reachable and returns cleanup for daemon instances it starts.
type EnsureDaemonFunc func(context.Context, *config.Config) (func() error, error)

// MainActionOptions controls main action.
type MainActionOptions struct {
	Output              io.Writer
	NewChatClient       NewChatClientFunc
	EnsureDaemonRunning EnsureDaemonFunc
	PullOllamaModel     func(context.Context, string, string, map[string]string, func(provider_ollama.PullProgress)) error
}

// NewMainAction returns the root CLI action wired to the supplied chat client factory.
func NewMainAction(opts MainActionOptions) func(context.Context, *urfavecli.Command) error {
	output := opts.Output
	if output == nil {
		output = io.Discard
	}

	newChatClient := opts.NewChatClient
	if newChatClient == nil {
		newChatClient = newDefaultChatClient
	}
	ensureDaemonRunning := opts.EnsureDaemonRunning
	if ensureDaemonRunning == nil {
		ensureDaemonRunning = EnsureDaemonRunning
	}
	pullOllamaModel := opts.PullOllamaModel
	if pullOllamaModel == nil {
		pullOllamaModel = provider_ollama.EnsureModel
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
		_ = logutils.ConfigureLogger("morph", cfg.Log.NoColor)
		logutils.SetLogLevel(cfg.Log.Level)

		if cmd.Bool("pull") {
			onProgress := pullProgressWriter(output, !cmd.Bool("pull-quiet"))
			if err := pullSelectedOllamaModel(ctx, cfg, pullOllamaModel, onProgress); err != nil {
				return err
			}
		}

		daemonCleanup, err := ensureDaemonRunning(ctx, cfg)
		if err != nil {
			return err
		}
		if daemonCleanup != nil {
			defer func() {
				_ = daemonCleanup()
			}()
		}

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

func pullSelectedOllamaModel(
	ctx context.Context,
	cfg *config.Config,
	pullOllamaModel func(context.Context, string, string, map[string]string, func(provider_ollama.PullProgress)) error,
	onProgress func(provider_ollama.PullProgress),
) error {
	if cfg == nil {
		return fmt.Errorf("config is required")
	}
	if pullOllamaModel == nil {
		return fmt.Errorf("ollama puller is required")
	}

	cfg.Normalize()
	if cfg.Models.Main.Provider != constants.ModelProviderOllama {
		return fmt.Errorf("--pull is only supported with provider %q", constants.ModelProviderOllama)
	}
	api := cfg.MainModelAPIEffective()
	if api != modelprovider.APIOllamaNative && api != modelprovider.APIOpenAICompletions {
		return fmt.Errorf("--pull is only supported with Ollama chat APIs")
	}

	auth, err := cfg.ResolveModelAuth()
	if err != nil {
		return err
	}

	return pullOllamaModel(ctx, auth.BaseURL, cfg.Models.Main.Name, auth.Headers, onProgress)
}

func pullProgressWriter(output io.Writer, enabled bool) func(provider_ollama.PullProgress) {
	if !enabled || output == nil {
		return nil
	}

	return func(progress provider_ollama.PullProgress) {
		text := strings.TrimSpace(progress.Status)
		if text == "" {
			return
		}
		if progress.Total > 0 && progress.Completed >= 0 {
			percent := int((progress.Completed * 100) / progress.Total)
			_, _ = fmt.Fprintf(output, "Ollama pull: %s %d%%\n", text, percent)
			return
		}

		_, _ = fmt.Fprintf(output, "Ollama pull: %s\n", text)
	}
}

func newDefaultChatClient(ctx context.Context, cfg *config.Config) (rpcclient.ChatClient, error) {
	return rpcclient.NewClient(ctx, rpcclient.Options{
		Address: cfg.RPC.Address,
		Port:    cfg.RPC.Port,
	})
}

// FormatChatEvent formats one streamed chat event for terminal output.
func FormatChatEvent(cfg *config.Config, event rpcclient.Event) string {
	if event.TraceEvent != nil {
		return ""
	}
	if strings.TrimSpace(event.Channel) != "reasoning" || cfg == nil || cfg.Log.NoColor {
		return event.Text
	}

	return rootColorGray + event.Text + rootColorReset
}
