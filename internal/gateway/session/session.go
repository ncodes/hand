package session

import (
	"context"
	"errors"
	"strings"
	"time"

	storage "github.com/wandxy/hand/internal/state/core"
	agentcore "github.com/wandxy/hand/pkg/agent"
	"github.com/wandxy/hand/pkg/gateway/bindings"
)

type Service interface {
	Respond(context.Context, string, agentcore.RespondOptions) (string, error)
	CreateSession(context.Context, string) (storage.Session, error)
	SaveGatewayBinding(context.Context, storage.GatewayBinding) error
	GetGatewayBinding(context.Context, string) (storage.GatewayBinding, bool, error)
}

type Resolver struct {
	service Service
	now     func() time.Time
}

func NewResolver(service Service) *Resolver {
	return &Resolver{
		service: service,
		now:     func() time.Time { return time.Now().UTC() },
	}
}

func (r *Resolver) Resolve(ctx context.Context, key bindings.Key) (storage.Session, error) {
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
