package gateway

import (
	"context"
	"net"
	"net/http"

	"github.com/wandxy/hand/internal/config"
	storage "github.com/wandxy/hand/internal/state/core"
	agentcore "github.com/wandxy/hand/pkg/agent"
	"github.com/wandxy/hand/pkg/nanoid"
)

var (
	genericCreatedSessionID  = nanoid.MustFromSeed(storage.SessionIDPrefix, "generic-created", "generic-created-test")
	genericExistingSessionID = nanoid.MustFromSeed(storage.SessionIDPrefix, "generic-existing", "generic-existing-test")
)

type genericResponderStub struct {
	message        string
	options        agentcore.RespondOptions
	binding        storage.GatewayBinding
	savedBinding   storage.GatewayBinding
	createdSession storage.Session
	reply          string
	err            error
	contextErr     error
	getBindingErr  error
	saveBindingErr error
	bindingFound   bool
	called         bool
	created        bool
}

func (s *genericResponderStub) Respond(
	ctx context.Context,
	message string,
	opts agentcore.RespondOptions,
) (string, error) {
	s.called = true
	s.message = message
	s.options = opts
	s.contextErr = ctx.Err()
	if opts.OnEvent != nil {
		opts.OnEvent(agentcore.Event{Kind: agentcore.EventKindTextDelta, Channel: "reasoning", Text: "ignored"})
		opts.OnEvent(agentcore.Event{Kind: agentcore.EventKindTextDelta, Channel: "assistant", Text: "stream "})
		opts.OnEvent(agentcore.Event{Kind: agentcore.EventKindTrace})
		opts.OnEvent(agentcore.Event{Kind: agentcore.EventKindTextDelta, Channel: "assistant", Text: "delta"})
	}
	return s.reply, s.err
}

func (s *genericResponderStub) CreateSession(context.Context, string) (storage.Session, error) {
	return s.CreateSessionWithOrigin(context.Background(), "", storage.SessionOrigin{})
}

func (s *genericResponderStub) CreateSessionWithOrigin(
	context.Context,
	string,
	storage.SessionOrigin,
) (storage.Session, error) {
	s.created = true
	if s.createdSession.ID == "" {
		s.createdSession = storage.Session{ID: genericCreatedSessionID}
	}

	return s.createdSession, nil
}

func (s *genericResponderStub) Get(
	context.Context,
	string,
	storage.SessionGetOptions,
) (storage.Session, bool, error) {
	if s.bindingFound {
		return storage.Session{ID: s.binding.SessionID}, true, nil
	}

	return storage.Session{}, false, nil
}

func (s *genericResponderStub) SaveGatewayBinding(
	_ context.Context,
	binding storage.GatewayBinding,
) error {
	s.savedBinding = binding
	return s.saveBindingErr
}

func (s *genericResponderStub) GetGatewayBinding(
	context.Context,
	string,
) (storage.GatewayBinding, bool, error) {
	return s.binding, s.bindingFound, s.getBindingErr
}

type fakeHTTPServer struct {
	serveErr   error
	closed     bool
	onShutdown func()
	done       chan struct{}
}

func (s *fakeHTTPServer) Serve(net.Listener) error {
	if s.serveErr != nil {
		return s.serveErr
	}
	<-s.getDone()
	return http.ErrServerClosed
}

func (s *fakeHTTPServer) Shutdown(context.Context) error {
	if s.onShutdown != nil {
		s.onShutdown()
	}
	return s.Close()
}

func (s *fakeHTTPServer) Close() error {
	if !s.closed {
		s.closed = true
		close(s.getDone())
	}
	return nil
}

func (s *fakeHTTPServer) getDone() chan struct{} {
	if s.done == nil {
		s.done = make(chan struct{})
	}

	return s.done
}

func testGatewayConfig() config.GatewayConfig {
	return config.GatewayConfig{
		Enabled:   true,
		Address:   "127.0.0.1",
		Port:      0,
		AuthToken: "token",
		Telegram: config.GatewayTelegramConfig{
			Mode:     config.GatewayTelegramModePolling,
			BotToken: "telegram-token",
		},
		Slack: config.GatewaySlackConfig{
			Mode:     config.GatewaySlackModeSocket,
			BotToken: "slack-token",
			AppToken: "app-token",
		},
	}
}
