package state

import (
	"errors"
	"strings"
	"time"

	handmsg "github.com/wandxy/hand/internal/messages"
	"github.com/wandxy/hand/pkg/nanoid"
)

const DefaultSessionID = "default"
const SessionIDPrefix = "ses_"
const ArchiveIDPrefix = "arc_"

const (
	MessageOrderAsc  = "asc"
	MessageOrderDesc = "desc"
)

func NewSessionID() (string, error) {
	return nanoid.Generate(SessionIDPrefix)
}

func NewArchiveID() (string, error) {
	return nanoid.Generate(ArchiveIDPrefix)
}

type Session struct {
	CreatedAt        time.Time
	Compaction       SessionCompaction
	ID               string
	LastPromptTokens int
	UpdatedAt        time.Time
}

type SessionCompactionStatus string

const (
	CompactionStatusPending   SessionCompactionStatus = "pending"
	CompactionStatusRunning   SessionCompactionStatus = "running"
	CompactionStatusSucceeded SessionCompactionStatus = "succeeded"
	CompactionStatusFailed    SessionCompactionStatus = "failed"
)

type SessionCompaction struct {
	CompletedAt        time.Time
	FailedAt           time.Time
	LastError          string
	RequestedAt        time.Time
	StartedAt          time.Time
	Status             SessionCompactionStatus
	TargetMessageCount int
	TargetOffset       int
}

type ArchivedSession struct {
	ID              string
	SourceSessionID string
	ArchivedAt      time.Time
	ExpiresAt       time.Time
}

type SessionSummary struct {
	SessionID          string
	SourceEndOffset    int
	SourceMessageCount int
	UpdatedAt          time.Time
	SessionSummary     string
	CurrentTask        string
	Discoveries        []string
	OpenQuestions      []string
	NextActions        []string
}

type MessageQueryOptions struct {
	Archived bool
	Limit    int
	Name     string
	Order    string
	Offset   int
	Role     handmsg.Role
}

type SearchMessageOptions struct {
	IgnoreSessionID       string
	MaxMessagesPerSession int
	MaxSessions           int
	Query                 string
	Role                  handmsg.Role
	ToolName              string
}

type SearchMessageHit struct {
	SessionID       string
	Message         handmsg.Message
	MatchedText     string
	MatchedToolName string
}

type SearchMessageResult struct {
	SessionID     string
	LastMatchedAt time.Time
	MatchCount    int
	Messages      []SearchMessageHit
}

type MessageRecord struct {
	Offset  int
	Message handmsg.Message
}

func NormalizeMessageQueryOrder(order string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(order)) {
	case "", MessageOrderAsc:
		return MessageOrderAsc, nil
	case MessageOrderDesc:
		return MessageOrderDesc, nil
	default:
		return "", errors.New("message order must be asc or desc")
	}
}
