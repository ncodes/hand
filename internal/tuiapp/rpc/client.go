package rpc

import (
	"context"

	rpcclient "github.com/wandxy/hand/internal/rpc/client"
)

type ChatAPI = rpcclient.ChatAPI
type SessionTimelineLoader interface {
	GetSessionTimeline(
		ctx context.Context,
		opts rpcclient.SessionTimelineOptions,
	) (rpcclient.SessionTimeline, error)
}

type Event = rpcclient.Event
type RespondOptions = rpcclient.RespondOptions
type SessionTimeline = rpcclient.SessionTimeline
type SessionTimelineOptions = rpcclient.SessionTimelineOptions
