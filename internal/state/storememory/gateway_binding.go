package storememory

import (
	"context"
	"errors"
	"time"

	base "github.com/wandxy/morph/internal/state/core"
	"github.com/wandxy/morph/pkg/stringx"
)

func (s *Store) SaveGatewayBinding(ctx context.Context, binding base.GatewayBinding) error {
	_ = ctx

	if s == nil {
		return errors.New("store is required")
	}

	key := stringx.String(binding.Key).Trim()
	if key == "" {
		return errors.New("gateway binding key is required")
	}

	sessionID := stringx.String(binding.SessionID).Trim()
	if err := base.ValidateSessionID(sessionID); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.sessions[sessionID]; !ok {
		return errors.New("session not found")
	}

	now := time.Now().UTC()
	existing, ok := s.gatewayBindings[key]
	if ok && !existing.CreatedAt.IsZero() {
		binding.CreatedAt = existing.CreatedAt
	} else if binding.CreatedAt.IsZero() {
		binding.CreatedAt = now
	}
	if binding.UpdatedAt.IsZero() {
		binding.UpdatedAt = now
	}

	binding.Key = key
	binding.SessionID = sessionID
	s.gatewayBindings[key] = binding

	return nil
}

func (s *Store) GetGatewayBinding(ctx context.Context, key string) (base.GatewayBinding, bool, error) {
	_ = ctx

	if s == nil {
		return base.GatewayBinding{}, false, errors.New("store is required")
	}

	key = stringx.String(key).Trim()
	if key == "" {
		return base.GatewayBinding{}, false, errors.New("gateway binding key is required")
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	binding, ok := s.gatewayBindings[key]
	return binding, ok, nil
}
