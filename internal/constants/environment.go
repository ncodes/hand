package constants

import "time"

const (
	// DefaultProcessOutputBufferBytes is the fallback retained output size per tracked process.
	DefaultProcessOutputBufferBytes = 64 * 1024
	// DefaultProcessMaxTracked is the fallback maximum number of tracked processes.
	DefaultProcessMaxTracked = 32
	// DefaultProcessStopGracePeriod is the fallback grace period before force-stopping processes.
	DefaultProcessStopGracePeriod = 2 * time.Second
)

const (
	// DefaultSessionSearchMaxResults is the fallback result limit for session search.
	DefaultSessionSearchMaxResults = 10
	// MaxSessionSearchResults is the hard maximum result limit for session search.
	MaxSessionSearchResults = 20
	// MaxSessionMatchedMessages is the maximum matched messages returned per session search result.
	MaxSessionMatchedMessages = 3
	// SessionSearchSnippetRunes is the maximum rune count for session search snippets.
	SessionSearchSnippetRunes = 200
)
