package browser

import (
	"context"
	"errors"
	"time"

	"github.com/wandxy/morph/internal/permissions"
)

type Action string

const (
	ActionStatus        Action = "status"
	ActionStart         Action = "start"
	ActionStop          Action = "stop"
	ActionConnect       Action = "connect"
	ActionProfiles      Action = "profiles"
	ActionTabs          Action = "tabs"
	ActionOpen          Action = "open"
	ActionFocus         Action = "focus"
	ActionClose         Action = "close"
	ActionNavigate      Action = "navigate"
	ActionReload        Action = "reload"
	ActionSnapshot      Action = "snapshot"
	ActionScreenshot    Action = "screenshot"
	ActionPDF           Action = "pdf"
	ActionConsole       Action = "console"
	ActionClick         Action = "click"
	ActionType          Action = "type"
	ActionPress         Action = "press"
	ActionScroll        Action = "scroll"
	ActionSelect        Action = "select"
	ActionUpload        Action = "upload"
	ActionDownload      Action = "download"
	ActionAcceptDialog  Action = "accept_dialog"
	ActionDismissDialog Action = "dismiss_dialog"
	ActionWait          Action = "wait"
	ActionBack          Action = "back"
	ActionForward       Action = "forward"
)

type ErrorCode string

const (
	ErrorInvalidRequest ErrorCode = "invalid_request"
	ErrorUnavailable    ErrorCode = "browser_unavailable"
	ErrorStartFailed    ErrorCode = "browser_start_failed"
	ErrorHealthFailed   ErrorCode = "browser_health_failed"
	ErrorNotFound       ErrorCode = "browser_not_found"
	ErrorOwnership      ErrorCode = "browser_ownership"
	ErrorClosed         ErrorCode = "browser_closed"
)

type Error struct {
	Code      ErrorCode
	Operation Action
	Retryable bool
	Err       error
}

func (e *Error) Error() string {
	if e == nil || e.Err == nil {
		return ""
	}

	return e.Err.Error()
}

func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}

	return e.Err
}

func GetError(err error) (*Error, bool) {
	var browserErr *Error
	ok := errors.As(err, &browserErr)
	return browserErr, ok
}

type SessionState string

const (
	SessionStarting SessionState = "starting"
	SessionReady    SessionState = "ready"
	SessionStopping SessionState = "stopping"
	SessionFailed   SessionState = "failed"
	SessionStopped  SessionState = "stopped"
)

type Owner struct {
	Actor     permissions.Actor
	Profile   string
	SessionID string
	RunID     string
}

type Session struct {
	ID          string
	Profile     string
	ProfileMode string
	State       SessionState
	Owner       Owner
	CreatedAt   time.Time
	LastActive  time.Time
	Error       string
}

type Profile struct {
	Name      string
	Mode      string
	Default   bool
	Available bool
}

type Tab struct {
	ID        string
	SessionID string
	Title     string
	URL       string
}

type ArtifactKind string

const (
	ArtifactScreenshot ArtifactKind = "screenshot"
	ArtifactPDF        ArtifactKind = "pdf"
	ArtifactDownload   ArtifactKind = "download"
)

type Artifact struct {
	Handle    string
	Kind      ArtifactKind
	MIMEType  string
	Size      int64
	CreatedAt time.Time
	ExpiresAt time.Time
}

type StartRequest struct {
	Profile string
}

type Status struct {
	Enabled  bool
	Profiles []Profile
	Sessions []Session
}

type LaunchOptions struct {
	Executable  string
	Mode        string
	DataDir     string
	CDPEndpoint string
	ProxyURL    string
	ProxyUser   string
	ProxySecret string
	Timeout     time.Duration
}

type Backend interface {
	Start(context.Context, LaunchOptions) (BackendSession, error)
}

type BackendSession interface {
	Health(context.Context) error
	Close(context.Context) error
}
