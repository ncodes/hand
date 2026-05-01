package constants

const (
	// ToolMaxListEntries is the hard maximum number of entries returned by list-style tools.
	ToolMaxListEntries = 500
	// ToolMaxSearchResults is the hard maximum number of results returned by search-style tools.
	ToolMaxSearchResults = 200
	// ToolMaxReadBytes is the hard maximum number of bytes read by file/content tools.
	ToolMaxReadBytes = 256 * 1024
	// ToolMaxOutputBytes is the hard maximum number of bytes emitted by tool output.
	ToolMaxOutputBytes = 256 * 1024
	// ToolDefaultTimeout is the fallback tool timeout in seconds.
	ToolDefaultTimeout = 30
	// ToolMaxTimeout is the hard maximum tool timeout in seconds.
	ToolMaxTimeout = 120
)
