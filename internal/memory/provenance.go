package memory

import (
	"strings"

	"github.com/wandxy/hand/pkg/agent/runcontext"
)

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

	runcontext.SetMetadata(item.Metadata, runcontext.MemoryMetadataPublicSessionID, runCtx.Session.PublicID)
	runcontext.SetMetadata(item.Metadata, runcontext.MemoryMetadataEffectiveSessionID, runCtx.Session.EffectiveID)
	runcontext.SetMetadata(item.Metadata, runcontext.MemoryMetadataParentSessionID, runCtx.Lineage.ParentSessionID)
	runcontext.SetMetadata(item.Metadata, runcontext.MemoryMetadataChildSessionID, getRunChildSessionID(runCtx))
	runcontext.SetMetadata(item.Metadata, runcontext.MemoryMetadataRunID, runCtx.Lineage.RunID)
	runcontext.SetMetadata(item.Metadata, runcontext.MemoryMetadataSourcePersonality, runCtx.Personality.Name)
	runcontext.SetMetadata(item.Metadata, runcontext.MemoryMetadataStateMode, runCtx.State.Mode)
	runcontext.SetMetadata(item.Metadata, runcontext.MemoryMetadataSourceProfile, runCtx.ProfileName)
	runcontext.SetMetadata(item.Metadata, runcontext.MemoryMetadataTrigger, trigger)

	for index := range item.SourceLinks {
		fillSourceLinkProvenance(&item.SourceLinks[index], runCtx, trigger)
	}

	return item
}

func getRunChildSessionID(runCtx runcontext.Context) string {
	if strings.TrimSpace(runCtx.Lineage.ParentSessionID) == "" {
		return ""
	}
	if strings.TrimSpace(runCtx.Lineage.ChildSessionID) != "" {
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
		link.SourceTrigger = strings.TrimSpace(trigger)
	}
}
