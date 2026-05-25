package state

// EffectKind classifies side effects emitted by the TUI state reducer.
type EffectKind string

const (
	EffectSendPrompt          EffectKind = "send_prompt"
	EffectCopyTranscript      EffectKind = "copy_transcript"
	EffectLoadSessionTimeline EffectKind = "load_session_timeline"
)

// Effect represents a side effect emitted by the TUI reducer.
type Effect struct {
	Kind EffectKind
	Text string
}
