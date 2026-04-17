package e2e

import (
	"maps"
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/wandxy/hand/internal/agent"
	"github.com/wandxy/hand/internal/config"
	"github.com/wandxy/hand/internal/datadir"
	handmsg "github.com/wandxy/hand/internal/messages"
	"github.com/wandxy/hand/internal/models"
	"github.com/wandxy/hand/internal/storage"
	storagefactory "github.com/wandxy/hand/internal/storage/factory"
)

var setHarnessEnv = os.Setenv

type HarnessOptions struct {
	Spec          HarnessSpec
	Config        *config.Config
	ModelClient   models.Client
	SummaryClient models.Client
}

type Harness struct {
	cfg          *config.Config
	agent        harnessAgent
	inspectStore storage.SessionStore
	cancel       context.CancelFunc
	restoreEnv   func()
	stdout       *bytes.Buffer
	stderr       *bytes.Buffer
}

type harnessAgent interface {
	Respond(context.Context, string, agent.RespondOptions) (string, error)
	CurrentSession(context.Context) (string, error)
}

var openHarnessInspectStore = openInspectStore

func NewHarness(ctx context.Context, opts HarnessOptions) (*Harness, error) {
	if err := opts.Spec.Validate(); err != nil {
		return nil, err
	}
	if opts.ModelClient == nil {
		return nil, errors.New("e2e harness model client is required")
	}

	restoreEnv, err := applyHarnessEnv(opts.Spec)
	if err != nil {
		return nil, err
	}

	cfg, err := resolveHarnessConfig(opts.Spec, opts.Config)
	if err != nil {
		restoreEnv()
		return nil, err
	}

	if strings.TrimSpace(opts.Spec.Isolation.TraceDir) != "" {
		cfg.DebugTraceDir = opts.Spec.Isolation.TraceDir
	}

	runCtx, cancel := context.WithCancel(normalizeHarnessContext(ctx))
	ag := agent.NewAgent(runCtx, cfg, opts.ModelClient, opts.SummaryClient)
	if err := ag.Start(runCtx); err != nil {
		cancel()
		restoreEnv()
		return nil, err
	}

	inspectStore, err := openHarnessInspectStore(cfg)
	if err != nil {
		cancel()
		restoreEnv()
		return nil, err
	}

	return &Harness{
		cfg:          cfg,
		agent:        ag,
		inspectStore: inspectStore,
		cancel:       cancel,
		restoreEnv:   restoreEnv,
		stdout:       &bytes.Buffer{},
		stderr:       &bytes.Buffer{},
	}, nil
}

func (h *Harness) Close() error {
	if h == nil {
		return nil
	}
	if h.cancel != nil {
		h.cancel()
	}
	if h.restoreEnv != nil {
		h.restoreEnv()
	}
	return nil
}

func (h *Harness) Config() *config.Config {
	if h == nil || h.cfg == nil {
		return nil
	}
	cloned := *h.cfg
	return &cloned
}

func (h *Harness) Stdout() string {
	if h == nil || h.stdout == nil {
		return ""
	}
	return h.stdout.String()
}

func (h *Harness) Stderr() string {
	if h == nil || h.stderr == nil {
		return ""
	}
	return h.stderr.String()
}

func (h *Harness) Send(ctx context.Context, req RootChatRequest) (RootChatResult, error) {
	if h == nil || h.agent == nil {
		return RootChatResult{}, errors.New("e2e harness is required")
	}
	if err := req.Validate(); err != nil {
		h.writeStderr(err.Error())
		return RootChatResult{}, err
	}

	events := make([]Event, 0, 4)
	reply, err := h.agent.Respond(normalizeHarnessContext(ctx), req.Message, agent.RespondOptions{
		Instruct:  req.Instruct,
		SessionID: req.SessionID,
		Stream:    req.Stream,
		OnEvent: func(event agent.Event) {
			events = append(events, Event{Channel: event.Channel, Text: event.Text})
			h.writeStdout(event.Text)
		},
	})
	if err != nil {
		h.writeStderr(err.Error())
		return RootChatResult{}, err
	}

	sessionID := strings.TrimSpace(req.SessionID)
	if sessionID == "" {
		sessionID, err = h.agent.CurrentSession(normalizeHarnessContext(ctx))
		if err != nil {
			h.writeStderr(err.Error())
			return RootChatResult{}, err
		}
	}

	return RootChatResult{
		Reply:     reply,
		SessionID: sessionID,
		Events:    events,
	}, nil
}

