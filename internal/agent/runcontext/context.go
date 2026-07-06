package runcontext

import (
	"context"
	"errors"
	"time"

	statecore "github.com/wandxy/morph/internal/state/core"
	"github.com/wandxy/morph/pkg/str"
)

const (
	// DefaultSessionID is the canonical fallback session for root runs.
	DefaultSessionID = statecore.DefaultSessionID
	// SessionIDPrefix is the required prefix for generated run session IDs.
	SessionIDPrefix = statecore.SessionIDPrefix
	// StateModeShared lets a child run read and write the same state namespace as its parent.
	StateModeShared = "shared"
	// StateModeIsolated gives a child run a separate state namespace.
	StateModeIsolated = "isolated"
	// StateModeReadonly lets a child run read parent state without writing back to it.
	StateModeReadonly = "readonly"
)

type contextKey struct{}

// Context describes the profile, session, personality, state mode, and lineage
// for one agent-managed run.
//
// A run can have two session identifiers:
//   - Session.PublicID is the user-facing conversation/session.
//   - Session.EffectiveID is the namespace used for mutable run state.
//
// Parent runs usually use the same value for both IDs. Child runs keep the
// public parent session for user-visible provenance while using their own
// effective session when state isolation is requested.
type Context struct {
	ProfileName string
	Session     Session
	Personality Personality
	State       State
	Lineage     Lineage
}

// Session separates user-visible session identity from the state namespace used
// while executing a run.
type Session struct {
	PublicID    string
	EffectiveID string
}

// Personality identifies the active personality overlay for a run.
type Personality struct {
	Name string
}

// State describes how a run should read or write persisted state.
type State struct {
	Mode string
}

// Lineage links child runs back to their parent session and spawning run.
type Lineage struct {
	ParentSessionID string
	ChildSessionID  string
	RunID           string
	SpawnedAt       time.Time
	CompletedAt     time.Time
}

// ChildOptions controls how a child run is derived from a parent context.
type ChildOptions struct {
	ChildSessionID  string
	RunID           string
	PersonalityName string
	StateMode       string
	ProfileName     string
	SpawnedAt       time.Time
	CompletedAt     time.Time
}

// PersonalityOptions controls how a named personality is applied to an existing
// run context without changing the session lineage.
type PersonalityOptions struct {
	PersonalityName string
	StateMode       string
	ProfileName     string
}

// NewParent returns a root run context for a user-facing session ID.
func NewParent(sessionID string) (Context, error) {
	stringValue1 := str.String(sessionID)
	stringValue2 := str.String(sessionID)
	runCtx := Context{
		Session: Session{
			PublicID:    stringValue1.Trim(),
			EffectiveID: stringValue2.Trim(),
		},
	}

	return runCtx.Normalize()
}

// NewChild returns a child run context derived from runCtx.
func (runCtx Context) NewChild(opts ChildOptions) (Context, error) {
	parent, err := runCtx.Normalize()
	if err != nil {
		return Context{}, err
	}
	stringValue3 := str.String(opts.ChildSessionID)
	if stringValue3.Trim() == "" {
		return Context{}, errors.New("child session id is required")
	}
	stringValue4 := str.String(opts.ProfileName)
	stringValue5 := str.String(opts.ChildSessionID)
	stringValue6 := str.String(opts.PersonalityName)
	stringValue7 := str.String(opts.StateMode)
	stringValue8 := str.String(opts.ChildSessionID)
	stringValue9 := str.String(opts.RunID)
	childCtx := Context{
		ProfileName: stringValue4.Trim(),
		Session: Session{
			PublicID:    parent.Session.PublicID,
			EffectiveID: stringValue5.Trim(),
		},
		Personality: Personality{Name: stringValue6.Trim()},
		State:       State{Mode: stringValue7.Trim()},
		Lineage: Lineage{
			ParentSessionID: parent.Session.PublicID,
			ChildSessionID:  stringValue8.Trim(),
			RunID:           stringValue9.Trim(),
			SpawnedAt:       opts.SpawnedAt,
			CompletedAt:     opts.CompletedAt,
		},
	}
	if childCtx.ProfileName == "" {
		childCtx.ProfileName = parent.ProfileName
	}
	if childCtx.Personality.Name == "" {
		childCtx.Personality.Name = parent.Personality.Name
	}
	if childCtx.State.Mode == "" {
		childCtx.State.Mode = StateModeShared
	}

	return childCtx.Normalize()
}

