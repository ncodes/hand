package rpc

import (
	"context"

	rpcclient "github.com/wandxy/hand/internal/rpc/client"
)

// ChatAPI is the RPC chat surface consumed by the TUI.
type ChatAPI = rpcclient.ChatAPI

// SessionTimelineLoader loads session transcript and trace data for the TUI.
type SessionTimelineLoader interface {
	Timeline(
		ctx context.Context,
		opts rpcclient.SessionTimelineOptions,
	) (rpcclient.SessionTimeline, error)
}

// Event is the streaming event type consumed by the TUI RPC client.
type Event = rpcclient.Event

// RespondOptions mirrors agent response options at this package boundary.
type RespondOptions = rpcclient.RespondOptions

// SessionTimeline mirrors the agent timeline type at this package boundary.
type SessionTimeline = rpcclient.SessionTimeline

// SessionTimelineOptions mirrors agent timeline query options at this package boundary.
type SessionTimelineOptions = rpcclient.SessionTimelineOptions
