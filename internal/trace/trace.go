package trace

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/wandxy/hand/internal/guardrails"
)

var (
	mkdirAll       = os.MkdirAll
	globTraceFiles = filepath.Glob
	openTraceFile  = func(name string, flag int, perm os.FileMode) (*os.File, error) {
		return os.OpenFile(name, flag, perm)
	}
	statOpenedFile = func(f *os.File) (os.FileInfo, error) {
		return f.Stat()
	}
	closeTraceFile = func(f *os.File) error {
		return f.Close()
	}
)

// ErrAmbiguousTraceFiles is returned when more than one file matches *<session_id>.jsonl in the trace directory.
var ErrAmbiguousTraceFiles = errors.New("multiple trace files match session id")

// traceTimeLayout is the UTC timestamp prefix for new trace filenames: "<layout>-<session_id>.jsonl".
const traceTimeLayout = "20060102T150405.000000000Z"

type Session interface {
	ID() string
	Record(string, any)
	Close()
}

type Factory interface {
	OpenSession(context.Context, string, Metadata) Session
}

type Metadata struct {
	AgentName string `json:"agent_name"`
	Model     string `json:"model"`
	APIMode   string `json:"api_mode"`
	Source    string `json:"source"`
	TraceDir  string `json:"trace_dir,omitempty"`
}

type Event struct {
	SessionID string    `json:"session_id"`
	Type      string    `json:"type"`
	Timestamp time.Time `json:"timestamp"`
	Payload   any       `json:"payload,omitempty"`
}

type JSONLFactory struct {
	directory string
	redactor  guardrails.Redactor
	now       func() time.Time
	pathLocks sync.Map // path -> *sync.Mutex
}

type jsonlSession struct {
	id       string
	encoder  *json.Encoder
	file     *os.File
	redactor guardrails.Redactor
	closed   bool
	pathLock *sync.Mutex
	path     string
	noop     bool
}

type noopSession struct{}

type noopFactory struct{}

func NewFactory(directory string, redactor guardrails.Redactor) *JSONLFactory {
	if redactor == nil {
		redactor = guardrails.NewRedactor()
	}

	return &JSONLFactory{
		directory: strings.TrimSpace(directory),
		redactor:  redactor,
		now:       func() time.Time { return time.Now().UTC() },
	}
}

func NoopFactory() Factory {
	return noopFactory{}
}

func NoopSession() Session {
	return noopSession{}
}

