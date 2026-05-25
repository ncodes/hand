package rpc

// SessionTimelineLoaded describes session timeline loaded.
type SessionTimelineLoaded struct {
	Timeline SessionTimeline
}

// SessionTimelineLoadFailed describes session timeline load failed.
type SessionTimelineLoadFailed struct {
	Err error
}
