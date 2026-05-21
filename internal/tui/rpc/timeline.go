package rpc

type SessionTimelineLoaded struct {
	Timeline SessionTimeline
}

type SessionTimelineLoadFailed struct {
	Err error
}
