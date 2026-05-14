package runcontext

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	storage "github.com/wandxy/hand/internal/state/core"
	"github.com/wandxy/hand/pkg/nanoid"
)

func TestNewParent_DefaultsPublicAndEffectiveSession(t *testing.T) {
	runCtx, err := NewParent("")

	require.NoError(t, err)
	require.Equal(t, storage.DefaultSessionID, runCtx.Session.PublicID)
	require.Equal(t, storage.DefaultSessionID, runCtx.Session.EffectiveID)
	require.Equal(t, storage.DefaultSessionID, runCtx.StateSessionID())
}

func TestNewChild_KeepsPublicSessionAndUsesChildStateSession(t *testing.T) {
	parentID := nanoid.MustFromSeed(storage.SessionIDPrefix, "parent", "RunContextTestSeed")
	childID := nanoid.MustFromSeed(storage.SessionIDPrefix, "child", "RunContextTestSeed")
	spawnedAt := time.Date(2026, 5, 14, 9, 0, 0, 0, time.UTC)

	parent, err := NewParent(parentID)
	require.NoError(t, err)
	child, err := parent.NewChild(ChildOptions{
		ChildSessionID:  childID,
		RunID:           "run_1",
		PersonalityName: "researcher",
		StateMode:       StateModeIsolated,
		ProfileName:     "work",
		SpawnedAt:       spawnedAt,
	})

	require.NoError(t, err)
	require.Equal(t, parentID, child.Session.PublicID)
	require.Equal(t, childID, child.Session.EffectiveID)
	require.Equal(t, parentID, child.Lineage.ParentSessionID)
	require.Equal(t, childID, child.Lineage.ChildSessionID)
	require.Equal(t, "run_1", child.Lineage.RunID)
	require.Equal(t, "researcher", child.Personality.Name)
	require.Equal(t, StateModeIsolated, child.State.Mode)
	require.Equal(t, "work", child.ProfileName)
	require.Equal(t, spawnedAt, child.Lineage.SpawnedAt)
}

func TestNewChild_RequiresChildSession(t *testing.T) {
	parent, err := NewParent(storage.DefaultSessionID)
	require.NoError(t, err)

	_, err = parent.NewChild(ChildOptions{})
	require.ErrorContains(t, err, "child session id is required")
}

func TestNewChild_RejectsInvalidParent(t *testing.T) {
	childID := nanoid.MustFromSeed(storage.SessionIDPrefix, "child", "InvalidParentSeed")

	_, err := (Context{Session: Session{PublicID: "session-1"}}).NewChild(ChildOptions{
		ChildSessionID: childID,
	})

	require.ErrorContains(t, err, "session id must be a valid ses_ nanoid")
}

func TestNewChild_InheritsParentDefaults(t *testing.T) {
	parentID := nanoid.MustFromSeed(storage.SessionIDPrefix, "parent", "ChildDefaultsSeed")
	childID := nanoid.MustFromSeed(storage.SessionIDPrefix, "child", "ChildDefaultsSeed")
	parent, err := (Context{
		ProfileName: "work",
		Session: Session{
			PublicID: parentID,
		},
	}).NewPersonality(PersonalityOptions{
		PersonalityName: "researcher",
	})
	require.NoError(t, err)

	child, err := parent.NewChild(ChildOptions{ChildSessionID: childID})

	require.NoError(t, err)
	require.Equal(t, "work", child.ProfileName)
	require.Equal(t, "researcher", child.Personality.Name)
	require.Equal(t, StateModeShared, child.State.Mode)
}

func TestNewPersonality_KeepsPublicAndEffectiveSession(t *testing.T) {
	sessionID := nanoid.MustFromSeed(storage.SessionIDPrefix, "named", "RunContextTestSeed")
	parent, err := NewParent(sessionID)
	require.NoError(t, err)

	runCtx, err := parent.NewPersonality(PersonalityOptions{
		PersonalityName: "researcher",
		StateMode:       StateModeReadonly,
		ProfileName:     "work",
	})

	require.NoError(t, err)
	require.Equal(t, sessionID, runCtx.Session.PublicID)
	require.Equal(t, sessionID, runCtx.Session.EffectiveID)
	require.Equal(t, "researcher", runCtx.Personality.Name)
	require.Equal(t, StateModeReadonly, runCtx.State.Mode)
	require.Equal(t, "work", runCtx.ProfileName)
	require.Empty(t, runCtx.Lineage.ParentSessionID)
}

