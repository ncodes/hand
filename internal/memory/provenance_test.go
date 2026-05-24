package memory

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wandxy/hand/pkg/agent/runcontext"
	"github.com/wandxy/hand/pkg/nanoid"
)

func TestApplyRunProvenance_AddsLineageMetadataAndSourceLinks(t *testing.T) {
	parentID := nanoid.MustFromSeed(runcontext.SessionIDPrefix, "parent", "MemoryLineageTestSeed")
	childID := nanoid.MustFromSeed(runcontext.SessionIDPrefix, "child", "MemoryLineageTestSeed")
	parent, err := runcontext.NewParent(parentID)
	require.NoError(t, err)
	child, err := parent.NewChild(runcontext.ChildOptions{
		ChildSessionID:  childID,
		RunID:           "run_memory",
		PersonalityName: "researcher",
		StateMode:       runcontext.StateModeReadonly,
		ProfileName:     "work",
	})
	require.NoError(t, err)

	item := ApplyRunProvenance(MemoryItem{
		Metadata:    map[string]string{"source_session_id": parentID},
		SourceLinks: []SourceLink{{SessionID: parentID, MessageIDs: []uint{1}}},
	}, child, "tool_write")

	require.Equal(t, parentID, item.Metadata[runcontext.MemoryMetadataPublicSessionID])
	require.Equal(t, childID, item.Metadata[runcontext.MemoryMetadataEffectiveSessionID])
	require.Equal(t, parentID, item.Metadata[runcontext.MemoryMetadataParentSessionID])
	require.Equal(t, childID, item.Metadata[runcontext.MemoryMetadataChildSessionID])
	require.Equal(t, "run_memory", item.Metadata[runcontext.MemoryMetadataRunID])
	require.Equal(t, "researcher", item.Metadata[runcontext.MemoryMetadataSourcePersonality])
	require.Equal(t, runcontext.StateModeReadonly, item.Metadata[runcontext.MemoryMetadataStateMode])
	require.Equal(t, "work", item.Metadata[runcontext.MemoryMetadataSourceProfile])
	require.Equal(t, "tool_write", item.Metadata[runcontext.MemoryMetadataTrigger])
	require.Equal(t, parentID, item.SourceLinks[0].ParentSessionID)
	require.Equal(t, childID, item.SourceLinks[0].ChildSessionID)
	require.Equal(t, "run_memory", item.SourceLinks[0].RunID)
	require.Equal(t, "researcher", item.SourceLinks[0].SourcePersonality)
	require.Equal(t, "tool_write", item.SourceLinks[0].SourceTrigger)
}

func TestApplyRunProvenance_SkipsInvalidRunContext(t *testing.T) {
	item := MemoryItem{Metadata: map[string]string{"existing": "value"}}

	result := ApplyRunProvenance(item, runcontext.Context{Session: runcontext.Session{PublicID: "session-1"}}, "tool_write")

	require.Equal(t, map[string]string{"existing": "value"}, result.Metadata)
}

func TestApplyRunProvenance_PreservesExistingSourceLinkFields(t *testing.T) {
	parentID := nanoid.MustFromSeed(runcontext.SessionIDPrefix, "parent", "PreserveSourceLinkSeed")
	childID := nanoid.MustFromSeed(runcontext.SessionIDPrefix, "child", "PreserveSourceLinkSeed")
	parent, err := runcontext.NewParent(parentID)
	require.NoError(t, err)
	child, err := parent.NewChild(runcontext.ChildOptions{
		ChildSessionID:  childID,
		RunID:           "run_memory",
		PersonalityName: "researcher",
		StateMode:       runcontext.StateModeReadonly,
		ProfileName:     "work",
	})
	require.NoError(t, err)

	item := ApplyRunProvenance(MemoryItem{
		SourceLinks: []SourceLink{{
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

func TestGetRunChildSessionID_FallsBackToEffectiveSessionOnlyForChildren(t *testing.T) {
	childID := nanoid.MustFromSeed(runcontext.SessionIDPrefix, "child", "ChildSessionFallbackSeed")

	require.Empty(t, getRunChildSessionID(runcontext.Context{
		Session: runcontext.Session{EffectiveID: childID},
	}))
	require.Equal(t, childID, getRunChildSessionID(runcontext.Context{
		Session: runcontext.Session{EffectiveID: childID},
		Lineage: runcontext.Lineage{ParentSessionID: runcontext.DefaultSessionID},
	}))
}

func TestFillSourceLinkProvenance_IgnoresNilLinks(t *testing.T) {
	require.NotPanics(t, func() {
		fillSourceLinkProvenance(nil, runcontext.Context{}, "tool_write")
	})
}
