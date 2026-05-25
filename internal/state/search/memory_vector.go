package search

import (
	"fmt"
	"strings"

	statememory "github.com/wandxy/hand/internal/state/core"
)

// MemoryVectorTags builds vector tags for a memory item.
func MemoryVectorTags(item statememory.MemoryItem) []string {
	tags := make([]string, 0, 4+len(item.Tags))
	if kind := strings.TrimSpace(string(item.Kind)); kind != "" {
		tags = append(tags, MemoryVectorTag("memory_kind", kind))
	}
	if status := strings.TrimSpace(string(item.Status)); status != "" {
		tags = append(tags, MemoryVectorTag("memory_status", status))
	}
	if sessionID := MemoryVectorSessionID(item); sessionID != "" {
		tags = append(tags, MemoryVectorTag("memory_session", sessionID))
	}
	tags = append(tags, MemoryVectorTag("memory_reflected", fmt.Sprint(item.Reflected)))
	for _, tag := range item.Tags {
		if tag = strings.TrimSpace(tag); tag != "" {
			tags = append(tags, MemoryVectorTag("memory_tag", tag))
		}
	}

	return NormalizeVectorTags(tags)
}

// MemoryVectorSessionID extracts the session ID associated with memory vector tags.
func MemoryVectorSessionID(item statememory.MemoryItem) string {
	if sessionID := strings.TrimSpace(item.Metadata["source_session_id"]); sessionID != "" {
		return sessionID
	}
	for _, link := range item.SourceLinks {
		if sessionID := strings.TrimSpace(link.SessionID); sessionID != "" {
			return sessionID
		}
	}

	return ""
}

// MemoryVectorTag builds one key/value vector tag.
func MemoryVectorTag(key string, value string) string {
	return strings.TrimSpace(key) + ":" + strings.TrimSpace(value)
}
