package search

import (
	"fmt"
	"strings"

	statememory "github.com/wandxy/hand/internal/state/core"
)

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

func MemoryVectorTag(key string, value string) string {
	return strings.TrimSpace(key) + ":" + strings.TrimSpace(value)
}
