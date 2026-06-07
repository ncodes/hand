package session

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wandxy/hand/internal/constants"
	storage "github.com/wandxy/hand/internal/state/core"
	statemanager "github.com/wandxy/hand/internal/state/manager"
	"github.com/wandxy/hand/internal/state/storememory"
	agentcore "github.com/wandxy/hand/pkg/agent"
	"github.com/wandxy/hand/pkg/gateway/bindings"
	"github.com/wandxy/hand/pkg/nanoid"
)

var (
	existingSessionID = nanoid.MustFromSeed(storage.SessionIDPrefix, "existing-binding", "existing-binding-test")
	createdSessionID  = nanoid.MustFromSeed(storage.SessionIDPrefix, "created-binding", "created-binding-test")
)

func TestResolver_ReusesExistingBinding(t *testing.T) {
	service := &serviceStub{
		binding: storage.GatewayBinding{
			Key:       "generic::chat-1:",
			SessionID: existingSessionID,
		},
		bindingFound: true,
	}
	resolver := NewResolver(service)

	session, err := resolver.Resolve(context.Background(), keyFromString("generic::chat-1:"))

	require.NoError(t, err)
	require.Equal(t, existingSessionID, session.ID)
	require.False(t, service.created)
}

func TestResolver_CreatesAndPersistsMissingBinding(t *testing.T) {
	service := &serviceStub{
		createdSession: storage.Session{ID: createdSessionID},
	}
	resolver := NewResolver(service)

	session, err := resolver.Resolve(context.Background(), keyFromString("generic::chat-1:"))

	require.NoError(t, err)
	require.Equal(t, createdSessionID, session.ID)
	require.True(t, service.created)
	require.Equal(t, storage.GatewayBinding{
		Key:       "generic::chat-1:",
		SessionID: createdSessionID,
		CreatedAt: service.savedBinding.CreatedAt,
		UpdatedAt: service.savedBinding.UpdatedAt,
	}, service.savedBinding)
	require.False(t, service.savedBinding.CreatedAt.IsZero())
	require.False(t, service.savedBinding.UpdatedAt.IsZero())
}

func TestResolver_DifferentSourcesProduceDifferentSessions(t *testing.T) {
	service := newMapBackedService()
	resolver := NewResolver(service)
	genericKey, err := bindings.Generic("same")
	require.NoError(t, err)
	telegramKey, err := bindings.Telegram("same", "")
	require.NoError(t, err)

	genericSession, err := resolver.Resolve(context.Background(), genericKey)
	require.NoError(t, err)
	telegramSession, err := resolver.Resolve(context.Background(), telegramKey)
	require.NoError(t, err)

	require.NotEqual(t, genericSession.ID, telegramSession.ID)
}

func TestResolver_SameKeyResolvesSameSession(t *testing.T) {
	service := newMapBackedService()
	resolver := NewResolver(service)
	key, err := bindings.Generic("same")
	require.NoError(t, err)

	first, err := resolver.Resolve(context.Background(), key)
	require.NoError(t, err)
	second, err := resolver.Resolve(context.Background(), key)
	require.NoError(t, err)

	require.Equal(t, first.ID, second.ID)
}

func TestResolver_KeepsCurrentSessionUnchanged(t *testing.T) {
	store := storememory.NewStore()
	manager, err := statemanager.NewManager(
		store,
		constants.DefaultSessionIdleExpiry,
		constants.DefaultArchiveRetention,
	)
	require.NoError(t, err)
	require.NoError(t, manager.Start(context.Background()))

	before, err := manager.CurrentSession(context.Background())
	require.NoError(t, err)

	service := &stateManagerService{manager: manager}
	key, err := bindings.Generic("chat-1")
	require.NoError(t, err)

	session, err := NewResolver(service).Resolve(context.Background(), key)
	require.NoError(t, err)

	after, err := manager.CurrentSession(context.Background())
	require.NoError(t, err)
	require.Equal(t, storage.DefaultSessionID, before)
	require.Equal(t, before, after)
	require.NotEqual(t, after, session.ID)
}

func TestResolver_ReturnsStoreErrorBeforeCreatingSession(t *testing.T) {
	expected := errors.New("binding store unavailable")
	service := &serviceStub{getErr: expected}
	resolver := NewResolver(service)

	_, err := resolver.Resolve(context.Background(), keyFromString("generic::chat-1:"))

	require.ErrorIs(t, err, expected)
	require.False(t, service.created)
}

