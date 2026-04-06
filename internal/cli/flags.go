package cli

import (
	"strings"

	cli "github.com/urfave/cli/v3"

	"github.com/wandxy/hand/internal/config"
)

const AppDescription = "Hand is a personal assistant that works and exists for you."

func RootFlags(envFile, configFile *string) []cli.Flag {
	flags := []cli.Flag{
		&cli.StringFlag{
			Name:  "name",
			Usage: "The name of your hand",
			Value: config.Get().Name,
		},
		&cli.StringFlag{
			Name:   "model.provider",
			Usage:  "Model provider: openrouter (default) or openai",
			Value:  config.Get().ModelProvider,
			Hidden: true,
		},
		&cli.StringFlag{
			Name:   "model.key",
			Usage:  "Authentication key for the selected model provider",
			Hidden: true,
		},
		&cli.StringFlag{
			Name:  "model",
			Usage: "Model slug to send to the provider, for example openai/gpt-4o-mini",
			Value: config.Get().Model,
		},
		&cli.StringFlag{
			Name:  "model.summary",
			Usage: "Optional model slug for compaction summarization; defaults to --model when unset",
			Value: config.Get().SummaryModel,
		},
		&cli.BoolFlag{
			Name:  "model.stream",
			Usage: "Stream assistant text responses as they are generated",
			Value: config.Get().StreamEnabled(),
		},
		&cli.StringFlag{
			Name:   "model.base-url",
			Usage:  "Base URL for the model provider API",
			Value:  config.Get().ModelBaseURL,
			Hidden: true,
		},
		&cli.StringFlag{
			Name:   "model.api-mode",
			Usage:  "Provider API mode: chat-completions or responses",
			Value:  config.Get().ModelAPIMode,
			Hidden: true,
		},
		&cli.BoolFlag{
			Name:   "model.verify-model",
			Usage:  "Verify model existence and clamp configured context length against provider metadata",
			Value:  config.Get().VerifyModelEnabled(),
			Hidden: true,
		},
		&cli.StringFlag{
			Name:   "rpc.address",
			Usage:  "Bind address for the RPC service",
			Value:  config.Get().RPCAddress,
			Hidden: true,
		},
		&cli.IntFlag{
			Name:   "rpc.port",
			Usage:  "Bind port for the RPC service",
			Value:  config.Get().RPCPort,
			Hidden: true,
		},
		&cli.IntFlag{
			Name:   "max-iterations",
			Usage:  "Maximum model iterations allowed in a tool-calling loop",
			Value:  config.Get().MaxIterations,
			Hidden: true,
		},
		&cli.StringFlag{
			Name:  "log.level",
			Usage: "Set the minimum log level: debug, info, warn, or error",
			Value: config.Get().LogLevel,
		},
		&cli.BoolFlag{
			Name:   "log.no-color",
			Usage:  "Emit plain log output without ANSI color codes",
			Hidden: true,
		},
		&cli.BoolFlag{
			Name:  "debug.requests",
			Usage: "Log sanitized model request payloads at debug level",
		},
		&cli.BoolFlag{
			Name:  "debug.traces",
			Usage: "Persist sanitized per-session trace events for debugging",
		},
		&cli.StringFlag{
			Name:   "debug.trace-dir",
			Usage:  "Directory for persisted debug trace files",
			Value:  config.Get().DebugTraceDir,
			Hidden: true,
		},
		&cli.StringFlag{
			Name:   "rules.files",
			Usage:  "Comma-separated rule file paths to load in addition to workspace defaults",
			Value:  strings.Join(config.Get().RulesFiles, ","),
			Hidden: true,
		},
		&cli.StringFlag{
			Name:   "platform",
			Usage:  "Active runtime platform used for tool filtering",
			Value:  config.Get().Platform,
			Hidden: true,
		},
		&cli.StringFlag{
			Name:   "fs.roots",
			Usage:  "Comma-separated filesystem roots allowed for file tools",
			Value:  strings.Join(config.Get().FSRoots, ","),
			Hidden: true,
		},
		&cli.BoolFlag{
			Name:   "cap.fs",
			Usage:  "Enable filesystem tool capability filtering",
			Value:  boolValue(config.Get().CapFilesystem),
			Hidden: true,
		},
		&cli.BoolFlag{
			Name:   "cap.net",
			Usage:  "Enable network tool capability filtering",
			Value:  boolValue(config.Get().CapNetwork),
			Hidden: true,
		},
		&cli.BoolFlag{
			Name:   "cap.exec",
			Usage:  "Enable exec tool capability filtering",
			Value:  boolValue(config.Get().CapExec),
			Hidden: true,
		},
		&cli.BoolFlag{
			Name:   "cap.mem",
			Usage:  "Enable memory tool capability filtering",
			Value:  boolValue(config.Get().CapMemory),
			Hidden: true,
		},
		&cli.BoolFlag{
			Name:   "cap.browser",
			Usage:  "Enable browser tool capability filtering",
			Value:  boolValue(config.Get().CapBrowser),
			Hidden: true,
		},
		&cli.StringFlag{
			Name:   "exec.allow",
			Usage:  "Comma-separated allowed command prefixes",
			Value:  strings.Join(config.Get().ExecAllow, ","),
			Hidden: true,
		},
		&cli.StringFlag{
			Name:   "exec.ask",
			Usage:  "Comma-separated command prefixes that require approval",
			Value:  strings.Join(config.Get().ExecAsk, ","),
			Hidden: true,
		},
		&cli.StringFlag{
			Name:   "exec.deny",
			Usage:  "Comma-separated denied command prefixes",
			Value:  strings.Join(config.Get().ExecDeny, ","),
			Hidden: true,
		},
		&cli.StringFlag{
			Name:   "storage.backend",
			Usage:  "Storage backend: memory or sqlite",
			Value:  config.Get().StorageBackend,
			Hidden: true,
		},
		&cli.DurationFlag{
			Name:   "session.default-idle-expiry",
			Usage:  "Idle duration before the default session is archived and cleared",
			Value:  config.Get().SessionDefaultIdleExpiry,
			Hidden: true,
		},
		&cli.DurationFlag{
			Name:   "session.archive-retention",
			Usage:  "How long archived default-session conversations are retained before deletion",
			Value:  config.Get().SessionArchiveRetention,
			Hidden: true,
		},
		&cli.StringFlag{
			Name:  "session",
			Usage: "Session id to use for this chat request; defaults to the persistent default session",
		},
	}

	if envFile != nil {
		flags = append([]cli.Flag{
			&cli.StringFlag{
				Name:        "env-file",
				Usage:       "Load environment overrides from this .env file",
				Value:       ".env",
				Destination: envFile,
				Sources: cli.NewValueSourceChain(
					cli.EnvVar("AGENT_ENV_FILE"),
				),
			},
		}, flags...)
	}

	if configFile != nil {
		insertAt := 1
		if envFile == nil {
			insertAt = 0
		}
		configFlag := &cli.StringFlag{
			Name:        "config",
			Aliases:     []string{"c"},
			Usage:       "Read base settings from this YAML config file",
			Value:       "config.yaml",
			Destination: configFile,
			Sources: cli.NewValueSourceChain(
				cli.EnvVar("AGENT_CONFIG"),
			),
		}
		flags = append(flags[:insertAt], append([]cli.Flag{configFlag}, flags[insertAt:]...)...)
	}

	return flags
}

