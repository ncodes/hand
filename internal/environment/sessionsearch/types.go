package sessionsearch

type SessionSearchRequest struct {
	SessionID       string `json:"session_id,omitempty"`
	IgnoreSessionID string `json:"ignore_session_id,omitempty"`
	Query           string `json:"query"`
	Role            string `json:"role,omitempty"`
	ToolName        string `json:"tool_name,omitempty"`
	MaxResults      int    `json:"max_results,omitempty"`
}

type SessionSearchResult struct {
	SessionID      string                    `json:"session_id"`
	SessionCreated string                    `json:"session_created_at,omitempty"`
	SessionUpdated string                    `json:"session_updated_at,omitempty"`
	MatchCount     int                       `json:"match_count"`
	SessionSummary string                    `json:"session_summary,omitempty"`
	Messages       []SessionSearchMessageHit `json:"messages"`
}

type SessionSearchMessageHit struct {
	MessageID     uint   `json:"message_id"`
	Role          string `json:"role"`
	ToolName      string `json:"tool_name,omitempty"`
	CreatedAt     string `json:"created_at"`
	Snippet       string `json:"snippet"`
	FullTextBytes int    `json:"full_text_bytes"`
	MatchIndex    int    `json:"match_index"`
}