func TestNewPersonality_RequiresName(t *testing.T) {
	parent, err := NewParent(storage.DefaultSessionID)
	require.NoError(t, err)

	_, err = parent.NewPersonality(PersonalityOptions{})

	require.ErrorContains(t, err, "personality name is required")
}

func TestNewPersonality_RejectsInvalidParent(t *testing.T) {
	_, err := (Context{Session: Session{PublicID: "session-1"}}).NewPersonality(PersonalityOptions{
		PersonalityName: "researcher",
	})

	require.ErrorContains(t, err, "session id must be a valid ses_ nanoid")
}

func TestNewPersonality_InheritsParentProfile(t *testing.T) {
	sessionID := nanoid.MustFromSeed(storage.SessionIDPrefix, "named", "PersonalityDefaultsSeed")
	parent := Context{
		ProfileName: "work",
		Session: Session{
			PublicID: sessionID,
		},
	}

	runCtx, err := parent.NewPersonality(PersonalityOptions{PersonalityName: "researcher"})

	require.NoError(t, err)
	require.Equal(t, "work", runCtx.ProfileName)
	require.Equal(t, StateModeShared, runCtx.State.Mode)
}

func TestContext_NormalizeRejectsInvalidPublicOrEffectiveSession(t *testing.T) {
	_, err := Context{Session: Session{PublicID: "session-1"}}.Normalize()
	require.ErrorContains(t, err, "session id must be a valid ses_ nanoid")

	_, err = Context{Session: Session{PublicID: storage.DefaultSessionID, EffectiveID: "session-2"}}.Normalize()
	require.ErrorContains(t, err, "session id must be a valid ses_ nanoid")
}

func TestContext_NormalizeRejectsInvalidLineageSessions(t *testing.T) {
	_, err := Context{
		Session: Session{PublicID: storage.DefaultSessionID},
		Lineage: Lineage{
			ParentSessionID: "session-1",
		},
	}.Normalize()
	require.ErrorContains(t, err, "session id must be a valid ses_ nanoid")

	_, err = Context{
		Session: Session{PublicID: storage.DefaultSessionID},
		Lineage: Lineage{
			ChildSessionID: "session-2",
		},
	}.Normalize()
	require.ErrorContains(t, err, "session id must be a valid ses_ nanoid")
}

func TestContext_StateSessionIDFallsBackToPublicAndDefault(t *testing.T) {
	sessionID := nanoid.MustFromSeed(storage.SessionIDPrefix, "public", "StateSessionSeed")

	require.Equal(t, sessionID, Context{Session: Session{PublicID: " " + sessionID + " "}}.StateSessionID())
	require.Equal(t, storage.DefaultSessionID, Context{}.StateSessionID())
}

func TestFromContext_ReturnsNormalizedRunContext(t *testing.T) {
	ctx := WithContext(context.Background(), Context{Session: Session{PublicID: " default "}})
	runCtx, ok := FromContext(ctx)

	require.True(t, ok)
	require.Equal(t, storage.DefaultSessionID, runCtx.Session.PublicID)
	require.Equal(t, storage.DefaultSessionID, runCtx.Session.EffectiveID)
}

func TestFromContext_HandlesNilMissingAndInvalidContexts(t *testing.T) {
	ctx := WithContext(nil, Context{Session: Session{PublicID: storage.DefaultSessionID}})
	runCtx, ok := FromContext(ctx)
	require.True(t, ok)
	require.Equal(t, storage.DefaultSessionID, runCtx.StateSessionID())

	invalidCtx := WithContext(context.Background(), Context{Session: Session{PublicID: "session-1"}})
	_, ok = FromContext(invalidCtx)
	require.False(t, ok)

	_, ok = FromContext(nil)
	require.False(t, ok)

	_, ok = FromContext(context.Background())
	require.False(t, ok)

	_, ok = FromContext(context.WithValue(context.Background(), contextKey{}, Context{
		Session: Session{PublicID: "session-1"},
	}))
	require.False(t, ok)
}

