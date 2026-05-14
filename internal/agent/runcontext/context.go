package runcontext

import (
	"context"
	"errors"
	"strings"
	"time"

	storage "github.com/wandxy/hand/internal/state/core"
)

const (
	// StateModeShared lets a child run read and write the same state namespace as its parent.
	StateModeShared = "shared"
	// StateModeIsolated gives a child run a separate state namespace.
	StateModeIsolated = "isolated"
	// StateModeReadonly lets a child run read parent state without writing back to it.
	StateModeReadonly = "readonly"

	// MemoryMetadataSourceProfile records the profile that produced a memory item.
	MemoryMetadataSourceProfile = "source_profile"
	// MemoryMetadataSourcePersonality records the personality that produced a memory item.
	MemoryMetadataSourcePersonality = "source_personality"
	// MemoryMetadataParentSessionID records the public parent session for child-produced memory.
	MemoryMetadataParentSessionID = "source_parent_session_id"
	// MemoryMetadataChildSessionID records the effective child session for child-produced memory.
	MemoryMetadataChildSessionID = "source_child_session_id"
	// MemoryMetadataRunID records the agent run that produced a memory item.
	MemoryMetadataRunID = "source_run_id"
	// MemoryMetadataStateMode records whether the producing run used shared, isolated, or readonly state.
	MemoryMetadataStateMode = "source_state_mode"
	// MemoryMetadataTrigger records the memory path that produced the item.
	MemoryMetadataTrigger = "source_trigger"
	// MemoryMetadataPublicSessionID records the user-facing session ID.
	MemoryMetadataPublicSessionID = "source_public_session_id"
	// MemoryMetadataEffectiveSessionID records the state namespace session ID used by the run.
	MemoryMetadataEffectiveSessionID = "source_effective_session_id"
)

type contextKey struct{}

// Context describes the profile, session, personality, state mode, and lineage for one agent run.
type Context struct {
	ProfileName string
	Session     Session
	Personality Personality
	State       State
	Lineage     Lineage
}

// Session separates the user-facing session ID from the effective state namespace ID.
type Session struct {
	PublicID    string
	EffectiveID string
}

// Personality identifies the active personality overlay for a run.
type Personality struct {
	Name string
}

// State describes how a run should access persisted state.
type State struct {
	Mode string
}

// Lineage links child runs back to their parent session and run metadata.
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

// PersonalityOptions controls how a named personality is applied to an existing context.
type PersonalityOptions struct {
	PersonalityName string
	StateMode       string
	ProfileName     string
}

