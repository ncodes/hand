package process

import "time"

const (
	StatusRunning = "running"
	StatusExited  = "exited"
	StatusFailed  = "failed"
	StatusStopped = "stopped"
)

type StartRequest struct {
	Command           string
	Args              []string
	CWD               string
	Env               map[string]string
	OutputBufferBytes int
}

type Info struct {
	ID              string     `json:"id"`
	Command         string     `json:"command"`
	Args            []string   `json:"args,omitempty"`
	CWD             string     `json:"cwd,omitempty"`
	Status          string     `json:"status"`
	ExitCode        *int       `json:"exit_code,omitempty"`
	StartedAt       time.Time  `json:"started_at"`
	EndedAt         *time.Time `json:"ended_at,omitempty"`
	StdoutBytes     int        `json:"stdout_bytes"`
	StderrBytes     int        `json:"stderr_bytes"`
	StdoutTruncated bool       `json:"stdout_truncated,omitempty"`
	StderrTruncated bool       `json:"stderr_truncated,omitempty"`
}

type Output struct {
	Stdout          string `json:"stdout"`
	Stderr          string `json:"stderr"`
	StdoutBytes     int    `json:"stdout_bytes"`
	StderrBytes     int    `json:"stderr_bytes"`
	StdoutTruncated bool   `json:"stdout_truncated,omitempty"`
	StderrTruncated bool   `json:"stderr_truncated,omitempty"`
}
