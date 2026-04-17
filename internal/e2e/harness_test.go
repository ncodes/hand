package e2e

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wandxy/hand/internal/agent"
	"github.com/wandxy/hand/internal/config"
	handmsg "github.com/wandxy/hand/internal/messages"
	"github.com/wandxy/hand/internal/mocks"
	"github.com/wandxy/hand/internal/models"
	"github.com/wandxy/hand/internal/storage"
)

func TestNewHarness_InMemoryConfigSmoke(t *testing.T) {
	spec := testHarnessSpec(t)
	client := &mocks.ModelClientStub{
		Responses: []*models.Response{{OutputText: "hello from hand"}},
	}

	harness, err := NewHarness(context.Background(), HarnessOptions{
		Spec:        spec,
		Config:      testHarnessConfig(),
		ModelClient: client,
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, harness.Close())
	})

	result, err := harness.Send(context.Background(), RootChatRequest{Message: "hello"})
	require.NoError(t, err)
	assert.Equal(t, "hello from hand", result.Reply)
	assert.NotEmpty(t, result.SessionID)
	assert.Empty(t, result.Events)

	messages, err := harness.Messages(context.Background(), result.SessionID)
	require.NoError(t, err)
	require.Len(t, messages, 2)
	assert.Equal(t, handmsg.RoleUser, messages[0].Role)
	assert.Equal(t, "hello", messages[0].Content)
	assert.Equal(t, handmsg.RoleAssistant, messages[1].Role)
	assert.Equal(t, "hello from hand", messages[1].Content)

	cfg := harness.Config()
	require.NotNil(t, cfg)
	assert.Equal(t, "Test Hand", cfg.Name)
}

func TestNewHarness_RealConfigLoadAndEnvOverride(t *testing.T) {
	spec := testHarnessSpec(t)
	configPath := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte(""+
		"name: File Hand\n"+
		"model:\n"+
		"  name: test-model\n"+
		"  stream: true\n"+
		"storage:\n"+
		"  backend: sqlite\n"), 0o644))
	spec.Config = ConfigInput{
		ConfigFilePath: configPath,
		Env: map[string]string{
			"NAME":         "Env Hand",
			"MODEL_STREAM": "false",
		},
	}

	harness, err := NewHarness(context.Background(), HarnessOptions{
		Spec:        spec,
		ModelClient: &mocks.ModelClientStub{Responses: []*models.Response{{OutputText: "loaded"}}},
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, harness.Close())
	})

	cfg := harness.Config()
	require.NotNil(t, cfg)
	assert.Equal(t, "Env Hand", cfg.Name)
	require.NotNil(t, cfg.Stream)
	assert.False(t, *cfg.Stream)

	result, err := harness.Send(context.Background(), RootChatRequest{Message: "ping"})
	require.NoError(t, err)
	assert.Equal(t, "loaded", result.Reply)
}

func TestNewHarness_Errors(t *testing.T) {
	validSpec := testHarnessSpec(t)
	validConfig := testHarnessConfig()
	validClient := &mocks.ModelClientStub{Responses: []*models.Response{{OutputText: "ok"}}}

	t.Run("invalid spec", func(t *testing.T) {
		_, err := NewHarness(context.Background(), HarnessOptions{})
		require.Error(t, err)
		assert.EqualError(t, err, "e2e entrypoint is required")
	})

	t.Run("missing model client", func(t *testing.T) {
		_, err := NewHarness(context.Background(), HarnessOptions{
			Spec:   validSpec,
			Config: validConfig,
		})
		require.Error(t, err)
		assert.EqualError(t, err, "e2e harness model client is required")
	})

	t.Run("real config with in-memory config provided", func(t *testing.T) {
		spec := validSpec
		spec.Config = ConfigInput{ConfigFilePath: filepath.Join(t.TempDir(), "config.yaml")}
		require.NoError(t, os.WriteFile(spec.Config.ConfigFilePath, []byte("model:\n  name: test-model\n"), 0o644))

		_, err := NewHarness(context.Background(), HarnessOptions{
			Spec:        spec,
			Config:      validConfig,
			ModelClient: validClient,
		})
		require.Error(t, err)
		assert.EqualError(t, err, "e2e harness must not use in-memory config when real config inputs are present")
	})

	t.Run("in-memory mode without config", func(t *testing.T) {
		_, err := NewHarness(context.Background(), HarnessOptions{
			Spec:        validSpec,
			ModelClient: validClient,
		})
		require.Error(t, err)
		assert.EqualError(t, err, "e2e harness requires config for in-memory mode")
	})

	t.Run("bad isolation path", func(t *testing.T) {
		spec := validSpec
		spec.Isolation.StoragePath = filepath.Join(t.TempDir(), "wrong.db")
		_, err := NewHarness(context.Background(), HarnessOptions{
			Spec:        spec,
			Config:      validConfig,
			ModelClient: validClient,
		})
		require.Error(t, err)
		assert.EqualError(t, err, "e2e isolation storage path must match HAND_HOME/data/state.db")
	})

	t.Run("bad isolation data dir", func(t *testing.T) {
		spec := validSpec
		spec.Isolation.DataDir = filepath.Join(t.TempDir(), "custom")
		spec.Isolation.StoragePath = filepath.Join(filepath.Dir(spec.Isolation.DataDir), "data", "state.db")
		_, err := NewHarness(context.Background(), HarnessOptions{
			Spec:        spec,
			Config:      validConfig,
			ModelClient: validClient,
		})
		require.Error(t, err)
		assert.EqualError(t, err, "e2e isolation data dir must match HAND_HOME/data")
	})

	t.Run("agent start error", func(t *testing.T) {
		cfg := testHarnessConfig()
		cfg.StorageBackend = "bogus"
		_, err := NewHarness(context.Background(), HarnessOptions{
			Spec:        validSpec,
			Config:      cfg,
			ModelClient: validClient,
		})
		require.Error(t, err)
		assert.EqualError(t, err, "storage backend must be one of: memory, sqlite")
	})

	t.Run("inspect store open error", func(t *testing.T) {
		original := openHarnessInspectStore
		openHarnessInspectStore = func(*config.Config) (storage.SessionStore, error) {
			return nil, errors.New("inspect failed")
		}
		t.Cleanup(func() {
			openHarnessInspectStore = original
		})

		_, err := NewHarness(context.Background(), HarnessOptions{
			Spec:        validSpec,
			Config:      validConfig,
			ModelClient: validClient,
		})
		require.Error(t, err)
		assert.EqualError(t, err, "inspect failed")
	})
}

