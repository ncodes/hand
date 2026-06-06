package gateway

import (
	"context"
	"errors"
	"strings"
	"time"

	storage "github.com/wandxy/hand/internal/state/core"
	"github.com/wandxy/hand/pkg/gateway/bindings"
)

type SessionResolver struct {
	service AgentService
	now     func() time.Time
}

func NewSessionResolver(service AgentService) *SessionResolver {
	return &SessionResolver{
		service: service,
		now:     func() time.Time { return time.Now().UTC() },
	}
}

func (r *SessionResolver) Resolve(ctx context.Context, key bindings.Key) (storage.Session, error) {
	if r == nil || r.service == nil {
		return storage.Session{}, errors.New("gateway session resolver service is required")
	}

	keyString := strings.TrimSpace(key.String())
	if keyString == "" {
		return storage.Session{}, errors.New("gateway binding key is required")
	}

	binding, ok, err := r.service.GetGatewayBinding(ctx, keyString)
	if err != nil {
		return storage.Session{}, err
	}
	if ok {
		return storage.Session{ID: binding.SessionID}, nil
	}

	session, err := r.service.CreateSession(ctx, "")
	if err != nil {
		return storage.Session{}, err
	}

	now := r.now().UTC()
	if err := r.service.SaveGatewayBinding(ctx, storage.GatewayBinding{
		Key:       keyString,
		SessionID: session.ID,
		CreatedAt: now,
		UpdatedAt: now,
	}); err != nil {
		return storage.Session{}, err
	}

	return session, nil
}
