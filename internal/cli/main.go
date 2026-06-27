package cli

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/charmbracelet/x/term"
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
	rootColorGray         = "\x1b[90m"
	rootColorReset        = "\x1b[0m"
	pullProgressLineLimit = 5
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
	Now                 func() time.Time
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
	now := opts.Now
	if now == nil {
		now = time.Now
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
			progressPrinter := newPullProgressPrinter(output, !cmd.Bool("pull-quiet"))
			var onProgress func(provider_ollama.PullProgress)
			if progressPrinter != nil {
				onProgress = progressPrinter.Progress
			}
			if err := pullSelectedOllamaModel(ctx, cfg, pullOllamaModel, onProgress); err != nil {
				if progressPrinter != nil {
					progressPrinter.Finish()
				}
				return err
			}
			if progressPrinter != nil {
				progressPrinter.Finish()
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
			formatter := newChatStreamFormatter(cfg, now, cmd.Bool("no-color"))
			responseOptions.OnEvent = func(event rpcclient.Event) {
				_, _ = fmt.Fprint(output, formatter.Format(event))
			}

			_, err = client.Respond(ctx, message, responseOptions)
			if err != nil {
				return err
			}
			_, err = fmt.Fprint(output, formatter.Finish())
			return err
		}

		reply, err := client.Respond(ctx, message, responseOptions)
		if err != nil {
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

type pullProgressPrinter struct {
	output   io.Writer
	live     bool
	lines    []string
	rendered int
}

func newPullProgressPrinter(output io.Writer, enabled bool) *pullProgressPrinter {
	if !enabled || output == nil {
		return nil
	}

	return &pullProgressPrinter{
		output: output,
		live:   isTerminalWriter(output),
	}
}

func (p *pullProgressPrinter) Progress(progress provider_ollama.PullProgress) {
	if p == nil {
		return
	}

	line := formatPullProgress(progress)
	if line == "" {
		return
	}
	if len(p.lines) > 0 && p.lines[len(p.lines)-1] == line {
		return
	}

	p.lines = append(p.lines, line)
	if len(p.lines) > pullProgressLineLimit {
		p.lines = p.lines[len(p.lines)-pullProgressLineLimit:]
	}
	if p.live {
		p.renderLive()
	}
}

func (p *pullProgressPrinter) Finish() {
	if p == nil || p.live {
		return
	}

	for _, line := range p.lines {
		_, _ = fmt.Fprintln(p.output, line)
	}
}

func (p *pullProgressPrinter) renderLive() {
	if p.rendered > 0 {
		_, _ = fmt.Fprintf(p.output, "\x1b[%dF", p.rendered)
	}

	rows := max(p.rendered, len(p.lines))
	for i := 0; i < rows; i++ {
		_, _ = fmt.Fprint(p.output, "\r\x1b[2K")
		if i < len(p.lines) {
			_, _ = fmt.Fprint(p.output, p.lines[i])
		}
		_, _ = fmt.Fprintln(p.output)
	}
	p.rendered = len(p.lines)
}

func formatPullProgress(progress provider_ollama.PullProgress) string {
	text := strings.TrimSpace(progress.Status)
	if text == "" {
		return ""
	}
	if progress.Total > 0 && progress.Completed >= 0 {
		percent := int((progress.Completed * 100) / progress.Total)
		return fmt.Sprintf("Ollama pull: %s %d%%", text, percent)
	}

	return fmt.Sprintf("Ollama pull: %s", text)
}

type fdWriter interface {
	Fd() uintptr
}

func isTerminalWriter(output io.Writer) bool {
	if writer, ok := output.(fdWriter); ok {
		return term.IsTerminal(writer.Fd())
	}

	return false
}

func newDefaultChatClient(ctx context.Context, cfg *config.Config) (rpcclient.ChatClient, error) {
	return rpcclient.NewClient(ctx, rpcclient.Options{
		Address: cfg.RPC.Address,
		Port:    cfg.RPC.Port,
	})
}

type chatStreamFormatter struct {
	cfg                      *config.Config
	now                      func() time.Time
	noColor                  bool
	turnStarted              time.Time
	reasoningStarted         time.Time
	reasoningActive          bool
	lastChannel              string
	wroteText                bool
	lastTextTrailingNewlines int
}

func newChatStreamFormatter(cfg *config.Config, now func() time.Time, noColor bool) *chatStreamFormatter {
	clock := now
	if clock == nil {
		clock = time.Now
	}
	return &chatStreamFormatter{
		cfg:         cfg,
		now:         clock,
		noColor:     noColor,
		turnStarted: clock(),
	}
}

func (f *chatStreamFormatter) Format(event rpcclient.Event) string {
	if event.TraceEvent != nil {
		return ""
	}

	channel := strings.TrimSpace(event.Channel)
	if channel == "reasoning" && !f.reasoningActive {
		f.reasoningStarted = f.now()
		f.reasoningActive = true
	}

	output := formatChatEvent(f.cfg, event, f.noColor)
	if output == "" {
		return ""
	}

	prefix := ""
	if f.wroteText && f.lastChannel == "reasoning" && channel != "reasoning" {
		prefix = f.finishReasoning()
	}

	f.wroteText = true
	f.lastChannel = channel
	f.lastTextTrailingNewlines = countTrailingNewlines(event.Text, 2)

	return prefix + output
}

func (f *chatStreamFormatter) Finish() string {
	if f == nil {
		return ""
	}

	output := ""
	if f.reasoningActive {
		output += f.finishReasoning()
	}
	if !strings.HasSuffix(output, "\n\n") {
		output += strings.Repeat("\n", max(0, 2-f.lastTextTrailingNewlines))
	}
	output += f.formatMutedLabel("Worked for " + formatElapsed(f.now().Sub(f.turnStarted)))
	output += "\n"

	return output
}

func (f *chatStreamFormatter) finishReasoning() string {
	f.reasoningActive = false

	output := strings.Repeat("\n", max(0, 2-f.lastTextTrailingNewlines))
	output += f.formatMutedLabel("Thought for " + formatElapsed(f.now().Sub(f.reasoningStarted)))
	output += "\n\n"

	return output
}

func (f *chatStreamFormatter) formatMutedLabel(label string) string {
	if f == nil || f.noColor {
		return label
	}

	return rootColorGray + label + rootColorReset
}

func formatElapsed(duration time.Duration) string {
	duration = duration.Round(time.Second)
	if duration < time.Second {
		return "0s"
	}

	if duration < time.Minute {
		return fmt.Sprintf("%ds", int(duration/time.Second))
	}

	minutes := int(duration / time.Minute)
	seconds := int((duration % time.Minute) / time.Second)
	if seconds == 0 {
		return fmt.Sprintf("%dm", minutes)
	}

	return fmt.Sprintf("%dm%ds", minutes, seconds)
}

func countTrailingNewlines(text string, limit int) int {
	count := 0
	for i := len(text) - 1; i >= 0 && count < limit; i-- {
		if text[i] != '\n' {
			break
		}
		count++
	}

	return count
}

// FormatChatEvent formats one streamed chat event for terminal output.
func FormatChatEvent(cfg *config.Config, event rpcclient.Event) string {
	return formatChatEvent(cfg, event, false)
}

func formatChatEvent(cfg *config.Config, event rpcclient.Event, noColor bool) string {
	if event.TraceEvent != nil {
		return ""
	}
	if strings.TrimSpace(event.Channel) != "reasoning" || cfg == nil || noColor {
		return event.Text
	}

	return rootColorGray + event.Text + rootColorReset
}
