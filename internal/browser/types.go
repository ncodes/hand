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
	ErrorNotReady       ErrorCode = "browser_not_ready"
	ErrorStaleReference ErrorCode = "browser_stale_reference"
	ErrorTimeout        ErrorCode = "browser_timeout"
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
	ID          string       `json:"id"`
	Profile     string       `json:"profile"`
	ProfileMode string       `json:"profile_mode"`
	State       SessionState `json:"state"`
	Owner       Owner        `json:"-"`
	CreatedAt   time.Time    `json:"created_at"`
	LastActive  time.Time    `json:"last_active"`
	Error       string       `json:"error,omitempty"`
}

type Profile struct {
	Name      string `json:"name"`
	Mode      string `json:"mode"`
	Default   bool   `json:"default"`
	Available bool   `json:"available"`
}

type Tab struct {
	ID         string `json:"id"`
	SessionID  string `json:"session_id"`
	Title      string `json:"title"`
	URL        string `json:"url"`
	Active     bool   `json:"active"`
	Generation uint64 `json:"generation"`
}

type SnapshotNode struct {
	Ref         string            `json:"ref,omitempty"`
	Role        string            `json:"role"`
	Name        string            `json:"name,omitempty"`
	Value       string            `json:"value,omitempty"`
	Description string            `json:"description,omitempty"`
	Disabled    bool              `json:"disabled,omitempty"`
	Properties  map[string]string `json:"properties,omitempty"`
}

type Snapshot struct {
	TabID      string         `json:"tab_id"`
	URL        string         `json:"url"`
	Title      string         `json:"title"`
	Generation uint64         `json:"generation"`
	Nodes      []SnapshotNode `json:"nodes"`
	Truncated  bool           `json:"truncated,omitempty"`
}

type WaitCondition string

const (
	WaitLoad    WaitCondition = "load"
	WaitText    WaitCondition = "text"
	WaitURL     WaitCondition = "url"
	WaitVisible WaitCondition = "visible"
)

type ActionRequest struct {
	Profile   string
	SessionID string
	TabID     string
	URL       string
	Ref       string
	Text      string
	Value     string
	Key       string
	X         int64
	Y         int64
	Condition WaitCondition
	Timeout   time.Duration
	Replace   bool
}

type BackendTab struct {
	ID     string
	Title  string
	URL    string
	Active bool
}

type BackendSnapshotNode struct {
	BackendNodeID int64
	Sensitive     bool
	Role          string
	Name          string
	Value         string
	Description   string
	Disabled      bool
	Properties    map[string]string
}

type BackendSnapshot struct {
	URL   string
	Title string
	Nodes []BackendSnapshotNode
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
	Enabled  bool      `json:"enabled"`
	Profiles []Profile `json:"profiles"`
	Sessions []Session `json:"sessions"`
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

type InteractiveBackendSession interface {
	ListTabs(context.Context) ([]BackendTab, error)
	OpenTab(context.Context, string) (BackendTab, error)
	FocusTab(context.Context, string) error
	CloseTab(context.Context, string) error
	Navigate(context.Context, string, string) (BackendTab, error)
	Back(context.Context, string) (BackendTab, error)
	Forward(context.Context, string) (BackendTab, error)
	Reload(context.Context, string) (BackendTab, error)
	Snapshot(context.Context, string) (BackendSnapshot, error)
	Click(context.Context, string, int64) error
	Type(context.Context, string, int64, string, bool) error
	Press(context.Context, string, string) error
	Scroll(context.Context, string, int64, int64) error
	Select(context.Context, string, int64, string) error
	Wait(context.Context, string, WaitCondition, string, int64) error
}

type NetworkRequestAuthorizer func(context.Context, permissions.NetworkTarget) error

type NetworkAuthorizingBackendSession interface {
	SetNetworkAuthorizer(string, NetworkRequestAuthorizer) func()
}