func TestResolver_RejectsMissingServiceAndKey(t *testing.T) {
	_, err := (*Resolver)(nil).Resolve(context.Background(), keyFromString("generic::chat-1:"))
	require.EqualError(t, err, "gateway session resolver service is required")

	_, err = NewResolver(nil).Resolve(context.Background(), keyFromString("generic::chat-1:"))
	require.EqualError(t, err, "gateway session resolver service is required")

	_, err = NewResolver(&serviceStub{}).Resolve(context.Background(), keyFromString(" "))
	require.EqualError(t, err, "gateway binding key is required")
}

func TestResolver_ReturnsCreateAndSaveErrors(t *testing.T) {
	for _, tt := range []struct {
		name    string
		service *serviceStub
		err     error
	}{
		{
			name:    "create",
			service: &serviceStub{createErr: errors.New("create failed")},
			err:     errors.New("create failed"),
		},
		{
			name: "save",
			service: &serviceStub{
				createdSession: storage.Session{ID: createdSessionID},
				saveErr:        errors.New("save failed"),
			},
			err: errors.New("save failed"),
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewResolver(tt.service).Resolve(context.Background(), keyFromString("generic::chat-1:"))
			require.EqualError(t, err, tt.err.Error())
		})
	}
}

type serviceStub struct {
	binding        storage.GatewayBinding
	savedBinding   storage.GatewayBinding
	createdSession storage.Session
	getErr         error
	saveErr        error
	createErr      error
	bindingFound   bool
	created        bool
}

func (s *serviceStub) Respond(context.Context, string, agentcore.RespondOptions) (string, error) {
	return "", nil
}

func (s *serviceStub) CreateSession(context.Context, string) (storage.Session, error) {
	s.created = true
	if s.createErr != nil {
		return storage.Session{}, s.createErr
	}
	if s.createdSession.ID == "" {
		s.createdSession = storage.Session{ID: createdSessionID}
	}

	return s.createdSession, nil
}

func (s *serviceStub) SaveGatewayBinding(
	_ context.Context,
	binding storage.GatewayBinding,
) error {
	s.savedBinding = binding
	return s.saveErr
}

func (s *serviceStub) GetGatewayBinding(
	context.Context,
	string,
) (storage.GatewayBinding, bool, error) {
	return s.binding, s.bindingFound, s.getErr
}

type mapBackedService struct {
	bindings map[string]storage.GatewayBinding
	nextID   int
}

func newMapBackedService() *mapBackedService {
	return &mapBackedService{bindings: make(map[string]storage.GatewayBinding)}
}

func (s *mapBackedService) Respond(context.Context, string, agentcore.RespondOptions) (string, error) {
	return "", nil
}

func (s *mapBackedService) CreateSession(context.Context, string) (storage.Session, error) {
	s.nextID++
	return storage.Session{
		ID: nanoid.MustFromSeed(storage.SessionIDPrefix, fmt.Sprintf("generated-binding-%d", s.nextID), "binding"),
	}, nil
}

func (s *mapBackedService) SaveGatewayBinding(
	_ context.Context,
	binding storage.GatewayBinding,
) error {
	s.bindings[binding.Key] = binding
	return nil
}

func (s *mapBackedService) GetGatewayBinding(
	_ context.Context,
	key string,
) (storage.GatewayBinding, bool, error) {
	binding, ok := s.bindings[key]
	return binding, ok, nil
}

type stateManagerService struct {
	manager *statemanager.Manager
}

func (s *stateManagerService) Respond(context.Context, string, agentcore.RespondOptions) (string, error) {
	return "", nil
}

func (s *stateManagerService) CreateSession(ctx context.Context, id string) (storage.Session, error) {
	return s.manager.CreateSession(ctx, id)
}

func (s *stateManagerService) SaveGatewayBinding(
	ctx context.Context,
	binding storage.GatewayBinding,
) error {
	return s.manager.SaveGatewayBinding(ctx, binding)
}

func (s *stateManagerService) GetGatewayBinding(
	ctx context.Context,
	key string,
) (storage.GatewayBinding, bool, error) {
	return s.manager.GetGatewayBinding(ctx, key)
}

func keyFromString(value string) bindings.Key {
	return bindings.Key(value)
}
