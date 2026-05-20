package events

import rpcclient "github.com/wandxy/hand/internal/rpc/client"

type Event interface {
	TUIEvent()
}

type ViewportResizedEvent struct {
	Width  int
	Height int
}

type SubmitComposerEvent struct{}
type CopyTranscriptEvent struct{}
type JumpTranscriptToBottomEvent struct{}
type ShowPreviousPromptEvent struct{}
type ShowNextPromptEvent struct{}
type InsertInputNewlineEvent struct{}
type DeleteInputLineEvent struct{}

type ApplyTUIMessageEvent struct {
	Message any
}

type HydrateTimelineEvent struct {
	Timeline rpcclient.SessionTimeline
}

func (ViewportResizedEvent) TUIEvent()        {}
func (SubmitComposerEvent) TUIEvent()         {}
func (CopyTranscriptEvent) TUIEvent()         {}
func (JumpTranscriptToBottomEvent) TUIEvent() {}
func (ShowPreviousPromptEvent) TUIEvent()     {}
func (ShowNextPromptEvent) TUIEvent()         {}
func (InsertInputNewlineEvent) TUIEvent()     {}
func (DeleteInputLineEvent) TUIEvent()        {}
func (ApplyTUIMessageEvent) TUIEvent()        {}
func (HydrateTimelineEvent) TUIEvent()        {}