// validateSessionID mirrors internal/trace/inspect resolveSessionPath rules for the id segment.
func validateSessionID(id string) bool {
	id = strings.TrimSpace(id)
	if id == "" {
		return false
	}
	if strings.Contains(id, "/") || strings.Contains(id, `\`) {
		return false
	}
	if filepath.Base(id) != id || id == "." || id == ".." {
		return false
	}

	return true
}

// SessionIDFromTraceFilename returns the storage session id from a time-prefixed trace file basename without ".jsonl".
// Filenames are "<UTC>Z-<session_id>"; the segment after "Z-" is the id. If "Z-" is absent, the whole stem is returned.
func SessionIDFromTraceFilename(stem string) string {
	stem = strings.TrimSpace(stem)
	if _, after, ok := strings.Cut(stem, "Z-"); ok {
		return after
	}

	return stem
}

// ResolveTraceFilePath returns the path to the JSONL trace file for a storage session id.
// It looks for exactly one file matching "*<session_id>.jsonl" (time-prefixed names included). 
// If none exist, it returns [os.ErrNotExist]. If more than one match, it returns [ErrAmbiguousTraceFiles].
func ResolveTraceFilePath(directory, sessionID string) (string, error) {
	directory = strings.TrimSpace(directory)
	if !validateSessionID(sessionID) || directory == "" {
		return "", os.ErrNotExist
	}

	pattern := filepath.Join(directory, "*"+sessionID+".jsonl")
	matches, err := globTraceFiles(pattern)
	if err != nil {
		return "", err
	}

	switch len(matches) {
	case 0:
		return "", os.ErrNotExist
	case 1:
		return matches[0], nil
	default:
		return "", ErrAmbiguousTraceFiles
	}
}

func newTraceFilename(sessionID string, utc time.Time) string {
	return utc.Format(traceTimeLayout) + "-" + sessionID + ".jsonl"
}

func (f *JSONLFactory) tracePathForSession(sessionID string) (string, error) {
	dir := f.directory
	pattern := filepath.Join(dir, "*"+sessionID+".jsonl")
	matches, err := globTraceFiles(pattern)
	if err != nil {
		return "", err
	}

	switch len(matches) {
	case 0:
		return filepath.Join(dir, newTraceFilename(sessionID, f.now())), nil
	case 1:
		return matches[0], nil
	default:
		return "", ErrAmbiguousTraceFiles
	}
}

func (f *JSONLFactory) lockForPath(absPath string) *sync.Mutex {
	v, _ := f.pathLocks.LoadOrStore(absPath, new(sync.Mutex))
	return v.(*sync.Mutex)
}

func (f *JSONLFactory) OpenSession(_ context.Context, sessionID string, metadata Metadata) Session {
	if f == nil || strings.TrimSpace(f.directory) == "" {
		return NoopSession()
	}
	if !validateSessionID(sessionID) {
		log.Warn().Str("sessionID", sessionID).Msg("Invalid trace session id; skipping trace file")
		return NoopSession()
	}

	if err := mkdirAll(f.directory, 0o755); err != nil {
		log.Warn().Err(err).Str("traceDir", f.directory).Msg("Failed to initialize trace directory")
		return NoopSession()
	}

	path, err := f.tracePathForSession(sessionID)
	if err != nil {
		log.Warn().Err(err).Str("sessionID", sessionID).Msg("Failed to resolve trace file path")
		return NoopSession()
	}

	pathLock := f.lockForPath(path)
	pathLock.Lock()
	defer pathLock.Unlock()

	file, err := openTraceFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		log.Warn().Err(err).Str("tracePath", path).Msg("Failed to open trace session file")
		return NoopSession()
	}

	info, err := statOpenedFile(file)
	if err != nil {
		_ = closeTraceFile(file)
		log.Warn().Err(err).Str("tracePath", path).Msg("Failed to stat trace session file")
		return NoopSession()
	}

	session := &jsonlSession{
		id:       sessionID,
		encoder:  json.NewEncoder(file),
		file:     file,
		redactor: f.redactor,
		pathLock: pathLock,
		path:     path,
	}

	metadata.TraceDir = f.directory
	if info.Size() == 0 {
		session.recordUnlocked(EvtChatStarted, metadata)
	}

	return session
}

func (noopFactory) OpenSession(context.Context, string, Metadata) Session {
	return NoopSession()
}

func (s *jsonlSession) ID() string {
	if s == nil {
		return ""
	}

	return s.id
}

func (s *jsonlSession) Record(eventType string, payload any) {
	if s == nil || s.noop {
		return
	}

	s.pathLock.Lock()
	defer s.pathLock.Unlock()

	if s.closed {
		return
	}

	s.recordUnlocked(eventType, payload)
}

func (s *jsonlSession) recordUnlocked(eventType string, payload any) {
	event := Event{
		SessionID: s.id,
		Type:      strings.TrimSpace(eventType),
		Timestamp: time.Now().UTC(),
	}
	if payload != nil {
		event.Payload = s.redactor.Sanitize(payload)
	}
	if err := s.encoder.Encode(event); err != nil {
		log.Warn().Err(err).Str("tracePath", s.path).Str("eventType", event.Type).Msg("Failed to write trace event")
	}
}

func (s *jsonlSession) Close() {
	if s == nil || s.noop {
		return
	}

	s.pathLock.Lock()
	defer s.pathLock.Unlock()

	if s.closed {
		return
	}

	s.closed = true
	if err := closeTraceFile(s.file); err != nil {
		log.Warn().Err(err).Str("tracePath", s.path).Msg("Failed to close trace session file")
	}
}

func (s noopSession) ID() string {
	return ""
}

func (s noopSession) Record(string, any) {
	_ = s
}

func (s noopSession) Close() {
	_ = s
}
