package state

type EffectKind string

const (
	EffectSendPrompt          EffectKind = "send_prompt"
	EffectCopyTranscript      EffectKind = "copy_transcript"
	EffectLoadSessionTimeline EffectKind = "load_session_timeline"
)

type Effect struct {
	Kind EffectKind
	Text string
}
