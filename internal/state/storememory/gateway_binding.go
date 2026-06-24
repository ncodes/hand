package storememory

import (
	"context"
	"errors"
	"strings"
	"time"

	base "github.com/wandxy/morph/internal/state/core"
)

func (s *Store) SaveGatewayBinding(ctx context.Context, binding base.GatewayBinding) error {
	_ = ctx

	if s == nil {
		return errors.New("store is required")
	}

	key := strings.TrimSpace(binding.Key)
	if key == "" {
		return errors.New("gateway binding key is required")
	}

	sessionID := strings.TrimSpace(binding.SessionID)
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

	key = strings.TrimSpace(key)
	if key == "" {
		return base.GatewayBinding{}, false, errors.New("gateway binding key is required")
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	binding, ok := s.gatewayBindings[key]
	return binding, ok, nil
}