func TestHarnessCloseAndConfigNilPaths(t *testing.T) {
	assert.NoError(t, (*Harness)(nil).Close())
	assert.Nil(t, (*Harness)(nil).Config())
	assert.Nil(t, (&Harness{}).Config())
	assert.Empty(t, (*Harness)(nil).Stdout())
	assert.Empty(t, (*Harness)(nil).Stderr())
}

func TestHarnessSendAndMessagesErrors(t *testing.T) {
	t.Run("nil harness send", func(t *testing.T) {
		_, err := (*Harness)(nil).Send(context.Background(), RootChatRequest{Message: "hello"})
		require.Error(t, err)
		assert.EqualError(t, err, "e2e harness is required")
	})

	t.Run("invalid request", func(t *testing.T) {
		spec := testHarnessSpec(t)
		h, err := NewHarness(context.Background(), HarnessOptions{
			Spec:        spec,
			Config:      testHarnessConfig(),
			ModelClient: &mocks.ModelClientStub{Responses: []*models.Response{{OutputText: "ok"}}},
		})
		require.NoError(t, err)
		t.Cleanup(func() {
			require.NoError(t, h.Close())
		})

		_, err = h.Send(context.Background(), RootChatRequest{})
		require.Error(t, err)
		assert.EqualError(t, err, "e2e root chat message is required")
		assert.Equal(t, "e2e root chat message is required", h.Stderr())
	})

	t.Run("agent missing", func(t *testing.T) {
		_, err := (&Harness{}).Send(context.Background(), RootChatRequest{Message: "hello"})
		require.Error(t, err)
		assert.EqualError(t, err, "e2e harness is required")
	})

	t.Run("respond error", func(t *testing.T) {
		h := &Harness{agent: harnessAgentStub{respondErr: errors.New("respond failed")}, stdout: &bytes.Buffer{}, stderr: &bytes.Buffer{}}
		_, err := h.Send(context.Background(), RootChatRequest{Message: "hello"})
		require.Error(t, err)
		assert.EqualError(t, err, "respond failed")
		assert.Equal(t, "respond failed", h.Stderr())
	})

	t.Run("current session error after send", func(t *testing.T) {
		h := &Harness{agent: harnessAgentStub{reply: "ok", currentErr: errors.New("current failed")}, stdout: &bytes.Buffer{}, stderr: &bytes.Buffer{}}
		_, err := h.Send(context.Background(), RootChatRequest{Message: "hello"})
		require.Error(t, err)
		assert.EqualError(t, err, "current failed")
		assert.Equal(t, "current failed", h.Stderr())
	})

	t.Run("explicit session id skips current lookup", func(t *testing.T) {
		h := &Harness{agent: harnessAgentStub{reply: "ok", currentErr: errors.New("unused")}, stdout: &bytes.Buffer{}, stderr: &bytes.Buffer{}}
		result, err := h.Send(context.Background(), RootChatRequest{Message: "hello", SessionID: "ses_test"})
		require.NoError(t, err)
		assert.Equal(t, "ses_test", result.SessionID)
		assert.Equal(t, "ok", result.Reply)
	})

	t.Run("events are captured", func(t *testing.T) {
		h := &Harness{agent: harnessAgentStub{
			reply:   "ok",
			current: "ses_current",
			events:  []agent.Event{{Channel: "assistant", Text: "a"}, {Channel: "reasoning", Text: "b"}},
		}, stdout: &bytes.Buffer{}, stderr: &bytes.Buffer{}}
		result, err := h.Send(context.Background(), RootChatRequest{Message: "hello"})
		require.NoError(t, err)
		require.Len(t, result.Events, 2)
		assert.Equal(t, "assistant", result.Events[0].Channel)
		assert.Equal(t, "a", result.Events[0].Text)
		assert.Equal(t, "reasoning", result.Events[1].Channel)
		assert.Equal(t, "b", result.Events[1].Text)
		assert.Equal(t, "ab", h.Stdout())
	})

	t.Run("nil harness messages", func(t *testing.T) {
		_, err := (*Harness)(nil).Messages(context.Background(), "")
		require.Error(t, err)
		assert.EqualError(t, err, "e2e harness is required")
	})

	t.Run("messages current session lookup", func(t *testing.T) {
		h := &Harness{
			agent:        harnessAgentStub{current: "ses_current"},
			inspectStore: &storageStoreStub{messages: []handmsg.Message{{Role: handmsg.RoleUser, Content: "hello"}}},
		}
		messages, err := h.Messages(context.Background(), "")
		require.NoError(t, err)
		require.Len(t, messages, 1)
		assert.Equal(t, "hello", messages[0].Content)
	})

	t.Run("messages current session error", func(t *testing.T) {
		h := &Harness{
			agent:        harnessAgentStub{currentErr: errors.New("current failed")},
			inspectStore: &storageStoreStub{},
		}
		_, err := h.Messages(context.Background(), "")
		require.Error(t, err)
		assert.EqualError(t, err, "current failed")
	})

	t.Run("messages unavailable for memory store", func(t *testing.T) {
		spec := testHarnessSpec(t)
		cfg := testHarnessConfig()
		cfg.StorageBackend = "memory"

		harness, err := NewHarness(context.Background(), HarnessOptions{
			Spec:        spec,
			Config:      cfg,
			ModelClient: &mocks.ModelClientStub{Responses: []*models.Response{{OutputText: "ok"}}},
		})
		require.NoError(t, err)
		t.Cleanup(func() {
			require.NoError(t, harness.Close())
		})

		_, err = harness.Messages(context.Background(), "")
		require.Error(t, err)
		assert.EqualError(t, err, "e2e harness message inspection is unavailable for non-persistent storage")
	})
}

