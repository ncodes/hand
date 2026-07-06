package search

import (
	"fmt"

	statememory "github.com/wandxy/morph/internal/state/core"
	"github.com/wandxy/morph/pkg/str"
)

// MemoryVectorTags builds vector tags for a memory item.
func MemoryVectorTags(item statememory.MemoryItem) []string {
	tags := make([]string, 0, 4+len(item.Tags))
	stringValue1 := str.String(string(item.Kind))
	if kind := stringValue1.Trim(); kind != "" {
		tags = append(tags, MemoryVectorTag("memory_kind", kind))
	}
	stringValue2 := str.String(string(item.Status))
	if status := stringValue2.Trim(); status != "" {
		tags = append(tags, MemoryVectorTag("memory_status", status))
	}
	if sessionID := MemoryVectorSessionID(item); sessionID != "" {
		tags = append(tags, MemoryVectorTag("memory_session", sessionID))
	}
	tags = append(tags, MemoryVectorTag("memory_reflected", fmt.Sprint(item.Reflected)))
	for _, tag := range item.Tags {
		stringValue3 := str.String(tag)
		if tag = stringValue3.Trim(); tag != "" {
			tags = append(tags, MemoryVectorTag("memory_tag", tag))
		}
	}

	return NormalizeVectorTags(tags)
}

// MemoryVectorSessionID extracts the session ID associated with memory vector tags.
func MemoryVectorSessionID(item statememory.MemoryItem) string {
	stringValue4 := str.String(item.Metadata["source_session_id"])
	if sessionID := stringValue4.Trim(); sessionID != "" {
		return sessionID
	}
	for _, link := range item.SourceLinks {
		stringValue5 := str.String(link.SessionID)
		if sessionID := stringValue5.Trim(); sessionID != "" {
			return sessionID
		}
	}

	return ""
}

// MemoryVectorTag builds one key/value vector tag.
func MemoryVectorTag(key string, value string) string {
	stringValue6 := str.String(key)
	stringValue7 := str.String(value)
	return stringValue6.Trim() + ":" + stringValue7.Trim()
}
