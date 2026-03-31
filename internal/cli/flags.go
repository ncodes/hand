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
			Name:  "model.router",
			Usage: "Model router identifier",
			Value: config.Get().ModelRouter,
		},
		&cli.StringFlag{
			Name:  "model.key",
			Usage: "Authentication key for the selected model router",
		},
		&cli.StringFlag{
			Name:  "model",
			Usage: "Model slug to send to the provider, for example openai/gpt-4o-mini",
			Value: config.Get().Model,
		},
		&cli.StringFlag{
			Name:  "model.base-url",
			Usage: "Base URL for the model provider API",
			Value: config.Get().ModelBaseURL,
		},
		&cli.StringFlag{
			Name:  "model.api-mode",
			Usage: "Provider API mode: chat-completions or responses",
			Value: config.Get().ModelAPIMode,
		},
		&cli.StringFlag{
			Name:  "rpc.address",
			Usage: "Bind address for the RPC service",
			Value: config.Get().RPCAddress,
		},
		&cli.IntFlag{
			Name:  "rpc.port",
			Usage: "Bind port for the RPC service",
			Value: config.Get().RPCPort,
		},
		&cli.IntFlag{
			Name:  "max-iterations",
			Usage: "Maximum model iterations allowed in a tool-calling loop",
			Value: config.Get().MaxIterations,
		},
		&cli.StringFlag{
			Name:  "log.level",
			Usage: "Set the minimum log level: debug, info, warn, or error",
			Value: config.Get().LogLevel,
		},
		&cli.BoolFlag{
			Name:  "log.no-color",
			Usage: "Emit plain log output without ANSI color codes",
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
			Name:  "debug.trace-dir",
			Usage: "Directory for persisted debug trace files",
			Value: config.Get().DebugTraceDir,
		},
		&cli.StringFlag{
			Name:  "rules.files",
			Usage: "Comma-separated rule file paths to load in addition to workspace defaults",
			Value: strings.Join(config.Get().RulesFiles, ","),
		},
		&cli.StringFlag{
			Name:  "platform",
			Usage: "Active runtime platform used for tool filtering",
			Value: config.Get().Platform,
		},
		&cli.StringFlag{
			Name:  "agent.fs.roots",
			Usage: "Comma-separated filesystem roots allowed for file tools",
			Value: strings.Join(config.Get().FSRoots, ","),
		},
		&cli.BoolFlag{
			Name:  "cap.fs",
			Usage: "Enable filesystem tool capability filtering",
			Value: boolValue(config.Get().CapFilesystem),
		},
		&cli.BoolFlag{
			Name:  "cap.net",
			Usage: "Enable network tool capability filtering",
			Value: boolValue(config.Get().CapNetwork),
		},
		&cli.BoolFlag{
			Name:  "cap.exec",
			Usage: "Enable exec tool capability filtering",
			Value: boolValue(config.Get().CapExec),
		},
		&cli.BoolFlag{
			Name:  "cap.mem",
			Usage: "Enable memory tool capability filtering",
			Value: boolValue(config.Get().CapMemory),
		},
		&cli.BoolFlag{
			Name:  "cap.browser",
			Usage: "Enable browser tool capability filtering",
			Value: boolValue(config.Get().CapBrowser),
		},
		&cli.StringFlag{
			Name:  "agent.exec.allow",
			Usage: "Comma-separated allowed command prefixes",
			Value: strings.Join(config.Get().ExecAllow, ","),
		},
		&cli.StringFlag{
			Name:  "agent.exec.ask",
			Usage: "Comma-separated command prefixes that require approval",
			Value: strings.Join(config.Get().ExecAsk, ","),
		},
		&cli.StringFlag{
			Name:  "agent.exec.deny",
			Usage: "Comma-separated denied command prefixes",
			Value: strings.Join(config.Get().ExecDeny, ","),
		},
		&cli.StringFlag{
			Name:  "session.backend",
			Usage: "Session persistence backend: memory or sqlite",
			Value: config.Get().SessionBackend,
		},
		&cli.DurationFlag{
			Name:  "session.default-idle-expiry",
			Usage: "Idle duration before the default session is archived and cleared",
			Value: config.Get().SessionDefaultIdleExpiry,
		},
		&cli.DurationFlag{
			Name:  "session.archive-retention",
			Usage: "How long archived default-session conversations are retained before deletion",
			Value: config.Get().SessionArchiveRetention,
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
	if cmd.IsSet("model.router") {
		cfg.ModelRouter = strings.TrimSpace(cmd.String("model.router"))
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
	if cmd.IsSet("agent.fs.roots") {
		cfg.FSRoots = configSplitAndTrimCSV(cmd.String("agent.fs.roots"))
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
	if cmd.IsSet("agent.exec.allow") {
		cfg.ExecAllow = configSplitAndTrimCSV(cmd.String("agent.exec.allow"))
	}
	if cmd.IsSet("agent.exec.ask") {
		cfg.ExecAsk = configSplitAndTrimCSV(cmd.String("agent.exec.ask"))
	}
	if cmd.IsSet("agent.exec.deny") {
		cfg.ExecDeny = configSplitAndTrimCSV(cmd.String("agent.exec.deny"))
	}
	if cmd.IsSet("session.backend") {
		cfg.SessionBackend = strings.TrimSpace(cmd.String("session.backend"))
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