func TestOpenInspectStoreAndHelpers(t *testing.T) {
	t.Run("nil config", func(t *testing.T) {
		store, err := openInspectStore(nil)
		require.Error(t, err)
		assert.Nil(t, store)
		assert.EqualError(t, err, "config is required")
	})

	t.Run("memory backend", func(t *testing.T) {
		store, err := openInspectStore(&config.Config{StorageBackend: "memory"})
		require.NoError(t, err)
		assert.Nil(t, store)
	})

	t.Run("normalize nil context", func(t *testing.T) {
		assert.NotNil(t, normalizeHarnessContext(nil))
	})

	t.Run("write output helpers", func(t *testing.T) {
		h := &Harness{stdout: &bytes.Buffer{}, stderr: &bytes.Buffer{}}
		h.writeStdout("hello")
		h.writeStdout("   ")
		h.writeStderr("warn")
		h.writeStderr("")
		assert.Equal(t, "hello", h.Stdout())
		assert.Equal(t, "warn", h.Stderr())
	})

	t.Run("capture env restore", func(t *testing.T) {
		require.NoError(t, os.Setenv("E2E_CAPTURE_ENV", "old"))
		restore := captureEnv(map[string]string{
			"E2E_CAPTURE_ENV": "new",
			"E2E_CAPTURE_NEW": "x",
		})
		require.NoError(t, os.Setenv("E2E_CAPTURE_ENV", "new"))
		require.NoError(t, os.Setenv("E2E_CAPTURE_NEW", "x"))
		restore()
		assert.Equal(t, "old", os.Getenv("E2E_CAPTURE_ENV"))
		assert.Empty(t, os.Getenv("E2E_CAPTURE_NEW"))
	})

	t.Run("apply env with explicit hand home", func(t *testing.T) {
		spec := testHarnessSpec(t)
		home := filepath.Dir(spec.Isolation.DataDir)
		spec.Config.Env = map[string]string{"HAND_HOME": home}
		restore, err := applyHarnessEnv(spec)
		require.NoError(t, err)
		t.Cleanup(restore)
		assert.Equal(t, home, os.Getenv("HAND_HOME"))
	})

	t.Run("apply env set failure", func(t *testing.T) {
		original := setHarnessEnv
		setHarnessEnv = func(string, string) error { return errors.New("setenv failed") }
		t.Cleanup(func() {
			setHarnessEnv = original
		})

		_, err := applyHarnessEnv(testHarnessSpec(t))
		require.Error(t, err)
		assert.EqualError(t, err, "setenv failed")
	})
}

