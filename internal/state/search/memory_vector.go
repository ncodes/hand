package search

import (
	"fmt"

	statememory "github.com/wandxy/morph/internal/state/core"
	"github.com/wandxy/morph/pkg/stringx"
)

// MemoryVectorTags builds vector tags for a memory item.
func MemoryVectorTags(item statememory.MemoryItem) []string {
	tags := make([]string, 0, 4+len(item.Tags))
	if kind := stringx.String(string(item.Kind)).Trim(); kind != "" {
		tags = append(tags, MemoryVectorTag("memory_kind", kind))
	}
	if status := stringx.String(string(item.Status)).Trim(); status != "" {
		tags = append(tags, MemoryVectorTag("memory_status", status))
	}
	if sessionID := MemoryVectorSessionID(item); sessionID != "" {
		tags = append(tags, MemoryVectorTag("memory_session", sessionID))
	}
	tags = append(tags, MemoryVectorTag("memory_reflected", fmt.Sprint(item.Reflected)))
	for _, tag := range item.Tags {
		if tag = stringx.String(tag).Trim(); tag != "" {
			tags = append(tags, MemoryVectorTag("memory_tag", tag))
		}
	}

	return NormalizeVectorTags(tags)
}

// MemoryVectorSessionID extracts the session ID associated with memory vector tags.
func MemoryVectorSessionID(item statememory.MemoryItem) string {
	if sessionID := stringx.String(item.Metadata["source_session_id"]).Trim(); sessionID != "" {
		return sessionID
	}
	for _, link := range item.SourceLinks {
		if sessionID := stringx.String(link.SessionID).Trim(); sessionID != "" {
			return sessionID
		}
	}

	return ""
}

// MemoryVectorTag builds one key/value vector tag.
func MemoryVectorTag(key string, value string) string {
	return stringx.String(key).Trim() + ":" + stringx.String(value).Trim()
}
