package episodic

type episodeMetadataField struct {
	key      string
	identity bool
}

// episodeMetadataFields is the strict metadata vocabulary for episodic
// extraction. identity=true means the field participates in deterministic
// candidate identity, so changing it should create a distinct memory candidate.
var episodeMetadataFields = []episodeMetadataField{
	{key: "reason"},
	{key: "memory_importance", identity: true},
	{key: "memory_granularity", identity: true},
	{key: "canonical_group", identity: true},
	{key: "trace_refs", identity: true},
	{key: "tool_name", identity: true},
	{key: "purpose", identity: true},
	{key: "status", identity: true},
	{key: "artifact_or_command_ref", identity: true},
	{key: "chosen_option", identity: true},
	{key: "rejected_alternatives"},
	{key: "source_range", identity: true},
	{key: "requested_goal", identity: true},
	{key: "resulting_change", identity: true},
	{key: "verification_status", identity: true},
	{key: "remaining_risk", identity: true},
	{key: "outcome_status", identity: true},
	{key: "attempt_status", identity: true},
	{key: "progress_status", identity: true},
	{key: "follow_up_status", identity: true},
	{key: "blocker_status", identity: true},
	{key: "emotion", identity: true},
	{key: "emotional_valence", identity: true},
	{key: "emotional_intensity", identity: true},
	{key: "emotion_target", identity: true},
	{key: "life_domain", identity: true},
	{key: "sensitivity", identity: true},
	{key: "uncertainty"},
}

// episodeMetadataFieldKeys feeds the structured-output schema. Keeping the list
// centralized avoids the prompt schema, parser, and identity hashing drifting
// apart.
func episodeMetadataFieldKeys() []string {
	keys := make([]string, 0, len(episodeMetadataFields))
	for _, field := range episodeMetadataFields {
		keys = append(keys, field.key)
	}

	return keys
}

// episodeIdentityMetadataKeys identifies metadata that helps define whether two
// extracted candidates are meaningfully the same memory.
func episodeIdentityMetadataKeys() []string {
	keys := make([]string, 0, len(episodeMetadataFields))
	for _, field := range episodeMetadataFields {
		if field.identity {
			keys = append(keys, field.key)
		}
	}

	return keys
}