func testHarnessSpec(t *testing.T) HarnessSpec {
	t.Helper()

	home := filepath.Join(t.TempDir(), "hand-home")
	dataDir := filepath.Join(home, "data")
	return HarnessSpec{
		PrimaryEntrypoint:   EntrypointDirectAgent,
		SecondaryEntrypoint: EntrypointCommandRPC,
		Config:              ConfigInput{AllowInMemory: true},
		Isolation: Isolation{
			WorkspaceDir: filepath.Join(home, "workspace"),
			DataDir:      dataDir,
			StoragePath:  filepath.Join(dataDir, "state.db"),
			TraceDir:     filepath.Join(home, "traces"),
		},
	}
}

func testHarnessConfig() *config.Config {
	stream := false
	return &config.Config{
		Name:                     "Test Hand",
		Model:                    "test-model",
		Stream:                   &stream,
		StorageBackend:           "sqlite",
		SessionDefaultIdleExpiry: time.Hour,
		SessionArchiveRetention:  24 * time.Hour,
	}
}

type harnessAgentStub struct {
	reply      string
	respondErr error
	current    string
	currentErr error
	events     []agent.Event
}

func (s harnessAgentStub) Respond(_ context.Context, _ string, opts agent.RespondOptions) (string, error) {
	if s.respondErr != nil {
		return "", s.respondErr
	}
	if opts.OnEvent != nil {
		for _, event := range s.events {
			opts.OnEvent(event)
		}
	}
	return s.reply, nil
}

func (s harnessAgentStub) CurrentSession(context.Context) (string, error) {
	if s.currentErr != nil {
		return "", s.currentErr
	}
	return s.current, nil
}

type storageStoreStub struct {
	messages []handmsg.Message
}

func (s *storageStoreStub) Save(context.Context, storage.Session) error { return nil }
func (s *storageStoreStub) Get(context.Context, string) (storage.Session, bool, error) {
	return storage.Session{}, false, nil
}
func (s *storageStoreStub) List(context.Context) ([]storage.Session, error) { return nil, nil }
func (s *storageStoreStub) Delete(context.Context, string) error            { return nil }
func (s *storageStoreStub) AppendMessages(context.Context, string, []handmsg.Message) error {
	return nil
}
func (s *storageStoreStub) GetMessages(context.Context, string, storage.MessageQueryOptions) ([]handmsg.Message, error) {
	return s.messages, nil
}
func (s *storageStoreStub) SearchMessages(context.Context, string, storage.SearchMessageOptions) ([]handmsg.Message, error) {
	return nil, nil
}
func (s *storageStoreStub) CountMessages(context.Context, string, storage.MessageQueryOptions) (int, error) {
	return 0, nil
}
func (s *storageStoreStub) GetMessage(context.Context, string, int, storage.MessageQueryOptions) (handmsg.Message, bool, error) {
	return handmsg.Message{}, false, nil
}
func (s *storageStoreStub) SaveSummary(context.Context, storage.SessionSummary) error { return nil }
func (s *storageStoreStub) GetSummary(context.Context, string) (storage.SessionSummary, bool, error) {
	return storage.SessionSummary{}, false, nil
}
func (s *storageStoreStub) DeleteSummary(context.Context, string) error { return nil }
func (s *storageStoreStub) CreateArchive(context.Context, storage.ArchivedSession) error {
	return nil
}
func (s *storageStoreStub) GetArchive(context.Context, string) (storage.ArchivedSession, bool, error) {
	return storage.ArchivedSession{}, false, nil
}
func (s *storageStoreStub) ListArchives(context.Context, string) ([]storage.ArchivedSession, error) {
	return nil, nil
}
func (s *storageStoreStub) DeleteArchive(context.Context, string) error { return nil }
func (s *storageStoreStub) DeleteExpiredArchives(context.Context, time.Time) error {
	return nil
}
func (s *storageStoreStub) ClearMessages(context.Context, string, storage.MessageQueryOptions) error {
	return nil
}
func (s *storageStoreStub) SetCurrent(context.Context, string) error { return nil }
func (s *storageStoreStub) Current(context.Context) (string, bool, error) {
	return "", false, nil
}
