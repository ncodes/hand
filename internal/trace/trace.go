package trace

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/wandxy/hand/internal/guardrails"
)

var (
	mkdirAll   = os.MkdirAll
	createFile = os.Create
)

type Session interface {
	ID() string
	Record(string, any)
	Close()
}

type Factory interface {
	NewSession(context.Context, Metadata) Session
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
}

type jsonlSession struct {
	id       string
	encoder  *json.Encoder
	file     *os.File
	redactor guardrails.Redactor
	closed   bool
	mu       sync.Mutex
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

func (f *JSONLFactory) NewSession(_ context.Context, metadata Metadata) Session {
	if f == nil || strings.TrimSpace(f.directory) == "" {
		return NoopSession()
	}
	if err := mkdirAll(f.directory, 0o755); err != nil {
		log.Warn().Err(err).Str("traceDir", f.directory).Msg("Failed to initialize trace directory")
		return NoopSession()
	}

	now := f.now()
	sessionID := fmt.Sprintf("%s-%s", now.Format("20060102T150405.000000000Z"), randomSuffix())
	path := filepath.Join(f.directory, sessionID+".jsonl")
	file, err := createFile(path)
	if err != nil {
		log.Warn().Err(err).Str("tracePath", path).Msg("Failed to create trace session file")
		return NoopSession()
	}

	session := &jsonlSession{
		id:       sessionID,
		encoder:  json.NewEncoder(file),
		file:     file,
		redactor: f.redactor,
		path:     path,
	}
	metadata.TraceDir = f.directory
	session.Record("chat.started", metadata)
	return session
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
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}

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
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	s.closed = true
	if err := s.file.Close(); err != nil {
		log.Warn().Err(err).Str("tracePath", s.path).Msg("Failed to close trace session file")
	}
}

func (noopFactory) NewSession(context.Context, Metadata) Session {
	return NoopSession()
}

func (s noopSession) ID() string         { return "" }
func (s noopSession) Record(string, any) {}
func (s noopSession) Close()             {}
