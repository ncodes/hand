package gateway

import (
	"context"
	"fmt"
	"net"
	"net/http"

	"github.com/wandxy/hand/internal/config"
	storage "github.com/wandxy/hand/internal/state/core"
	statemanager "github.com/wandxy/hand/internal/state/manager"
	agentcore "github.com/wandxy/hand/pkg/agent"
	"github.com/wandxy/hand/pkg/gateway/bindings"
	"github.com/wandxy/hand/pkg/nanoid"
)

var (
	genericCreatedSessionID  = nanoid.MustFromSeed(storage.SessionIDPrefix, "generic-created", "generic-created-test")
	genericExistingSessionID = nanoid.MustFromSeed(storage.SessionIDPrefix, "generic-existing", "generic-existing-test")
	existingSessionID        = nanoid.MustFromSeed(storage.SessionIDPrefix, "existing-binding", "existing-binding-test")
	createdSessionID         = nanoid.MustFromSeed(storage.SessionIDPrefix, "created-binding", "created-binding-test")
)

type genericResponderStub struct {
	message        string
	options        agentcore.RespondOptions
	binding        storage.GatewayBinding
	savedBinding   storage.GatewayBinding
	createdSession storage.Session
	reply          string
	err            error
	getBindingErr  error
	saveBindingErr error
	bindingFound   bool
	called         bool
	created        bool
}

func (s *genericResponderStub) Respond(
	_ context.Context,
	message string,
	opts agentcore.RespondOptions,
) (string, error) {
	s.called = true
	s.message = message
	s.options = opts
	return s.reply, s.err
}

func (s *genericResponderStub) CreateSession(context.Context, string) (storage.Session, error) {
	s.created = true
	if s.createdSession.ID == "" {
		s.createdSession = storage.Session{ID: genericCreatedSessionID}
	}

	return s.createdSession, nil
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

type sessionResolverServiceStub struct {
	binding        storage.GatewayBinding
	savedBinding   storage.GatewayBinding
	createdSession storage.Session
	getErr         error
	saveErr        error
	createErr      error
	bindingFound   bool
	created        bool
}

func (s *sessionResolverServiceStub) Respond(context.Context, string, agentcore.RespondOptions) (string, error) {
	return "", nil
}

func (s *sessionResolverServiceStub) CreateSession(context.Context, string) (storage.Session, error) {
	s.created = true
	if s.createErr != nil {
		return storage.Session{}, s.createErr
	}
	if s.createdSession.ID == "" {
		s.createdSession = storage.Session{ID: createdSessionID}
	}

	return s.createdSession, nil
}

func (s *sessionResolverServiceStub) SaveGatewayBinding(
	_ context.Context,
	binding storage.GatewayBinding,
) error {
	s.savedBinding = binding
	return s.saveErr
}

func (s *sessionResolverServiceStub) GetGatewayBinding(
	context.Context,
	string,
) (storage.GatewayBinding, bool, error) {
	return s.binding, s.bindingFound, s.getErr
}

type mapBackedSessionService struct {
	bindings map[string]storage.GatewayBinding
	nextID   int
}

func newMapBackedSessionService() *mapBackedSessionService {
	return &mapBackedSessionService{bindings: make(map[string]storage.GatewayBinding)}
}

func (s *mapBackedSessionService) Respond(context.Context, string, agentcore.RespondOptions) (string, error) {
	return "", nil
}

func (s *mapBackedSessionService) CreateSession(context.Context, string) (storage.Session, error) {
	s.nextID++
	return storage.Session{
		ID: nanoid.MustFromSeed(storage.SessionIDPrefix, fmt.Sprintf("generated-binding-%d", s.nextID), "binding"),
	}, nil
}

func (s *mapBackedSessionService) SaveGatewayBinding(
	_ context.Context,
	binding storage.GatewayBinding,
) error {
	s.bindings[binding.Key] = binding
	return nil
}

func (s *mapBackedSessionService) GetGatewayBinding(
	_ context.Context,
	key string,
) (storage.GatewayBinding, bool, error) {
	binding, ok := s.bindings[key]
	return binding, ok, nil
}

type stateManagerGatewayService struct {
	manager *statemanager.Manager
}

func (s *stateManagerGatewayService) Respond(context.Context, string, agentcore.RespondOptions) (string, error) {
	return "", nil
}

func (s *stateManagerGatewayService) CreateSession(ctx context.Context, id string) (storage.Session, error) {
	return s.manager.CreateSession(ctx, id)
}

func (s *stateManagerGatewayService) SaveGatewayBinding(
	ctx context.Context,
	binding storage.GatewayBinding,
) error {
	return s.manager.SaveGatewayBinding(ctx, binding)
}

func (s *stateManagerGatewayService) GetGatewayBinding(
	ctx context.Context,
	key string,
) (storage.GatewayBinding, bool, error) {
	return s.manager.GetGatewayBinding(ctx, key)
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

func keyFromString(value string) bindings.Key {
	return bindings.Key(value)
}
