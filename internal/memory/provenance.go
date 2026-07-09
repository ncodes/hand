package memory

import (
	"github.com/wandxy/morph/internal/agent/runcontext"
	"github.com/wandxy/morph/pkg/str"
)

const (
	MemoryMetadataSourceProfile      = "source_profile"
	MemoryMetadataSourcePersonality  = "source_personality"
	MemoryMetadataParentSessionID    = "source_parent_session_id"
	MemoryMetadataChildSessionID     = "source_child_session_id"
	MemoryMetadataRunID              = "source_run_id"
	MemoryMetadataStateMode          = "source_state_mode"
	MemoryMetadataTrigger            = "source_trigger"
	MemoryMetadataPublicSessionID    = "source_public_session_id"
	MemoryMetadataEffectiveSessionID = "source_effective_session_id"
)

// ApplyRunProvenance applies run provenance.
func ApplyRunProvenance(
	item MemoryItem,
	runCtx runcontext.Context,
	trigger string,
) MemoryItem {
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
	setMetadata(item.Metadata, MemoryMetadataChildSessionID, getRunChildSessionID(runCtx))
	setMetadata(item.Metadata, MemoryMetadataRunID, runCtx.Lineage.RunID)
	setMetadata(item.Metadata, MemoryMetadataSourcePersonality, runCtx.Personality.Name)
	setMetadata(item.Metadata, MemoryMetadataStateMode, runCtx.State.Mode)
	setMetadata(item.Metadata, MemoryMetadataSourceProfile, runCtx.ProfileName)
	setMetadata(item.Metadata, MemoryMetadataTrigger, trigger)

	for index := range item.SourceLinks {
		fillSourceLinkProvenance(&item.SourceLinks[index], runCtx, trigger)
	}

	return item
}

func setMetadata(metadata map[string]string, key string, value string) {
	valueText := str.String(value)
	if valueText := valueText.Trim(); valueText != "" {
		metadata[key] = valueText
	}
}

func getRunChildSessionID(runCtx runcontext.Context) string {
	parentSessionIDValue := str.String(runCtx.Lineage.ParentSessionID)
	if parentSessionIDValue.Trim() == "" {
		return ""
	}
	childSessionIDValue := str.String(runCtx.Lineage.ChildSessionID)
	if childSessionIDValue.Trim() != "" {
		return runCtx.Lineage.ChildSessionID
	}

	return runCtx.Session.EffectiveID
}

func fillSourceLinkProvenance(link *SourceLink, runCtx runcontext.Context, trigger string) {
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
		link.ChildSessionID = getRunChildSessionID(runCtx)
	}
	if link.RunID == "" {
		link.RunID = runCtx.Lineage.RunID
	}
	if link.StateMode == "" {
		link.StateMode = runCtx.State.Mode
	}
	if link.SourceTrigger == "" {
		triggerValue := str.String(trigger)
		link.SourceTrigger = triggerValue.Trim()
	}
}
