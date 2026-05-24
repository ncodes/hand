package agent

const (
	EventKindTextDelta = "text_delta"
	EventKindTrace     = "trace_event"
)

type Event struct {
	Kind       string
	Channel    string
	Text       string
	TraceEvent any
}