// NewPersonality returns runCtx with a named personality overlay applied.
func (runCtx Context) NewPersonality(opts PersonalityOptions) (Context, error) {
	parent, err := runCtx.Normalize()
	if err != nil {
		return Context{}, err
	}
	stringValue10 := str.String(opts.PersonalityName)
	if stringValue10.Trim() == "" {
		return Context{}, errors.New("personality name is required")
	}

	personalityCtx := parent
	stringValue11 := str.String(opts.PersonalityName)
	personalityCtx.Personality.Name = stringValue11.Trim()
	stringValue12 := str.String(opts.StateMode)
	personalityCtx.State.Mode = stringValue12.Trim()
	stringValue13 := str.String(opts.ProfileName)
	personalityCtx.ProfileName = stringValue13.Trim()
	if personalityCtx.ProfileName == "" {
		personalityCtx.ProfileName = parent.ProfileName
	}

	return personalityCtx.Normalize()
}

// Normalize trims values, fills default session/state fields, and validates all
// session IDs carried by the run context.
func (runCtx Context) Normalize() (Context, error) {
	stringValue14 := str.String(runCtx.ProfileName)
	runCtx.ProfileName = stringValue14.Trim()
	stringValue15 := str.String(runCtx.Session.PublicID)
	runCtx.Session.PublicID = stringValue15.Trim()
	if runCtx.Session.PublicID == "" {
		runCtx.Session.PublicID = DefaultSessionID
	}
	if err := ValidateSessionID(runCtx.Session.PublicID); err != nil {
		return Context{}, err
	}
	stringValue16 := str.String(runCtx.Session.EffectiveID)
	runCtx.Session.EffectiveID = stringValue16.Trim()
	if runCtx.Session.EffectiveID == "" {
		runCtx.Session.EffectiveID = runCtx.Session.PublicID
	}
	if err := ValidateSessionID(runCtx.Session.EffectiveID); err != nil {
		return Context{}, err
	}
	stringValue17 := str.String(runCtx.Lineage.ParentSessionID)
	runCtx.Lineage.ParentSessionID = stringValue17.Trim()
	if runCtx.Lineage.ParentSessionID != "" {
		if err := ValidateSessionID(runCtx.Lineage.ParentSessionID); err != nil {
			return Context{}, err
		}
	}
	stringValue18 := str.String(runCtx.Lineage.ChildSessionID)
	runCtx.Lineage.ChildSessionID = stringValue18.Trim()
	if runCtx.Lineage.ChildSessionID != "" {
		if err := ValidateSessionID(runCtx.Lineage.ChildSessionID); err != nil {
			return Context{}, err
		}
	}
	stringValue19 := str.String(runCtx.Lineage.RunID)
	runCtx.Lineage.RunID = stringValue19.Trim()
	stringValue20 := str.String(runCtx.Personality.Name)
	runCtx.Personality.Name = stringValue20.Trim()
	runCtx.State.Mode = normalizeStateMode(runCtx.State.Mode)

	return runCtx, nil
}

// StateSessionID returns the effective session ID used for state isolation.
func (runCtx Context) StateSessionID() string {
	stringValue21 := str.String(runCtx.Session.EffectiveID)
	if value := stringValue21.Trim(); value != "" {
		return value
	}
	stringValue22 := str.String(runCtx.Session.PublicID)
	if value := stringValue22.Trim(); value != "" {
		return value
	}

	return DefaultSessionID
}

// WithContext stores a normalized run context in ctx.
func WithContext(ctx context.Context, runCtx Context) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	runCtx, err := runCtx.Normalize()
	if err != nil {
		return ctx
	}

	return context.WithValue(ctx, contextKey{}, runCtx)
}

// FromContext loads a normalized run context from ctx.
func FromContext(ctx context.Context) (Context, bool) {
	if ctx == nil {
		return Context{}, false
	}

	runCtx, ok := ctx.Value(contextKey{}).(Context)
	if !ok {
		return Context{}, false
	}

	runCtx, err := runCtx.Normalize()
	if err != nil {
		return Context{}, false
	}

	return runCtx, true
}

func normalizeStateMode(value string) string {
	stringValue23 := str.String(value)
	switch stringValue23.Normalized() {
	case StateModeIsolated:
		return StateModeIsolated
	case StateModeReadonly:
		return StateModeReadonly
	default:
		return StateModeShared
	}
}

// ValidateSessionID validates agent run session IDs with the same rules as
// durable state sessions.
func ValidateSessionID(id string) error {
	return statecore.ValidateSessionID(id)
}