func RequestInstructFlag() cli.Flag {
	return &cli.StringFlag{
		Name:  "instruct",
		Usage: "Per-request instruction appended after workspace rules and cleared when the response finishes",
		Value: config.Get().Instruct,
	}
}

func PersistentInstructFlag() cli.Flag {
	return &cli.StringFlag{
		Name:  "instruct",
		Usage: "Server instruction appended after workspace rules and kept until the process exits",
		Value: config.Get().Instruct,
	}
}

func ApplyConfigOverrides(cmd *cli.Command, cfg *config.Config) {
	if cfg == nil || cmd == nil {
		return
	}

	if cmd.IsSet("name") {
		cfg.Name = strings.TrimSpace(cmd.String("name"))
	}
	if cmd.IsSet("model") {
		cfg.Model = strings.TrimSpace(cmd.String("model"))
	}
	if cmd.IsSet("model.summary") {
		cfg.SummaryModel = strings.TrimSpace(cmd.String("model.summary"))
	}
	if cmd.IsSet("model.stream") {
		cfg.Stream = new(cmd.Bool("model.stream"))
	}
	if cmd.IsSet("model.provider") {
		cfg.ModelProvider = strings.TrimSpace(cmd.String("model.provider"))
	}
	if cmd.IsSet("model.key") {
		cfg.ModelKey = strings.TrimSpace(cmd.String("model.key"))
	}
	if cmd.IsSet("model.base-url") {
		cfg.ModelBaseURL = strings.TrimSpace(cmd.String("model.base-url"))
	}
	if cmd.IsSet("model.api-mode") {
		cfg.ModelAPIMode = strings.TrimSpace(cmd.String("model.api-mode"))
	}
	if cmd.IsSet("model.verify-model") {
		cfg.VerifyModel = new(cmd.Bool("model.verify-model"))
	}
	if cmd.IsSet("rpc.address") {
		cfg.RPCAddress = strings.TrimSpace(cmd.String("rpc.address"))
	}
	if cmd.IsSet("rpc.port") {
		cfg.RPCPort = cmd.Int("rpc.port")
	}
	if cmd.IsSet("max-iterations") {
		cfg.MaxIterations = cmd.Int("max-iterations")
	}
	if cmd.IsSet("log.level") {
		cfg.LogLevel = strings.TrimSpace(cmd.String("log.level"))
	}
	if cmd.IsSet("log.no-color") {
		cfg.LogNoColor = cmd.Bool("log.no-color")
	}
	if cmd.IsSet("debug.requests") {
		cfg.DebugRequests = cmd.Bool("debug.requests")
	}
	if cmd.IsSet("debug.traces") {
		cfg.DebugTraces = cmd.Bool("debug.traces")
	}
	if cmd.IsSet("debug.trace-dir") {
		cfg.DebugTraceDir = strings.TrimSpace(cmd.String("debug.trace-dir"))
	}
	if cmd.IsSet("rules.files") {
		cfg.RulesFiles = configSplitAndTrimCSV(cmd.String("rules.files"))
	}
	if cmd.IsSet("instruct") {
		cfg.Instruct = strings.TrimSpace(cmd.String("instruct"))
	}
	if cmd.IsSet("platform") {
		cfg.Platform = strings.TrimSpace(cmd.String("platform"))
	}
	if cmd.IsSet("fs.roots") {
		cfg.FSRoots = configSplitAndTrimCSV(cmd.String("fs.roots"))
	}
	if cmd.IsSet("cap.fs") {
		cfg.CapFilesystem = new(cmd.Bool("cap.fs"))
	}
	if cmd.IsSet("cap.net") {
		cfg.CapNetwork = new(cmd.Bool("cap.net"))
	}
	if cmd.IsSet("cap.exec") {
		cfg.CapExec = new(cmd.Bool("cap.exec"))
	}
	if cmd.IsSet("cap.mem") {
		cfg.CapMemory = new(cmd.Bool("cap.mem"))
	}
	if cmd.IsSet("cap.browser") {
		cfg.CapBrowser = new(cmd.Bool("cap.browser"))
	}
	if cmd.IsSet("exec.allow") {
		cfg.ExecAllow = configSplitAndTrimCSV(cmd.String("exec.allow"))
	}
	if cmd.IsSet("exec.ask") {
		cfg.ExecAsk = configSplitAndTrimCSV(cmd.String("exec.ask"))
	}
	if cmd.IsSet("exec.deny") {
		cfg.ExecDeny = configSplitAndTrimCSV(cmd.String("exec.deny"))
	}
	if cmd.IsSet("storage.backend") {
		cfg.StorageBackend = strings.TrimSpace(cmd.String("storage.backend"))
	}
	if cmd.IsSet("session.default-idle-expiry") {
		cfg.SessionDefaultIdleExpiry = cmd.Duration("session.default-idle-expiry")
	}
	if cmd.IsSet("session.archive-retention") {
		cfg.SessionArchiveRetention = cmd.Duration("session.archive-retention")
	}
}

func configSplitAndTrimCSV(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}

	parts := strings.Split(value, ",")
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" {
			continue
		}
		values = append(values, trimmed)
	}

	return values
}

func boolValue(value *bool) bool {
	if value == nil {
		return false
	}
	return *value
}