func (h *Harness) Messages(ctx context.Context, sessionID string) ([]handmsg.Message, error) {
	if h == nil {
		return nil, errors.New("e2e harness is required")
	}
	if h.inspectStore == nil {
		return nil, errors.New("e2e harness message inspection is unavailable for non-persistent storage")
	}

	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		var err error
		sessionID, err = h.agent.CurrentSession(normalizeHarnessContext(ctx))
		if err != nil {
			return nil, err
		}
	}

	return h.inspectStore.GetMessages(normalizeHarnessContext(ctx), sessionID, storage.MessageQueryOptions{})
}

func resolveHarnessConfig(spec HarnessSpec, cfg *config.Config) (*config.Config, error) {
	if spec.Config.Mode() == ConfigModeRealInput {
		if cfg != nil {
			return nil, errors.New("e2e harness must not use in-memory config when real config inputs are present")
		}
		return config.Load(spec.Config.EnvFilePath, spec.Config.ConfigFilePath)
	}

	if cfg == nil {
		return nil, errors.New("e2e harness requires config for in-memory mode")
	}

	cloned := *cfg
	cloned.Normalize()
	return &cloned, nil
}

func openInspectStore(cfg *config.Config) (storage.SessionStore, error) {
	if cfg == nil {
		return nil, errors.New("config is required")
	}
	if strings.TrimSpace(strings.ToLower(cfg.StorageBackend)) == "memory" {
		return nil, nil
	}
	return storagefactory.OpenSessionStore(cfg)
}

func applyHarnessEnv(spec HarnessSpec) (func(), error) {
	homeDir := filepath.Dir(spec.Isolation.DataDir)
	updates := make(map[string]string, len(spec.Config.Env)+1)
	maps.Copy(updates, spec.Config.Env)
	if strings.TrimSpace(updates["HAND_HOME"]) == "" {
		updates["HAND_HOME"] = homeDir
	}

	restore := captureEnv(updates)
	for key, value := range updates {
		if err := setHarnessEnv(key, value); err != nil {
			restore()
			return nil, err
		}
	}

	if datadir.DataDir() != spec.Isolation.DataDir {
		restore()
		return nil, errors.New("e2e isolation data dir must match HAND_HOME/data")
	}
	if datadir.StateDBPath() != spec.Isolation.StoragePath {
		restore()
		return nil, errors.New("e2e isolation storage path must match HAND_HOME/data/state.db")
	}

	return restore, nil
}

func captureEnv(updates map[string]string) func() {
	originals := make(map[string]*string, len(updates))
	for key := range updates {
		value, ok := os.LookupEnv(key)
		if ok {
			copied := value
			originals[key] = &copied
			continue
		}
		originals[key] = nil
	}

	return func() {
		for key, value := range originals {
			if value == nil {
				_ = os.Unsetenv(key)
				continue
			}
			_ = os.Setenv(key, *value)
		}
	}
}

func normalizeHarnessContext(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}

func (h *Harness) writeStdout(text string) {
	if h == nil || h.stdout == nil || strings.TrimSpace(text) == "" {
		return
	}
	_, _ = io.WriteString(h.stdout, text)
}

func (h *Harness) writeStderr(text string) {
	if h == nil || h.stderr == nil || strings.TrimSpace(text) == "" {
		return
	}
	_, _ = io.WriteString(h.stderr, text)
}