func TestApplyMemoryProvenance_AddsLineageMetadataAndSourceLinks(t *testing.T) {
	parentID := nanoid.MustFromSeed(storage.SessionIDPrefix, "parent", "MemoryLineageTestSeed")
	childID := nanoid.MustFromSeed(storage.SessionIDPrefix, "child", "MemoryLineageTestSeed")
	parent, err := NewParent(parentID)
	require.NoError(t, err)
	child, err := parent.NewChild(ChildOptions{
		ChildSessionID:  childID,
		RunID:           "run_memory",
		PersonalityName: "researcher",
		StateMode:       StateModeReadonly,
		ProfileName:     "work",
	})
	require.NoError(t, err)

	item := ApplyMemoryProvenance(storage.MemoryItem{
		Metadata:    map[string]string{"source_session_id": parentID},
		SourceLinks: []storage.MemorySourceLink{{SessionID: parentID, MessageIDs: []uint{1}}},
	}, child, "tool_write")

	require.Equal(t, parentID, item.Metadata[MemoryMetadataPublicSessionID])
	require.Equal(t, childID, item.Metadata[MemoryMetadataEffectiveSessionID])
	require.Equal(t, parentID, item.Metadata[MemoryMetadataParentSessionID])
	require.Equal(t, childID, item.Metadata[MemoryMetadataChildSessionID])
	require.Equal(t, "run_memory", item.Metadata[MemoryMetadataRunID])
	require.Equal(t, "researcher", item.Metadata[MemoryMetadataSourcePersonality])
	require.Equal(t, StateModeReadonly, item.Metadata[MemoryMetadataStateMode])
	require.Equal(t, "work", item.Metadata[MemoryMetadataSourceProfile])
	require.Equal(t, "tool_write", item.Metadata[MemoryMetadataTrigger])
	require.Equal(t, parentID, item.SourceLinks[0].ParentSessionID)
	require.Equal(t, childID, item.SourceLinks[0].ChildSessionID)
	require.Equal(t, "run_memory", item.SourceLinks[0].RunID)
	require.Equal(t, "researcher", item.SourceLinks[0].SourcePersonality)
	require.Equal(t, "tool_write", item.SourceLinks[0].SourceTrigger)
}

func TestApplyMemoryProvenance_SkipsInvalidRunContext(t *testing.T) {
	item := storage.MemoryItem{Metadata: map[string]string{"existing": "value"}}

	result := ApplyMemoryProvenance(item, Context{Session: Session{PublicID: "session-1"}}, "tool_write")

	require.Equal(t, map[string]string{"existing": "value"}, result.Metadata)
}

func TestApplyMemoryProvenance_PreservesExistingSourceLinkFields(t *testing.T) {
	parentID := nanoid.MustFromSeed(storage.SessionIDPrefix, "parent", "PreserveSourceLinkSeed")
	childID := nanoid.MustFromSeed(storage.SessionIDPrefix, "child", "PreserveSourceLinkSeed")
	parent, err := NewParent(parentID)
	require.NoError(t, err)
	child, err := parent.NewChild(ChildOptions{
		ChildSessionID:  childID,
		RunID:           "run_memory",
		PersonalityName: "researcher",
		StateMode:       StateModeReadonly,
		ProfileName:     "work",
	})
	require.NoError(t, err)

	item := ApplyMemoryProvenance(storage.MemoryItem{
		SourceLinks: []storage.MemorySourceLink{{
			SourceProfile:     "existing_profile",
			SourcePersonality: "existing_personality",
			ParentSessionID:   "existing_parent",
			ChildSessionID:    "existing_child",
			RunID:             "existing_run",
			StateMode:         "existing_state",
			SourceTrigger:     "existing_trigger",
		}},
	}, child, "tool_write")

	require.Equal(t, "existing_profile", item.SourceLinks[0].SourceProfile)
	require.Equal(t, "existing_personality", item.SourceLinks[0].SourcePersonality)
	require.Equal(t, "existing_parent", item.SourceLinks[0].ParentSessionID)
	require.Equal(t, "existing_child", item.SourceLinks[0].ChildSessionID)
	require.Equal(t, "existing_run", item.SourceLinks[0].RunID)
	require.Equal(t, "existing_state", item.SourceLinks[0].StateMode)
	require.Equal(t, "existing_trigger", item.SourceLinks[0].SourceTrigger)
}

func TestGetChildSessionID_FallsBackToEffectiveSessionOnlyForChildren(t *testing.T) {
	childID := nanoid.MustFromSeed(storage.SessionIDPrefix, "child", "ChildSessionFallbackSeed")

	require.Empty(t, getChildSessionID(Context{
		Session: Session{EffectiveID: childID},
	}))
	require.Equal(t, childID, getChildSessionID(Context{
		Session: Session{EffectiveID: childID},
		Lineage: Lineage{ParentSessionID: storage.DefaultSessionID},
	}))
}

func TestFillSourceLink_IgnoresNilLinks(t *testing.T) {
	require.NotPanics(t, func() {
		fillSourceLink(nil, Context{}, "tool_write")
	})
}
