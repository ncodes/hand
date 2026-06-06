package gateway

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wandxy/hand/internal/constants"
	storage "github.com/wandxy/hand/internal/state/core"
	statemanager "github.com/wandxy/hand/internal/state/manager"
	"github.com/wandxy/hand/internal/state/storememory"
	"github.com/wandxy/hand/pkg/gateway/bindings"
)

func TestSessionResolver_ReusesExistingBinding(t *testing.T) {
	service := &sessionResolverServiceStub{
		binding: storage.GatewayBinding{
			Key:       "generic::chat-1:",
			SessionID: existingSessionID,
		},
		bindingFound: true,
	}
	resolver := NewSessionResolver(service)

	session, err := resolver.Resolve(context.Background(), keyFromString("generic::chat-1:"))

	require.NoError(t, err)
	require.Equal(t, existingSessionID, session.ID)
	require.False(t, service.created)
}

func TestSessionResolver_CreatesAndPersistsMissingBinding(t *testing.T) {
	service := &sessionResolverServiceStub{
		createdSession: storage.Session{ID: createdSessionID},
	}
	resolver := NewSessionResolver(service)

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

func TestSessionResolver_DifferentSourcesProduceDifferentSessions(t *testing.T) {
	service := newMapBackedSessionService()
	resolver := NewSessionResolver(service)
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

func TestSessionResolver_SameKeyResolvesSameSession(t *testing.T) {
	service := newMapBackedSessionService()
	resolver := NewSessionResolver(service)
	key, err := bindings.Generic("same")
	require.NoError(t, err)

	first, err := resolver.Resolve(context.Background(), key)
	require.NoError(t, err)
	second, err := resolver.Resolve(context.Background(), key)
	require.NoError(t, err)

	require.Equal(t, first.ID, second.ID)
}

func TestSessionResolver_KeepsCurrentSessionUnchanged(t *testing.T) {
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

	service := &stateManagerGatewayService{manager: manager}
	key, err := bindings.Generic("chat-1")
	require.NoError(t, err)

	session, err := NewSessionResolver(service).Resolve(context.Background(), key)
	require.NoError(t, err)

	after, err := manager.CurrentSession(context.Background())
	require.NoError(t, err)
	require.Equal(t, storage.DefaultSessionID, before)
	require.Equal(t, before, after)
	require.NotEqual(t, after, session.ID)
}

func TestSessionResolver_ReturnsStoreErrorBeforeCreatingSession(t *testing.T) {
	expected := errors.New("binding store unavailable")
	service := &sessionResolverServiceStub{getErr: expected}
	resolver := NewSessionResolver(service)

	_, err := resolver.Resolve(context.Background(), keyFromString("generic::chat-1:"))

	require.ErrorIs(t, err, expected)
	require.False(t, service.created)
}

func TestSessionResolver_RejectsMissingServiceAndKey(t *testing.T) {
	_, err := (*SessionResolver)(nil).Resolve(context.Background(), keyFromString("generic::chat-1:"))
	require.EqualError(t, err, "gateway session resolver service is required")

	_, err = NewSessionResolver(nil).Resolve(context.Background(), keyFromString("generic::chat-1:"))
	require.EqualError(t, err, "gateway session resolver service is required")

	_, err = NewSessionResolver(&sessionResolverServiceStub{}).Resolve(context.Background(), keyFromString(" "))
	require.EqualError(t, err, "gateway binding key is required")
}

func TestSessionResolver_ReturnsCreateAndSaveErrors(t *testing.T) {
	for _, tt := range []struct {
		name    string
		service *sessionResolverServiceStub
		err     error
	}{
		{
			name:    "create",
			service: &sessionResolverServiceStub{createErr: errors.New("create failed")},
			err:     errors.New("create failed"),
		},
		{
			name: "save",
			service: &sessionResolverServiceStub{
				createdSession: storage.Session{ID: createdSessionID},
				saveErr:        errors.New("save failed"),
			},
			err: errors.New("save failed"),
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewSessionResolver(tt.service).Resolve(context.Background(), keyFromString("generic::chat-1:"))
			require.EqualError(t, err, tt.err.Error())
		})
	}
}