// NewParent returns a root run context for a user-facing session ID.
func NewParent(sessionID string) (Context, error) {
	runCtx := Context{
		Session: Session{
			PublicID:    strings.TrimSpace(sessionID),
			EffectiveID: strings.TrimSpace(sessionID),
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
	if strings.TrimSpace(opts.ChildSessionID) == "" {
		return Context{}, errors.New("child session id is required")
	}

	childCtx := Context{
		ProfileName: strings.TrimSpace(opts.ProfileName),
		Session: Session{
			PublicID:    parent.Session.PublicID,
			EffectiveID: strings.TrimSpace(opts.ChildSessionID),
		},
		Personality: Personality{Name: strings.TrimSpace(opts.PersonalityName)},
		State:       State{Mode: strings.TrimSpace(opts.StateMode)},
		Lineage: Lineage{
			ParentSessionID: parent.Session.PublicID,
			ChildSessionID:  strings.TrimSpace(opts.ChildSessionID),
			RunID:           strings.TrimSpace(opts.RunID),
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
	if strings.TrimSpace(opts.PersonalityName) == "" {
		return Context{}, errors.New("personality name is required")
	}

	personalityCtx := parent
	personalityCtx.Personality.Name = strings.TrimSpace(opts.PersonalityName)
	personalityCtx.State.Mode = strings.TrimSpace(opts.StateMode)
	personalityCtx.ProfileName = strings.TrimSpace(opts.ProfileName)
	if personalityCtx.ProfileName == "" {
		personalityCtx.ProfileName = parent.ProfileName
	}

	return personalityCtx.Normalize()
}

// Normalize trims values, fills defaults, validates session IDs, and normalizes state mode.
func (runCtx Context) Normalize() (Context, error) {
	runCtx.ProfileName = strings.TrimSpace(runCtx.ProfileName)

	runCtx.Session.PublicID = strings.TrimSpace(runCtx.Session.PublicID)
	if runCtx.Session.PublicID == "" {
		runCtx.Session.PublicID = storage.DefaultSessionID
	}
	if err := storage.ValidateSessionID(runCtx.Session.PublicID); err != nil {
		return Context{}, err
	}

	runCtx.Session.EffectiveID = strings.TrimSpace(runCtx.Session.EffectiveID)
	if runCtx.Session.EffectiveID == "" {
		runCtx.Session.EffectiveID = runCtx.Session.PublicID
	}
	if err := storage.ValidateSessionID(runCtx.Session.EffectiveID); err != nil {
		return Context{}, err
	}

	runCtx.Lineage.ParentSessionID = strings.TrimSpace(runCtx.Lineage.ParentSessionID)
	if runCtx.Lineage.ParentSessionID != "" {
		if err := storage.ValidateSessionID(runCtx.Lineage.ParentSessionID); err != nil {
			return Context{}, err
		}
	}
	runCtx.Lineage.ChildSessionID = strings.TrimSpace(runCtx.Lineage.ChildSessionID)
	if runCtx.Lineage.ChildSessionID != "" {
		if err := storage.ValidateSessionID(runCtx.Lineage.ChildSessionID); err != nil {
			return Context{}, err
		}
	}

	runCtx.Lineage.RunID = strings.TrimSpace(runCtx.Lineage.RunID)
	runCtx.Personality.Name = strings.TrimSpace(runCtx.Personality.Name)
	runCtx.State.Mode = normalizeStateMode(runCtx.State.Mode)

	return runCtx, nil
}

// StateSessionID returns the effective session ID used for state isolation.
func (runCtx Context) StateSessionID() string {
	if value := strings.TrimSpace(runCtx.Session.EffectiveID); value != "" {
		return value
	}
	if value := strings.TrimSpace(runCtx.Session.PublicID); value != "" {
		return value
	}

	return storage.DefaultSessionID
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

// ApplyMemoryProvenance returns item with run lineage metadata attached.
func ApplyMemoryProvenance(
	item storage.MemoryItem,
	runCtx Context,
	trigger string,
) storage.MemoryItem {
	runCtx, err := runCtx.Normalize()
	if err != nil {
		return item
	}

	item = item.Clone()
	if item.Metadata == nil {
		item.Metadata = make(map[string]string)
	}

	setMetadata(item.Metadata, MemoryMetadataPublicSessionID, runCtx.Session.PublicID)
	setMetadata(item.Metadata, MemoryMetadataEffectiveSessionID, runCtx.Session.EffectiveID)
	setMetadata(item.Metadata, MemoryMetadataParentSessionID, runCtx.Lineage.ParentSessionID)
	setMetadata(item.Metadata, MemoryMetadataChildSessionID, getChildSessionID(runCtx))
	setMetadata(item.Metadata, MemoryMetadataRunID, runCtx.Lineage.RunID)
	setMetadata(item.Metadata, MemoryMetadataSourcePersonality, runCtx.Personality.Name)
	setMetadata(item.Metadata, MemoryMetadataStateMode, runCtx.State.Mode)
	setMetadata(item.Metadata, MemoryMetadataSourceProfile, runCtx.ProfileName)
	setMetadata(item.Metadata, MemoryMetadataTrigger, trigger)

	for index := range item.SourceLinks {
		fillSourceLink(&item.SourceLinks[index], runCtx, trigger)
	}

	return item
}

func normalizeStateMode(value string) string {
	switch strings.TrimSpace(strings.ToLower(value)) {
	case StateModeIsolated:
		return StateModeIsolated
	case StateModeReadonly:
		return StateModeReadonly
	default:
		return StateModeShared
	}
}

func getChildSessionID(runCtx Context) string {
	if strings.TrimSpace(runCtx.Lineage.ParentSessionID) == "" {
		return ""
	}
	if strings.TrimSpace(runCtx.Lineage.ChildSessionID) != "" {
		return runCtx.Lineage.ChildSessionID
	}

	return runCtx.Session.EffectiveID
}

func setMetadata(metadata map[string]string, key string, value string) {
	if value = strings.TrimSpace(value); value != "" {
		metadata[key] = value
	}
}

func fillSourceLink(link *storage.MemorySourceLink, runCtx Context, trigger string) {
	if link == nil {
		return
	}
	if link.SourceProfile == "" {
		link.SourceProfile = runCtx.ProfileName
	}
	if link.SourcePersonality == "" {
		link.SourcePersonality = runCtx.Personality.Name
	}
	if link.ParentSessionID == "" {
		link.ParentSessionID = runCtx.Lineage.ParentSessionID
	}
	if link.ChildSessionID == "" {
		link.ChildSessionID = getChildSessionID(runCtx)
	}
	if link.RunID == "" {
		link.RunID = runCtx.Lineage.RunID
	}
	if link.StateMode == "" {
		link.StateMode = runCtx.State.Mode
	}
	if link.SourceTrigger == "" {
		link.SourceTrigger = strings.TrimSpace(trigger)
	}
}
