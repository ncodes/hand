package trace

import (
	"context"
	"time"
)

type Factory interface {
	NewSession(context.Context, RunContext) Session
}

type Session interface {
	ID() string
	Record(string, any)
	Close()
}

type RunContext struct {
	SessionID          string
	PublicSessionID    string
	EffectiveSessionID string
	ChildSessionID     string
	ParentSessionID    string
	RunID              string
	ProfileName        string
}

type Event struct {
	SessionID string
	Type      string
	Timestamp time.Time
	Payload   any
}
