package slack

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	slack "github.com/wandxy/hand/pkg/gateway/slack"
)

const defaultSlackStreamFlushInterval = 150 * time.Millisecond

type Sender struct {
	api API
}

func NewSender(api API) *Sender {
	return &Sender{api: api}
}

func (s *Sender) SendFinal(ctx context.Context, target slack.Target, text string) error {
	if s == nil || s.api == nil {
		return errors.New("slack sender is required")
	}
	for _, chunk := range slack.ChunkMarkdown(text, slack.MarkdownTextLimit) {
		if _, err := s.api.PostMessage(ctx, target, chunk); err != nil {
			return err
		}
	}

	return nil
}

func (s *Sender) StreamTurn(
	ctx context.Context,
	target slack.Target,
	run func(func(string)) (string, error),
) error {
	if s == nil || s.api == nil {
		return errors.New("slack sender is required")
	}

	stream, err := s.api.StartStream(ctx, target, "")
	if err != nil || strings.TrimSpace(stream.TS) == "" {
		reply, runErr := run(func(string) {})
		if runErr != nil {
			return runErr
		}
		return s.SendFinal(ctx, target, reply)
	}

	appender := newSlackStreamAppender(ctx, s.api, stream, defaultSlackStreamFlushInterval)
	appender.Start()
	reply, err := run(appender.Append)
	appendErr, visible := appender.Stop()
	if err != nil {
		return err
	}
	if !visible && appendErr != nil {
		return s.SendFinal(ctx, target, reply)
	}
	stopText := reply
	if visible {
		stopText = ""
	}
	if stopErr := s.api.StopStream(ctx, stream, stopText); stopErr != nil {
		if visible {
			return stopErr
		}
		return s.SendFinal(ctx, target, reply)
	}

	return nil
}

type slackStreamAppender struct {
	ctx      context.Context
	api      API
	stream   slack.Stream
	interval time.Duration
	done     chan struct{}
	stopped  chan struct{}

	mu      sync.Mutex
	pending []string
	visible bool
	err     error
}

func newSlackStreamAppender(
	ctx context.Context,
	api API,
	stream slack.Stream,
	interval time.Duration,
) *slackStreamAppender {
	if interval <= 0 {
		interval = defaultSlackStreamFlushInterval
	}

	return &slackStreamAppender{
		ctx:      ctx,
		api:      api,
		stream:   stream,
		interval: interval,
		done:     make(chan struct{}),
		stopped:  make(chan struct{}),
	}
}

func (a *slackStreamAppender) Start() {
	go func() {
		ticker := time.NewTicker(a.interval)
		defer ticker.Stop()
		defer close(a.stopped)

		for {
			select {
			case <-a.ctx.Done():
				a.flush()
				return
			case <-a.done:
				a.flush()
				return
			case <-ticker.C:
				a.flush()
			}
		}
	}()
}

func (a *slackStreamAppender) Append(delta string) {
	if strings.TrimSpace(delta) == "" {
		return
	}

	chunks := getSlackStreamChunks(delta)
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.err != nil {
		return
	}
	a.pending = append(a.pending, chunks...)
}

func (a *slackStreamAppender) Stop() (error, bool) {
	close(a.done)
	<-a.stopped

	a.mu.Lock()
	defer a.mu.Unlock()

	return a.err, a.visible
}

func (a *slackStreamAppender) flush() {
	for {
		chunk, ok := a.nextChunk()
		if !ok {
			return
		}

		if err := a.api.AppendStream(a.ctx, a.stream, chunk); err != nil {
			a.mu.Lock()
			a.err = err
			a.pending = nil
			a.mu.Unlock()
			return
		}

		a.mu.Lock()
		a.visible = true
		a.mu.Unlock()
	}
}

func (a *slackStreamAppender) nextChunk() (string, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.err != nil || len(a.pending) == 0 {
		return "", false
	}

	chunk := a.pending[0]
	a.pending = a.pending[1:]
	return chunk, true
}

func getSlackStreamChunks(text string) []string {
	if strings.TrimSpace(text) == "" {
		return nil
	}

	runes := []rune(text)
	chunks := make([]string, 0, len(runes)/slack.MarkdownTextLimit+1)
	for len(runes) > 0 {
		n := min(len(runes), slack.MarkdownTextLimit)
		chunks = append(chunks, string(runes[:n]))
		runes = runes[n:]
	}

	return chunks
}
